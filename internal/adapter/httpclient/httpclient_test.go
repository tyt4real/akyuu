package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewDefaults(t *testing.T) {
	c := New("https://example.org/", "", nil)
	if c.base != "https://example.org" {
		t.Errorf("base = %q", c.base)
	}
	if c.userAgent != defaultUserAgent {
		t.Errorf("userAgent = %q", c.userAgent)
	}
	if c.hc.Transport == nil {
		t.Error("transport should fall back to http.DefaultTransport")
	}
}

func TestResolve(t *testing.T) {
	c := New("https://example.org", "", nil)
	cases := map[string]string{
		"":                    "https://example.org",
		"/b/catalog.html":     "https://example.org/b/catalog.html",
		"b/catalog.html":      "https://example.org/b/catalog.html",
		"https://other.org/x": "https://other.org/x",
		"http://other.org/x":  "http://other.org/x",
		"/":                   "https://example.org/",
	}
	for path, want := range cases {
		if got := c.Resolve(path); got != want {
			t.Errorf("Resolve(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestGetSuccessSetsUserAgentAndAccept(t *testing.T) {
	var gotUA, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		io.WriteString(w, "hello")
	}))
	defer srv.Close()

	c := New(srv.URL, "my-agent/1.0", nil)
	resp, err := c.Get(context.Background(), "/x")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello" {
		t.Errorf("body = %q", body)
	}
	if gotUA != "my-agent/1.0" {
		t.Errorf("UA = %q", gotUA)
	}
	if !strings.Contains(gotAccept, "application/json") {
		t.Errorf("Accept = %q", gotAccept)
	}
	if resp.Request.URL.Path != "/x" {
		t.Errorf("final url path = %q", resp.Request.URL.Path)
	}
}

func TestGetNotFoundReturnsErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := New(srv.URL, "", nil)
	_, err := c.Get(context.Background(), "/gone")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetServerErrorIncludesStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL, "", nil)
	_, err := c.Get(context.Background(), "/x")
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("err %q missing status", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("err %q missing body", err)
	}
}

func TestGetTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("unreachable")
	}))
	addr := srv.URL
	srv.Close() // force dial errors

	c := New(addr, "", nil)
	_, err := c.Get(context.Background(), "/x")
	if err == nil {
		t.Fatal("expected dial error")
	}
	if !strings.Contains(err.Error(), "GET") {
		t.Errorf("err %q should mention the path", err)
	}
}

func TestGetBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "", nil)
	b, err := c.GetBytes(context.Background(), "/feed")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"ok":true}` {
		t.Errorf("bytes = %q", b)
	}
}
