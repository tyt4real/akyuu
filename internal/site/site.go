// Package site defines the Site interface that every supported imageboard
// site implements. This follows the Open/Closed Principle: new sites are
// added by creating new modules that implement Site, without modifying
// existing code.
package site

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"akyuu/internal/adapter"
)

// Board defines a single board on a site.
type Board struct {
	Code          string
	Title         string
	NSFW          bool
	PollInterval  time.Duration
	DownloadFull  *bool
	DownloadThumb *bool
}

// Site is the interface every site module implements.
// It encapsulates all site-specific configuration and behavior.
type Site interface {
	// Name returns the unique identifier for this site (e.g., "4chan", "lainchan").
	GetName() string

	// BaseURL returns the root URL of the site (scheme + host, no trailing slash).
	GetBaseURL() string

	// Platform returns the platform/engine family (lynxchan, vichan, fourchan, desuarchive).
	GetPlatform() adapter.Platform

	// APIAvailable indicates whether the site exposes a JSON API endpoint.
	GetAPIAvailable() bool

	// Boards returns the list of boards to archive on this site.
	GetBoards() []Board

	// PollInterval returns the default poll interval for boards on this site.
	GetPollInterval() time.Duration

	// RateLimitPerSec returns the maximum requests per second for this site.
	GetRateLimitPerSec() float64

	// MaxConcurrentRequests returns the maximum concurrent requests for this site.
	GetMaxConcurrentRequests() int

	// UserAgent returns a custom User-Agent string for this site, or "" for default.
	GetUserAgent() string

	// Proxy returns a proxy URL (e.g., "socks5://127.0.0.1:9050") for this site, or "" for none.
	GetProxy() string

	// AutoBoards returns true if boards should be auto-discovered at startup.
	GetAutoBoards() bool

	// PaginatedCatalog returns true if the site uses paginated catalog (archive mode).
	GetPaginatedCatalog() bool

	// CrawlPagesPerPoll returns how many catalog pages to crawl per poll cycle (archive sites).
	GetCrawlPagesPerPoll() int

	// DownloadFull returns the site-level default for full image downloads.
	GetDownloadFull() *bool

	// DownloadThumb returns the site-level default for thumbnail downloads.
	GetDownloadThumb() *bool

	// NewAdapter creates the platform adapter for this site.
	// The site can inject custom behavior (e.g., custom selectors) here.
	NewAdapter(ctx context.Context, baseURL, userAgent string, transport http.RoundTripper, logger *slog.Logger) (adapter.Adapter, error)
}
