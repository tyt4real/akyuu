package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"akyuu/internal/adapter"
	"akyuu/internal/adapter/httpclient"
	"akyuu/internal/config"
	"akyuu/internal/downloader"
	"akyuu/internal/store"
)

// runCatalog polls a board's thread list, upserts threads, archives threads
// that vanished from the catalog, and queues thread fetches for new ones. On
// paginated-catalog sites it walks the page cursor instead of the live board
// catalog and skips the "missing from catalog" archive pass entirely.
func (s *Scheduler) runCatalog(ctx context.Context, sc *config.SiteConfig, siteID int64, adapt adapter.Adapter, job *store.Job, lg *slog.Logger) {
	board, err := s.store.GetBoardByID(ctx, *job.BoardID)
	if err != nil {
		s.failJob(ctx, sc, siteID, job, lg, err)
		return
	}

	page, pages := catalogJobPages(job)
	var summaries []adapter.ThreadSummary
	nextPage := 0

	if page > 0 {
		pc, ok := adapt.(adapter.PaginatedCataloger)
		if !ok {
			s.failJob(ctx, sc, siteID, job, lg, fmt.Errorf("adapter does not support paginated catalogs"))
			return
		}
		nextPage = board.ArchivePage
		for i := 0; i < pages; i++ {
			p := page + i
			got, err := pc.FetchCatalogPage(ctx, board.Code, p)
			if err != nil {
				s.failJob(ctx, sc, siteID, job, lg, err)
				return
			}
			if len(got) == 0 {
				// Reached the end of history; wrap the cursor back to the
				// newest page so newly archived threads are still picked up.
				nextPage = 1
				break
			}
			nextPage = p + 1
			summaries = append(summaries, got...)
		}
	} else {
		summaries, err = adapt.FetchCatalog(ctx, board.Code)
		if err != nil {
			s.failJob(ctx, sc, siteID, job, lg, err)
			return
		}
	}
	s.store.RecordSiteSuccess(ctx, siteID)
	s.store.CompleteJob(ctx, job.ID)
	lg.Info("catalog polled", "board", board.Code, "threads", len(summaries))

	seen := map[string]bool{}
	now := time.Now()
	for _, sum := range summaries {
		seen[sum.ThreadID] = true

		// Mirror dedup: if a thread with this native id already exists on any
		// other fourchan-family board, skip it entirely so a thread is never
		// pulled twice (live 4chan and desuarchive mirrors share post numbers).
		mirror, err := s.store.FindThreadMirror(ctx, board.ID, sum.ThreadID)
		if err != nil {
			lg.Warn("mirror lookup", "thread", sum.ThreadID, "err", err)
			continue
		}
		if mirror != nil {
			lg.Info("thread already pulled elsewhere; skipping",
				"board", board.Code, "thread", sum.ThreadID, "mirror_board", mirror.BoardID)
			continue
		}

		threadID, created, err := s.store.UpsertThread(ctx, board.ID, sum.ThreadID, sum.Subject,
			sum.Sticky, sum.Locked, sum.Archived, sum.LastBump)
		if err != nil {
			lg.Warn("upsert thread", "board", board.Code, "thread", sum.ThreadID, "err", err)
			continue
		}
		if err := s.store.MarkThreadSeen(ctx, threadID, sum.ReplyCount, sum.FileCount, sum.LastBump); err != nil {
			lg.Warn("mark thread seen", "err", err)
		}
		if created {
			if _, err := s.store.EnqueueJob(ctx, siteID, &board.ID, &threadID, store.JobThread, nil, now); err != nil {
				lg.Warn("enqueue new thread job", "thread", sum.ThreadID, "err", err)
			}
			lg.Info("new thread", "board", board.Code, "thread", sum.ThreadID)
		}
	}

	// Paginated crawls walk thread history, so "active but missing from this
	// page" is meaningless; just persist the advanced cursor.
	if page > 0 {
		if err := s.store.SetBoardArchivePage(ctx, board.ID, nextPage); err != nil {
			lg.Warn("advance archive cursor", "board", board.Code, "err", err)
		}
		return
	}

	// Threads that are active in the DB but missing from the catalog.
	active, err := s.store.ListActiveThreads(ctx, board.ID)
	if err != nil {
		lg.Warn("list active threads", "err", err)
		return
	}
	for _, t := range active {
		if seen[t.NativeID] {
			continue
		}
		n, err := s.store.RecordThreadMissing(ctx, t.ID)
		if err != nil {
			lg.Warn("record thread missing", "thread", t.NativeID, "err", err)
			continue
		}
		if n >= s.cfg.Scheduler.MissingBeforeArchive {
			if err := s.store.MarkThreadArchived(ctx, t.ID); err != nil {
				lg.Warn("archive thread", "thread", t.NativeID, "err", err)
			} else {
				lg.Info("thread archived (missing from catalog)",
					"board", board.Code, "thread", t.NativeID, "misses", n)
			}
		}
	}
}

