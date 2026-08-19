package scheduler

import (
	"context"
	"log/slog"
	"time"

	"akyuu/internal/config"
	"akyuu/internal/store"
)

// enqueueWork is the per-tick scheduler pass for a site: it enqueues catalog
// jobs for boards whose poll interval has elapsed, thread jobs for stale
// threads, and a bounded backfill job when downloads are enabled. Deduplication
// is enforced by the partial unique index on active jobs.
func (s *Scheduler) enqueueWork(ctx context.Context, sc *config.SiteConfig, siteID int64, lastPoll map[string]time.Time, lg *slog.Logger) {
	now := time.Now()
	for _, bc := range sc.Boards {
		board, err := s.store.GetBoard(ctx, sc.Name, bc.Code)
		if err != nil {
			lg.Warn("board not found in database", "board", bc.Code, "err", err)
			continue
		}

		interval := bc.PollInterval.D()
		if interval <= 0 {
			interval = sc.PollInterval.D()
		}
		if interval <= 0 {
			interval = 30 * time.Second
		}
		if last := lastPoll[bc.Code]; now.Sub(last) < interval {
			continue
		}
		lastPoll[bc.Code] = now

		// Board catalog poll. Paginated-catalog sites carry the crawl cursor
		// (page) and the number of pages to crawl per poll in the payload.
		if n, _ := s.store.PendingJobCount(ctx, siteID, store.JobCatalog, &board.ID, nil); n == 0 {
			var payload any
			if sc.PaginatedCatalog {
				payload = map[string]any{
					"page":  board.ArchivePage,
					"pages": sc.CrawlPagesPerPoll,
				}
			}
			if _, err := s.store.EnqueueJob(ctx, siteID, &board.ID, nil, store.JobCatalog, payload, now); err != nil {
				lg.Warn("enqueue catalog job", "board", bc.Code, "err", err)
			}
		}

		// Stale threads (silently unreachable, never explicitly 404'd).
		stale, err := s.store.ListStaleThreads(ctx, board.ID, s.cfg.Scheduler.ThreadStaleAfter.D())
		if err != nil {
			lg.Warn("list stale threads", "board", bc.Code, "err", err)
		}
		for _, t := range stale {
			if n, _ := s.store.PendingJobCount(ctx, siteID, store.JobThread, &board.ID, &t.ID); n == 0 {
				if _, err := s.store.EnqueueJob(ctx, siteID, &board.ID, &t.ID, store.JobThread, nil, now); err != nil {
					lg.Warn("enqueue stale thread job", "board", bc.Code, "thread", t.NativeID, "err", err)
				}
			}
		}

		// Backfill downloads that were skipped earlier (flags turned on later).
		full, thumb := s.downloadPolicy(sc, &bc)
		if full || thumb {
			if n, _ := s.store.PendingJobCount(ctx, siteID, store.JobBackfill, &board.ID, nil); n == 0 {
				payload := map[string]any{
					"download_full":  full,
					"download_thumb": thumb,
					"limit":          s.cfg.Scheduler.DownloadBackfillBatch,
				}
				if _, err := s.store.EnqueueJob(ctx, siteID, &board.ID, nil, store.JobBackfill, payload, now); err != nil {
					lg.Warn("enqueue backfill job", "board", bc.Code, "err", err)
				}
			}
		}
	}
}

// downloadPolicy resolves the three-level download flags for a board:
// global default -> per-site override -> per-board override (most specific wins).
// text_only is a global kill-switch that disables file storage entirely,
// regardless of any site/board overrides.
func (s *Scheduler) downloadPolicy(sc *config.SiteConfig, bc *config.BoardConfig) (full, thumb bool) {
	if s.cfg.Scheduler.TextOnly {
		return false, false
	}
	full = config.DownloadFlags(s.cfg.Scheduler.DownloadFull, sc.DownloadFull, bc.DownloadFull)
	thumb = config.DownloadFlags(s.cfg.Scheduler.DownloadThumb, sc.DownloadThumb, bc.DownloadThumb)
	return full, thumb
}
