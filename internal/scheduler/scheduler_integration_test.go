package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"akyuu/internal/adapter"
	"akyuu/internal/adapter/httpclient"
	"akyuu/internal/config"
	"akyuu/internal/downloader"
	"akyuu/internal/store"
)

// fakeAdapter is a scripted adapter.Adapter for scheduler tests.
type fakeAdapter struct {
	summaries  []adapter.ThreadSummary
	thread     *adapter.Thread
	catalogErr error
	threadErr  error
}

func (f *fakeAdapter) PlatformName() string { return "fake" }
func (f *fakeAdapter) FetchCatalog(context.Context, string) ([]adapter.ThreadSummary, error) {
	return f.summaries, f.catalogErr
}
func (f *fakeAdapter) FetchThread(context.Context, string, string) (*adapter.Thread, error) {
	return f.thread, f.threadErr
}
func (f *fakeAdapter) ParsePost([]byte) (*adapter.Post, error) { return nil, nil }

// fakePaginatedAdapter adds a paginated catalog and board listing on top of
// fakeAdapter, for archive-site tests.
type fakePaginatedAdapter struct {
	fakeAdapter
	pages        map[int][]adapter.ThreadSummary
	boards       []adapter.BoardInfo
	pageErr      error
	listBoardErr error
}

func (f *fakePaginatedAdapter) FetchCatalogPage(ctx context.Context, board string, page int) ([]adapter.ThreadSummary, error) {
	if f.pageErr != nil {
		return nil, f.pageErr
	}
	return f.pages[page], nil
}

func (f *fakePaginatedAdapter) ListBoards(ctx context.Context) ([]adapter.BoardInfo, error) {
	if f.listBoardErr != nil {
		return nil, f.listBoardErr
	}
	return f.boards, nil
}

