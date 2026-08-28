// Command api runs the semantic-search HTTP API: it loads the config, opens
// the archive store, loads the ONNX embedding model, and serves GET /health
// and POST /search. It is a separate binary so the archiver and the search
// front end scale independently.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"akyuu/internal/apiserver"
	"akyuu/internal/config"
	"akyuu/internal/embedder"
	"akyuu/internal/store"
)

func main() {
	configPath := flag.String("config", "config/config.yaml", "path to the global config file")
	addr := flag.String("listen", ":8080", "HTTP listen address")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	logger := newLogger(cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if !cfg.Embeddings.Enabled {
		logger.Error("embeddings are disabled; semantic search is unavailable")
		os.Exit(1)
	}

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

	model, err := embedder.NewONNX(cfg.Embeddings.ModelDir, cfg.Embeddings.ModelName, cfg.Embeddings.Dimensions)
	if err != nil {
		logger.Error("load embedding model", "err", err)
		os.Exit(1)
	}
	defer model.Close()

	srv := &http.Server{
		Addr:              *addr,
		Handler:           apiserver.New(model, st, st, st, logger).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()

	logger.Info("search api listening", "addr", *addr, "model", cfg.Embeddings.ModelName)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("http server", "err", err)
		os.Exit(1)
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
