package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"akyuu/internal/embedder"
	"akyuu/internal/store"
)

// fakeSearcher returns canned results and records the search options it got.
type fakeSearcher struct {
	results []store.SearchResult
	opts    store.SearchOpts
	model   string
	err     error
}

func (f *fakeSearcher) SearchEmbeddings(_ context.Context, q []float32, model string, opts store.SearchOpts) ([]store.SearchResult, error) {
	f.model = model
	f.opts = opts
	return f.results, f.err
}

type fakeHealth struct{ err error }

func (f fakeHealth) Ping(context.Context) error { return f.err }

func newTestServer(searcher Searcher, health HealthChecker, store *store.Store) *httptest.Server {
	ts := httptest.NewServer(New(embedder.NewFake(), searcher, health, store, nil).Routes())
	return ts
}

func TestHealthOK(t *testing.T) {
	srv := newTestServer(&fakeSearcher{}, fakeHealth{}, nil)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var h healthResponse
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		t.Fatal(err)
	}
	if h.Status != "ok" {
		t.Errorf("expected ok, got %q", h.Status)
	}
}

func TestHealthDegraded(t *testing.T) {
	srv := newTestServer(&fakeSearcher{}, fakeHealth{err: errors.New("down")}, nil)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestSearchFindsResults(t *testing.T) {
	ts := time.Unix(1000, 0)
	fs := &fakeSearcher{results: []store.SearchResult{
		{PostID: 9, Score: 0.93, Site: "fourchan", Board: "g", ThreadID: 2,
			ThreadNativeID: "100", Timestamp: &ts, Excerpt: "best linux distro debates"},
	}}
	srv := newTestServer(fs, fakeHealth{}, nil)
	defer srv.Close()

	body := `{"query":"linux distro","site":"fourchan","board":"g","limit":5}`
	resp, err := http.Post(srv.URL+"/search", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var out SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Count != 1 || len(out.Results) != 1 {
		t.Fatalf("expected 1 result, got count=%d len=%d", out.Count, len(out.Results))
	}
	r := out.Results[0]
	if r.PostID != 9 || r.Site != "fourchan" || r.Board != "g" || r.ThreadNativeID != "100" {
		t.Errorf("result fields wrong: %+v", r)
	}
	if fs.opts.Site != "fourchan" || fs.opts.Board != "g" || fs.opts.Limit != 5 {
		t.Errorf("search options not forwarded: %+v", fs.opts)
	}
	if fs.model != "fake" {
		t.Errorf("model version not forwarded, got %q", fs.model)
	}
}

func TestSearchRequiresQuery(t *testing.T) {
	srv := newTestServer(&fakeSearcher{}, fakeHealth{}, nil)
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/search", "application/json", strings.NewReader(`{"query":""}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty query, got %d", resp.StatusCode)
	}
}

func TestSearchBadJSON(t *testing.T) {
	srv := newTestServer(&fakeSearcher{}, fakeHealth{}, nil)
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/search", "application/json", strings.NewReader(`{not json`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad JSON, got %d", resp.StatusCode)
	}
}

func TestSearchStoreError(t *testing.T) {
	srv := newTestServer(&fakeSearcher{err: errors.New("boom")}, fakeHealth{}, nil)
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/search", "application/json",
		strings.NewReader(`{"query":"linux distro"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}
