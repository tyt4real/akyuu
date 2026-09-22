// Package desuarchive provides the desuarchive.org site module.
// Platform: desuarchive (FoolFuuka JSON API, archive mode)
package desuarchive

import (
	"context"
	"time"

	"akyuu/internal/adapter"
	"akyuu/internal/adapter/desuarchive"
	"akyuu/internal/site"
	"akyuu/internal/siteregistry"
	"log/slog"
	"net/http"
)

func init() {
	siteregistry.Register(
		site.NewSiteConfig().
			WithName("desuarchive").
			WithBaseURL("https://desuarchive.org").
			WithPlatform(adapter.PlatformDesuarchive).
			WithAPIAvailable(true).
			WithPollInterval(60 * time.Second).
			WithRateLimitPerSec(1).
			WithMaxConcurrentReqs(1).
			WithAutoBoards(true).
			WithPaginatedCatalog(true).
			WithCrawlPagesPerPoll(1).
			WithBoards([]site.Board{}),
	)
}

func NewAdapter(ctx context.Context, baseURL, userAgent string, transport http.RoundTripper, logger *slog.Logger) (adapter.Adapter, error) {
	return desuarchive.New(baseURL, userAgent, transport, logger)
}
