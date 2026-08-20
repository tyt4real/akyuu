package embedder

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"akyuu/internal/adapter"
	"akyuu/internal/store"
)

// TestONNXWorkerEndToEnd runs the full pipeline with the real model and real
// pgvector search: posts are inserted, the worker embeds them, and a free-text
// query finds the on-topic post over an off-topic one. Requires both the ONNX
// model files and a test database:
//
//	AKYUU_TEST_MODEL_DIR=/path/to/all-MiniLM-L6-v2 \
//	AKYUU_TEST_DSN='postgres://akyuu:akyuu@localhost:5433/akyuu?sslmode=disable' \
//	ORT_LIBRARY_PATH=/path/to/libonnxruntime.so \
//	  go test -count=1 ./internal/embedder/ -run TestONNXWorkerEndToEnd -v
func TestONNXWorkerEndToEnd(t *testing.T) {
	dsn := os.Getenv("AKYUU_TEST_DSN")
	modelDir := os.Getenv("AKYUU_TEST_MODEL_DIR")
	if dsn == "" || modelDir == "" {
		t.Skip("set AKYUU_TEST_DSN and AKYUU_TEST_MODEL_DIR for the real-model e2e test")
	}
	ctx := context.Background()
	st, err := store.New(ctx, dsn, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.ResetAll(ctx); err != nil {
		t.Fatal(err)
	}

	model, err := NewONNX(modelDir, "all-MiniLM-L6-v2", 384)
	if err != nil {
		t.Fatalf("load model: %v", err)
	}
	defer model.Close()

	siteID, err := st.UpsertSite(ctx, "fourchan", "https://boards.4chan.org", "fourchan", false)
	if err != nil {
		t.Fatal(err)
	}
	boardID, err := st.UpsertBoard(ctx, siteID, "g", "Technology", true, true)
	if err != nil {
		t.Fatal(err)
	}
	threadID, _, err := st.UpsertThread(ctx, boardID, "200", "distro thread", false, false, false, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertThreadPosts(ctx, threadID, []*adapter.Post{
		{NativeID: "201", ThreadID: "200", ParentID: "200", Timestamp: 2001,
			CommentHTML: "which distro should I switch to, arch or debian? I hear great things about both for desktop linux."},
		{NativeID: "202", ThreadID: "200", ParentID: "200", Timestamp: 2002,
			CommentHTML: "my cat knocked over a cup of water and it landed on the kitchen floor."},
	}); err != nil {
		t.Fatal(err)
	}

	w := NewWorker(st, model, WorkerConfig{BatchSize: 10, MinTextLength: 8}, testLogger())
	for i := 0; i < 3; i++ {
		pending, err := st.PendingEmbeddingBatch(ctx, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) == 0 {
			break
		}
		w.processBatch(ctx, pending)
		if i == 2 {
			t.Fatal("queue did not drain")
		}
	}

	query, err := model.EmbedBatch(ctx, []string{"recommend me a good operating system"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := st.SearchEmbeddings(ctx, query[0], "all-MiniLM-L6-v2", store.SearchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) < 2 {
		t.Fatalf("expected both posts in the archive, got %d", len(res))
	}
	if !strings.Contains(res[0].Excerpt, "distro") {
		t.Errorf("on-topic post should rank first, got %+v", res[0])
	}
	if res[0].Score < res[1].Score {
		t.Errorf("on-topic score %.4f should exceed off-topic %.4f", res[0].Score, res[1].Score)
	}
}
