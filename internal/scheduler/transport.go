package scheduler

import (
	"math"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"

	"akyuu/internal/config"
)

// roundTripperFunc adapts a function to http.RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// newSiteTransport builds an http.RoundTripper for one site that enforces the
// site's rate limit (requests/sec) and max concurrent requests on every HTTP
// request the adapter or downloader makes. If the site configures a proxy
// (e.g. "socks5://127.0.0.1:9050" for Tor), all requests for the site are
// routed through it; otherwise the environment's HTTP(S)_PROXY is used.
func newSiteTransport(sc *config.SiteConfig) http.RoundTripper {
	base := &http.Transport{
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	if sc.Proxy != "" {
		if u, err := url.Parse(sc.Proxy); err == nil {
			base.Proxy = http.ProxyURL(u)
		} else {
			base.Proxy = http.ProxyFromEnvironment
		}
	} else {
		base.Proxy = http.ProxyFromEnvironment
	}
	rps := sc.RateLimitPerSec
	if rps <= 0 {
		rps = 1
	}
	burst := int(math.Ceil(rps)) + 1
	if burst < 1 {
		burst = 1
	}
	lim := rate.NewLimiter(rate.Limit(rps), burst)
	maxConcurrent := sc.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	sem := make(chan struct{}, maxConcurrent)

	return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		select {
		case sem <- struct{}{}:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		defer func() { <-sem }()
		if err := lim.Wait(req.Context()); err != nil {
			return nil, err
		}
		return base.RoundTrip(req)
	})
}