func testScheduler(t *testing.T) (*Scheduler, *store.Store) {
	t.Helper()
	dsn := os.Getenv("AKYUU_TEST_DSN")
	if dsn == "" {
		t.Skip("set AKYUU_TEST_DSN to run scheduler integration tests")
	}
	ctx := context.Background()
	st, err := store.New(ctx, dsn, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.ResetAll(ctx); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	s := &Scheduler{store: st, cfg: cfg, logger: slog.New(slog.DiscardHandler)}
	return s, st
}

// seedSite creates a site and one board, returning their ids.
func seedSite(t *testing.T, st *store.Store) (siteID, boardID int64) {
	t.Helper()
	ctx := context.Background()
	siteID, err := st.UpsertSite(ctx, "sched-site", "https://sched.example", "vichan", true)
	if err != nil {
		t.Fatal(err)
	}
	boardID, err = st.UpsertBoard(ctx, siteID, "b", "Board", false, true)
	if err != nil {
		t.Fatal(err)
	}
	return siteID, boardID
}

// enqueueAndClaim inserts a job of the given kind and claims it.
func enqueueAndClaim(t *testing.T, st *store.Store, siteID, boardID, threadID *int64, kind string, payload any) *store.Job {
	t.Helper()
	ctx := context.Background()
	now := time.Now().Add(-time.Second)
	if _, err := st.EnqueueJob(ctx, *siteID, boardID, threadID, kind, payload, now); err != nil {
		t.Fatal(err)
	}
	job, err := st.ClaimJob(ctx, *siteID)
	if err != nil || job == nil {
		t.Fatalf("claim: job=%v err=%v", job, err)
	}
	return job
}

func jobPending(t *testing.T, st *store.Store, siteID int64, job *store.Job) int {
	t.Helper()
	n, err := st.PendingJobCount(context.Background(), siteID, job.Kind, job.BoardID, job.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRunCatalogUpsertsAndEnqueues(t *testing.T) {
	s, st := testScheduler(t)
	siteID, boardID := seedSite(t, st)
	ctx := context.Background()

	// One thread already known, one brand new.
	_, _, err := st.UpsertThread(ctx, boardID, "100", "old", false, false, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	adapt := &fakeAdapter{summaries: []adapter.ThreadSummary{
		{ThreadID: "100", Subject: "old", ReplyCount: 3, FileCount: 2},
		{ThreadID: "200", Subject: "fresh", ReplyCount: 0, FileCount: 0, LastBump: 1000},
	}}

	job := enqueueAndClaim(t, st, &siteID, &boardID, nil, store.JobCatalog, nil)
	s.runCatalog(ctx, &config.SiteConfig{Name: "sched-site"}, siteID, adapt, job, s.logger)

	if n := jobPending(t, st, siteID, job); n != 0 {
		t.Fatalf("catalog job still pending: %d", n)
	}
	// New thread created with metadata.
	th, err := st.GetThreadByNative(ctx, boardID, "200")
	if err != nil {
		t.Fatal(err)
	}
	if th.Subject != "fresh" || th.ReplyCount != 0 || th.MissingCount != 0 {
		t.Errorf("thread = %+v", th)
	}
	// A thread job was enqueued only for the new thread.
	n, _ := st.PendingJobCount(ctx, siteID, store.JobThread, &boardID, &th.ID)
	if n != 1 {
		t.Errorf("thread job for new thread = %d, want 1", n)
	}
	old, _ := st.GetThreadByNative(ctx, boardID, "100")
	if n, _ := st.PendingJobCount(ctx, siteID, store.JobThread, &boardID, &old.ID); n != 0 {
		t.Errorf("thread job for existing thread = %d, want 0", n)
	}
	// Site success recorded.
	site, _ := st.GetSite(ctx, "sched-site")
	if site.ConsecutiveFailures != 0 || site.LastSuccessfulPoll == nil {
		t.Errorf("site health = %+v", site)
	}
}

func TestRunCatalogArchivesMissingThread(t *testing.T) {
	s, st := testScheduler(t)
	siteID, boardID := seedSite(t, st)
	ctx := context.Background()
	s.cfg.Scheduler.MissingBeforeArchive = 1

	threadID, _, _ := st.UpsertThread(ctx, boardID, "100", "ghost", false, false, false, 0)
	adapt := &fakeAdapter{summaries: []adapter.ThreadSummary{}} // thread vanished

	job := enqueueAndClaim(t, st, &siteID, &boardID, nil, store.JobCatalog, nil)
	s.runCatalog(ctx, &config.SiteConfig{Name: "sched-site"}, siteID, adapt, job, s.logger)

	th, err := st.GetThreadByID(ctx, threadID)
	if err != nil {
		t.Fatal(err)
	}
	if th.Status != "archived" || !th.Archived {
		t.Errorf("thread not archived: %+v", th)
	}
}

func TestRunThreadArchivesOn404(t *testing.T) {
	s, st := testScheduler(t)
	siteID, boardID := seedSite(t, st)
	ctx := context.Background()

	threadID, _, _ := st.UpsertThread(ctx, boardID, "100", "gone", false, false, false, 0)
	adapt := &fakeAdapter{threadErr: httpclient.ErrNotFound}

	job := enqueueAndClaim(t, st, &siteID, &boardID, &threadID, store.JobThread, nil)
	s.runThread(ctx, &config.SiteConfig{Name: "sched-site"}, siteID, adapt, nil, job, s.logger)

	if n := jobPending(t, st, siteID, job); n != 0 {
		t.Errorf("job still pending, want done")
	}
	th, _ := st.GetThreadByID(ctx, threadID)
	if th.Status != "archived" {
		t.Errorf("thread status = %q, want archived", th.Status)
	}
}

func TestRunThreadSuccessStoresPostsAndEnqueuesDownload(t *testing.T) {
	s, st := testScheduler(t)
	siteID, boardID := seedSite(t, st)
	ctx := context.Background()
	s.cfg.Scheduler.DownloadThumb = true

	threadID, _, _ := st.UpsertThread(ctx, boardID, "100", "t", false, false, false, 0)
	adapt := &fakeAdapter{thread: &adapter.Thread{
		ThreadID: "100", LastBump: 2000,
		Posts: []*adapter.Post{
			{NativeID: "100", ThreadID: "100", CommentRaw: "op", Files: []*adapter.File{{FullURL: "https://x/1.png"}}},
			{NativeID: "101", ThreadID: "100", ParentID: "100", CommentRaw: "re"},
		},
	}}

	job := enqueueAndClaim(t, st, &siteID, &boardID, &threadID, store.JobThread, nil)
	s.runThread(ctx, &config.SiteConfig{Name: "sched-site"}, siteID, adapt, nil, job, s.logger)

	if n := jobPending(t, st, siteID, job); n != 0 {
		t.Errorf("job still pending")
	}
	// Posts + file landed (the file surfaces as a pending download).
	pendings, err := st.ListPendingDownloads(ctx, threadID)
	if err != nil || len(pendings) != 1 {
		t.Fatalf("pending downloads = %d err=%v, want 1", len(pendings), err)
	}
	// Download job enqueued because DownloadThumb is on.
	n, _ := st.PendingJobCount(ctx, siteID, store.JobDownload, &boardID, &threadID)
	if n != 1 {
		t.Errorf("download jobs = %d, want 1", n)
	}
	th, _ := st.GetThreadByID(ctx, threadID)
	if th.ReplyCount != 1 || th.FileCount != 1 {
		t.Errorf("thread stats = (%d,%d), want (1,1)", th.ReplyCount, th.FileCount)
	}
}

func TestRunThreadFailureFailsJob(t *testing.T) {
	s, st := testScheduler(t)
	siteID, boardID := seedSite(t, st)
	ctx := context.Background()

	threadID, _, _ := st.UpsertThread(ctx, boardID, "100", "t", false, false, false, 0)
	adapt := &fakeAdapter{threadErr: errors.New("boom")}

	job := enqueueAndClaim(t, st, &siteID, &boardID, &threadID, store.JobThread, nil)
	s.runThread(ctx, &config.SiteConfig{Name: "sched-site"}, siteID, adapt, nil, job, s.logger)

	// Failed jobs are rescheduled as pending with backoff.
	if n := jobPending(t, st, siteID, job); n != 1 {
		t.Errorf("job pending count = %d, want 1 (rescheduled)", n)
	}
	site, _ := st.GetSite(ctx, "sched-site")
	if site.ConsecutiveFailures != 1 {
		t.Errorf("consecutive failures = %d, want 1", site.ConsecutiveFailures)
	}
}

func TestRunDownloadTextOnlyCompletesWithoutDownloading(t *testing.T) {
	s, st := testScheduler(t)
	siteID, boardID := seedSite(t, st)
	ctx := context.Background()
	s.cfg.Scheduler.TextOnly = true

	threadID, _, _ := st.UpsertThread(ctx, boardID, "100", "t", false, false, false, 0)
	st.UpsertThreadPosts(ctx, threadID, []*adapter.Post{{
		NativeID: "100", ThreadID: "100", CommentRaw: "x",
		Files: []*adapter.File{{OriginalFilename: "f.png", Ext: ".png",
			FullURL: "https://x/f.png", ThumbURL: "https://x/f.png"}},
	}})
	payload := map[string]any{"download_full": true, "download_thumb": true}
	job := enqueueAndClaim(t, st, &siteID, &boardID, &threadID, store.JobDownload, payload)

	dl := downloader.New(nil, nil, t.TempDir(), downloader.AlwaysAllow{}, slog.New(slog.DiscardHandler))
	s.runDownload(ctx, dl, job, s.logger)

	if n := jobPending(t, st, siteID, job); n != 0 {
		t.Errorf("job still pending")
	}
	if n, _ := st.BlobCount(ctx); n != 0 {
		t.Errorf("blobs = %d, want 0", n)
	}
}

func TestRunBackfillTextOnlyCompletes(t *testing.T) {
	s, st := testScheduler(t)
	siteID, boardID := seedSite(t, st)
	ctx := context.Background()
	s.cfg.Scheduler.TextOnly = true

	threadID, _, _ := st.UpsertThread(ctx, boardID, "100", "t", false, false, false, 0)
	st.UpsertThreadPosts(ctx, threadID, []*adapter.Post{{
		NativeID: "100", ThreadID: "100", CommentRaw: "x",
		Files: []*adapter.File{{OriginalFilename: "f.png", Ext: ".png", FullURL: "https://x/f.png"}},
	}})
	job := enqueueAndClaim(t, st, &siteID, &boardID, nil, store.JobBackfill,
		map[string]any{"download_full": true, "download_thumb": true})

	dl := downloader.New(nil, nil, t.TempDir(), downloader.AlwaysAllow{}, slog.New(slog.DiscardHandler))
	s.runBackfill(ctx, &config.SiteConfig{Name: "sched-site"}, dl, job, s.logger)

	if n := jobPending(t, st, siteID, job); n != 0 {
		t.Errorf("job still pending")
	}
	if n, _ := st.BlobCount(ctx); n != 0 {
		t.Errorf("blobs = %d, want 0", n)
	}
}

func TestEnqueueWorkRespectsPollIntervalAndFlags(t *testing.T) {
	s, st := testScheduler(t)
	siteID, boardID := seedSite(t, st)
	ctx := context.Background()

	sc := &config.SiteConfig{Name: "sched-site", PollInterval: config.Duration(30 * time.Second),
		Boards: []config.BoardConfig{{Code: "b"}}}
	lastPoll := map[string]time.Time{}
	s.cfg.Scheduler.DownloadThumb = true

	s.enqueueWork(ctx, sc, siteID, lastPoll, s.logger)
	n, _ := st.PendingJobCount(ctx, siteID, store.JobCatalog, &boardID, nil)
	if n != 1 {
		t.Errorf("catalog jobs = %d, want 1", n)
	}
	n, _ = st.PendingJobCount(ctx, siteID, store.JobBackfill, &boardID, nil)
	if n != 1 {
		t.Errorf("backfill jobs = %d, want 1", n)
	}

	// Second immediate pass must be skipped (poll interval not elapsed) and the
	// existing jobs dedup.
	s.enqueueWork(ctx, sc, siteID, lastPoll, s.logger)
	n, _ = st.PendingJobCount(ctx, siteID, store.JobCatalog, &boardID, nil)
	if n != 1 {
		t.Errorf("catalog jobs after second pass = %d, want 1", n)
	}
}

func TestEnqueueWorkTextOnlySkipsBackfill(t *testing.T) {
	s, st := testScheduler(t)
	siteID, boardID := seedSite(t, st)
	ctx := context.Background()
	s.cfg.Scheduler.DownloadThumb = true
	s.cfg.Scheduler.TextOnly = true

	sc := &config.SiteConfig{Name: "sched-site", PollInterval: config.Duration(30 * time.Second),
		Boards: []config.BoardConfig{{Code: "b"}}}
	s.enqueueWork(ctx, sc, siteID, map[string]time.Time{}, s.logger)

	n, _ := st.PendingJobCount(ctx, siteID, store.JobCatalog, &boardID, nil)
	if n != 1 {
		t.Errorf("catalog jobs = %d, want 1", n)
	}
	if n, _ := st.PendingJobCount(ctx, siteID, store.JobBackfill, &boardID, nil); n != 0 {
		t.Errorf("backfill jobs = %d, want 0 under text_only", n)
	}
}

func TestCircuitBreakerOpensAtThreshold(t *testing.T) {
	s, st := testScheduler(t)
	siteID, _ := seedSite(t, st)
	ctx := context.Background()
	s.cfg.Scheduler.CircuitBreakerThreshold = 3

	job := enqueueAndClaim(t, st, &siteID, nil, nil, store.JobCatalog, nil)
	sc := &config.SiteConfig{Name: "sched-site"}
	for i := 0; i < 3; i++ {
		s.failJob(ctx, sc, siteID, job, s.logger, errors.New("nope"))
	}
	open, _, err := st.CircuitStatus(ctx, siteID)
	if err != nil {
		t.Fatal(err)
	}
	if !open {
		t.Error("circuit should be open after threshold failures")
	}
	// A failed job under the threshold leaves the circuit closed.
	if err := st.ResetAll(ctx); err != nil {
		t.Fatal(err)
	}
	siteID2, _ := st.UpsertSite(ctx, "s2", "https://s2", "vichan", true)
	job2 := enqueueAndClaim(t, st, &siteID2, nil, nil, store.JobCatalog, nil)
	s.failJob(ctx, sc, siteID2, job2, s.logger, errors.New("once"))
	open, _, _ = st.CircuitStatus(ctx, siteID2)
	if open {
		t.Error("circuit should stay closed below threshold")
	}
}

func TestCircuitOpenShortCircuitsOnClosedAndExpired(t *testing.T) {
	s, st := testScheduler(t)
	siteID, _ := seedSite(t, st)
	ctx := context.Background()
	sc := &config.SiteConfig{Name: "sched-site", PollInterval: config.Duration(time.Minute)}

	// No circuit at all: not open.
	if s.circuitOpen(ctx, siteID, sc, s.logger) {
		t.Error("circuitOpen = true without any circuit")
	}
	// Expired circuit: not open, returns immediately.
	if err := st.SetCircuitOpen(ctx, siteID, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if s.circuitOpen(ctx, siteID, sc, s.logger) {
		t.Error("circuitOpen = true for expired circuit")
	}
}

// seedArchiveSite creates a desuarchive site and board.
func seedArchiveSite(t *testing.T, st *store.Store) (siteID, boardID int64) {
	t.Helper()
	ctx := context.Background()
	siteID, err := st.UpsertSite(ctx, "archive", "https://archive.example", "desuarchive", true)
	if err != nil {
		t.Fatal(err)
	}
	boardID, err = st.UpsertBoard(ctx, siteID, "a", "Anime", false, true)
	if err != nil {
		t.Fatal(err)
	}
	return siteID, boardID
}

// seedLiveSite creates a live 4chan site sharing the fourchan platform family.
func seedLiveSite(t *testing.T, st *store.Store) (siteID, boardID int64) {
	t.Helper()
	ctx := context.Background()
	siteID, err := st.UpsertSite(ctx, "live", "https://boards.4chan.org", "fourchan", true)
	if err != nil {
		t.Fatal(err)
	}
	boardID, err = st.UpsertBoard(ctx, siteID, "a", "Anime", false, true)
	if err != nil {
		t.Fatal(err)
	}
	return siteID, boardID
}

func TestRunCatalogPaginatedAdvancesCursor(t *testing.T) {
	s, st := testScheduler(t)
	siteID, boardID := seedArchiveSite(t, st)
	ctx := context.Background()
	if err := st.SetBoardArchivePage(ctx, boardID, 5); err != nil {
		t.Fatal(err)
	}

	adapt := &fakePaginatedAdapter{pages: map[int][]adapter.ThreadSummary{
		5: {{ThreadID: "501", Subject: "t5", Archived: true}},
		6: {{ThreadID: "601", Subject: "t6", Archived: true}},
	}}
	sc := &config.SiteConfig{Name: "archive", PaginatedCatalog: true}
	payload := map[string]any{"page": 5, "pages": 2}
	job := enqueueAndClaim(t, st, &siteID, &boardID, nil, store.JobCatalog, payload)
	s.runCatalog(ctx, sc, siteID, adapt, job, s.logger)

	if n := jobPending(t, st, siteID, job); n != 0 {
		t.Fatalf("catalog job still pending: %d", n)
	}
	board, err := st.GetBoardByID(ctx, boardID)
	if err != nil {
		t.Fatal(err)
	}
	if board.ArchivePage != 7 {
		t.Errorf("cursor = %d, want 7", board.ArchivePage)
	}
	for _, id := range []string{"501", "601"} {
		th, err := st.GetThreadByNative(ctx, boardID, id)
		if err != nil {
			t.Fatalf("thread %s missing: %v", id, err)
		}
		if n, _ := st.PendingJobCount(ctx, siteID, store.JobThread, &boardID, &th.ID); n != 1 {
			t.Errorf("thread job for %s = %d, want 1", id, n)
		}
	}
}

func TestRunCatalogPaginatedWrapsAtEndOfHistory(t *testing.T) {
	s, st := testScheduler(t)
	siteID, boardID := seedArchiveSite(t, st)
	ctx := context.Background()
	if err := st.SetBoardArchivePage(ctx, boardID, 5); err != nil {
		t.Fatal(err)
	}

	// Page 6 is the end of history (empty); the cursor must wrap back to 1 so
	// newly archived threads at the top are picked up on the next pass.
	adapt := &fakePaginatedAdapter{pages: map[int][]adapter.ThreadSummary{
		5: {{ThreadID: "501", Subject: "t5", Archived: true}},
		6: {},
	}}
	sc := &config.SiteConfig{Name: "archive", PaginatedCatalog: true}
	job := enqueueAndClaim(t, st, &siteID, &boardID, nil, store.JobCatalog,
		map[string]any{"page": 5, "pages": 2})
	s.runCatalog(ctx, sc, siteID, adapt, job, s.logger)

	board, err := st.GetBoardByID(ctx, boardID)
	if err != nil {
		t.Fatal(err)
	}
	if board.ArchivePage != 1 {
		t.Errorf("cursor = %d, want 1 (wrap)", board.ArchivePage)
	}
}

func TestRunCatalogPaginatedDoesNotAdvanceOnError(t *testing.T) {
	s, st := testScheduler(t)
	siteID, boardID := seedArchiveSite(t, st)
	ctx := context.Background()
	if err := st.SetBoardArchivePage(ctx, boardID, 5); err != nil {
		t.Fatal(err)
	}

	adapt := &fakePaginatedAdapter{pageErr: errors.New("boom")}
	sc := &config.SiteConfig{Name: "archive", PaginatedCatalog: true}
	job := enqueueAndClaim(t, st, &siteID, &boardID, nil, store.JobCatalog,
		map[string]any{"page": 5, "pages": 1})
	s.runCatalog(ctx, sc, siteID, adapt, job, s.logger)

	board, _ := st.GetBoardByID(ctx, boardID)
	if board.ArchivePage != 5 {
		t.Errorf("cursor advanced on error: %d", board.ArchivePage)
	}
}

func TestRunCatalogPaginatedSkipsMissingArchivePass(t *testing.T) {
	s, st := testScheduler(t)
	siteID, boardID := seedArchiveSite(t, st)
	ctx := context.Background()
	s.cfg.Scheduler.MissingBeforeArchive = 1

	// Active thread that is not part of this history page must NOT be archived.
	ghostID, _, _ := st.UpsertThread(ctx, boardID, "999", "ghost", false, false, false, 0)
	adapt := &fakePaginatedAdapter{pages: map[int][]adapter.ThreadSummary{
		5: {{ThreadID: "501", Subject: "t5", Archived: true}},
	}}
	sc := &config.SiteConfig{Name: "archive", PaginatedCatalog: true}
	job := enqueueAndClaim(t, st, &siteID, &boardID, nil, store.JobCatalog,
		map[string]any{"page": 5, "pages": 1})
	s.runCatalog(ctx, sc, siteID, adapt, job, s.logger)

	th, err := st.GetThreadByID(ctx, ghostID)
	if err != nil {
		t.Fatal(err)
	}
	if th.Status != "active" {
		t.Errorf("thread status = %q, want active (no missing pass on paginated crawls)", th.Status)
	}
}

func TestRunCatalogMirrorDedupSkipsAlreadyPulled(t *testing.T) {
	s, st := testScheduler(t)
	ctx := context.Background()

	// Live 4chan already has thread 501 on its "a" board.
	_, liveBoardID := seedLiveSite(t, st)
	if _, _, err := st.UpsertThread(ctx, liveBoardID, "501", "pulled from live", false, false, true, 1000); err != nil {
		t.Fatal(err)
	}

	// The archive catalog lists 501 (a mirror) and 502 (new).
	siteID, boardID := seedArchiveSite(t, st)
	adapt := &fakePaginatedAdapter{pages: map[int][]adapter.ThreadSummary{
		5: {
			{ThreadID: "501", Subject: "dup", Archived: true},
			{ThreadID: "502", Subject: "new", Archived: true},
		},
	}}
	sc := &config.SiteConfig{Name: "archive", PaginatedCatalog: true}
	job := enqueueAndClaim(t, st, &siteID, &boardID, nil, store.JobCatalog,
		map[string]any{"page": 5, "pages": 1})
	s.runCatalog(ctx, sc, siteID, adapt, job, s.logger)

	// 501 must NOT be duplicated on the archive board.
	if _, err := st.GetThreadByNative(ctx, boardID, "501"); err == nil {
		t.Error("mirror thread duplicated on archive board")
	}
	// 502 is new on the archive board and gets a thread job.
	th, err := st.GetThreadByNative(ctx, boardID, "502")
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := st.PendingJobCount(ctx, siteID, store.JobThread, &boardID, &th.ID); n != 1 {
		t.Errorf("thread job for 502 = %d, want 1", n)
	}
}

func TestRunCatalogMirrorDedupSymmetricFromLive(t *testing.T) {
	s, st := testScheduler(t)
	ctx := context.Background()

	// The archive already pulled 501.
	_, archiveBoardID := seedArchiveSite(t, st)
	if _, _, err := st.UpsertThread(ctx, archiveBoardID, "501", "pulled from archive", false, false, true, 1000); err != nil {
		t.Fatal(err)
	}

	// The live catalog now lists 501 and 503; the mirror must be skipped there
	// too (whole-catalog path).
	liveSiteID, liveBoardID := seedLiveSite(t, st)
	adapt := &fakeAdapter{summaries: []adapter.ThreadSummary{
		{ThreadID: "501", Subject: "dup", Archived: false},
		{ThreadID: "503", Subject: "fresh", Archived: false},
	}}
	sc := &config.SiteConfig{Name: "live"}
	job := enqueueAndClaim(t, st, &liveSiteID, &liveBoardID, nil, store.JobCatalog, nil)
	s.runCatalog(ctx, sc, liveSiteID, adapt, job, s.logger)

	if _, err := st.GetThreadByNative(ctx, liveBoardID, "501"); err == nil {
		t.Error("mirror thread duplicated on live board")
	}
	th, err := st.GetThreadByNative(ctx, liveBoardID, "503")
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := st.PendingJobCount(ctx, liveSiteID, store.JobThread, &liveBoardID, &th.ID); n != 1 {
		t.Errorf("thread job for 503 = %d, want 1", n)
	}
}

func TestEnqueueWorkPaginatedPayload(t *testing.T) {
	s, st := testScheduler(t)
	siteID, boardID := seedArchiveSite(t, st)
	ctx := context.Background()
	if err := st.SetBoardArchivePage(ctx, boardID, 3); err != nil {
		t.Fatal(err)
	}

	sc := &config.SiteConfig{Name: "archive",
		PollInterval:      config.Duration(30 * time.Second),
		PaginatedCatalog:  true,
		CrawlPagesPerPoll: 2,
		Boards:            []config.BoardConfig{{Code: "a"}}}
	s.enqueueWork(ctx, sc, siteID, map[string]time.Time{}, s.logger)

	job, err := st.ClaimJob(ctx, siteID)
	if err != nil || job == nil {
		t.Fatalf("claim catalog job: job=%v err=%v", job, err)
	}
	page, pages := catalogJobPages(job)
	if page != 3 || pages != 2 {
		t.Errorf("payload page=%d pages=%d, want 3/2", page, pages)
	}
}

func TestDiscoverBoardsUpsertsAndAppends(t *testing.T) {
	s, st := testScheduler(t)
	siteID, err := st.UpsertSite(context.Background(), "archive", "https://archive.example", "desuarchive", true)
	if err != nil {
		t.Fatal(err)
	}

	adapt := &fakePaginatedAdapter{boards: []adapter.BoardInfo{
		{Code: "a", Title: "Anime & Manga", NSFW: false},
		{Code: "pol", Title: "Politically Incorrect", NSFW: true},
	}}
	sc := &config.SiteConfig{Name: "archive", Platform: "desuarchive", AutoBoards: true}
	if err := s.discoverBoards(context.Background(), sc, siteID, adapt, s.logger); err != nil {
		t.Fatal(err)
	}

	if len(sc.Boards) != 2 {
		t.Fatalf("sc.Boards = %+v", sc.Boards)
	}
	b, err := st.GetBoard(context.Background(), "archive", "a")
	if err != nil {
		t.Fatal(err)
	}
	if b.Title != "Anime & Manga" || b.Worksafe != true || b.NSFW != false {
		t.Errorf("board a = %+v", b)
	}
	pb, err := st.GetBoard(context.Background(), "archive", "pol")
	if err != nil {
		t.Fatal(err)
	}
	if pb.Worksafe != false || pb.NSFW != true {
		t.Errorf("board pol = %+v", pb)
	}
}

func TestDiscoverBoardsUnsupportedPlatform(t *testing.T) {
	s, st := testScheduler(t)
	siteID, _ := st.UpsertSite(context.Background(), "archive", "https://archive.example", "desuarchive", true)
	adapt := &fakeAdapter{} // no BoardLister
	if err := s.discoverBoards(context.Background(), &config.SiteConfig{Name: "archive"}, siteID, adapt, s.logger); err == nil {
		t.Error("expected error for adapter without BoardLister")
	}
}
