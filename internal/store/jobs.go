package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Job kinds.
const (
	JobCatalog      = "catalog"
	JobThread       = "thread"
	JobDownload     = "download"
	JobBackfill     = "backfill"
	JobRawCapture   = "raw_capture"
	JobOCR          = "ocr"
	JobCLIP         = "clip"
	JobWhisper      = "whisper"
	JobComputePHASH = "compute_phash"
	JobBuildLineage = "build_lineage"
	JobContinuity   = "continuity"
	JobCrosspost    = "crosspost"
	JobTrends       = "trends"
	JobSummarize    = "summarize"
	JobAnomaly      = "anomaly"
	JobReprocess    = "reprocess"
)

// EnqueueJob inserts a job if no identical active job exists (guarded by a
// partial unique index). Returns created=false if one is already pending.
func (s *Store) EnqueueJob(ctx context.Context, siteID int64, boardID, threadID *int64, kind string, payload any, runAfter time.Time) (bool, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO jobs (site_id, board_id, thread_id, kind, payload, run_after)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT DO NOTHING
		RETURNING id`,
		siteID, boardID, threadID, kind, payload, runAfter).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("store: enqueue job %s: %w", kind, err)
	}
	return true, nil
}

// ClaimJob atomically claims the oldest due pending job for a site. Returns
// nil when nothing is claimable.
func (s *Store) ClaimJob(ctx context.Context, siteID int64) (*Job, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE jobs SET status='in_progress', updated_at=now()
		WHERE id = (
			SELECT id FROM jobs
			WHERE site_id=$1 AND status='pending' AND run_after <= now()
			ORDER BY run_after, id
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, site_id, board_id, thread_id, kind, status, attempts, max_attempts, run_after, coalesce(last_error,''), payload`,
		siteID)
	var j Job
	err := row.Scan(&j.ID, &j.SiteID, &j.BoardID, &j.ThreadID, &j.Kind, &j.Status,
		&j.Attempts, &j.MaxAttempts, &j.RunAfter, &j.LastError, &j.Payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: claim job: %w", err)
	}
	return &j, nil
}

// CompleteJob marks a job done.
func (s *Store) CompleteJob(ctx context.Context, jobID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE jobs SET status='done', updated_at=now() WHERE id=$1`, jobID)
	if err != nil {
		return fmt.Errorf("store: complete job %d: %w", jobID, err)
	}
	return nil
}

// FailJob bumps attempts and either fails the job (past max_attempts) or
// reschedules it as pending after a backoff delay. The backoff is computed by
// the caller from the current attempt count.
func (s *Store) FailJob(ctx context.Context, jobID int64, errMsg string, backoff time.Duration) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET attempts = attempts + 1,
		    last_error = $2,
		    status = CASE WHEN attempts + 1 >= max_attempts THEN 'failed'
		                  ELSE 'pending' END,
		    run_after = CASE WHEN attempts + 1 >= max_attempts THEN run_after
		                     ELSE now() + $3 END,
		    updated_at = now()
		WHERE id = $1`, jobID, errMsg, backoff)
	if err != nil {
		return fmt.Errorf("store: fail job %d: %w", jobID, err)
	}
	return nil
}

// PendingJobCount returns the number of pending/in_progress jobs for a site
// and kind (used to avoid enqueuing duplicate work).
func (s *Store) PendingJobCount(ctx context.Context, siteID int64, kind string, boardID, threadID *int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM jobs
		WHERE site_id=$1 AND kind=$2 AND status IN ('pending','in_progress')
		  AND board_id IS NOT DISTINCT FROM $3
		  AND thread_id IS NOT DISTINCT FROM $4`, siteID, kind, boardID, threadID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count jobs: %w", err)
	}
	return n, nil
}
