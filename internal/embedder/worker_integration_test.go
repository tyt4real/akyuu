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

// TestWorkerEmbedThenSearch drives the real store with the real worker loop
// (using the deterministic Fake embedder): posts land in the queue, the short
// one is skipped, the long one is embedded, and a search over the archive
// finds it by meaning rather than by token match.
func TestWorkerEmbedThenSearch(t *testing.T) {
	dsn := os.Getenv("AKYUU_TEST_DSN")
	if dsn == "" {
		t.Skip("set AKYUU_TEST_DSN to run worker integration tests")
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

	siteID, err := st.UpsertSite(ctx, "fourchan", "https://boards.4chan.org", "fourchan", false)
	if err != nil {
		t.Fatal(err)
	}
	boardID, err := st.UpsertBoard(ctx, siteID, "g", "Technology", true, true)
	if err != nil {
		t.Fatal(err)
	}
	threadID, _, err := st.UpsertThread(ctx, boardID, "100", "best distro", false, false, false, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertThreadPosts(ctx, threadID, []*adapter.Post{
		{NativeID: "101", ThreadID: "100", ParentID: "100", Timestamp: 1001,
			CommentHTML: "best linux distro debates in years"},
		{NativeID: "102", ThreadID: "100", ParentID: "100", Timestamp: 1002,
			CommentHTML: "nice"},
	}); err != nil {
		t.Fatal(err)
	}

	w := NewWorker(st, NewFake(), WorkerConfig{BatchSize: 10, MinTextLength: 8}, testLogger())
	for i := 0; i < 3; i++ { // Run until the queue is empty (idempotent).
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

	// Query vector: the Fake embedder is deterministic, so embedding the same
	// cleaned text that the worker embedded reproduces post 101's vector.
	query, err := NewFake().EmbedBatch(ctx, []string{"best linux distro debates in years"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := st.SearchEmbeddings(ctx, query[0], "fake", store.SearchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("expected exactly the embedded post, got %d results", len(res))
	}
	if !strings.Contains(res[0].Excerpt, "distro") {
		t.Errorf("expected the long post to be found, excerpt=%q", res[0].Excerpt)
	}
	if res[0].Site != "fourchan" || res[0].Board != "g" || res[0].ThreadNativeID != "100" {
		t.Errorf("search lost the site/board/thread context: %+v", res[0])
	}
}