// runThread fetches a full thread, upserts posts/files/quotes, updates thread
// stats, archives on 404, and queues downloads for new files.
func (s *Scheduler) runThread(ctx context.Context, sc *config.SiteConfig, siteID int64, adapt adapter.Adapter, dl *downloader.Downloader, job *store.Job, lg *slog.Logger) {
	board, err := s.store.GetBoardByID(ctx, *job.BoardID)
	if err != nil {
		s.failJob(ctx, sc, siteID, job, lg, err)
		return
	}
	t, err := s.store.GetThreadByID(ctx, *job.ThreadID)
	if err != nil {
		s.failJob(ctx, sc, siteID, job, lg, err)
		return
	}

	th, err := adapt.FetchThread(ctx, board.Code, t.NativeID)
	if err != nil {
		if errors.Is(err, httpclient.ErrNotFound) {
			s.store.MarkThreadArchived(ctx, t.ID)
			s.store.RecordSiteSuccess(ctx, siteID)
			s.store.CompleteJob(ctx, job.ID)
			lg.Info("thread 404, archived", "board", board.Code, "thread", t.NativeID)
			return
		}
		s.failJob(ctx, sc, siteID, job, lg, err)
		return
	}

	if err := s.store.UpsertThreadPosts(ctx, t.ID, th.Posts); err != nil {
		s.failJob(ctx, sc, siteID, job, lg, err)
		return
	}

	replyCount, fileCount := countThreadStats(th)
	if err := s.store.MarkThreadSeen(ctx, t.ID, replyCount, fileCount, th.LastBump); err != nil {
		lg.Warn("mark thread seen", "err", err)
	}
	if th.Archived {
		s.store.MarkThreadArchived(ctx, t.ID)
	}
	s.store.RecordSiteSuccess(ctx, siteID)
	s.store.CompleteJob(ctx, job.ID)
	lg.Info("thread polled", "board", board.Code, "thread", t.NativeID,
		"posts", len(th.Posts), "files", fileCount)

	// Queue a download job if this board stores anything locally.
	full, thumb := s.downloadPolicy(sc, boardCfg(sc, board.Code))
	if full || thumb {
		payload := map[string]any{"download_full": full, "download_thumb": thumb}
		if _, err := s.store.EnqueueJob(ctx, siteID, &board.ID, &t.ID, store.JobDownload, payload, time.Now()); err != nil {
			lg.Warn("enqueue download job", "thread", t.NativeID, "err", err)
		}
	}
}

// runDownload downloads the pending files of one thread.
func (s *Scheduler) runDownload(ctx context.Context, dl *downloader.Downloader, job *store.Job, lg *slog.Logger) {
	if s.cfg.Scheduler.TextOnly {
		s.store.CompleteJob(ctx, job.ID)
		return
	}
	full, thumb := jobPayloadFlags(job)
	pending, err := s.store.ListPendingDownloads(ctx, *job.ThreadID)
	if err != nil {
		s.failJobNoSite(ctx, job, lg, err)
		return
	}

	var firstErr error
	for _, pd := range pending {
		if full {
			if err := dl.DownloadFull(ctx, pd); err != nil {
				if errors.Is(err, downloader.ErrBlocked) {
					lg.Warn("download blocked by CSAM check; content discarded",
						"url", pd.FullURL)
					continue
				}
				if firstErr == nil {
					firstErr = err
				}
				lg.Warn("full download failed", "url", pd.FullURL, "err", err)
				continue
			}
			// Enqueue multimodal jobs for this file
			if err := s.enqueueMultimodalJobs(ctx, pd.FileID, pd.PostID, pd.FullURL, lg); err != nil {
				lg.Warn("enqueue multimodal jobs failed", "file", pd.FileID, "err", err)
			}
		}
		if thumb {
			if err := dl.DownloadThumb(ctx, pd); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				lg.Warn("thumb download failed", "url", pd.ThumbURL, "err", err)
			}
		}
	}

	if firstErr != nil {
		s.failJobNoSite(ctx, job, lg, firstErr)
		return
	}
	s.store.CompleteJob(ctx, job.ID)
}

