// Package adapter defines the platform interface and the shared data model
// that every imageboard platform implementation must fill in.
//
// Each platform gets its own package under internal/adapter/<platform>/.
// A config file selects a platform by name; adding a new site on an already
// supported platform only requires a new YAML file, never new code.
package adapter

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"akyuu/internal/adapter/desuarchive"
	"akyuu/internal/adapter/fourchan"
	"akyuu/internal/adapter/lynxchan"
	"akyuu/internal/adapter/model"
	"akyuu/internal/adapter/vichan"
)

// Public data types are re-exported from the model leaf package so callers can
// use adapter.Post etc. while the platform packages stay cycle-free.
type (
	ThreadSummary = model.ThreadSummary
	Thread        = model.Thread
	Post          = model.Post
	QuoteRef      = model.QuoteRef
	File          = model.File
	BoardInfo     = model.BoardInfo
)

// Platform is the canonical name of an engine family. It must match the
// "platform" key in a site config file.
type Platform string

const (
	PlatformLynxChan    Platform = "lynxchan"
	PlatformVichan      Platform = "vichan"
	PlatformFourChan    Platform = "fourchan"
	PlatformDesuarchive Platform = "desuarchive"
)

// Adapter is the interface every platform implementation satisfies.
//
// FetchCatalog returns the list of threads currently on a board.
// FetchThread returns the full thread (OP + every reply) for a board.
// ParsePost parses a single raw post (JSON or an HTML fragment) into a Post.
// CatalogURL returns the URL for a board's catalog page (used for raw capture).
type Adapter interface {
	PlatformName() string
	FetchCatalog(ctx context.Context, board string) ([]ThreadSummary, error)
	FetchThread(ctx context.Context, board string, threadID string) (*Thread, error)
	ParsePost(raw []byte) (*Post, error)
	CatalogURL(board string) string
}

// BoardLister is implemented by adapters that can enumerate their boards at
// runtime (FoolFuuka-style archives). The scheduler calls it at startup when
// the site config sets auto_boards: true.
type BoardLister interface {
	ListBoards(ctx context.Context) ([]BoardInfo, error)
}

// PaginatedCataloger is implemented by adapters whose board catalog is a
// paginated crawl of thread history (archive sites). The scheduler stores a
// per-board page cursor and passes it in the catalog job payload; runCatalog
// advances the cursor once the page has been processed.
type PaginatedCataloger interface {
	FetchCatalogPage(ctx context.Context, board string, page int) ([]ThreadSummary, error)
}

// New builds an adapter for the given platform. baseURL must include scheme
// and host (no trailing slash). transport is the per-site round tripper the
// scheduler builds so that rate limits and concurrency caps are respected by
// every request the adapter makes.
func New(platform Platform, baseURL, userAgent string, transport http.RoundTripper, logger *slog.Logger) (Adapter, error) {
	switch platform {
	case PlatformLynxChan:
		return lynxchan.New(baseURL, userAgent, transport, logger)
	case PlatformVichan:
		return vichan.New(baseURL, userAgent, transport, logger)
	case PlatformFourChan:
		return fourchan.New(baseURL, userAgent, transport, logger)
	case PlatformDesuarchive:
		return desuarchive.New(baseURL, userAgent, transport, logger)
	default:
		return nil, fmt.Errorf("adapter: unknown platform %q", platform)
	}
}
