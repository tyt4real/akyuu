// Command evalbench-runner runs the retrieval evaluation harness.
// It loads the config, connects to the database, loads the embedder,
// and runs the evaluation arms against a set of query cases.
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"akyuu/internal/config"
	"akyuu/internal/embedder"
	"akyuu/internal/evalbench"
	"akyuu/internal/store"
)

func main() {
	configPath := flag.String("config", "config/config.yaml", "path to the global config file")
	_ = flag.String("queries", "", "path to CSV file with eval queries (query_id,query_text,source,relevant_post_ids)")
	k := flag.Int("k", 20, "number of results per query")
	outputPath := flag.String("output", "", "path to write CSV results (stdout if empty)")
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
	var modelVersion string
	if cfg.Embeddings.Enabled {
		model, err := embedder.NewONNX(cfg.Embeddings.ModelDir, cfg.Embeddings.ModelName, cfg.Embeddings.Dimensions)
		if err != nil {
			logger.Error("load embedder", "err", err)
			os.Exit(1)
		}
		defer model.Close()
		emb = model
		modelVersion = cfg.Embeddings.ModelName
	} else {
		emb = embedder.NewFake()
		modelVersion = "fake"
	}

	// Build arms
	arms := []evalbench.Arm{
		evalbench.BaselineSearch(evalbench.BaselineConfig{
			Store:        st,
			Embedder:     emb,
			ModelVersion: modelVersion,
		}),
		evalbench.HalfvecSearch(evalbench.HalfvecConfig{
			Store:        st,
			Embedder:     emb,
			ModelVersion: modelVersion,
		}),
		evalbench.DotProductSearch(evalbench.DotProductConfig{
			Store:        st,
			Embedder:     emb,
			ModelVersion: modelVersion,
		}),
	}

	// Add ef_search arms with different ef values
	efValues := []int{40, 100, 200, 400}
	for _, ef := range efValues {
		arms = append(arms, evalbench.EfSearchSearch(evalbench.EfSearchConfig{
			Store:        st,
			Embedder:     emb,
			ModelVersion: modelVersion,
			EfSearch:     ef,
		}))
	}

	// Add combined arms
	combinedCfg := evalbench.CombinedConfig{
		Store:        st,
		Embedder:     emb,
		ModelVersion: modelVersion,
	}

	arms = append(arms,
		evalbench.ContextAugHalfvec(combinedCfg),
		evalbench.ContextAugDotProduct(combinedCfg),
		evalbench.HybridHalfvec(combinedCfg),
		evalbench.HybridDotProduct(combinedCfg),
		evalbench.ContextAugHybrid(combinedCfg),
		evalbench.DotProductHalfvec(combinedCfg),
		evalbench.StructSearch(combinedCfg),
	)

	// Run evaluation
	var allResults []evalbench.RunResult
	for _, arm := range arms {
		logger.Info("running arm", "name", arm.Name)
		result := evalbench.Run(context.Background(), arm, []evalbench.QueryCase{
			{QueryID: 1, QueryText: "linux kernel", Relevant: map[int64]int{}},
			{QueryID: 2, QueryText: "golang programming", Relevant: map[int64]int{}},
		}, *k)
		allResults = append(allResults, result)
	}

	// Print summary
	for _, r := range allResults {
		var totalLatency float64
		for _, qr := range r.PerQuery {
			totalLatency += qr.LatencyMS
		}
		avgLatency := totalLatency / float64(len(r.PerQuery))
		fmt.Printf("Arm: %s | Queries: %d | Avg Latency: %.2fms\n",
			r.ArmName, len(r.PerQuery), avgLatency)
	}

	// Write CSV output
	var w *csv.Writer
	if *outputPath == "" {
		w = csv.NewWriter(os.Stdout)
	} else {
		f, err := os.Create(*outputPath)
		if err != nil {
			logger.Error("create output file", "err", err)
			os.Exit(1)
		}
		defer f.Close()
		w = csv.NewWriter(f)
	}
	defer w.Flush()

	if err := w.Write([]string{"arm", "query_id", "recall_at_k", "ndcg_at_k", "latency_ms"}); err != nil {
		logger.Error("write csv header", "err", err)
		os.Exit(1)
	}
	defer w.Flush()

	for _, r := range allResults {
		for _, qr := range r.PerQuery {
			if err := w.Write([]string{
				r.ArmName,
				fmt.Sprintf("%d", qr.QueryID),
				"0", // recall_at_k (no relevance data)
				"0", // ndcg_at_k (no relevance data)
				fmt.Sprintf("%.2f", qr.LatencyMS),
			}); err != nil {
				logger.Error("write csv row", "err", err)
				os.Exit(1)
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
