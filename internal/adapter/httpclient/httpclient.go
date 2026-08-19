// Package httpclient is a small HTTP helper shared by every platform adapter.
// It resolves paths against a site's base URL and carries the per-site round
// tripper (rate limit + concurrency cap) installed by the scheduler.
package httpclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// defaultUserAgent is used when a site config does not specify one. Imageboards
// commonly 403 bare Go HTTP clients.
const defaultUserAgent = "akyuu/0.1 (private archive; contact the operator)"

// Client wraps an http.Client with a base URL for path resolution.
type Client struct {
	base      string
	userAgent string
	hc        *http.Client
}

// New builds an adapter HTTP client rooted at base. transport may be nil, in
// which case http.DefaultTransport is used (no rate limiting applied).
func New(base, userAgent string, transport http.RoundTripper) *Client {
	if transport == nil {
		transport = http.DefaultTransport
	}
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	return &Client{
		base:      strings.TrimRight(base, "/"),
		userAgent: userAgent,
		hc:        &http.Client{Transport: transport},
	}
}

// Resolve joins a (possibly relative) path with the base URL.
func (c *Client) Resolve(path string) string {
	if path == "" {
		return c.base
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return c.base + "/" + strings.TrimLeft(path, "/")
}

// Get fetches the given path and returns the response body plus the final URL
// (after redirects). The caller must close the body.
func (c *Client) Get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Resolve(path), nil)
	if err != nil {
		return nil, fmt.Errorf("adapter: build request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json, text/html;q=0.9, */*;q=0.5")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("adapter: GET %s: %w", path, err)
	}
	if resp.StatusCode == http.StatusNotFound {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, ErrNotFound
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("adapter: GET %s: %s: %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	return resp, nil
}

// GetBytes fetches a path and returns the body bytes (used for JSON feeds).
func (c *Client) GetBytes(ctx context.Context, path string) ([]byte, error) {
	resp, err := c.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// ErrNotFound marks a request that returned HTTP 404. The scheduler treats it
// specially: a thread that 404s is archived instead of retried with backoff.
var ErrNotFound = fmt.Errorf("adapter: not found (404)")
