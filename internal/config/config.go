// Package config loads the global archiver configuration plus the per-site
// YAML definitions under config/sites/. Nothing here depends on Postgres or
// the adapters, keeping site definitions plain data.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level archiver configuration (config/config.yaml).
type Config struct {
	Database struct {
		DSN string `yaml:"dsn"`
	} `yaml:"database"`

	Storage struct {
		Dir string `yaml:"dir"` // root for content-addressable file storage
	} `yaml:"storage"`

	// Safety configures the CSAM hash-matching service. This runs on every
	// downloaded file and cannot be disabled per-site/per-board.
	Safety struct {
		// HashListPath points at a text file of hex SHA-256 hashes, one per
		// line. May be empty if the API source is used instead.
		HashListPath string `yaml:"hash_list_path"`
		// APIURL is a PhotoDNA-style endpoint that accepts a hash and returns
		// a match verdict. Empty disables the remote source.
		APIURL   string `yaml:"api_url"`
		APIToken string `yaml:"api_token"`
	} `yaml:"safety"`

	Scheduler SchedulerConfig `yaml:"scheduler"`

	SitesDir string `yaml:"sites_dir"`

	LogLevel string `yaml:"log_level"`
}

// SchedulerConfig holds global defaults. Site/board configs override the
// download flags; the three-level resolution is global -> site -> board.
type SchedulerConfig struct {
	PollInterval             Duration `yaml:"poll_interval"`
	RateLimitPerSec          float64  `yaml:"rate_limit_per_sec"`
	MaxConcurrentRequests    int      `yaml:"max_concurrent_requests"`
	DownloadFull             bool     `yaml:"download_full"`
	DownloadThumb            bool     `yaml:"download_thumb"`
	TextOnly                 bool     `yaml:"text_only"`
	CircuitBreakerThreshold  int      `yaml:"circuit_breaker_threshold"`
	MaxAttempts              int      `yaml:"max_attempts"`
	ThreadStaleAfter         Duration `yaml:"thread_stale_after"`
	MissingBeforeArchive     int      `yaml:"missing_before_archive"`
	WorkersPerSite           int      `yaml:"workers_per_site"`
	DownloadBackfillBatch    int      `yaml:"download_backfill_batch"`
	ConsecutiveFailBlocklist int      `yaml:"consecutive_fail_blocklist"`
}

// Defaults returns a Config with sane defaults applied.
func Defaults() *Config {
	c := &Config{}
	c.Scheduler.PollInterval = Duration(30 * time.Second)
	c.Scheduler.RateLimitPerSec = 1
	c.Scheduler.MaxConcurrentRequests = 1
	c.Scheduler.DownloadFull = false
	c.Scheduler.DownloadThumb = true
	c.Scheduler.CircuitBreakerThreshold = 5
	c.Scheduler.MaxAttempts = 10
	c.Scheduler.ThreadStaleAfter = Duration(6 * time.Hour)
	c.Scheduler.MissingBeforeArchive = 3
	c.Scheduler.WorkersPerSite = 1
	c.Scheduler.DownloadBackfillBatch = 50
	c.Scheduler.ConsecutiveFailBlocklist = 3
	c.Storage.Dir = "./storage"
	c.SitesDir = "./config/sites"
	c.LogLevel = "info"
	return c
}

// Load reads a YAML file into Config.
func Load(path string) (*Config, error) {
	c := Defaults()
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(raw, c); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	if c.Database.DSN == "" {
		return nil, fmt.Errorf("config: database.dsn is required")
	}
	if c.Scheduler.CircuitBreakerThreshold <= 0 {
		c.Scheduler.CircuitBreakerThreshold = 5
	}
	if c.Scheduler.MaxAttempts <= 0 {
		c.Scheduler.MaxAttempts = 10
	}
	if c.Scheduler.MissingBeforeArchive <= 0 {
		c.Scheduler.MissingBeforeArchive = 3
	}
	if c.Scheduler.WorkersPerSite <= 0 {
		c.Scheduler.WorkersPerSite = 1
	}
	return c, nil
}

