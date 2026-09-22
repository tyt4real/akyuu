// Package siteadapters converts site.Site modules to config.SiteConfig for use by the scheduler.
package siteadapters

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"akyuu/internal/adapter"
	"akyuu/internal/config"
	"akyuu/internal/site"
)

// ToConfig converts a site.Site to a config.SiteConfig.
// This allows the scheduler to continue using config.SiteConfig internally
// while site definitions live in Go modules.
func ToConfig(s site.Site) *config.SiteConfig {
	boards := make([]config.BoardConfig, len(s.GetBoards()))
	for i, b := range s.GetBoards() {
		boards[i] = config.BoardConfig{
			Code:          b.Code,
			Title:         b.Title,
			NSFW:          b.NSFW,
			PollInterval:  config.Duration(b.PollInterval),
			DownloadFull:  b.DownloadFull,
			DownloadThumb: b.DownloadThumb,
		}
	}

	var dlFull, dlThumb *bool
	if s.GetDownloadFull() != nil {
		v := *s.GetDownloadFull()
		dlFull = &v
	}
	if s.GetDownloadThumb() != nil {
		v := *s.GetDownloadThumb()
		dlThumb = &v
	}

	return &config.SiteConfig{
		Name:              s.GetName(),
		BaseURL:           s.GetBaseURL(),
		Platform:          string(s.GetPlatform()),
		APIAvailable:      s.GetAPIAvailable(),
		PollInterval:      config.Duration(s.GetPollInterval()),
		RateLimitPerSec:   s.GetRateLimitPerSec(),
		MaxConcurrent:     s.GetMaxConcurrentRequests(),
		UserAgent:         s.GetUserAgent(),
		Proxy:             s.GetProxy(),
		Boards:            boards,
		AutoBoards:        s.GetAutoBoards(),
		PaginatedCatalog:  s.GetPaginatedCatalog(),
		CrawlPagesPerPoll: s.GetCrawlPagesPerPoll(),
		DownloadFull:      dlFull,
		DownloadThumb:     dlThumb,
	}
}

// ToConfigs converts a slice of site.Site to config.SiteConfig.
func ToConfigs(sites []site.Site) []*config.SiteConfig {
	out := make([]*config.SiteConfig, len(sites))
	for i, s := range sites {
		out[i] = ToConfig(s)
	}
	return out
}

// AddSiteMethods adds Site interface methods to config.SiteConfig for compatibility.
// This is a temporary bridge until the scheduler is fully migrated.
func AddSiteMethods(sc *config.SiteConfig) site.Site {
	return &configSiteAdapter{sc}
}

type configSiteAdapter struct {
	*config.SiteConfig
}

func (a *configSiteAdapter) GetName() string    { return a.SiteConfig.Name }
func (a *configSiteAdapter) GetBaseURL() string { return a.SiteConfig.BaseURL }
func (a *configSiteAdapter) GetPlatform() adapter.Platform {
	return adapter.Platform(a.SiteConfig.Platform)
}
func (a *configSiteAdapter) GetAPIAvailable() bool { return a.SiteConfig.APIAvailable }
func (a *configSiteAdapter) GetBoards() []site.Board {
	boards := make([]site.Board, len(a.SiteConfig.Boards))
	for i, b := range a.SiteConfig.Boards {
		boards[i] = site.Board{
			Code:          b.Code,
			Title:         b.Title,
			NSFW:          b.NSFW,
			PollInterval:  b.PollInterval.D(),
			DownloadFull:  b.DownloadFull,
			DownloadThumb: b.DownloadThumb,
		}
	}
	return boards
}
func (a *configSiteAdapter) GetPollInterval() time.Duration { return a.SiteConfig.PollInterval.D() }
func (a *configSiteAdapter) GetRateLimitPerSec() float64    { return a.SiteConfig.RateLimitPerSec }
func (a *configSiteAdapter) GetMaxConcurrentRequests() int  { return a.SiteConfig.MaxConcurrent }
func (a *configSiteAdapter) GetUserAgent() string           { return a.SiteConfig.UserAgent }
func (a *configSiteAdapter) GetProxy() string               { return a.SiteConfig.Proxy }
func (a *configSiteAdapter) GetAutoBoards() bool            { return a.SiteConfig.AutoBoards }
func (a *configSiteAdapter) GetPaginatedCatalog() bool      { return a.SiteConfig.PaginatedCatalog }
func (a *configSiteAdapter) GetCrawlPagesPerPoll() int      { return a.SiteConfig.CrawlPagesPerPoll }
func (a *configSiteAdapter) GetDownloadFull() *bool         { return a.SiteConfig.DownloadFull }
func (a *configSiteAdapter) GetDownloadThumb() *bool        { return a.SiteConfig.DownloadThumb }
func (a *configSiteAdapter) NewAdapter(ctx context.Context, baseURL, userAgent string, transport http.RoundTripper, logger *slog.Logger) (adapter.Adapter, error) {
	return nil, nil // Not implemented for config-based sites
}
