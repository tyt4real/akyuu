// Package downloader fetches full-res files and thumbnails, hashes them,
// deduplicates globally by content hash, runs the mandatory CSAM safety check,
// and writes them into the content-addressable storage tree.
package downloader

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"akyuu/internal/store"
)

// ErrBlocked is returned when the safety checker flags a file's hash. Blocked
// content is discarded and never persisted; the scheduler treats it as done
// (no retry) and the event is logged.
var ErrBlocked = fmt.Errorf("downloader: content blocked by safety check")

// Downloader fetches and stores files for a single site. It carries the site's
// rate-limited transport so downloads respect the same limits as scraping.
type Downloader struct {
	store   *store.Store
	client  *http.Client
	storage *Storage
	safety  SafetyChecker
	logger  *slog.Logger
}

// New builds a Downloader writing under root with the given transport. The
// root directory (and the temp-file workspace inside it) is created on demand.
func New(st *store.Store, transport http.RoundTripper, root string, checker SafetyChecker, logger *slog.Logger) *Downloader {
	if transport == nil {
		transport = http.DefaultTransport
	}
	if err := os.MkdirAll(root, 0o755); err != nil && logger != nil {
		logger.Warn("downloader: create storage root", "root", root, "err", err)
	}
	return &Downloader{
		store:   st,
		client:  &http.Client{Transport: transport, Timeout: httpTimeout},
		storage: NewStorage(root),
		safety:  checker,
		logger:  logger,
	}
}

// DownloadFull fetches the full-res file for a pending download, hashes it,
// safety-checks it, dedups against blobs, and stores it. Returns ErrBlocked on
// a safety match, nil on success (or when there is nothing to fetch).
func (d *Downloader) DownloadFull(ctx context.Context, pd *store.PendingDownload) error {
	if pd.FullURL == "" {
		return nil
	}

	tmp, err := os.CreateTemp(d.storage.root, ".dl-*")
	if err != nil {
		return fmt.Errorf("downloader: temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	h := newHashes()
	written, err := d.fetchTo(ctx, pd.FullURL, io.MultiWriter(tmp, h.multi()))
	if err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("downloader: close temp: %w", err)
	}
	if written == 0 {
		return fmt.Errorf("downloader: empty body from %s", pd.FullURL)
	}

	sha := h.SHA256Hex()
	if md5 := h.MD5Hex(); pd.PlatformMD5 != "" && pd.PlatformMD5 != md5 {
		d.logger.Warn("downloader: content md5 differs from platform-reported md5",
			"url", pd.FullURL, "platform", pd.PlatformMD5, "computed", md5)
	}

	// Mandatory safety gate: runs for every full download, never skippable.
	blocked, err := d.safety.Check(ctx, sha)
	if err != nil {
		return fmt.Errorf("downloader: safety check: %w", err)
	}
	if blocked {
		d.logger.Warn("downloader: content blocked by CSAM hash match; discarding",
			"url", pd.FullURL, "sha256", sha)
		return ErrBlocked
	}

	mimeType := sniffMime(tmpName, pd.FullURL)
	absPath := d.storage.FullPath(sha, mimeType)
	if err := d.storage.EnsureDirs(absPath); err != nil {
		return fmt.Errorf("downloader: mkdir: %w", err)
	}
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		if err := os.Rename(tmpName, absPath); err != nil {
			// Cross-device or partial failure: fall back to a copy.
			if err2 := copyFile(tmpName, absPath); err2 != nil {
				return fmt.Errorf("downloader: store %s: %w", sha, err2)
			}
		}
	}

	return d.store.RecordDownloadedFile(ctx, pd.FileID, &store.Blob{
		FileHash:        sha,
		MimeType:        mimeType,
		SizeBytes:       written,
		StoragePath:     d.storage.Rel(absPath),
		PlatformMD5:     h.MD5Hex(),
		PlatformSHA1:    pd.PlatformSHA1,
		FirstSeenPostID: pd.PostID,
	}, true, false)
}

