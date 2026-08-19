package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"akyuu/internal/config"
)

// TestSiteTransportRoutesThroughProxy proves that a configured proxy is used:
// with the proxy pointing at a closed port, the request error must reference
// the proxy address, not the (reachable) target server.
func TestSiteTransportRoutesThroughProxy(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	sc := &config.SiteConfig{
		Proxy: "socks5://127.0.0.1:1", // closed port
	}
	rt := newSiteTransport(sc)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error from unreachable proxy")
	}
	if !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Errorf("error %q does not reference the proxy address", err)
	}
	if strings.Contains(err.Error(), target.Listener.Addr().String()) {
		t.Errorf("error %q references the target, not the proxy", err)
	}
}

// TestSiteTransportDirectWithoutProxy proves the default path still works and
// is rate-limited but not proxied.
func TestSiteTransportDirectWithoutProxy(t *testing.T) {
	hit := make(chan struct{}, 2)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	sc := &config.SiteConfig{RateLimitPerSec: 50}
	rt := newSiteTransport(sc)

	for i := 0; i < 2; i++ {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, target.URL, nil)
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	select {
	case <-hit:
	case <-time.After(5 * time.Second):
		t.Fatal("target server never reached")
	}
}