// SiteConfig is one file under config/sites/. A site is a specific instance of
// a platform; boards is the authoritative list of boards to poll.
type SiteConfig struct {
	Name            string        `yaml:"name"`
	BaseURL         string        `yaml:"base_url"`
	Platform        string        `yaml:"platform"`
	APIAvailable    bool          `yaml:"api_available"`
	PollInterval    Duration      `yaml:"poll_interval"`
	RateLimitPerSec float64       `yaml:"rate_limit_per_sec"`
	MaxConcurrent   int           `yaml:"max_concurrent_requests"`
	DownloadFull    *bool         `yaml:"download_full"`
	DownloadThumb   *bool         `yaml:"download_thumb"`
	Proxy           string        `yaml:"proxy"`
	UserAgent       string        `yaml:"user_agent"`
	Boards          []BoardConfig `yaml:"boards"`

	// AutoBoards makes the scheduler discover boards at startup from the
	// adapter's board listing instead of using the static boards list.
	AutoBoards bool `yaml:"auto_boards"`

	// PaginatedCatalog marks a site whose board catalog is a paginated crawl
	// of full thread history (archive sites). The scheduler tracks a per-board
	// page cursor and crawls CrawlPagesPerPoll pages each poll cycle.
	PaginatedCatalog bool `yaml:"paginated_catalog"`

	// CrawlPagesPerPoll bounds how many catalog pages one board advances per
	// poll cycle. Defaults to 1.
	CrawlPagesPerPoll int `yaml:"crawl_pages_per_poll"`
}

// BoardConfig is a single board on a site.
type BoardConfig struct {
	Code          string   `yaml:"code"`
	Title         string   `yaml:"title"`
	NSFW          bool     `yaml:"nsfw"`
	Worksafe      bool     `yaml:"worksafe"`
	PollInterval  Duration `yaml:"poll_interval"`
	DownloadFull  *bool    `yaml:"download_full"`
	DownloadThumb *bool    `yaml:"download_thumb"`
}

// LoadSites reads every *.yaml file under dir, sorted by filename.
func LoadSites(dir string) ([]*SiteConfig, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("config: read sites dir: %w", err)
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		paths = append(paths, filepath.Join(dir, e.Name()))
	}
	sort.Strings(paths)
	var out []*SiteConfig
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("config: read %s: %w", p, err)
		}
		var sc SiteConfig
		if err := yaml.Unmarshal(raw, &sc); err != nil {
			return nil, fmt.Errorf("config: parse %s: %w", p, err)
		}
		if sc.Name == "" {
			sc.Name = filepath.Base(p[:len(p)-len(filepath.Ext(p))])
		}
		if sc.BaseURL == "" {
			return nil, fmt.Errorf("config: %s: base_url is required", p)
		}
		if sc.Platform == "" {
			return nil, fmt.Errorf("config: %s: platform is required", p)
		}
		if sc.PollInterval == 0 {
			sc.PollInterval = Duration(30 * time.Second)
		}
		if sc.RateLimitPerSec <= 0 {
			sc.RateLimitPerSec = 1
		}
		if sc.MaxConcurrent <= 0 {
			sc.MaxConcurrent = 1
		}
		if sc.CrawlPagesPerPoll <= 0 {
			sc.CrawlPagesPerPoll = 1
		}
		if sc.PaginatedCatalog && !sc.APIAvailable {
			return nil, fmt.Errorf("config: %s: paginated_catalog requires api_available", p)
		}
		if sc.AutoBoards && sc.Boards != nil && len(sc.Boards) > 0 {
			return nil, fmt.Errorf("config: %s: auto_boards cannot be combined with a static boards list", p)
		}
		for i := range sc.Boards {
			if sc.Boards[i].Code == "" {
				return nil, fmt.Errorf("config: %s: board with empty code", p)
			}
			if sc.Boards[i].Worksafe && sc.Boards[i].NSFW {
				return nil, fmt.Errorf("config: %s: board %q cannot be both nsfw and worksafe", p, sc.Boards[i].Code)
			}
		}
		out = append(out, &sc)
	}
	return out, nil
}

// DownloadFlags resolves the three-level download policy (global -> site ->
// board). Most specific wins; nil means "not set at this level".
func DownloadFlags(global bool, site *bool, board *bool) bool {
	if board != nil {
		return *board
	}
	if site != nil {
		return *site
	}
	return global
}

// Duration is a time.Duration that unmarshals from YAML strings like "30s".
type Duration time.Duration

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return fmt.Errorf("config: duration must be a string like \"30s\": %w", err)
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("config: invalid duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

// D returns the value as a time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }
