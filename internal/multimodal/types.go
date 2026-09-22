package multimodal

import (
	"context"
	"time"

	"akyuu/internal/store"
)

// JobPayload is the payload for multimodal jobs.
type JobPayload struct {
	FileID   int64  `json:"file_id"`
	PostID   int64  `json:"post_id"`
	MimeType string `json:"mime_type"`
}

// StoreAPI is the interface the multimodal workers need from the store.
type StoreAPI interface {
	// QueryRow executes a query that returns a single row.
	QueryRow(ctx context.Context, query string, args ...any) store.RowScanner

	// ListSites returns all sites.
	ListSites(ctx context.Context) ([]*store.Site, error)

	// OCR
	UpsertOCR(ctx context.Context, postID int64, text, textCleaned, language string, confidence float32, engine, engineVersion, model string) error

	// CLIP
	UpsertCLIPEmbedding(ctx context.Context, postID, fileID int64, embedding []float32, modelVersion string) error

	// Whisper
	UpsertTranscript(ctx context.Context, postID, fileID int64, text, language string, durationSec float32, segments any, engine, model string) error

	// Job control
	FailJob(ctx context.Context, jobID int64, errMsg string, backoff time.Duration) error
	CompleteJob(ctx context.Context, jobID int64) error
}

// Worker processes multimodal jobs of a specific kind.
type Worker interface {
	Kind() string
	Process(ctx context.Context, payload JobPayload) error
}