// runBackfill downloads a bounded batch of previously-skipped files for a board.
func (s *Scheduler) runBackfill(ctx context.Context, sc *config.SiteConfig, dl *downloader.Downloader, job *store.Job, lg *slog.Logger) {
	if s.cfg.Scheduler.TextOnly {
		s.store.CompleteJob(ctx, job.ID)
		return
	}
	full, thumb := jobPayloadFlags(job)
	limit := s.cfg.Scheduler.DownloadBackfillBatch
	if v, ok := jobPayloadInt(job, "limit"); ok && v > 0 {
		limit = v
	}

	pending, err := s.store.ListBoardPendingDownloads(ctx, *job.BoardID, limit)
	if err != nil {
		s.failJobNoSite(ctx, job, lg, err)
		return
	}

	var firstErr error
	for _, pd := range pending {
		if full {
			if err := dl.DownloadFull(ctx, pd); err != nil {
				if errors.Is(err, downloader.ErrBlocked) {
					continue
				}
				if firstErr == nil {
					firstErr = err
				}
				lg.Warn("backfill full failed", "url", pd.FullURL, "err", err)
				continue
			}
		}
		if thumb {
			if err := dl.DownloadThumb(ctx, pd); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				lg.Warn("backfill thumb failed", "url", pd.ThumbURL, "err", err)
			}
		}
	}
	lg.Info("backfill pass done", "board", *job.BoardID, "files", len(pending))

	if firstErr != nil {
		s.failJobNoSite(ctx, job, lg, firstErr)
		return
	}
	s.store.CompleteJob(ctx, job.ID)
}

// --- health / failure handling ---

// failJob records a site failure (drives the circuit breaker), reschedules the
// job with exponential backoff, and marks the site failed.
func (s *Scheduler) failJob(ctx context.Context, sc *config.SiteConfig, siteID int64, job *store.Job, lg *slog.Logger, err error) {
	s.siteFailed(ctx, siteID, sc, lg, err)
	s.failJobNoSite(ctx, job, lg, err)
}

func (s *Scheduler) failJobNoSite(ctx context.Context, job *store.Job, lg *slog.Logger, err error) {
	lg.Warn("job failed", "err", err)
	delay := backoffDelay(job.Attempts, time.Minute, 30*time.Minute)
	if err := s.store.FailJob(ctx, job.ID, err.Error(), delay); err != nil {
		lg.Warn("fail job", "err", err)
	}
}

func (s *Scheduler) siteFailed(ctx context.Context, siteID int64, sc *config.SiteConfig, lg *slog.Logger, err error) {
	n, err2 := s.store.RecordSiteFailure(ctx, siteID, err.Error())
	if err2 != nil {
		lg.Warn("record site failure", "err", err2)
		return
	}
	if n >= s.cfg.Scheduler.CircuitBreakerThreshold {
		cooldown := circuitCooldown(sc.PollInterval.D(), n)
		if err := s.store.SetCircuitOpen(ctx, siteID, time.Now().Add(cooldown)); err != nil {
			lg.Warn("open circuit", "err", err)
		} else {
			lg.Error("circuit breaker opened", "site", sc.Name, "failures", n, "cooldown", cooldown)
		}
	}
}

