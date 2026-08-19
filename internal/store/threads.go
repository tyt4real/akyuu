package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrThreadNotFound is returned when a native thread id is not in the DB.
var ErrThreadNotFound = errors.New("store: thread not found")

// UpsertThread creates a thread if missing (returning created=true) or updates
// its catalog-level metadata. Returns the thread id.
func (s *Store) UpsertThread(ctx context.Context, boardID int64, nativeID, subject string, sticky, locked, archived bool, lastBump int64) (id int64, created bool, err error) {
	err = s.pool.QueryRow(ctx, `
		INSERT INTO threads (board_id, thread_native_id, subject, sticky, locked, archived, last_bump_time)
		VALUES ($1, $2, NULLIF($3,''), $4, $5, $6, $7)
		ON CONFLICT (board_id, thread_native_id) DO UPDATE
			SET subject       = EXCLUDED.subject,
			    sticky        = EXCLUDED.sticky,
			    locked        = EXCLUDED.locked,
			    archived      = EXCLUDED.archived,
			    last_bump_time = COALESCE(EXCLUDED.last_bump_time, threads.last_bump_time),
			    missing_count = 0,
			    status        = CASE WHEN threads.status = 'archived' THEN 'active' ELSE threads.status END
		RETURNING id, (xmax = 0)`,
		boardID, nativeID, subject, sticky, locked, archived, tsOrNull(lastBump)).Scan(&id, &created)
	if err != nil {
		return 0, false, fmt.Errorf("store: upsert thread %s: %w", nativeID, err)
	}
	return id, created, nil
}

// GetThreadByNative resolves a thread row by its native id.
func (s *Store) GetThreadByNative(ctx context.Context, boardID int64, nativeID string) (*Thread, error) {
	return s.scanThread(s.pool.QueryRow(ctx, `
		SELECT id, board_id, thread_native_id, coalesce(subject,''), sticky, locked, archived,
		       status, last_bump_time, reply_count, file_count, missing_count, last_seen_at
		FROM threads WHERE board_id=$1 AND thread_native_id=$2`, boardID, nativeID))
}

// GetThreadByID resolves a thread row by its primary key.
func (s *Store) GetThreadByID(ctx context.Context, id int64) (*Thread, error) {
	return s.scanThread(s.pool.QueryRow(ctx, `
		SELECT id, board_id, thread_native_id, coalesce(subject,''), sticky, locked, archived,
		       status, last_bump_time, reply_count, file_count, missing_count, last_seen_at
		FROM threads WHERE id=$1`, id))
}

// ListActiveThreads returns non-archived threads for a board.
func (s *Store) ListActiveThreads(ctx context.Context, boardID int64) ([]*Thread, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, board_id, thread_native_id, coalesce(subject,''), sticky, locked, archived,
		       status, last_bump_time, reply_count, file_count, missing_count, last_seen_at
		FROM threads WHERE board_id=$1 AND status='active' ORDER BY id`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Thread
	for rows.Next() {
		t, err := scanThread(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListStaleThreads returns active threads for a board whose last_seen_at is
// older than staleAfter (or that were never seen), so the scheduler can re-poll.
func (s *Store) ListStaleThreads(ctx context.Context, boardID int64, staleAfter time.Duration) ([]*Thread, error) {
	cutoff := time.Now().Add(-staleAfter)
	rows, err := s.pool.Query(ctx, `
		SELECT id, board_id, thread_native_id, coalesce(subject,''), sticky, locked, archived,
		       status, last_bump_time, reply_count, file_count, missing_count, last_seen_at
		FROM threads
		WHERE board_id=$1 AND status='active'
		  AND (last_seen_at IS NULL OR last_seen_at < $2)
		ORDER BY id`, boardID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Thread
	for rows.Next() {
		t, err := scanThread(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// MarkThreadSeen refreshes last_seen_at and catalog stats after a successful poll.
func (s *Store) MarkThreadSeen(ctx context.Context, threadID int64, replyCount, fileCount int, lastBump int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE threads
		SET last_seen_at = now(), missing_count = 0,
		    reply_count = GREATEST($2, reply_count),
		    file_count  = GREATEST($3, file_count),
		    last_bump_time = COALESCE($4, last_bump_time)
		WHERE id = $1`, threadID, replyCount, fileCount, tsOrNull(lastBump))
	if err != nil {
		return fmt.Errorf("store: mark thread %d seen: %w", threadID, err)
	}
	return nil
}

// MarkThreadArchived moves a thread to archived and stops further polling.
func (s *Store) MarkThreadArchived(ctx context.Context, threadID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE threads SET status='archived', archived=TRUE, missing_count=0 WHERE id=$1`, threadID)
	if err != nil {
		return fmt.Errorf("store: archive thread %d: %w", threadID, err)
	}
	return nil
}

// RecordThreadMissing increments the consecutive-miss counter; returns the new
// count so the caller can decide whether to archive.
func (s *Store) RecordThreadMissing(ctx context.Context, threadID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		UPDATE threads SET missing_count = missing_count + 1
		WHERE id=$1 RETURNING missing_count`, threadID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: record missing thread %d: %w", threadID, err)
	}
	return n, nil
}

type rowScanner interface{ Scan(...any) error }

func scanThread(r rowScanner) (*Thread, error) {
	var t Thread
	if err := r.Scan(&t.ID, &t.BoardID, &t.NativeID, &t.Subject, &t.Sticky, &t.Locked,
		&t.Archived, &t.Status, &t.LastBumpTime, &t.ReplyCount, &t.FileCount,
		&t.MissingCount, &t.LastSeenAt); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) scanThread(r rowScanner) (*Thread, error) {
	t, err := scanThread(r)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrThreadNotFound
		}
		return nil, fmt.Errorf("store: scan thread: %w", err)
	}
	return t, nil
}

func tsOrNull(ts int64) any {
	if ts <= 0 {
		return nil
	}
	return time.Unix(ts, 0)
}
