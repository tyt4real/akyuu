package store

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"akyuu/internal/adapter"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("AKYUU_TEST_DSN")
	if dsn == "" {
		t.Skip("set AKYUU_TEST_DSN to run store integration tests")
	}
	st, err := New(context.Background(), dsn, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return st
}

// mustClean wipes tables between tests so ids stay predictable.
func mustClean(t *testing.T, st *Store) {
	t.Helper()
	_, err := st.pool.Exec(context.Background(),
		`TRUNCATE files, post_quotes, posts, threads, boards, sites, blobs, jobs RESTART IDENTITY`)
	if err != nil {
		t.Fatal(err)
	}
}

func postCount(t *testing.T, st *Store, threadID int64) int {
	t.Helper()
	var n int
	if err := st.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM posts WHERE thread_id=$1`, threadID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSiteBoardLifecycle(t *testing.T) {
	st := testStore(t)
	mustClean(t, st)
	ctx := context.Background()

	siteID, err := st.UpsertSite(ctx, "lainchan", "https://lainchan.org", "lynxchan", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertBoard(ctx, siteID, "r", "Random", false, true); err != nil {
		t.Fatal(err)
	}

	sites, err := st.ListSites(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || sites[0].Name != "lainchan" {
		t.Fatalf("sites = %+v", sites)
	}
	board, err := st.GetBoard(ctx, "lainchan", "r")
	if err != nil {
		t.Fatal(err)
	}
	if board.Code != "r" || board.SiteID != siteID {
		t.Errorf("board = %+v", board)
	}
}

func TestThreadPostsQuotesFiles(t *testing.T) {
	st := testStore(t)
	mustClean(t, st)
	ctx := context.Background()

	siteID, _ := st.UpsertSite(ctx, "4chan", "https://boards.4chan.org", "fourchan", false)
	boardID, _ := st.UpsertBoard(ctx, siteID, "g", "Tech", false, true)

	threadID, _, err := st.UpsertThread(ctx, boardID, "952683023", "cel ai", false, false, false, 0)
	if err != nil {
		t.Fatal(err)
	}

	posts := []*adapter.Post{
		{
			NativeID:   "952683023",
			ThreadID:   "952683023",
			Subject:    "cel ai",
			Name:       "Anonymous",
			PosterID:   "AbCd12",
			Timestamp:  1755400133,
			CommentRaw: "Just a little more shine",
			Files: []*adapter.File{
				{
					OriginalFilename: "1772300396251763.jpg",
					ServerFilename:   "1786892933109351.jpg",
					Ext:              ".jpg",
					SizeBytes:        630 * 1024,
					Width:            1792,
					Height:           2400,
					FullURL:          "https://i.4cdn.org/b/1786892933109351.jpg",
					ThumbURL:         "https://i.4cdn.org/b/1786892933109351s.jpg",
				},
			},
		},
		{
			NativeID:   "952685994",
			ThreadID:   "952683023",
			ParentID:   "952683023",
			Timestamp:  1755406705,
			CommentRaw: ">>952685912",
			Quotes:     []adapter.QuoteRef{{PostID: "952685912"}},
		},
	}
	if err := st.UpsertThreadPosts(ctx, threadID, posts); err != nil {
		t.Fatal(err)
	}
	if n := postCount(t, st, threadID); n != 2 {
		t.Fatalf("post count = %d, want 2", n)
	}

	var posterID string
	if err := st.pool.QueryRow(ctx,
		`SELECT p.poster_id FROM posts p
		 WHERE p.thread_id=$1 AND p.post_native_id='952683023'`, threadID).
		Scan(&posterID); err != nil {
		t.Fatal(err)
	}
	if posterID != "AbCd12" {
		t.Errorf("poster_id = %q", posterID)
	}

	var qTarget string
	if err := st.pool.QueryRow(ctx,
		`SELECT q.quoted_post_native_id FROM posts p JOIN post_quotes q ON q.post_id=p.id
		 WHERE p.thread_id=$1 AND p.post_native_id='952685994'`, threadID).Scan(&qTarget); err != nil {
		t.Fatal(err)
	}
	if qTarget != "952685912" {
		t.Errorf("reply quote = %q", qTarget)
	}

	var fileCount int
	if err := st.pool.QueryRow(ctx, `SELECT count(*) FROM files`).Scan(&fileCount); err != nil {
		t.Fatal(err)
	}
	if fileCount != 1 {
		t.Errorf("file count = %d, want 1", fileCount)
	}
}

func TestJobsLifecycle(t *testing.T) {
	st := testStore(t)
	mustClean(t, st)
	ctx := context.Background()

	siteID, _ := st.UpsertSite(ctx, "s", "https://s", "lynxchan", true)
	boardID, _ := st.UpsertBoard(ctx, siteID, "b", "Board", false, true)
	threadID, _, _ := st.UpsertThread(ctx, boardID, "1", "", false, false, false, 0)

	created, err := st.EnqueueJob(ctx, siteID, &boardID, &threadID, JobThread,
		map[string]any{"board": "b", "thread_id": "1"}, time.Now().Add(-time.Minute))
	if err != nil || !created {
		t.Fatalf("enqueue: created=%v err=%v", created, err)
	}
	// Dedup: same job again must not create a second one.
	created, err = st.EnqueueJob(ctx, siteID, &boardID, &threadID, JobThread,
		map[string]any{"board": "b", "thread_id": "1"}, time.Now().Add(-time.Minute))
	if err != nil || created {
		t.Fatalf("dedup enqueue: created=%v err=%v", created, err)
	}

	job, err := st.ClaimJob(ctx, siteID)
	if err != nil {
		t.Fatal(err)
	}
	if job == nil || job.Kind != JobThread || job.MaxAttempts != 10 {
		t.Fatalf("job = %+v", job)
	}
	if err := st.CompleteJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	n, err := st.PendingJobCount(ctx, siteID, JobThread, &boardID, &threadID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("pending = %d, want 0", n)
	}
}

func TestDownloadLifecycle(t *testing.T) {
	st := testStore(t)
	mustClean(t, st)
	ctx := context.Background()

	siteID, _ := st.UpsertSite(ctx, "s", "https://s", "lynxchan", true)
	boardID, _ := st.UpsertBoard(ctx, siteID, "b", "Board", false, true)
	threadID, _, _ := st.UpsertThread(ctx, boardID, "1", "", false, false, false, 0)
	if err := st.UpsertThreadPosts(ctx, threadID, []*adapter.Post{{
		NativeID: "1", ThreadID: "1", CommentRaw: "x",
		Files: []*adapter.File{{
			OriginalFilename: "img.jpg", Ext: ".jpg", SizeBytes: 100,
			FullURL: "https://s/b/src/1.jpg", ThumbURL: "https://s/b/thumb/1.jpg",
		}},
	}}); err != nil {
		t.Fatal(err)
	}

	pending, err := st.ListPendingDownloads(ctx, threadID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending = %d, want 1", len(pending))
	}
	fileID := pending[0].FileID

	blob := &Blob{FileHash: "hash1234", MimeType: "image/jpeg", SizeBytes: 100,
		StoragePath: "full/ab/cd/hash1234.jpg", FirstSeenPostID: pending[0].PostID}
	if err := st.RecordDownloadedFile(ctx, fileID, blob, true, false); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordThumbDownloaded(ctx, fileID, "thumbhash", "thumb/ab/cd/thumbhash.jpg"); err != nil {
		t.Fatal(err)
	}

	got, err := st.GetBlobByHash(ctx, "hash1234")
	if err != nil {
		t.Fatal(err)
	}
	if got.SizeBytes != 100 || got.MimeType != "image/jpeg" || got.RefCount != 1 {
		t.Errorf("blob = %+v", got)
	}
	if got.ThumbStoragePath != "thumb/ab/cd/thumbhash.jpg" {
		t.Errorf("blob thumb path = %q", got.ThumbStoragePath)
	}

	pending, err = st.ListBoardPendingDownloads(ctx, boardID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("pending downloads after record = %+v", pending)
	}
}

func TestHealth(t *testing.T) {
	st := testStore(t)
	mustClean(t, st)
	ctx := context.Background()

	siteID, _ := st.UpsertSite(ctx, "s", "https://s", "lynxchan", true)
	for i := 0; i < 5; i++ {
		if _, err := st.RecordSiteFailure(ctx, siteID, "boom"); err != nil {
			t.Fatal(err)
		}
	}
	// The scheduler opens the circuit once failures reach the threshold.
	if err := st.SetCircuitOpen(ctx, siteID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	open, _, err := st.CircuitStatus(ctx, siteID)
	if err != nil {
		t.Fatal(err)
	}
	if !open {
		t.Error("circuit should be open after SetCircuitOpen")
	}
	// A circuit whose cooldown has expired reports as closed.
	if err := st.SetCircuitOpen(ctx, siteID, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	open, _, err = st.CircuitStatus(ctx, siteID)
	if err != nil {
		t.Fatal(err)
	}
	if open {
		t.Error("expired circuit should report closed")
	}
	if err := st.RecordSiteSuccess(ctx, siteID); err != nil {
		t.Fatal(err)
	}
	open, _, err = st.CircuitStatus(ctx, siteID)
	if err != nil {
		t.Fatal(err)
	}
	if open {
		t.Error("circuit should reset on success")
	}
}
