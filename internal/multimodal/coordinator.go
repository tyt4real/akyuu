package multimodal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"akyuu/internal/store"
)

type Coordinator struct {
	store        StoreAPI
	logger       *slog.Logger
	workers      map[string]Worker
	storageRoot  string
	pollInterval time.Duration
}

type CoordinatorConfig struct {
	StorageRoot  string
	PollInterval time.Duration
	OCR          OCRConfig
	CLIP         CLIPConfig
	Whisper      WhisperConfig
}

type OCRConfig struct {
	Enabled      bool
	TesseractCmd string
	Languages    string
}

type CLIPConfig struct {
	Enabled      bool
	ModelDir     string
	ModelVersion string
	Dimensions   int
}

type WhisperConfig struct {
	Enabled    bool
	WhisperCmd string
	ModelPath  string
	ModelName  string
}

func NewCoordinator(st StoreAPI, cfg CoordinatorConfig, logger *slog.Logger) (*Coordinator, error) {
	workers := make(map[string]Worker)

	if cfg.OCR.Enabled {
		ocr := NewOCRWorker(st, cfg.StorageRoot, cfg.OCR.TesseractCmd, cfg.OCR.Languages)
		workers[store.JobOCR] = ocr
		logger.Info("OCR worker enabled", "languages", cfg.OCR.Languages)
	}

	if cfg.CLIP.Enabled {
		clip, err := NewCLIPWorker(st, cfg.StorageRoot, cfg.CLIP.ModelDir, cfg.CLIP.ModelVersion, cfg.CLIP.Dimensions)
		if err != nil {
			return nil, fmt.Errorf("clip worker: %w", err)
		}
		workers[store.JobCLIP] = clip
		logger.Info("CLIP worker enabled", "model", cfg.CLIP.ModelDir, "dim", cfg.CLIP.Dimensions)
	}

	if cfg.Whisper.Enabled {
		whisper := NewWhisperWorker(st, cfg.StorageRoot, cfg.Whisper.WhisperCmd, cfg.Whisper.ModelPath, cfg.Whisper.ModelName)
		workers[store.JobWhisper] = whisper
		logger.Info("Whisper worker enabled", "model", cfg.Whisper.ModelName)
	}

	if len(workers) == 0 {
		return nil, fmt.Errorf("no multimodal workers enabled")
	}

	pollInterval := cfg.PollInterval
	if pollInterval == 0 {
		pollInterval = 10 * time.Second
	}

	return &Coordinator{
		store:        st,
		logger:       logger,
		workers:      workers,
		storageRoot:  cfg.StorageRoot,
		pollInterval: pollInterval,
	}, nil
}

func (c *Coordinator) Run(ctx context.Context) {
	c.logger.Info("multimodal coordinator starting", "workers", len(c.workers))
	defer c.logger.Info("multimodal coordinator stopped")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			c.processOnce(ctx)
			select {
			case <-ctx.Done():
				return
			case <-time.After(c.pollInterval):
			}
		}
	}
}

func (c *Coordinator) processOnce(ctx context.Context) {
	for kind, worker := range c.workers {
		job, err := c.claimJob(ctx, kind)
		if err != nil {
			c.logger.Error("claim job failed", "kind", kind, "err", err)
			continue
		}
		if job == nil {
			continue
		}

		var payload JobPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			c.logger.Error("unmarshal job payload", "kind", kind, "job", job.ID, "err", err)
			c.store.FailJob(ctx, job.ID, "invalid payload", time.Minute)
			continue
		}

		c.logger.Debug("processing multimodal job", "kind", kind, "job", job.ID, "file", payload.FileID)

		if err := worker.Process(ctx, payload); err != nil {
			c.logger.Error("multimodal job failed", "kind", kind, "job", job.ID, "err", err)
			c.store.FailJob(ctx, job.ID, err.Error(), time.Minute*5)
			continue
		}

		if err := c.store.CompleteJob(ctx, job.ID); err != nil {
			c.logger.Error("complete job failed", "job", job.ID, "err", err)
		}
	}
}

func (c *Coordinator) claimJob(ctx context.Context, kind string) (*store.Job, error) {
	// We need to claim across all sites - for multimodal workers, we can process any site's jobs
	// For simplicity, we'll query all sites and try to claim from each
	sites, err := c.store.ListSites(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sites: %w", err)
	}

	for _, site := range sites {
		row := c.store.QueryRow(ctx, `
			UPDATE jobs SET status='in_progress', updated_at=now()
			WHERE id = (
				SELECT id FROM jobs
				WHERE site_id=$1 AND kind=$2 AND status='pending' AND run_after <= now()
				ORDER BY run_after, id
				LIMIT 1
				FOR UPDATE SKIP LOCKED
			)
			RETURNING id, site_id, board_id, thread_id, kind, status, attempts, max_attempts, run_after, coalesce(last_error,''), payload`,
			site.ID, kind)

		var job store.Job
		err := row.Scan(&job.ID, &job.SiteID, &job.BoardID, &job.ThreadID, &job.Kind, &job.Status,
			&job.Attempts, &job.MaxAttempts, &job.RunAfter, &job.LastError, &job.Payload)
		if err == nil {
			return &job, nil
		}
		// pgx.ErrNoRows means no job for this site, try next
	}

	return nil, nil
}
