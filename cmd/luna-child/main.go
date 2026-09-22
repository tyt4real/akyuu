// Command luna-child runs the search & discovery workers (saved search evaluator, trend aggregator).
// Luna Child - the curious fairy of the Three Fairies of Light, seeks knowledge and trends.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"akyuu/internal/config"
	"akyuu/internal/embedder"
	"akyuu/internal/search"
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

	var emb embedder.Embedder
	var embCloser embedder.Closer
	if cfg.Embeddings.Enabled {
		emb, err = embedder.NewONNX(cfg.Embeddings.ModelDir, cfg.Embeddings.ModelName, cfg.Embeddings.Dimensions)
		if err != nil {
			logger.Error("load embedder", "err", err)
			os.Exit(1)
		}
		if c, ok := emb.(embedder.Closer); ok {
			embCloser = c
			defer embCloser.Close()
		}
	}

	searchCfg := search.CoordinatorConfig{
		PollInterval:     cfg.Search.PollInterval.D(),
		EvaluatorEnabled: cfg.Search.Evaluator.Enabled,
		TrendsEnabled:    cfg.Search.Trends.Enabled,
	}

	coordinator, err := search.NewCoordinator(st, emb, searchCfg, logger)
	if err != nil {
		logger.Error("create luna-child coordinator", "err", err)
		os.Exit(1)
	}

	logger.Info("luna-child starting - searching for answers in the dark")
	coordinator.Run(ctx)
	logger.Info("luna-child drifting away")
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
