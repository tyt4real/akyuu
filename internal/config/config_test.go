package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func writeTemp(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAndDefaults(t *testing.T) {
	path := writeTemp(t, "config.yaml", `
database:
  dsn: "postgres://u:p@localhost:5432/db"
scheduler:
  poll_interval: 90s
  rate_limit_per_sec: 2
  download_full: true
sites_dir: "./sites"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.DSN != "postgres://u:p@localhost:5432/db" {
		t.Errorf("dsn = %q", cfg.Database.DSN)
	}
	if cfg.Scheduler.PollInterval.D() != 90*time.Second {
		t.Errorf("poll_interval = %v", cfg.Scheduler.PollInterval.D())
	}
	if !cfg.Scheduler.DownloadFull {
		t.Error("download_full should be true")
	}
	// Unset fields keep defaults.
	if cfg.Scheduler.MaxAttempts != 10 || !cfg.Scheduler.DownloadThumb {
		t.Errorf("defaults lost: max_attempts=%d download_thumb=%v", cfg.Scheduler.MaxAttempts, cfg.Scheduler.DownloadThumb)
	}
	if cfg.Scheduler.WorkersPerSite != 1 {
		t.Errorf("workers_per_site = %d", cfg.Scheduler.WorkersPerSite)
	}
}

func TestLoadRequiresDSN(t *testing.T) {
	path := writeTemp(t, "config.yaml", "scheduler:\n  poll_interval: 30s\n")
	if _, err := Load(path); err == nil {
		t.Error("expected error for missing dsn")
	}
}

func TestDurationUnmarshalInvalid(t *testing.T) {
	var d Duration
	if err := d.UnmarshalYAML(&yaml.Node{Value: "not-a-duration"}); err == nil {
		t.Error("expected error for invalid duration")
	}
}

func TestLoadSitesProxy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "site.yaml")
	body := `
name: site
base_url: "https://example.org"
platform: fourchan
proxy: "socks5://127.0.0.1:9050"
boards:
  - code: a
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	sites, err := LoadSites(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || sites[0].Proxy != "socks5://127.0.0.1:9050" {
		t.Fatalf("proxy = %q", sites[0].Proxy)
	}
}

func TestLoadSitesArchiveOptions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "archive.yaml")
	body := `
name: desu
base_url: "https://desuarchive.org"
platform: desuarchive
api_available: true
auto_boards: true
paginated_catalog: true
crawl_pages_per_poll: 3
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	sites, err := LoadSites(dir)
	if err != nil {
		t.Fatal(err)
	}
	sc := sites[0]
	if !sc.AutoBoards || !sc.PaginatedCatalog || sc.CrawlPagesPerPoll != 3 {
		t.Errorf("site = %+v", sc)
	}
}

func TestLoadSitesCrawlPagesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "site.yaml")
	body := `
name: desu
base_url: "https://desuarchive.org"
platform: desuarchive
api_available: true
paginated_catalog: true
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	sites, err := LoadSites(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sites[0].CrawlPagesPerPoll != 1 {
		t.Errorf("crawl_pages_per_poll default = %d, want 1", sites[0].CrawlPagesPerPoll)
	}
}

func TestLoadSitesValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"paginated without api", `
name: s
base_url: "https://x.org"
platform: desuarchive
paginated_catalog: true
`},
		{"auto_boards with static boards", `
name: s
base_url: "https://x.org"
platform: desuarchive
auto_boards: true
boards:
  - code: a
`},
	}
	for _, c := range cases {
		dir := t.TempDir()
		path := filepath.Join(dir, "site.yaml")
		if err := os.WriteFile(path, []byte(c.body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadSites(dir); err == nil {
			t.Errorf("%s: expected validation error", c.name)
		}
	}
}

func TestDownloadFlagsResolution(t *testing.T) {
	cases := []struct {
		global bool
		site   *bool
		board  *bool
		want   bool
	}{
		{false, nil, nil, false},
		{true, nil, nil, true},
		{true, ptr(false), nil, false},
		{false, ptr(true), nil, true},
		{true, ptr(true), ptr(false), false},
		{false, ptr(false), ptr(true), true},
	}
	for _, c := range cases {
		if got := DownloadFlags(c.global, c.site, c.board); got != c.want {
			t.Errorf("DownloadFlags(%v,%v,%v) = %v, want %v", c.global, c.site, c.board, got, c.want)
		}
	}
}

func ptr(b bool) *bool { return &b }
