// Package scheduler runs the polling loop: a per-site supervisor enqueues
// catalog/thread/backfill jobs into the Postgres-backed queue, and per-site
// worker pools claim and execute them. Rate limiting, concurrency caps, retry
// backoff and the per-site circuit breaker all live here.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"akyuu/internal/adapter"
	"akyuu/internal/config"
	"akyuu/internal/downloader"
	"akyuu/internal/store"
)

// claimIdle is how long a worker sleeps when there is nothing to claim.
const claimIdle = 2 * time.Second

// Scheduler drives all configured sites.
type Scheduler struct {
	store   *store.Store
	cfg     *config.Config
	sites   []*config.SiteConfig
	checker downloader.SafetyChecker
	logger  *slog.Logger
}

// New builds a Scheduler.
func New(st *store.Store, cfg *config.Config, sites []*config.SiteConfig, checker downloader.SafetyChecker, logger *slog.Logger) *Scheduler {
	return &Scheduler{store: st, cfg: cfg, sites: sites, checker: checker, logger: logger}
}

// Run starts one supervisor goroutine per site and blocks until ctx is done.
func (s *Scheduler) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, sc := range s.sites {
		wg.Add(1)
		go func(sc *config.SiteConfig) {
			defer wg.Done()
			s.runSite(ctx, sc)
		}(sc)
	}
	<-ctx.Done()
	s.logger.Info("shutting down scheduler")
	wg.Wait()
}

// runSite supervises a single site: a worker pool drains that site's jobs from
// the queue while a ticker enqueues work respecting per-board poll intervals.
func (s *Scheduler) runSite(ctx context.Context, sc *config.SiteConfig) {
	lg := s.logger.With("site", sc.Name)

	transport := newSiteTransport(sc)
	adapt, err := adapter.New(adapter.Platform(sc.Platform), sc.BaseURL, sc.UserAgent, transport, lg)
	if err != nil {
		lg.Error("adapter init failed; site disabled", "err", err)
		return
	}
	dl := downloader.New(s.store, transport, s.cfg.Storage.Dir, s.checker, lg)

	siteRec, err := s.store.GetSite(ctx, sc.Name)
	if err != nil {
		lg.Error("site not in database; ensure SyncSites ran", "err", err)
		return
	}
	siteID := siteRec.ID

	// Auto-discover boards for archive sites before workers start so the
	// first enqueue pass sees them.
	if sc.AutoBoards {
		if err := s.discoverBoards(ctx, sc, siteID, adapt, lg); err != nil {
			lg.Error("board discovery failed; site disabled", "err", err)
			return
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < s.cfg.Scheduler.WorkersPerSite; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.worker(ctx, sc, siteID, adapt, dl, lg)
		}()
	}

	lastPoll := map[string]time.Time{}
	ticker := time.NewTicker(s.shortestInterval(sc))
	defer ticker.Stop()

	// First pass immediately so a fresh install starts working right away.
	s.enqueueWork(ctx, sc, siteID, lastPoll, lg)

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-ticker.C:
			s.enqueueWork(ctx, sc, siteID, lastPoll, lg)
		}
	}
}

// discoverBoards lists the site's boards through its adapter and upserts any
// that are missing, appending them to the site config so the rest of the
// pipeline (enqueueWork, downloadPolicy) treats them like static boards.
func (s *Scheduler) discoverBoards(ctx context.Context, sc *config.SiteConfig, siteID int64, adapt adapter.Adapter, lg *slog.Logger) error {
	bl, ok := adapt.(adapter.BoardLister)
	if !ok {
		return fmt.Errorf("platform %q does not support board listing", sc.Platform)
	}
	infos, err := bl.ListBoards(ctx)
	if err != nil {
		return err
	}
	for _, bi := range infos {
		if _, err := s.store.UpsertBoard(ctx, siteID, bi.Code, bi.Title, bi.NSFW, !bi.NSFW); err != nil {
			lg.Warn("upsert discovered board", "board", bi.Code, "err", err)
			continue
		}
		sc.Boards = append(sc.Boards, config.BoardConfig{
			Code:     bi.Code,
			Title:    bi.Title,
			NSFW:     bi.NSFW,
			Worksafe: !bi.NSFW,
		})
		lg.Info("board discovered", "board", bi.Code, "title", bi.Title, "nsfw", bi.NSFW)
	}
	return nil
}

func (s *Scheduler) shortestInterval(sc *config.SiteConfig) time.Duration {
	min := sc.PollInterval.D()
	if min <= 0 {
		min = 30 * time.Second
	}
	for _, bc := range sc.Boards {
		if i := bc.PollInterval.D(); i > 0 && i < min {
			min = i
		}
	}
	if min < time.Second {
		min = time.Second
	}
	return min
}

// worker claims and runs one job at a time for a site.
func (s *Scheduler) worker(ctx context.Context, sc *config.SiteConfig, siteID int64, adapt adapter.Adapter, dl *downloader.Downloader, lg *slog.Logger) {
	for {
		if ctx.Err() != nil {
			return
		}
		if s.circuitOpen(ctx, siteID, sc, lg) {
			return
		}
		job, err := s.store.ClaimJob(ctx, siteID)
		if err != nil {
			lg.Warn("claim job failed", "err", err)
			if !sleep(ctx, claimIdle) {
				return
			}
			continue
		}
		if job == nil {
			if !sleep(ctx, claimIdle) {
				return
			}
			continue
		}
		s.runJob(ctx, sc, siteID, adapt, dl, job, lg)
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// runJob dispatches a claimed job by kind.
func (s *Scheduler) runJob(ctx context.Context, sc *config.SiteConfig, siteID int64, adapt adapter.Adapter, dl *downloader.Downloader, job *store.Job, lg *slog.Logger) {
	jlg := lg.With("job", job.ID, "kind", job.Kind)
	switch job.Kind {
	case store.JobCatalog:
		s.runCatalog(ctx, sc, siteID, adapt, job, jlg)
	case store.JobThread:
		s.runThread(ctx, sc, siteID, adapt, dl, job, jlg)
	case store.JobDownload:
		s.runDownload(ctx, dl, job, jlg)
	case store.JobBackfill:
		s.runBackfill(ctx, sc, dl, job, jlg)
	default:
		jlg.Warn("unknown job kind; marking failed", "kind", job.Kind)
		s.store.FailJob(ctx, job.ID, "unknown job kind", time.Minute)
	}
}
