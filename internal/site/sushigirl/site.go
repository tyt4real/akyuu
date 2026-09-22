// Package sushigirl provides the sushigirl.cafe site module.
// Platform: vichan (HTML-only, no JSON API)
package sushigirl

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
			WithName("sushigirl").
			WithBaseURL("https://sushigirl.cafe").
			WithPlatform(adapter.PlatformVichan).
			WithAPIAvailable(false).
			WithPollInterval(60 * time.Second).
			WithRateLimitPerSec(1).
			WithMaxConcurrentReqs(1).
			WithBoards([]site.Board{
				{Code: "lounge", Title: "Lounge", NSFW: false},
				{Code: "kawaii", Title: "Kawaii", NSFW: false},
				{Code: "kitchen", Title: "Kitchen", NSFW: false},
				{Code: "tunes", Title: "Tunes", NSFW: false},
				{Code: "culture", Title: "Culture", NSFW: false},
				{Code: "silicon", Title: "Silicon", NSFW: false},
				{Code: "otaku", Title: "Otaku", NSFW: false},
				{Code: "hell", Title: "Hell", NSFW: true},
				{Code: "yakuza", Title: "Yakuza", NSFW: false},
				{Code: "arcade", Title: "Arcade", NSFW: false},
				{Code: "chat", Title: "Chat", NSFW: false},
				{Code: "kaitensushi", Title: "Fresh Posts", NSFW: false},
			}),
	)
}

func NewAdapter(ctx context.Context, baseURL, userAgent string, transport http.RoundTripper, logger *slog.Logger) (adapter.Adapter, error) {
	return vichan.New(baseURL, userAgent, transport, logger)
}
