// Command star-sapphire runs the ambitious feature workers (stylometric clustering, anomaly detection).
// Star Sapphire - the strategist of the Three Fairies of Light, detects patterns and anomalies.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"akyuu/internal/ambitious"
	"akyuu/internal/config"
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

	ambCfg := ambitious.CoordinatorConfig{
		PollInterval:       cfg.Ambitious.PollInterval.D(),
		StylometricEnabled: cfg.Ambitious.Stylometric.Enabled,
		AnomalyEnabled:     cfg.Ambitious.Anomaly.Enabled,

		StylometricMinPosts:     cfg.Ambitious.Stylometric.MinPosts,
		StylometricNGramMin:     cfg.Ambitious.Stylometric.NGramMin,
		StylometricNGramMax:     cfg.Ambitious.Stylometric.NGramMax,
		StylometricTopFeatures:  cfg.Ambitious.Stylometric.TopFeatures,
		StylometricSimThreshold: cfg.Ambitious.Stylometric.SimThreshold,

		AnomalyVolumeZThreshold: cfg.Ambitious.Anomaly.VolumeZThreshold,
		AnomalyBurstThreshold:   cfg.Ambitious.Anomaly.BurstThreshold,
		AnomalyBurstWindowMin:   cfg.Ambitious.Anomaly.BurstWindowMin,
		AnomalyLookbackHours:    cfg.Ambitious.Anomaly.LookbackHours,
	}

	coordinator, err := ambitious.NewCoordinator(st, ambCfg, logger)
	if err != nil {
		logger.Error("create star-sapphire coordinator", "err", err)
		os.Exit(1)
	}

	logger.Info("star-sapphire starting - plotting the perfect scheme")
	coordinator.Run(ctx)
	logger.Info("star-sapphire vanishing into the stars")
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