// DownloadThumb fetches (or locally generates) a thumbnail for a pending
// download and marks it stored. Thumb failures are non-fatal: the remote URL
// is kept in the files row and the thumbnail can be backfilled later.
func (d *Downloader) DownloadThumb(ctx context.Context, pd *store.PendingDownload) error {
	// Prefer a locally generated thumbnail from already-stored full content.
	if pd.ThumbURL == "" && pd.FileHash != "" {
		return d.generateThumbFromFull(ctx, pd)
	}
	if pd.ThumbURL == "" {
		return nil
	}

	tmp, err := os.CreateTemp(d.storage.root, ".dl-*")
	if err != nil {
		return fmt.Errorf("downloader: temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	h := newHashes()
	written, err := d.fetchTo(ctx, pd.ThumbURL, io.MultiWriter(tmp, h.multi()))
	if err != nil {
		if err == errNotFound {
			d.logger.Debug("downloader: remote thumb 404; trying local generation",
				"url", pd.ThumbURL)
			return d.generateThumbFromFull(ctx, pd)
		}
		d.logger.Warn("downloader: thumb fetch failed; remote URL kept",
			"url", pd.ThumbURL, "err", err)
		return nil
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("downloader: close temp: %w", err)
	}
	if written == 0 {
		return nil
	}

	thumbHash := h.SHA256Hex()
	absPath := d.storage.ThumbPath(thumbHash)
	if err := d.storage.EnsureDirs(absPath); err != nil {
		return fmt.Errorf("downloader: mkdir: %w", err)
	}
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		if err := os.Rename(tmpName, absPath); err != nil {
			if err2 := copyFile(tmpName, absPath); err2 != nil {
				return fmt.Errorf("downloader: store thumb %s: %w", thumbHash, err2)
			}
		}
	}
	return d.store.RecordThumbDownloaded(ctx, pd.FileID, thumbHash, d.storage.Rel(absPath))
}

// generateThumbFromFull renders a thumbnail from the locally stored full file,
// used when a platform serves no thumbnail or its thumb 404s.
func (d *Downloader) generateThumbFromFull(ctx context.Context, pd *store.PendingDownload) error {
	blob, err := d.store.GetBlobByHash(ctx, pd.FileHash)
	if err != nil || blob.StoragePath == "" {
		return nil
	}
	full, err := os.Open(joinRel(d.storage.root, blob.StoragePath))
	if err != nil {
		return nil
	}
	defer full.Close()

	tmp, err := os.CreateTemp(d.storage.root, ".thumb-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := GenerateThumbnail(full, blob.MimeType, tmp); err != nil {
		tmp.Close()
		d.logger.Debug("downloader: local thumb generation failed",
			"hash", pd.FileHash, "err", err)
		return nil
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// The generated thumbnail is keyed by its own content hash.
	raw, err := os.ReadFile(tmpName)
	if err != nil {
		return err
	}
	thumbHash := hashBytes(raw)
	absPath := d.storage.ThumbPath(thumbHash)
	if err := d.storage.EnsureDirs(absPath); err != nil {
		return err
	}
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		if err := copyFile(tmpName, absPath); err != nil {
			return err
		}
	}
	return d.store.RecordThumbDownloaded(ctx, pd.FileID, thumbHash, d.storage.Rel(absPath))
}

// fetchTo GETs url and streams the body to w, returning bytes written. A 404
// is reported as errNotFound so callers can fall back gracefully.
func (d *Downloader) fetchTo(ctx context.Context, url string, w io.Writer) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("downloader: build request: %w", err)
	}
	req.Header.Set("User-Agent", "akyuu/0.1 (private archive)")
	resp, err := d.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("downloader: GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return 0, errNotFound
	}
	if resp.StatusCode >= 400 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return 0, fmt.Errorf("downloader: GET %s: %s", url, resp.Status)
	}
	n, err := io.Copy(w, resp.Body)
	if err != nil {
		return n, fmt.Errorf("downloader: read %s: %w", url, err)
	}
	return n, nil
}

var errNotFound = fmt.Errorf("downloader: not found (404)")

func sniffMime(tmpPath, url string) string {
	f, err := os.Open(tmpPath)
	if err == nil {
		defer f.Close()
		var head [512]byte
		if n, _ := io.ReadFull(f, head[:]); n > 0 {
			mt := http.DetectContentType(head[:n])
			if !strings.HasPrefix(mt, "application/octet-stream") {
				return mt
			}
		}
	}
	if m := MimeFromName(url); m != "" {
		return m
	}
	return "application/octet-stream"
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func joinRel(root, rel string) string {
	if rel == "" || strings.HasPrefix(rel, "/") {
		return rel
	}
	return root + "/" + rel
}

func hashBytes(b []byte) string {
	h := newHashes()
	h.multi().Write(b)
	return h.SHA256Hex()
}
