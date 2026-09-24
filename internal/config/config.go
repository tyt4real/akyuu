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

	// Embeddings drives the semantic-search pipeline: an in-process ONNX
	// sentence-embedding model embeds post bodies into a pgvector column. All
	// of it is opt-in; with enabled:false the scheduler never touches the
	// model and the search API is unavailable.
	Embeddings EmbeddingsConfig `yaml:"embeddings"`

	// Multimodal configures OCR, CLIP, and Whisper workers for multimodal search.
	Multimodal MultimodalConfig `yaml:"multimodal"`

	// Lineage configures crosspost detection, meme clustering, thread continuity.
	Lineage LineageConfig `yaml:"lineage"`

	// Search configures saved search evaluation, alerts, and trend aggregation.
	Search SearchConfig `yaml:"search"`

	// Ambitious configures stylometric clustering and anomaly detection.
	Ambitious AmbitiousConfig `yaml:"ambitious"`

	SitesDir string `yaml:"sites_dir"`

	LogLevel string `yaml:"log_level"`
}

// EmbeddingsConfig configures the semantic-search ingestion and query path.
type EmbeddingsConfig struct {
	// Enabled turns the embed worker on (cmd/archiver) and unlocks /search
	// (cmd/api). When false, posts are never embedded and search is disabled.
	Enabled bool `yaml:"enabled"`

	// ModelDir holds model.onnx plus tokenizer.json (a HuggingFace
	// sentence-embedding model, e.g. all-MiniLM-L6-v2). Not shipped with the
	// binary; download it separately (see docs/semantic-search.md).
	ModelDir string `yaml:"model_dir"`

	// Dimensions is the vector width produced by the model. It is locked in at
	// migration time (post_embeddings.embedding is vector(384)); a model swap
	// requires a new migration.
	Dimensions int `yaml:"dimensions"`

	// ModelName identifies the model in post_embeddings.model_version. Search
	// only returns vectors stored under the same name, so a model change means
	// old vectors are ignored until re-embedded.
	ModelName string `yaml:"model_name"`

	// BatchSize is how many posts the worker embeds in one model call.
	BatchSize int `yaml:"batch_size"`

	// PollInterval is how long the worker idles when the embedding queue is
	// empty.
	PollInterval Duration `yaml:"poll_interval"`

	// MinTextLength is the minimum number of characters a cleaned post body
	// must have to be embedded; shorter/empty posts are marked non-searchable.
	MinTextLength int `yaml:"min_text_length"`

	// NormalizeBeforeEmbed enables LLM-based text normalization before embedding.
	// When enabled, each post body is passed through a TextNormalizer (e.g., LLM)
	// to clean up noisy text before embedding.
	NormalizeBeforeEmbed bool `yaml:"normalize_before_embed"`
}

// MultimodalConfig configures OCR, CLIP, and Whisper workers.
type MultimodalConfig struct {
	PollInterval Duration `yaml:"poll_interval"`

	OCR struct {
		Enabled      bool   `yaml:"enabled"`
		TesseractCmd string `yaml:"tesseract_cmd"`
		Languages    string `yaml:"languages"`
	} `yaml:"ocr"`

	CLIP struct {
		Enabled      bool   `yaml:"enabled"`
		ModelDir     string `yaml:"model_dir"`
		ModelVersion string `yaml:"model_version"`
		Dimensions   int    `yaml:"dimensions"`
	} `yaml:"clip"`

	Whisper struct {
		Enabled    bool   `yaml:"enabled"`
		WhisperCmd string `yaml:"whisper_cmd"`
		ModelPath  string `yaml:"model_path"`
		ModelName  string `yaml:"model_name"`
	} `yaml:"whisper"`
}

// LineageConfig configures crosspost detection, meme clustering, thread continuity.
type LineageConfig struct {
	PollInterval Duration `yaml:"poll_interval"`

	Crosspost struct {
		Enabled        bool `yaml:"enabled"`
		PHASHThreshold int  `yaml:"phash_threshold"`
	} `yaml:"crosspost"`

	Clustering struct {
		Enabled   bool   `yaml:"enabled"`
		Algo      string `yaml:"algo"`      // "phash" or "dhash"
		Threshold int    `yaml:"threshold"` // Hamming distance
		BatchSize int    `yaml:"batch_size"`
	} `yaml:"clustering"`

	Continuity struct {
		Enabled        bool    `yaml:"enabled"`
		TimeGapHours   float64 `yaml:"time_gap_hours"`
		TitleThreshold float64 `yaml:"title_threshold"`
		OpSimThreshold float64 `yaml:"op_sim_threshold"`
	} `yaml:"continuity"`

	ReplyGraph struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"reply_graph"`
}

// SearchConfig configures saved search evaluation, alerts, and trend aggregation.
type SearchConfig struct {
	PollInterval Duration `yaml:"poll_interval"`

	Evaluator struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"evaluator"`

	Trends struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"trends"`

	Alerts struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"alerts"`
}

