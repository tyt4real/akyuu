// Package lainchan provides the lainchan.org site module.
// Platform: lynxchan (native JSON API)
package lainchan

import (
	"context"
	"time"

	"akyuu/internal/adapter"
	"akyuu/internal/adapter/lynxchan"
	"akyuu/internal/site"
	"akyuu/internal/siteregistry"
	"log/slog"
	"net/http"
)

func init() {
	siteregistry.Register(
		site.NewSiteConfig().
			WithName("lainchan").
			WithBaseURL("https://lainchan.org").
			WithPlatform(adapter.PlatformLynxChan).
			WithAPIAvailable(true).
			WithPollInterval(60 * time.Second).
			WithRateLimitPerSec(2).
			WithMaxConcurrentReqs(1).
			WithBoards([]site.Board{
				{Code: "r", Title: "Random", NSFW: false, DownloadFull: boolPtr(true), DownloadThumb: boolPtr(true)},
				{Code: "tech", Title: "Technology", NSFW: false},
				{Code: "q", Title: "Questions", NSFW: false},
			}),
	)
}

func NewAdapter(ctx context.Context, baseURL, userAgent string, transport http.RoundTripper, logger *slog.Logger) (adapter.Adapter, error) {
	return lynxchan.New(baseURL, userAgent, transport, logger)
}

func boolPtr(b bool) *bool { return &b }