func (s *Scheduler) circuitOpen(ctx context.Context, siteID int64, sc *config.SiteConfig, lg *slog.Logger) bool {
	open, until, err := s.store.CircuitStatus(ctx, siteID)
	if err != nil || !open {
		return false
	}
	wait := 15 * time.Second
	if until != nil && until.After(time.Now()) {
		wait = time.Until(*until)
	} else {
		return false
	}
	lg.Warn("circuit open; site paused", "site", sc.Name, "for", wait.Round(time.Second))
	return !sleep(ctx, wait)
}

// --- helpers ---

func countThreadStats(th *adapter.Thread) (replyCount, fileCount int) {
	for _, p := range th.Posts {
		fileCount += len(p.Files)
	}
	if len(th.Posts) > 0 {
		replyCount = len(th.Posts) - 1
	}
	return replyCount, fileCount
}

func boardCfg(sc *config.SiteConfig, code string) *config.BoardConfig {
	for i := range sc.Boards {
		if sc.Boards[i].Code == code {
			return &sc.Boards[i]
		}
	}
	return &config.BoardConfig{}
}

func jobPayloadFlags(job *store.Job) (full, thumb bool) {
	if job.Payload == nil {
		return false, false
	}
	var p struct {
		DownloadFull  bool `json:"download_full"`
		DownloadThumb bool `json:"download_thumb"`
	}
	_ = json.Unmarshal(job.Payload, &p)
	return p.DownloadFull, p.DownloadThumb
}

func jobPayloadInt(job *store.Job, key string) (int, bool) {
	if job.Payload == nil {
		return 0, false
	}
	var p map[string]any
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return 0, false
	}
	v, ok := p[key].(float64)
	if !ok {
		return 0, false
	}
	return int(v), true
}

// catalogJobPages reads the crawl cursor from a catalog job payload. page 0
// means "whole live catalog" (no payload set).
func catalogJobPages(job *store.Job) (page, pages int) {
	if job.Payload == nil {
		return 0, 0
	}
	var p struct {
		Page  int `json:"page"`
		Pages int `json:"pages"`
	}
	_ = json.Unmarshal(job.Payload, &p)
	return p.Page, p.Pages
}

// enqueueMultimodalJobs enqueues OCR, CLIP, and Whisper jobs for a downloaded file
// based on its MIME type and the multimodal config.
func (s *Scheduler) enqueueMultimodalJobs(ctx context.Context, fileID, postID int64, fullURL string, lg *slog.Logger) error {
	// Get the file's mime type
	var mimeType string
	err := s.store.QueryRow(ctx, `
		SELECT mime_type FROM blobs b
		JOIN files f ON f.file_hash = b.file_hash
		WHERE f.id = $1`, fileID).Scan(&mimeType)
	if err != nil {
		return err
	}

	now := time.Now()
	payload := map[string]any{
		"file_id":   fileID,
		"post_id":   postID,
		"mime_type": mimeType,
	}
	payloadJSON, _ := json.Marshal(payload)

	// OCR for images
	if s.cfg.Multimodal.OCR.Enabled && isImageMime(mimeType) {
		if _, err := s.store.EnqueueJob(ctx, 0, nil, nil, store.JobOCR, payloadJSON, now); err != nil {
			lg.Warn("enqueue ocr job failed", "file", fileID, "err", err)
		}
	}

	// CLIP for images
	if s.cfg.Multimodal.CLIP.Enabled && isImageMime(mimeType) {
		if _, err := s.store.EnqueueJob(ctx, 0, nil, nil, store.JobCLIP, payloadJSON, now); err != nil {
			lg.Warn("enqueue clip job failed", "file", fileID, "err", err)
		}
	}

	// Whisper for audio/video
	if s.cfg.Multimodal.Whisper.Enabled && (isAudioMime(mimeType) || isVideoMime(mimeType)) {
		if _, err := s.store.EnqueueJob(ctx, 0, nil, nil, store.JobWhisper, payloadJSON, now); err != nil {
			lg.Warn("enqueue whisper job failed", "file", fileID, "err", err)
		}
	}

	return nil
}

func isImageMime(mime string) bool {
	return len(mime) > 6 && mime[:6] == "image/"
}

func isAudioMime(mime string) bool {
	return len(mime) > 6 && mime[:6] == "audio/"
}

func isVideoMime(mime string) bool {
	return len(mime) > 6 && mime[:6] == "video/"
}
