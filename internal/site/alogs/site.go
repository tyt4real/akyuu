// Package alogs provides the alogs.space site module.
// Platform: lynxchan (native JSON API)
package alogs

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
			WithName("alogs").
			WithBaseURL("https://alogs.space").
			WithPlatform(adapter.PlatformLynxChan).
			WithAPIAvailable(true).
			WithPollInterval(60 * time.Second).
			WithRateLimitPerSec(1).
			WithMaxConcurrentReqs(1).
			WithBoards([]site.Board{
				{Code: "cow", Title: "Lolcows", NSFW: false},
				{Code: "robowaifu", Title: "DIY Robot Wives", NSFW: false, DownloadFull: boolPtr(true)},
			}),
	)
}

func NewAdapter(ctx context.Context, baseURL, userAgent string, transport http.RoundTripper, logger *slog.Logger) (adapter.Adapter, error) {
	return lynxchan.New(baseURL, userAgent, transport, logger)
}

func boolPtr(b bool) *bool { return &b }
