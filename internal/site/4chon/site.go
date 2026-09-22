// Package fourchon provides the 4chon.me site module.
// Platform: vichan (HTML-only, no JSON API)
package fourchon

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
			WithName("4chon").
			WithBaseURL("https://4chon.me").
			WithPlatform(adapter.PlatformVichan).
			WithAPIAvailable(false).
			WithPollInterval(60 * time.Second).
			WithRateLimitPerSec(1).
			WithMaxConcurrentReqs(1).
			WithBoards([]site.Board{
				{Code: "new", Title: "News", NSFW: false},
				{Code: "lounge", Title: "Lounge", NSFW: false},
			}),
	)
}

func NewAdapter(ctx context.Context, baseURL, userAgent string, transport http.RoundTripper, logger *slog.Logger) (adapter.Adapter, error) {
	return vichan.New(baseURL, userAgent, transport, logger)
}
