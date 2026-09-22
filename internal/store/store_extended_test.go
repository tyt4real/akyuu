package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"akyuu/internal/adapter"
)

func TestArchivePageCursorAndMirrorDedup(t *testing.T) {
	st := testStore(t)
	mustClean(t, st)
	ctx := context.Background()

	fourchanID, err := st.UpsertSite(ctx, "4chan", "https://boards.4chan.org", "fourchan", true)
	if err != nil {
		t.Fatal(err)
	}
	desuID, err := st.UpsertSite(ctx, "desuarchive", "https://desuarchive.org", "desuarchive", true)
	if err != nil {
		t.Fatal(err)
	}

	// A fresh auto-discovered board starts at page 1.
	desuA, err := st.UpsertBoard(ctx, desuID, "a", "Anime & Manga", false, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.GetBoard(ctx, "desuarchive", "a")
	if err != nil {
		t.Fatal(err)
	}
	if b.ArchivePage != 1 {
		t.Fatalf("archive_page = %d, want 1", b.ArchivePage)
	}

	// Advancing the cursor survives a reload.
	if err := st.SetBoardArchivePage(ctx, desuA, 42); err != nil {
		t.Fatal(err)
	}
	b, err = st.GetBoardByID(ctx, desuA)
	if err != nil {
		t.Fatal(err)
	}
	if b.ArchivePage != 42 {
		t.Errorf("archive_page = %d, want 42", b.ArchivePage)
	}

	// A live-4chan thread for the same post number exists on the 4chan board.
	liveBoard, err := st.UpsertBoard(ctx, fourchanID, "a", "Anime & Manga", false, true)
	if err != nil {
		t.Fatal(err)
	}
	liveThread, created, err := st.UpsertThread(ctx, liveBoard, "290240714", "live op", false, false, false, 1787078099)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected live thread created")
	}

	// The archive mirror resolves to the live thread, so the scheduler skips
	// re-pulling it. The board's own rows are not matches.
	mirror, err := st.FindThreadMirror(ctx, desuA, "290240714")
	if err != nil {
		t.Fatal(err)
	}
	if mirror == nil || mirror.ID != liveThread {
		t.Fatalf("mirror = %+v, want live thread %d", mirror, liveThread)
	}
	if _, err := st.FindThreadMirror(ctx, liveBoard, "290240714"); err != nil {
		t.Fatalf("same-board lookup must not error: %v", err)
	}

	// A thread that has no mirror anywhere returns nil.
	noMirror, err := st.FindThreadMirror(ctx, desuA, "555555555")
	if err != nil {
		t.Fatal(err)
	}
	if noMirror != nil {
		t.Fatalf("noMirror = %+v, want nil", noMirror)
	}

	// vichan sites are not part of the fourchan family: a matching native id
	// there must not be treated as a mirror.
	vichanID, err := st.UpsertSite(ctx, "lainchan", "https://lainchan.org", "vichan", true)
	if err != nil {
		t.Fatal(err)
	}
	vichanBoard, err := st.UpsertBoard(ctx, vichanID, "a", "Anime", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.UpsertThread(ctx, vichanBoard, "290240714", "unrelated", false, false, false, 0); err != nil {
		t.Fatal(err)
	}
	mirror, err = st.FindThreadMirror(ctx, desuA, "290240714")
	if err != nil {
		t.Fatal(err)
	}
	if mirror == nil || mirror.ID != liveThread {
		t.Fatalf("mirror = %+v, want live 4chan thread %d", mirror, liveThread)
	}
}

func TestEmbeddingLifecycleAndSearch(t *testing.T) {
	st := testStore(t)
	mustClean(t, st)
	ctx := context.Background()

	siteID, err := st.UpsertSite(ctx, "4chan", "https://boards.4chan.org", "fourchan", true)
	if err != nil {
		t.Fatal(err)
	}
	boardID, err := st.UpsertBoard(ctx, siteID, "g", "Technology", false, true)
	if err != nil {
		t.Fatal(err)
	}
	threadID, _, err := st.UpsertThread(ctx, boardID, "100", "linux thread", false, false, false, 1000)
	if err != nil {
		t.Fatal(err)
	}

	// Three posts: two with text (one with a file), one empty.
	posts := []*adapter.Post{
		{NativeID: "100", ThreadID: "100", Timestamp: 1000, CommentHTML: "best linux distro debates",
			Files: []*adapter.File{{FullURL: "https://x/1.png"}}},
		{NativeID: "101", ThreadID: "100", ParentID: "100", Timestamp: 2000, CommentHTML: "cooking recipes thread"},
		{NativeID: "102", ThreadID: "100", ParentID: "100", Timestamp: 3000, CommentHTML: ""},
	}
	if err := st.UpsertThreadPosts(ctx, threadID, posts); err != nil {
		t.Fatal(err)
	}

	// All three are pending.
	batch, err := st.PendingEmbeddingBatch(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 3 {
		t.Fatalf("pending batch = %d, want 3", len(batch))
	}
	var id100, id101, id102 int64
	for _, p := range batch {
		switch p.NativeID {
		case "100":
			id100 = p.ID
		case "101":
			id101 = p.ID
		case "102":
			id102 = p.ID
		}
	}
	if id100 == 0 || id101 == 0 || id102 == 0 {
		t.Fatalf("ids not resolved: %d %d %d", id100, id101, id102)
	}

	// Embed the text posts, skip the empty one.
	if err := st.MarkEmbedding(ctx, id100, vecPrimary(0), "mini", "hash1"); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkEmbedding(ctx, id101, vecPrimary(1), "mini", "hash2"); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkEmbedding(ctx, id101, vecPrimary(1), "mini", "hash2b"); err != nil {
		t.Fatal(err) // idempotent upsert path
	}
	if err := st.MarkEmbeddingSkipped(ctx, id102); err != nil {
		t.Fatal(err)
	}

	// Flags cleared; only nothing left pending.
	if batch, _ := st.PendingEmbeddingBatch(ctx, 10); len(batch) != 0 {
		t.Fatalf("pending batch after processing = %d, want 0", len(batch))
	}
	var embedCount int
	if err := st.pool.QueryRow(ctx, `SELECT count(*) FROM post_embeddings`).Scan(&embedCount); err != nil {
		t.Fatal(err)
	}
	if embedCount != 2 {
		t.Errorf("embedding rows = %d, want 2", embedCount)
	}

	// Search: query aligned with post 100 ranks it first.
	now := time.Now()
	epoch := time.Unix(0, 0)
	late := time.Unix(4000, 0)
	res, err := st.SearchEmbeddings(ctx, vecPrimary(0), "mini", SearchOpts{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("results = %d, want 2", len(res))
	}
	if res[0].PostID != id100 {
		t.Errorf("top hit = %d, want %d (score %.3f)", res[0].PostID, id100, res[0].Score)
	}
	if res[0].Score <= res[1].Score {
		t.Errorf("ordering not by score: %.3f <= %.3f", res[0].Score, res[1].Score)
	}
	if res[0].Site != "4chan" || res[0].Board != "g" || res[0].ThreadNativeID != "100" ||
		res[0].ThreadID != threadID || res[0].Excerpt == "" {
		t.Errorf("result metadata = %+v", res[0])
	}

	// Different model_version must be isolated (nothing matches "other").
	if res, _ := st.SearchEmbeddings(ctx, vecPrimary(0), "other", SearchOpts{}); len(res) != 0 {
		t.Errorf("cross-model results = %d, want 0", len(res))
	}

	// Site/board/date/attachment/thread filters.
	if res, _ := st.SearchEmbeddings(ctx, vecPrimary(0), "mini", SearchOpts{Site: "nope"}); len(res) != 0 {
		t.Errorf("site filter not applied: %d", len(res))
	}
	if res, _ := st.SearchEmbeddings(ctx, vecPrimary(0), "mini", SearchOpts{Board: "g"}); len(res) != 2 {
		t.Errorf("board filter: %d", len(res))
	}
	if res, _ := st.SearchEmbeddings(ctx, vecPrimary(0), "mini", SearchOpts{DateFrom: &now}); len(res) != 0 {
		t.Errorf("date_from filter: %d", len(res))
	}
	if res, _ := st.SearchEmbeddings(ctx, vecPrimary(0), "mini", SearchOpts{DateTo: &now}); len(res) != 2 {
		t.Errorf("date_to filter: %d", len(res))
	}
	if res, _ := st.SearchEmbeddings(ctx, vecPrimary(0), "mini", SearchOpts{DateFrom: &epoch, DateTo: &late}); len(res) != 2 {
		t.Errorf("date range filter: %d", len(res))
	}
	has := true
	if res, _ := st.SearchEmbeddings(ctx, vecPrimary(0), "mini", SearchOpts{HasAttachment: &has}); len(res) != 1 || res[0].PostID != id100 {
		t.Errorf("has_attachment filter: %+v", res)
	}
	no := false
	if res, _ := st.SearchEmbeddings(ctx, vecPrimary(0), "mini", SearchOpts{HasAttachment: &no}); len(res) != 1 || res[0].PostID != id101 {
		t.Errorf("no-attachment filter: %+v", res)
	}
	if res, _ := st.SearchEmbeddings(ctx, vecPrimary(0), "mini", SearchOpts{ThreadID: &threadID}); len(res) != 2 {
		t.Errorf("thread filter: %d", len(res))
	}
}

func TestPendingEmbeddingSetOnEdit(t *testing.T) {
	st := testStore(t)
	mustClean(t, st)
	ctx := context.Background()

	siteID, _ := st.UpsertSite(ctx, "s", "https://s", "vichan", true)
	boardID, _ := st.UpsertBoard(ctx, siteID, "b", "B", false, true)
	threadID, _, _ := st.UpsertThread(ctx, boardID, "1", "t", false, false, false, 0)

	posts := []*adapter.Post{{NativeID: "1", ThreadID: "1", Timestamp: 1, CommentHTML: "hello world"}}
	if err := st.UpsertThreadPosts(ctx, threadID, posts); err != nil {
		t.Fatal(err)
	}
	batch, _ := st.PendingEmbeddingBatch(ctx, 10)
	if len(batch) != 1 {
		t.Fatalf("pending after insert = %d", len(batch))
	}
	id := batch[0].ID
	if err := st.MarkEmbeddingSkipped(ctx, id); err != nil {
		t.Fatal(err)
	}

	// Re-poll with unchanged text must NOT re-flag.
	if err := st.UpsertThreadPosts(ctx, threadID, posts); err != nil {
		t.Fatal(err)
	}
	if batch, _ := st.PendingEmbeddingBatch(ctx, 10); len(batch) != 0 {
		t.Errorf("unchanged re-poll re-flagged: %d", len(batch))
	}
	// Re-poll with changed text must re-flag.
	posts[0].CommentHTML = "hello world again"
	if err := st.UpsertThreadPosts(ctx, threadID, posts); err != nil {
		t.Fatal(err)
	}
	if batch, _ := st.PendingEmbeddingBatch(ctx, 10); len(batch) != 1 {
		t.Errorf("edited re-poll not re-flagged: %d", len(batch))
	}
}

// vecPrimary builds a unit vector of EMBEDDING_DIM dimensions with a 1 at
// index primary and 0 elsewhere.
func vecPrimary(primary int) []float32 {
	v := make([]float32, 384)
	v[primary] = 1
	return v
}

func TestThreadLifecycle(t *testing.T) {
	st := testStore(t)
	mustClean(t, st)
	ctx := context.Background()

	siteID, _ := st.UpsertSite(ctx, "s", "https://s", "lynxchan", true)
	boardID, _ := st.UpsertBoard(ctx, siteID, "b", "B", false, true)
	threadID, created, err := st.UpsertThread(ctx, boardID, "100", "t", false, false, false, 0)
	if err != nil || !created {
		t.Fatalf("upsert: created=%v err=%v", created, err)
	}

	if err := st.MarkThreadSeen(ctx, threadID, 5, 3, 0); err != nil {
		t.Fatal(err)
	}
	th, err := st.GetThreadByNative(ctx, boardID, "100")
	if err != nil {
		t.Fatal(err)
	}
	if th.ReplyCount != 5 || th.FileCount != 3 || th.LastSeenAt == nil {
		t.Errorf("after first seen: %+v", th)
	}

	// Stats are greatest-so-far; lower counts do not regress them.
	if err := st.MarkThreadSeen(ctx, threadID, 2, 1, 0); err != nil {
		t.Fatal(err)
	}
	th, _ = st.GetThreadByID(ctx, threadID)
	if th.ReplyCount != 5 || th.FileCount != 3 {
		t.Errorf("stats regressed: %+v", th)
	}

	// Missing count accumulates; archiving clears status from active.
	n, err := st.RecordThreadMissing(ctx, threadID)
	if err != nil || n != 1 {
		t.Errorf("missing #1 = %d err=%v", n, err)
	}
	n, _ = st.RecordThreadMissing(ctx, threadID)
	if n != 2 {
		t.Errorf("missing #2 = %d", n)
	}
	if err := st.MarkThreadArchived(ctx, threadID); err != nil {
		t.Fatal(err)
	}
	active, _ := st.ListActiveThreads(ctx, boardID)
	if len(active) != 0 {
		t.Errorf("active = %+v, want empty", active)
	}
	th, _ = st.GetThreadByID(ctx, threadID)
	if th.Status != "archived" || !th.Archived {
		t.Errorf("thread = %+v", th)
	}

	// Re-appearing in a catalog resurrects the thread.
	_, created, err = st.UpsertThread(ctx, boardID, "100", "t", false, false, false, 0)
	if err != nil || created {
		t.Fatalf("re-upsert: created=%v err=%v", created, err)
	}
	th, _ = st.GetThreadByID(ctx, threadID)
	if th.Status != "active" || th.MissingCount != 0 {
		t.Errorf("resurrected thread = %+v", th)
	}
}

func TestListStaleThreads(t *testing.T) {
	st := testStore(t)
	mustClean(t, st)
	ctx := context.Background()

	siteID, _ := st.UpsertSite(ctx, "s", "https://s", "lynxchan", true)
	boardID, _ := st.UpsertBoard(ctx, siteID, "b", "B", false, true)
	threadID, _, _ := st.UpsertThread(ctx, boardID, "100", "t", false, false, false, 0)
	st.MarkThreadSeen(ctx, threadID, 0, 0, 0)

	// Backdate the last-seen marker to simulate an old thread.
	if _, err := st.pool.Exec(ctx,
		`UPDATE threads SET last_seen_at = now() - interval '6 hours' WHERE id=$1`, threadID); err != nil {
		t.Fatal(err)
	}

	stale, err := st.ListStaleThreads(ctx, boardID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 1 || stale[0].NativeID != "100" {
		t.Errorf("stale = %+v", stale)
	}
	// Fresh threads are not stale.
	st.MarkThreadSeen(ctx, threadID, 0, 0, 0)
	stale, _ = st.ListStaleThreads(ctx, boardID, time.Hour)
	if len(stale) != 0 {
		t.Errorf("fresh thread reported stale: %+v", stale)
	}
}

func TestJobsFailExhaustsToFailed(t *testing.T) {
	st := testStore(t)
	mustClean(t, st)
	ctx := context.Background()

	siteID, _ := st.UpsertSite(ctx, "s", "https://s", "lynxchan", true)
	boardID, _ := st.UpsertBoard(ctx, siteID, "b", "B", false, true)
	threadID, _, _ := st.UpsertThread(ctx, boardID, "1", "", false, false, false, 0)

	if _, err := st.EnqueueJob(ctx, siteID, &boardID, &threadID, JobThread, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	job, err := st.ClaimJob(ctx, siteID)
	if err != nil || job == nil {
		t.Fatalf("claim: %v %v", job, err)
	}
	if job.MaxAttempts != 10 {
		t.Fatalf("max_attempts = %d, want 10", job.MaxAttempts)
	}
	// Fail until attempts exhaust; each failure reschedules pending.
	for i := 0; i < 9; i++ {
		if err := st.FailJob(ctx, job.ID, "boom", time.Millisecond); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := st.PendingJobCount(ctx, siteID, JobThread, &boardID, &threadID); n != 1 {
		t.Errorf("pending after 9 fails = %d", n)
	}
	// Tenth fail pushes past max_attempts -> failed, not claimable.
	if err := st.FailJob(ctx, job.ID, "boom", time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if n, _ := st.PendingJobCount(ctx, siteID, JobThread, &boardID, &threadID); n != 0 {
		t.Errorf("pending after exhaust = %d, want 0", n)
	}
	if again, err := st.ClaimJob(ctx, siteID); err != nil || again != nil {
		t.Errorf("exhausted job re-claimed: %+v err=%v", again, err)
	}
}

func TestPostOriginalData(t *testing.T) {
	st := testStore(t)
	mustClean(t, st)
	ctx := context.Background()

	siteID, _ := st.UpsertSite(ctx, "4chan", "https://boards.4chan.org", "fourchan", true)
	boardID, _ := st.UpsertBoard(ctx, siteID, "g", "Tech", false, true)
	threadID, _, _ := st.UpsertThread(ctx, boardID, "100", "test thread", false, false, false, 0)

	posts := []*adapter.Post{
		{
			NativeID:       "100",
			ThreadID:       "100",
			Timestamp:      1000,
			CommentHTML:    "original post",
			OriginalBoard:  "g",
			Website:        "4chan",
			OriginalThread: "100",
			OriginalLink:   "https://i.4cdn.org/g/thread/100/",
			Files: []*adapter.File{
				{
					FullURL:  "https://i.4cdn.org/g/100.jpg",
					ThumbURL: "https://i.4cdn.org/g/100s.jpg",
				},
			},
		},
	}

	if err := st.UpsertThreadPosts(ctx, threadID, posts); err != nil {
		t.Fatal(err)
	}

	// Verify the new columns are persisted
	var ob, website, otn, oal string
	err := st.pool.QueryRow(ctx,
		`SELECT original_board, website, original_thread_number, original_attachment_link
		 FROM posts WHERE thread_id=$1 AND post_native_id='100'`, threadID).
		Scan(&ob, &website, &otn, &oal)
	if err != nil {
		t.Fatal(err)
	}
	if ob != "g" {
		t.Errorf("original_board = %q, want %q", ob, "g")
	}
	if website != "4chan" {
		t.Errorf("website = %q, want %q", website, "4chan")
	}
	if otn != "100" {
		t.Errorf("original_thread_number = %q, want %q", otn, "100")
	}
	if oal != "https://i.4cdn.org/g/thread/100/" {
		t.Errorf("original_attachment_link = %q, want %q", oal, "https://i.4cdn.org/g/thread/100/")
	}
}

func TestPostOriginalDataIdempotentUpdate(t *testing.T) {
	st := testStore(t)
	mustClean(t, st)
	ctx := context.Background()

	siteID, _ := st.UpsertSite(ctx, "4chan", "https://boards.4chan.org", "fourchan", true)
	boardID, _ := st.UpsertBoard(ctx, siteID, "g", "Tech", false, true)
	threadID, _, _ := st.UpsertThread(ctx, boardID, "100", "test thread", false, false, false, 0)

	posts := []*adapter.Post{
		{
			NativeID:       "100",
			ThreadID:       "100",
			Timestamp:      1000,
			CommentHTML:    "original post",
			OriginalBoard:  "g",
			Website:        "4chan",
			OriginalThread: "100",
			OriginalLink:   "https://i.4cdn.org/g/thread/100/",
		},
	}

	// First insert
	if err := st.UpsertThreadPosts(ctx, threadID, posts); err != nil {
		t.Fatal(err)
	}

	// Verify values
	var ob, website, otn, oal string
	err := st.pool.QueryRow(ctx,
		`SELECT original_board, website, original_thread_number, original_attachment_link
		 FROM posts WHERE thread_id=$1 AND post_native_id='100'`, threadID).
		Scan(&ob, &website, &otn, &oal)
	if err != nil {
		t.Fatal(err)
	}
	if ob != "g" || website != "4chan" || otn != "100" || oal != "https://i.4cdn.org/g/thread/100/" {
		t.Errorf("initial values: ob=%q website=%q otn=%q oal=%q", ob, website, otn, oal)
	}

	// Re-upsert with different original data
	posts[0].OriginalBoard = "g2"
	posts[0].Website = "4chan2"
	posts[0].OriginalThread = "200"
	posts[0].OriginalLink = "https://i.4cdn.org/g2/thread/200/"

	if err := st.UpsertThreadPosts(ctx, threadID, posts); err != nil {
		t.Fatal(err)
	}

	// Verify updated values
	err = st.pool.QueryRow(ctx,
		`SELECT original_board, website, original_thread_number, original_attachment_link
		 FROM posts WHERE thread_id=$1 AND post_native_id='100'`, threadID).
		Scan(&ob, &website, &otn, &oal)
	if err != nil {
		t.Fatal(err)
	}
	if ob != "g2" {
		t.Errorf("after update original_board = %q, want %q", ob, "g2")
	}
	if website != "4chan2" {
		t.Errorf("after update website = %q, want %q", website, "4chan2")
	}
	if otn != "200" {
		t.Errorf("after update original_thread_number = %q, want %q", otn, "200")
	}
	if oal != "https://i.4cdn.org/g2/thread/200/" {
		t.Errorf("after update original_attachment_link = %q, want %q", oal, "https://i.4cdn.org/g2/thread/200/")
	}
}

func TestPostOriginalDataNilValues(t *testing.T) {
	st := testStore(t)
	mustClean(t, st)
	ctx := context.Background()

	siteID, _ := st.UpsertSite(ctx, "4chan", "https://boards.4chan.org", "fourchan", true)
	boardID, _ := st.UpsertBoard(ctx, siteID, "g", "Tech", false, true)
	threadID, _, _ := st.UpsertThread(ctx, boardID, "100", "test thread", false, false, false, 0)

	// Post without the new fields (zero values)
	posts := []*adapter.Post{
		{
			NativeID: "100",
			ThreadID: "100",
			// No OriginalBoard, Website, OriginalThread, OriginalLink set
		},
	}

	if err := st.UpsertThreadPosts(ctx, threadID, posts); err != nil {
		t.Fatal(err)
	}

	// Verify NULL values are stored
	var ob, website, otn, oal sql.NullString
	err := st.pool.QueryRow(ctx,
		`SELECT original_board, website, original_thread_number, original_attachment_link
		 FROM posts WHERE thread_id=$1 AND post_native_id='100'`, threadID).
		Scan(&ob, &website, &otn, &oal)
	if err != nil {
		t.Fatal(err)
	}
	if ob.Valid {
		t.Errorf("expected NULL original_board, got %q", ob.String)
	}
	if website.Valid {
		t.Errorf("expected NULL website, got %q", website.String)
	}
	if otn.Valid {
		t.Errorf("expected NULL original_thread_number, got %q", otn.String)
	}
	if oal.Valid {
		t.Errorf("expected NULL original_attachment_link, got %q", oal.String)
	}
}

// TestBlobCrossSiteDedup verifies platform-provided hashes bind file rows to
// existing blobs across sites before any bytes are downloaded.
func TestBlobCrossSiteDedup(t *testing.T) {
	st := testStore(t)
	mustClean(t, st)
	ctx := context.Background()

	// Site A downloads a file whose platform md5 is known.
	sa, _ := st.UpsertSite(ctx, "a", "https://a", "fourchan", false)
	ba, _ := st.UpsertBoard(ctx, sa, "g", "G", false, true)
	ta, _, _ := st.UpsertThread(ctx, ba, "100", "", false, false, false, 0)
	st.UpsertThreadPosts(ctx, ta, []*adapter.Post{{
		NativeID: "100", ThreadID: "100", CommentRaw: "x",
		Files: []*adapter.File{{OriginalFilename: "a.png", Ext: ".png",
			FullURL: "https://a/g/src/1.png", MD5: "abc123"}},
	}})
	pendings, _ := st.ListPendingDownloads(ctx, ta)
	if len(pendings) != 1 {
		t.Fatalf("pending = %d", len(pendings))
	}
	if err := st.RecordDownloadedFile(ctx, pendings[0].FileID, &Blob{
		FileHash: "shaHASH", MimeType: "image/png", SizeBytes: 10,
		StoragePath: "full/shaHASH.png", PlatformMD5: "abc123", FirstSeenPostID: pendings[0].PostID,
	}, true, false); err != nil {
		t.Fatal(err)
	}

	// Site B posts the same file; UpsertThreadPosts must bind it via md5.
	sb, _ := st.UpsertSite(ctx, "b", "https://b", "fourchan", false)
	bb, _ := st.UpsertBoard(ctx, sb, "b", "B", false, true)
	tb, _, _ := st.UpsertThread(ctx, bb, "200", "", false, false, false, 0)
	st.UpsertThreadPosts(ctx, tb, []*adapter.Post{{
		NativeID: "200", ThreadID: "200", CommentRaw: "y",
		Files: []*adapter.File{{OriginalFilename: "a.png", Ext: ".png",
			FullURL: "https://b/b/src/1.png", MD5: "abc123"}},
	}})

	pendings, _ = st.ListPendingDownloads(ctx, tb)
	if len(pendings) != 1 || pendings[0].FileHash != "shaHASH" {
		t.Fatalf("site B file not bound to blob: %+v", pendings)
	}
	blob, err := st.GetBlobByPlatformHash(ctx, "abc123", "")
	if err != nil || blob == nil || blob.FileHash != "shaHASH" {
		t.Errorf("GetBlobByPlatformHash = %+v err=%v", blob, err)
	}
	if _, err := st.GetBlobByPlatformHash(ctx, "", ""); err != ErrBlobNotFound {
		t.Errorf("empty hashes err = %v, want ErrBlobNotFound", err)
	}
}

func TestUpsertThreadPostsIdempotentAndReplacesQuotes(t *testing.T) {
	st := testStore(t)
	mustClean(t, st)
	ctx := context.Background()

	siteID, _ := st.UpsertSite(ctx, "s", "https://s", "lynxchan", true)
	boardID, _ := st.UpsertBoard(ctx, siteID, "b", "B", false, true)
	threadID, _, _ := st.UpsertThread(ctx, boardID, "1", "", false, false, false, 0)

	posts := []*adapter.Post{
		{NativeID: "1", ThreadID: "1", CommentRaw: "first version"},
		{NativeID: "2", ThreadID: "1", ParentID: "1", CommentRaw: ">>3 old", Quotes: []adapter.QuoteRef{{PostID: "3"}}},
	}
	if err := st.UpsertThreadPosts(ctx, threadID, posts); err != nil {
		t.Fatal(err)
	}
	if n := postCount(t, st, threadID); n != 2 {
		t.Fatalf("posts = %d", n)
	}

	// Re-upsert edits OP comment and changes the reply's quote set.
	posts[0].CommentRaw = "second version"
	posts[1].CommentRaw = ">>4 new"
	posts[1].Quotes = []adapter.QuoteRef{{PostID: "4"}}
	if err := st.UpsertThreadPosts(ctx, threadID, posts); err != nil {
		t.Fatal(err)
	}
	if n := postCount(t, st, threadID); n != 2 {
		t.Errorf("post count after re-upsert = %d, want 2 (no dupes)", n)
	}

	var comment string
	if err := st.pool.QueryRow(ctx,
		`SELECT comment_raw FROM posts WHERE thread_id=$1 AND post_native_id='1'`, threadID).
		Scan(&comment); err != nil {
		t.Fatal(err)
	}
	if comment != "second version" {
		t.Errorf("comment = %q, want updated value", comment)
	}

	// Quote set replaced, not accumulated.
	var quote string
	if err := st.pool.QueryRow(ctx,
		`SELECT quoted_post_native_id FROM posts p JOIN post_quotes q ON q.post_id=p.id
		 WHERE p.thread_id=$1 AND p.post_native_id='2'`, threadID).Scan(&quote); err != nil {
		t.Fatal(err)
	}
	if quote != "4" {
		t.Errorf("quote = %q, want '4'", quote)
	}
}

func TestUpsertSiteBoardUpdatesInPlace(t *testing.T) {
	st := testStore(t)
	mustClean(t, st)
	ctx := context.Background()

	siteID, err := st.UpsertSite(ctx, "s", "https://s", "lynxchan", true)
	if err != nil {
		t.Fatal(err)
	}
	// Same site, updated platform: same id, new values.
	id2, _ := st.UpsertSite(ctx, "s", "https://s2", "vichan", false)
	if id2 != siteID {
		t.Errorf("site id changed: %d -> %d", siteID, id2)
	}
	site, _ := st.GetSite(ctx, "s")
	if site.Platform != "vichan" || site.BaseURL != "https://s2" {
		t.Errorf("site not updated: %+v", site)
	}

	boardID, _ := st.UpsertBoard(ctx, siteID, "b", "Old", false, true)
	boardID2, _ := st.UpsertBoard(ctx, siteID, "b", "New", true, false)
	if boardID != boardID2 {
		t.Errorf("board id changed")
	}
	board, _ := st.GetBoardByID(ctx, boardID)
	if board.Title != "New" || !board.NSFW {
		t.Errorf("board not updated: %+v", board)
	}
}
