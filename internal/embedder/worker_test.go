package embedder

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"akyuu/internal/store"
)

// fakeStore implements StoreAPI in memory for pure worker-loop tests.
type fakeStore struct {
	pending []*store.Post
	marked  []int64 // posts that got a vector
	skipped []int64 // posts skipped as too short
	vecs    map[int64][]float32
}

func (f *fakeStore) PendingEmbeddingBatch(_ context.Context, limit int) ([]*store.Post, error) {
	if limit < len(f.pending) {
		return f.pending[:limit], nil
	}
	return f.pending, nil
}

func (f *fakeStore) MarkEmbedding(_ context.Context, postID int64, vec []float32, _, _ string) error {
	f.marked = append(f.marked, postID)
	f.vecs[postID] = vec
	return nil
}

func (f *fakeStore) MarkEmbeddingSkipped(_ context.Context, postID int64) error {
	f.skipped = append(f.skipped, postID)
	return nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func post(id int64, comment string) *store.Post {
	return &store.Post{ID: id, CommentParsed: comment}
}

func TestProcessBatchSkipsShortAndEmbedsLong(t *testing.T) {
	fs := &fakeStore{vecs: map[int64][]float32{}}
	fs.pending = []*store.Post{
		post(1, "hi"), // below MinTextLength
		post(2, ""),   // empty body
		post(3, "best linux distro debates in years"), // embeddable
	}
	w := NewWorker(fs, NewFake(), WorkerConfig{BatchSize: 10, MinTextLength: 8}, testLogger())

	embedded, skipped := w.processBatch(t.Context(), fs.pending)
	if embedded != 1 || skipped != 2 {
		t.Fatalf("want 1 embedded, 2 skipped; got %d embedded, %d skipped", embedded, skipped)
	}
	if len(fs.marked) != 1 || fs.marked[0] != 3 {
		t.Errorf("expected post 3 embedded, marked=%v", fs.marked)
	}
	if len(fs.skipped) != 2 || fs.skipped[0] != 1 || fs.skipped[1] != 2 {
		t.Errorf("expected posts 1,2 skipped, skipped=%v", fs.skipped)
	}
	if v := fs.vecs[3]; len(v) != 384 {
		t.Errorf("post 3 vector should be 384 wide, got %d", len(v))
	}
}

func TestProcessBatchEmbedderFailureLeavesPending(t *testing.T) {
	fs := &fakeStore{vecs: map[int64][]float32{}}
	fs.pending = []*store.Post{post(1, "a long enough text to embed")}
	// Force the embed call to fail: hand the worker an embedder that errors.
	badW := NewWorker(fs, failingEmbedder{}, WorkerConfig{BatchSize: 10, MinTextLength: 8}, testLogger())
	if embedded, skipped := badW.processBatch(t.Context(), fs.pending); embedded != 0 || skipped != 0 {
		t.Fatalf("expected nothing processed on failure, got %d/%d", embedded, skipped)
	}
	if len(fs.marked) != 0 {
		t.Errorf("post must stay pending when embedding fails, marked=%v", fs.marked)
	}
}

type failingEmbedder struct{}

func (failingEmbedder) EmbedBatch(_ context.Context, _ []string) ([][]float32, error) {
	return nil, errEmptySequence
}
func (failingEmbedder) Dimensions() int      { return 384 }
func (failingEmbedder) ModelVersion() string { return "broken" }

// testStore is the real store for the DSN-gated integration test.
type testStore interface {
	StoreAPI
}

var _ testStore = (*store.Store)(nil)