// AmbitiousConfig configures stylometric clustering and anomaly detection.
type AmbitiousConfig struct {
	PollInterval Duration `yaml:"poll_interval"`

	Stylometric struct {
		Enabled      bool    `yaml:"enabled"`
		MinPosts     int     `yaml:"min_posts"`
		NGramMin     int     `yaml:"ngram_min"`
		NGramMax     int     `yaml:"ngram_max"`
		TopFeatures  int     `yaml:"top_features"`
		SimThreshold float64 `yaml:"sim_threshold"`
	} `yaml:"stylometric"`

	Anomaly struct {
		Enabled          bool    `yaml:"enabled"`
		VolumeZThreshold float64 `yaml:"volume_z_threshold"`
		BurstThreshold   float64 `yaml:"burst_threshold"`
		BurstWindowMin   int     `yaml:"burst_window_min"`
		LookbackHours    int     `yaml:"lookback_hours"`
	} `yaml:"anomaly"`
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
	CaptureRaw               bool     `yaml:"capture_raw"`
	RawRetentionDays         int      `yaml:"raw_retention_days"`
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
	c.Scheduler.CaptureRaw = true
	c.Scheduler.RawRetentionDays = 90
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
	c.Embeddings.Dimensions = 384
	c.Embeddings.ModelName = "all-MiniLM-L6-v2"
	c.Embeddings.BatchSize = 32
	c.Embeddings.PollInterval = Duration(10 * time.Second)
	c.Embeddings.MinTextLength = 8
	c.Multimodal.PollInterval = Duration(10 * time.Second)
	c.Multimodal.OCR.Enabled = false
	c.Multimodal.OCR.TesseractCmd = "tesseract"
	c.Multimodal.OCR.Languages = "eng"
	c.Multimodal.CLIP.Enabled = false
	c.Multimodal.CLIP.ModelDir = "./models/mobileclip-s1"
	c.Multimodal.CLIP.ModelVersion = "mobileclip-s1@1.0"
	c.Multimodal.CLIP.Dimensions = 384
	c.Multimodal.Whisper.Enabled = false
	c.Multimodal.Whisper.WhisperCmd = "whisper-cli"
	c.Multimodal.Whisper.ModelPath = "./models/whisper"
	c.Multimodal.Whisper.ModelName = "base.en"
	c.Lineage.PollInterval = Duration(5 * time.Minute)
	c.Lineage.Crosspost.Enabled = false
	c.Lineage.Crosspost.PHASHThreshold = 8
	c.Lineage.Clustering.Enabled = false
	c.Lineage.Clustering.Algo = "phash"
	c.Lineage.Clustering.Threshold = 8
	c.Lineage.Clustering.BatchSize = 500
	c.Lineage.Continuity.Enabled = false
	c.Lineage.Continuity.TimeGapHours = 48
	c.Lineage.Continuity.TitleThreshold = 0.8
	c.Lineage.Continuity.OpSimThreshold = 0.75
	c.Lineage.ReplyGraph.Enabled = false
	c.Search.PollInterval = Duration(5 * time.Minute)
	c.Search.Evaluator.Enabled = false
	c.Search.Trends.Enabled = false
	c.Search.Alerts.Enabled = false
	c.Ambitious.PollInterval = Duration(15 * time.Minute)
	c.Ambitious.Stylometric.Enabled = false
	c.Ambitious.Stylometric.MinPosts = 5
	c.Ambitious.Stylometric.NGramMin = 3
	c.Ambitious.Stylometric.NGramMax = 5
	c.Ambitious.Stylometric.TopFeatures = 200
	c.Ambitious.Stylometric.SimThreshold = 0.7
	c.Ambitious.Anomaly.Enabled = false
	c.Ambitious.Anomaly.VolumeZThreshold = 3.0
	c.Ambitious.Anomaly.BurstThreshold = 5.0
	c.Ambitious.Anomaly.BurstWindowMin = 10
	c.Ambitious.Anomaly.LookbackHours = 168
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
	if c.Embeddings.Dimensions <= 0 {
		c.Embeddings.Dimensions = 384
	}
	if c.Embeddings.BatchSize <= 0 {
		c.Embeddings.BatchSize = 32
	}
	if c.Embeddings.PollInterval <= 0 {
		c.Embeddings.PollInterval = Duration(10 * time.Second)
	}
	if c.Embeddings.MinTextLength < 0 {
		c.Embeddings.MinTextLength = 0
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
