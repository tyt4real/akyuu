// Command cirno runs the multimodal processing workers (OCR, CLIP, Whisper).
// Cirno - the strongest fairy, handles images, video, and audio with icy precision.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"akyuu/internal/config"
	"akyuu/internal/multimodal"
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

	mmCfg := multimodal.CoordinatorConfig{
		StorageRoot:  cfg.Storage.Dir,
		PollInterval: cfg.Multimodal.PollInterval.D(),
		OCR: multimodal.OCRConfig{
			Enabled:      cfg.Multimodal.OCR.Enabled,
			TesseractCmd: cfg.Multimodal.OCR.TesseractCmd,
			Languages:    cfg.Multimodal.OCR.Languages,
		},
		CLIP: multimodal.CLIPConfig{
			Enabled:      cfg.Multimodal.CLIP.Enabled,
			ModelDir:     cfg.Multimodal.CLIP.ModelDir,
			ModelVersion: cfg.Multimodal.CLIP.ModelVersion,
			Dimensions:   cfg.Multimodal.CLIP.Dimensions,
		},
		Whisper: multimodal.WhisperConfig{
			Enabled:    cfg.Multimodal.Whisper.Enabled,
			WhisperCmd: cfg.Multimodal.Whisper.WhisperCmd,
			ModelPath:  cfg.Multimodal.Whisper.ModelPath,
			ModelName:  cfg.Multimodal.Whisper.ModelName,
		},
	}

	coordinator, err := multimodal.NewCoordinator(st, mmCfg, logger)
	if err != nil {
		logger.Error("create cirno coordinator", "err", err)
		os.Exit(1)
	}

	logger.Info("cirno starting - strongest fairy on the job")
	coordinator.Run(ctx)
	logger.Info("cirno melted away")
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
