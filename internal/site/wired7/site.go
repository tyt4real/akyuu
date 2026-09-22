// Package wired7 provides the wired-7.org site module.
// Platform: vichan (HTML-only, no JSON API, Spanish UI)
package wired7

import (
	"context"
	"time"

	"akyuu/internal/adapter"
	"akyuu/internal/adapter/vichan"
	"akyuu/internal/site"
	"akyuu/internal/siteregistry"
	"log/slog"
	"net/http"
)

func init() {
	siteregistry.Register(
		site.NewSiteConfig().
			WithName("wired-7").
			WithBaseURL("https://wired-7.org").
			WithPlatform(adapter.PlatformVichan).
			WithAPIAvailable(false).
			WithPollInterval(60 * time.Second).
			WithRateLimitPerSec(1).
			WithMaxConcurrentReqs(1).
			WithBoards([]site.Board{
				{Code: "a", Title: "Anime y manga", NSFW: false, DownloadFull: boolPtr(true)},
				{Code: "v", Title: "Videojuegos", NSFW: false},
				{Code: "b", Title: "Random", NSFW: true},
			}),
	)
}

func NewAdapter(ctx context.Context, baseURL, userAgent string, transport http.RoundTripper, logger *slog.Logger) (adapter.Adapter, error) {
	return vichan.New(baseURL, userAgent, transport, logger)
}

func boolPtr(b bool) *bool { return &b }
