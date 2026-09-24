package embedder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"time"

	"akyuu/internal/store"
)

// StoreAPI is the slice of the store the worker touches, so the loop can be
// exercised against a fake in unit tests and the real store in integration
// tests.
type StoreAPI interface {
	PendingEmbeddingBatch(ctx context.Context, limit int) ([]*store.Post, error)
	MarkEmbedding(ctx context.Context, postID int64, vec []float32, modelVersion, textHash string) error
	MarkEmbeddingSkipped(ctx context.Context, postID int64) error
}

// Worker drains the pending_embedding queue: it pulls batches of posts whose
// bodies still need a vector, embeds them and stores the result. Posts whose
// cleaned text is empty or below MinTextLength are marked non-searchable so
// they never come back around. Because every pending post is resolved one way
// or the other, the worker doubles as the one-time backfill for existing
// archives.
type Worker struct {
	store                StoreAPI
	embed                Embedder
	batchSize            int
	pollInterval         time.Duration
	minTextLength        int
	normalizeBeforeEmbed bool
	normalizer           TextNormalizer
	logger               *slog.Logger
}

// WorkerConfig carries the knobs for a Worker.
type WorkerConfig struct {
	BatchSize            int
	PollInterval         time.Duration
	MinTextLength        int
	NormalizeBeforeEmbed bool
	Normalizer           TextNormalizer
}

// NewWorker wires a worker onto a store, an embedder, and an optional normalizer.
func NewWorker(st StoreAPI, e Embedder, cfg WorkerConfig, logger *slog.Logger) *Worker {
	if cfg.Normalizer == nil {
		cfg.Normalizer = NewFakeNormalizer()
	}
	return &Worker{
		store:                st,
		embed:                e,
		batchSize:            cfg.BatchSize,
		pollInterval:         cfg.PollInterval,
		minTextLength:        cfg.MinTextLength,
		normalizeBeforeEmbed: cfg.NormalizeBeforeEmbed,
		normalizer:           cfg.Normalizer,
		logger:               logger,
	}
}

// Run processes pending posts until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	w.logger.Info("embed worker starting",
		"model", w.embed.ModelVersion(), "dimensions", w.embed.Dimensions())
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		posts, err := w.store.PendingEmbeddingBatch(ctx, w.batchSize)
		if err != nil {
			w.logger.Error("embed worker: pending batch", "err", err)
			if !sleep(ctx, w.pollInterval) {
				return
			}
			continue
		}
		if len(posts) == 0 {
			if !sleep(ctx, w.pollInterval) {
				return
			}
			continue
		}
		w.processBatch(ctx, posts)
	}
}

// processBatch resolves one pending batch: short/empty bodies are skipped,
// the rest are embedded and stored. It returns the numbers embedded and
// skipped for observability and tests.
func (w *Worker) processBatch(ctx context.Context, posts []*store.Post) (embedded, skipped int) {
	// Split the batch into embeddable texts and instant skips.
	type job struct {
		post *store.Post
		text string
	}
	var jobs []job
	for _, p := range posts {
		text, err := CleanText(p.CommentParsed)
		if err != nil {
			w.logger.Warn("embed worker: clean post", "post", p.ID, "err", err)
			continue
		}
		if w.normalizeBeforeEmbed {
			normalized, err := w.normalizer.Normalize(ctx, text)
			if err != nil {
				w.logger.Warn("embed worker: normalize text", "post", p.ID, "err", err)
			} else {
				text = normalized
			}
		}
		if len(text) < w.minTextLength {
			if err := w.store.MarkEmbeddingSkipped(ctx, p.ID); err != nil {
				w.logger.Error("embed worker: skip post", "post", p.ID, "err", err)
				continue
			}
			skipped++
			continue
		}
		jobs = append(jobs, job{post: p, text: text})
	}
	if len(jobs) == 0 {
		return embedded, skipped
	}

	texts := make([]string, len(jobs))
	for i, j := range jobs {
		texts[i] = j.text
	}
	vecs, err := w.embed.EmbedBatch(ctx, texts)
	if err != nil {
		w.logger.Error("embed worker: embed batch", "count", len(texts), "err", err)
		return embedded, skipped
	}
	for i, j := range jobs {
		if err := w.store.MarkEmbedding(ctx, j.post.ID, vecs[i],
			w.embed.ModelVersion(), hashText(j.text)); err != nil {
			w.logger.Error("embed worker: store vector", "post", j.post.ID, "err", err)
			continue
		}
		embedded++
	}
	return embedded, skipped
}

// hashText produces a stable content hash for a cleaned body, used to detect
// whether a stored vector still matches the current text.
func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// sleep waits for d unless ctx is cancelled first.
func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
