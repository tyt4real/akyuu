// Command sunny-milk runs the lineage processing workers (crosspost, clustering, continuity, reply graph).
// Sunny Milk - leader of the Three Fairies of Light, connects threads and tracks meme lineage.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"akyuu/internal/config"
	"akyuu/internal/lineage"
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

	lineageCfg := lineage.CoordinatorConfig{
		PollInterval:      cfg.Lineage.PollInterval.D(),
		CrosspostEnabled:  cfg.Lineage.Crosspost.Enabled,
		LineageEnabled:    cfg.Lineage.Clustering.Enabled,
		ContinuityEnabled: cfg.Lineage.Continuity.Enabled,
		ReplyGraphEnabled: cfg.Lineage.ReplyGraph.Enabled,

		CrosspostPHASHThreshold: cfg.Lineage.Crosspost.PHASHThreshold,
		LineageAlgo:             cfg.Lineage.Clustering.Algo,
		LineageThreshold:        cfg.Lineage.Clustering.Threshold,
		LineageBatchSize:        cfg.Lineage.Clustering.BatchSize,

		ContinuityTimeGapHours:   cfg.Lineage.Continuity.TimeGapHours,
		ContinuityTitleThreshold: cfg.Lineage.Continuity.TitleThreshold,
		ContinuityOpSimThreshold: cfg.Lineage.Continuity.OpSimThreshold,
	}

	coordinator, err := lineage.NewCoordinator(st, lineageCfg, logger)
	if err != nil {
		logger.Error("create sunny-milk coordinator", "err", err)
		os.Exit(1)
	}

	logger.Info("sunny-milk starting - weaving the threads of fate")
	coordinator.Run(ctx)
	logger.Info("sunny-milk fading into the light")
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
