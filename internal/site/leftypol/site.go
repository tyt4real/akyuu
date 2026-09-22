// Package leftypol provides the leftypol.org site module.
// Platform: lynxchan (native JSON API)
package leftypol

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
			WithName("leftypol").
			WithBaseURL("https://leftypol.org").
			WithPlatform(adapter.PlatformLynxChan).
			WithAPIAvailable(true).
			WithPollInterval(90 * time.Second).
			WithRateLimitPerSec(1).
			WithMaxConcurrentReqs(1).
			WithBoards([]site.Board{
				{Code: "leftypol", Title: "Politically Incorrect", NSFW: true},
				{Code: "tech", Title: "Technology", NSFW: false, DownloadFull: boolPtr(true)},
				{Code: "meta", Title: "Meta", NSFW: false},
			}),
	)
}

func NewAdapter(ctx context.Context, baseURL, userAgent string, transport http.RoundTripper, logger *slog.Logger) (adapter.Adapter, error) {
	return lynxchan.New(baseURL, userAgent, transport, logger)
}

func boolPtr(b bool) *bool { return &b }
