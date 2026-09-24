// Command structbuild computes and stores structural embeddings for all posts.
// It reads each post's existing embedding and its neighbors' embeddings from the
// reply graph, averages them, normalizes, and writes the result to embedding_struct.
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
	"akyuu/internal/store"
)

func main() {
	configPath := flag.String("config", "config/config.yaml", "path to the global config file")
	batchSize := flag.Int("batch", 500, "number of posts to process per batch")
	limit := flag.Int("limit", 0, "maximum posts to process (0 = no limit)")
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
		slog.Error("connect database", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := st.Migrate(ctx); err != nil {
		slog.Error("migrate", "err", err)
		os.Exit(1)
	}

	model, err := embedder.NewONNX(cfg.Embeddings.ModelDir, cfg.Embeddings.ModelName, cfg.Embeddings.Dimensions)
	if err != nil {
		slog.Error("load embedder", "err", err)
		os.Exit(1)
	}
	defer model.Close()

	processed := 0
	batch := 0

	for {
		if *limit > 0 && processed >= *limit {
			break
		}

		posts, err := st.GetPostsWithoutStructuralEmbedding(ctx, "default", *batchSize)
		if err != nil {
			slog.Error("fetch posts without structural embedding", "err", err)
			os.Exit(1)
		}

		if len(posts) == 0 {
			slog.Info("no more posts to process")
			break
		}

		batch++
		slog.Info("processing batch", "batch", batch, "size", len(posts))

		// For each post, get neighbors and compute structural embedding
		for _, p := range posts {
			ownVec, neighbors, err := st.NeighborEmbeddings(ctx, p.ID)
			if err != nil {
				slog.Error("get neighbors", "post", p.ID, "err", err)
				continue
			}

			// Combine own embedding with neighbors
			allVecs := append([][]float32{ownVec}, neighbors...)
			structVec := embedder.AverageVectors(allVecs...)

			if err := st.MarkStructuralEmbedding(ctx, p.ID, structVec, "default"); err != nil {
				slog.Error("mark structural embedding", "post", p.ID, "err", err)
				continue
			}
			processed++
		}

		if len(posts) < *batchSize {
			break
		}
	}

	slog.Info("structbuild complete", "processed", processed)
}

func newLogger(level string) *slog.Logger {
	switch level {
	case "debug":
		return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	case "warn":
		return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	case "error":
		return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	default:
		return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
}
