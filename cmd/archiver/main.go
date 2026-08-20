// Command archiver runs the multi-site imageboard archiver: it loads the
// global config plus per-site YAML definitions, syncs the site/board catalog
// into Postgres, applies migrations, and starts the scheduler.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"akyuu/internal/config"
	"akyuu/internal/downloader"
	"akyuu/internal/embedder"
	"akyuu/internal/scheduler"
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

	sites, err := config.LoadSites(cfg.SitesDir)
	if err != nil {
		logger.Error("load site configs", "err", err)
		os.Exit(1)
	}
	if len(sites) == 0 {
		logger.Warn("no site configs found", "dir", cfg.SitesDir)
	}
	syncSites(ctx, st, sites, logger)

	checker, err := downloader.NewBlocklistChecker(
		cfg.Safety.HashListPath, cfg.Safety.APIURL, cfg.Safety.APIToken, logger)
	if err != nil {
		logger.Error("init safety checker", "err", err)
		os.Exit(1)
	}

	sched := scheduler.New(st, cfg, sites, checker, logger)
	if cfg.Embeddings.Enabled {
		if err := startEmbedWorker(ctx, st, cfg, logger); err != nil {
			logger.Error("start embed worker", "err", err)
			os.Exit(1)
		}
	} else {
		logger.Info("embeddings disabled; semantic search is off")
	}
	logger.Info("archiver starting", "sites", len(sites))
	sched.Run(ctx)
	logger.Info("archiver stopped")
}

// startEmbedWorker loads the ONNX model and runs the embedding worker in the
// background until ctx is cancelled.
func startEmbedWorker(ctx context.Context, st *store.Store, cfg *config.Config, logger *slog.Logger) error {
	model, err := embedder.NewONNX(cfg.Embeddings.ModelDir, cfg.Embeddings.ModelName, cfg.Embeddings.Dimensions)
	if err != nil {
		return err
	}
	go func() {
		defer model.Close()
		embedder.NewWorker(st, model, embedder.WorkerConfig{
			BatchSize:     cfg.Embeddings.BatchSize,
			PollInterval:  cfg.Embeddings.PollInterval.D(),
			MinTextLength: cfg.Embeddings.MinTextLength,
		}, logger).Run(ctx)
	}()
	return nil
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
