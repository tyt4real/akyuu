// Command archiver runs the multi-site imageboard archiver: it loads the
// global config, discovers built-in site modules, loads optional per-site YAML
// definitions (for custom sites), syncs the site/board catalog into Postgres,
// applies migrations, and starts the scheduler.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "akyuu/internal/site/4chan"
	_ "akyuu/internal/site/4chon"
	_ "akyuu/internal/site/alogs"
	_ "akyuu/internal/site/desuarchive"
	_ "akyuu/internal/site/lainchan"
	_ "akyuu/internal/site/leftypol"
	_ "akyuu/internal/site/sushigirl"
	_ "akyuu/internal/site/wired7"

	"akyuu/internal/config"
	"akyuu/internal/downloader"
	"akyuu/internal/embedder"
	"akyuu/internal/scheduler"
	"akyuu/internal/siteadapters"
	"akyuu/internal/siteregistry"
	"akyuu/internal/store"
)

func main() {
	configPath := flag.String("config", "config/config.yaml", "path to the global config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	logger := newLogger(cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := store.New(ctx, cfg.Database.DSN, logger)
	if err != nil {
		logger.Error("connect database", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := st.Migrate(ctx); err != nil {
		logger.Error("migrate", "err", err)
		os.Exit(1)
	}

	// Load built-in site modules from the registry.
	builtinSites := siteadapters.ToConfigs(siteregistry.All())

	// Load custom sites from YAML files (if any).
	customSites, err := config.LoadSites(cfg.SitesDir)
	if err != nil {
		logger.Error("load custom site configs", "err", err)
		os.Exit(1)
	}

	// Merge: built-in sites first, custom sites can override by name.
	allSites := mergeSites(builtinSites, customSites)
	if len(allSites) == 0 {
		logger.Warn("no site configs found (built-in or custom)")
	}
	syncSites(ctx, st, allSites, logger)

	checker, err := downloader.NewBlocklistChecker(
		cfg.Safety.HashListPath, cfg.Safety.APIURL, cfg.Safety.APIToken, logger)
	if err != nil {
		logger.Error("init safety checker", "err", err)
		os.Exit(1)
	}

	sched := scheduler.New(st, cfg, allSites, checker, logger)
	if cfg.Embeddings.Enabled {
		if err := startEmbedWorker(ctx, st, cfg, logger); err != nil {
			logger.Error("start embed worker", "err", err)
			os.Exit(1)
		}
	} else {
		logger.Info("embeddings disabled; semantic search is off")
	}
	logger.Info("archiver starting", "builtin_sites", len(builtinSites), "custom_sites", len(customSites), "total", len(allSites))
	sched.Run(ctx)
	logger.Info("archiver stopped")
}

// mergeSites merges built-in and custom site configs. Custom sites override
// built-in sites with the same name.
func mergeSites(builtin, custom []*config.SiteConfig) []*config.SiteConfig {
	byName := make(map[string]*config.SiteConfig)
	for _, s := range builtin {
		byName[s.Name] = s
	}
	for _, s := range custom {
		byName[s.Name] = s
	}
	out := make([]*config.SiteConfig, 0, len(byName))
	for _, s := range byName {
		out = append(out, s)
	}
	return out
}

// startEmbedWorker loads the ONNX model and runs the embedding worker in the
// background until ctx is cancelled. If modal is enabled, uses ModalEmbedder instead.
func startEmbedWorker(ctx context.Context, st *store.Store, cfg *config.Config, logger *slog.Logger) error {
	if cfg.Modal.Enabled {
		return startModalEmbedWorker(ctx, st, cfg, logger)
	}
	model, err := embedder.NewONNX(cfg.Embeddings.ModelDir, cfg.Embeddings.ModelName, cfg.Embeddings.Dimensions)
	if err != nil {
		return err
	}
	go func() {
		defer model.Close()
		embedder.NewWorker(st, model, embedder.WorkerConfig{
			BatchSize:            cfg.Embeddings.BatchSize,
			PollInterval:         cfg.Embeddings.PollInterval.D(),
			MinTextLength:        cfg.Embeddings.MinTextLength,
			NormalizeBeforeEmbed: cfg.Embeddings.NormalizeBeforeEmbed,
			Normalizer:           embedder.NewFakeNormalizer(),
		}, logger).Run(ctx)
	}()
	return nil
}

func startModalEmbedWorker(ctx context.Context, st *store.Store, cfg *config.Config, logger *slog.Logger) error {
	modalEmb, err := embedder.NewModalEmbedder(cfg.Modal, logger)
	if err != nil {
		return fmt.Errorf("create modal embedder: %w", err)
	}

	go func() {
		defer modalEmb.Close()
		ticker := time.NewTicker(cfg.Embeddings.PollInterval.D())
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				posts, err := st.PendingEmbeddingBatch(ctx, cfg.Embeddings.BatchSize)
				if err != nil {
					logger.Error("modal embed worker: pending batch", "err", err)
					continue
				}
				if len(posts) == 0 {
					continue
				}
				processModalBatch(ctx, st, modalEmb, posts, logger)
			}
		}
	}()
	return nil
}

func processModalBatch(ctx context.Context, st *store.Store, emb *embedder.ModalEmbedder, posts []*store.Post, logger *slog.Logger) {
	type job struct {
		post *store.Post
		text string
	}
	var jobs []job
	for _, p := range posts {
		text, err := embedder.CleanText(p.CommentParsed)
		if err != nil {
			logger.Warn("modal embed worker: clean post", "post", p.ID, "err", err)
			continue
		}
		if len(text) < 8 {
			if err := st.MarkEmbeddingSkipped(ctx, p.ID); err != nil {
				logger.Error("modal embed worker: skip post", "post", p.ID, "err", err)
			}
			continue
		}
		jobs = append(jobs, job{post: p, text: text})
	}

	if len(jobs) == 0 {
		return
	}

	texts := make([]string, len(jobs))
	for i, j := range jobs {
		texts[i] = j.text
	}

	vecs, err := emb.EmbedBatch(ctx, texts)
	if err != nil {
		logger.Error("modal embed worker: embed batch", "count", len(texts), "err", err)
		return
	}

	for i, j := range jobs {
		if err := st.MarkEmbedding(ctx, j.post.ID, vecs[i], emb.ModelVersion(), embedder.HashText(j.text)); err != nil {
			logger.Error("modal embed worker: store vector", "post", j.post.ID, "err", err)
			continue
		}
		logger.Debug("modal embed worker: embedded post", "post", j.post.ID, "text_len", len(j.text))
	}
}

// syncSites upserts every configured site and board so the scheduler's lookups
// resolve. Boards removed from config are left in place (never deleted).
func syncSites(ctx context.Context, st *store.Store, sites []*config.SiteConfig, logger *slog.Logger) {
	for _, sc := range sites {
		siteID, err := st.UpsertSite(ctx, sc.Name, sc.BaseURL, sc.Platform, sc.APIAvailable)
		if err != nil {
			logger.Warn("upsert site", "site", sc.Name, "err", err)
			continue
		}
		for _, bc := range sc.Boards {
			worksafe := bc.Worksafe
			if !worksafe && !bc.NSFW {
				worksafe = true
			}
			if _, err := st.UpsertBoard(ctx, siteID, bc.Code, bc.Title, bc.NSFW, worksafe); err != nil {
				logger.Warn("upsert board", "site", sc.Name, "board", bc.Code, "err", err)
			}
		}
	}
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l}))
}
