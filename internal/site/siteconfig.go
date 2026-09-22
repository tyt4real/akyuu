// Package siteconfig provides a default implementation of the Site interface
// that site modules can embed to avoid boilerplate.
package site

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"akyuu/internal/adapter"
)

// SiteConfig holds the configuration for a site module.
// Use NewSiteConfig() to create and configure it.
type SiteConfig struct {
	Name              string
	BaseURL           string
	Platform          adapter.Platform
	APIAvailable      bool
	Boards            []Board
	PollInterval      time.Duration
	RateLimitPerSec   float64
	MaxConcurrentReqs int
	UserAgent         string
	Proxy             string
	AutoBoards        bool
	PaginatedCatalog  bool
	CrawlPagesPerPoll int
	DownloadFull      *bool
	DownloadThumb     *bool
}

// NewSiteConfig creates a new SiteConfig with defaults.
func NewSiteConfig() *SiteConfig {
	return &SiteConfig{
		PollInterval:      60 * time.Second,
		RateLimitPerSec:   1,
		MaxConcurrentReqs: 1,
		CrawlPagesPerPoll: 1,
	}
}

// WithName sets the site name.
func (c *SiteConfig) WithName(name string) *SiteConfig             { c.Name = name; return c }
func (c *SiteConfig) WithBaseURL(url string) *SiteConfig           { c.BaseURL = url; return c }
func (c *SiteConfig) WithPlatform(p adapter.Platform) *SiteConfig  { c.Platform = p; return c }
func (c *SiteConfig) WithAPIAvailable(v bool) *SiteConfig          { c.APIAvailable = v; return c }
func (c *SiteConfig) WithBoards(b []Board) *SiteConfig             { c.Boards = b; return c }
func (c *SiteConfig) WithPollInterval(d time.Duration) *SiteConfig { c.PollInterval = d; return c }
func (c *SiteConfig) WithRateLimitPerSec(v float64) *SiteConfig    { c.RateLimitPerSec = v; return c }
func (c *SiteConfig) WithMaxConcurrentReqs(v int) *SiteConfig      { c.MaxConcurrentReqs = v; return c }
func (c *SiteConfig) WithUserAgent(v string) *SiteConfig           { c.UserAgent = v; return c }
func (c *SiteConfig) WithProxy(v string) *SiteConfig               { c.Proxy = v; return c }
func (c *SiteConfig) WithAutoBoards(v bool) *SiteConfig            { c.AutoBoards = v; return c }
func (c *SiteConfig) WithPaginatedCatalog(v bool) *SiteConfig      { c.PaginatedCatalog = v; return c }
func (c *SiteConfig) WithCrawlPagesPerPoll(v int) *SiteConfig      { c.CrawlPagesPerPoll = v; return c }
func (c *SiteConfig) WithDownloadFull(v *bool) *SiteConfig         { c.DownloadFull = v; return c }
func (c *SiteConfig) WithDownloadThumb(v *bool) *SiteConfig        { c.DownloadThumb = v; return c }

// Implement Site interface methods on SiteConfig.

func (c *SiteConfig) GetName() string                { return c.Name }
func (c *SiteConfig) GetBaseURL() string             { return c.BaseURL }
func (c *SiteConfig) GetPlatform() adapter.Platform  { return c.Platform }
func (c *SiteConfig) GetAPIAvailable() bool          { return c.APIAvailable }
func (c *SiteConfig) GetBoards() []Board             { return c.Boards }
func (c *SiteConfig) GetPollInterval() time.Duration { return c.PollInterval }
func (c *SiteConfig) GetRateLimitPerSec() float64    { return c.RateLimitPerSec }
func (c *SiteConfig) GetMaxConcurrentRequests() int  { return c.MaxConcurrentReqs }
func (c *SiteConfig) GetUserAgent() string           { return c.UserAgent }
func (c *SiteConfig) GetProxy() string               { return c.Proxy }
func (c *SiteConfig) GetAutoBoards() bool            { return c.AutoBoards }
func (c *SiteConfig) GetPaginatedCatalog() bool      { return c.PaginatedCatalog }
func (c *SiteConfig) GetCrawlPagesPerPoll() int      { return c.CrawlPagesPerPoll }
func (c *SiteConfig) GetDownloadFull() *bool         { return c.DownloadFull }
func (c *SiteConfig) GetDownloadThumb() *bool        { return c.DownloadThumb }

func (c *SiteConfig) NewAdapter(ctx context.Context, baseURL, userAgent string, transport http.RoundTripper, logger *slog.Logger) (adapter.Adapter, error) {
	return nil, nil // Must be overridden by site module
}
