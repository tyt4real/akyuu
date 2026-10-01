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
	"strconv"
	"strings"
	"syscall"

	"akyuu/internal/config"
	"akyuu/internal/embedder"
	"akyuu/internal/evalbench"
	"akyuu/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
)

func LoadQueryCases(ctx context.Context, pool *pgxpool.Pool) ([]evalbench.QueryCase, error) {
	rows, err := pool.Query(ctx, `SELECT id, query_text FROM eval_queries ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("load eval queries: %w", err)
	}
	defer rows.Close()

	var cases []evalbench.QueryCase
	for rows.Next() {
		var qc evalbench.QueryCase
		if err := rows.Scan(&qc.QueryID, &qc.QueryText); err != nil {
			return nil, err
		}
		qc.Relevant = make(map[int64]int)
		cases = append(cases, qc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	relRows, err := pool.Query(ctx, `SELECT query_id, post_id, grade FROM eval_relevance`)
	if err != nil {
		return nil, fmt.Errorf("load eval relevance: %w", err)
	}
	defer relRows.Close()

	relByQuery := make(map[int64]map[int64]int)
	for relRows.Next() {
		var queryID, postID int64
		var grade int
		if err := relRows.Scan(&queryID, &postID, &grade); err != nil {
			return nil, err
		}
		if relByQuery[queryID] == nil {
			relByQuery[queryID] = make(map[int64]int)
		}
		relByQuery[queryID][postID] = grade
	}
	if err := relRows.Err(); err != nil {
		return nil, err
	}

	for i := range cases {
		if rel, ok := relByQuery[cases[i].QueryID]; ok {
			cases[i].Relevant = rel
		}
	}
	return cases, nil
}

func main() {
	configPath := flag.String("config", "config/config.yaml", "path to the global config file")
	_ = flag.String("queries", "", "path to CSV file with eval queries (query_id,query_text,source,relevant_post_ids) - deprecated, now loads from database")
	k := flag.Int("k", 20, "number of results per query")
	outputPath := flag.String("output", "", "path to write CSV results (stdout if empty)")
	rawCSVPath := flag.String("raw-csv", "", "path to write per-query raw results CSV")
	summaryCSVPath := flag.String("summary-csv", "", "path to write per-arm summary CSV")
	evalOnly := flag.Bool("eval-only", false, "run evalbench only against existing data, do not start scrapers")
	useModal := flag.Bool("modal", false, "run evalbench on Modal cloud (requires MODAL_TOKEN_ID/MODAL_TOKEN_SECRET env vars)")
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

	// Load query cases from database (eval_queries + eval_relevance tables)
	queryCases, err := LoadQueryCases(ctx, st.Pool())
	if err != nil {
		logger.Error("load query cases", "err", err)
		os.Exit(1)
	}
	if len(queryCases) == 0 {
		logger.Error("evalbench: loaded 0 eval queries — check eval_queries table")
		os.Exit(1)
	}
	withJudgments := 0
	for _, qc := range queryCases {
		if len(qc.Relevant) > 0 {
			withJudgments++
		}
	}
	logger.Info("evalbench: loaded queries", "total", len(queryCases), "with_judgments", withJudgments)

	// If modal flag is set, run evalbench on Modal cloud
	if *useModal {
		if !cfg.Modal.Enabled {
			logger.Error("modal evalbench requested but modal not enabled in config")
			os.Exit(1)
		}
		runModalEvalbench(ctx, cfg, logger, queryCases, *k, modelVersion, *rawCSVPath, *summaryCSVPath, *outputPath)
		return
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

	// If eval-only, just run evalbench and exit
	if *evalOnly {
		runEvalOnly(ctx, st, emb, modelVersion, logger, *k, *rawCSVPath, *summaryCSVPath, *outputPath, queryCases)
		return
	}

	// Otherwise start the archiver (existing behavior)
	// Run evaluation
	var allResults []evalbench.RunResult
	for _, arm := range arms {
		logger.Info("running arm", "name", arm.Name)
		result := evalbench.Run(context.Background(), arm, queryCases, *k)
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

	// Write raw CSV if requested
	if *rawCSVPath != "" {
		f, err := os.Create(*rawCSVPath)
		if err != nil {
			logger.Error("create raw csv file", "err", err)
			os.Exit(1)
		}
		if err := evalbench.WriteRawCSV(f, allResults, queryCases, *k); err != nil {
			logger.Error("write raw csv", "err", err)
			os.Exit(1)
		}
		f.Close()
		logger.Info("wrote raw CSV", "path", *rawCSVPath)
	}

	// Write summary CSV if requested
	if *summaryCSVPath != "" {
		f, err := os.Create(*summaryCSVPath)
		if err != nil {
			logger.Error("create summary csv file", "err", err)
			os.Exit(1)
		}
		if err := evalbench.WriteSummaryCSV(f, allResults, queryCases, *k); err != nil {
			logger.Error("write summary csv", "err", err)
			os.Exit(1)
		}
		f.Close()
		logger.Info("wrote summary CSV", "path", *summaryCSVPath)
	}

	// Legacy output (for backward compatibility)
	if *outputPath != "" {
		f, err := os.Create(*outputPath)
		if err != nil {
			logger.Error("create output file", "err", err)
			os.Exit(1)
		}
		defer f.Close()
		w := csv.NewWriter(f)
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
}

// loadQueryCases loads query cases from a CSV file.
// Expected format: query_id,query_text,relevant_post_ids (comma-separated)
// If file doesn't exist or path is empty, returns default test queries.

// runEvalOnly runs the evalbench against existing data without starting scrapers.
func runEvalOnly(ctx context.Context, st *store.Store, emb embedder.Embedder, modelVersion string, logger *slog.Logger, k int, rawCSVPath, summaryCSVPath, outputPath string, queryCases []evalbench.QueryCase) {
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
		result := evalbench.Run(context.Background(), arm, queryCases, k)
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

	// Write raw CSV if requested
	if rawCSVPath != "" {
		f, err := os.Create(rawCSVPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create raw csv file: %v\n", err)
			os.Exit(1)
		}
		if err := evalbench.WriteRawCSV(f, allResults, queryCases, k); err != nil {
			fmt.Fprintf(os.Stderr, "write raw csv: %v\n", err)
			os.Exit(1)
		}
		f.Close()
		fmt.Printf("wrote raw CSV to %s\n", rawCSVPath)
	}

	// Write summary CSV if requested
	if summaryCSVPath != "" {
		f, err := os.Create(summaryCSVPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create summary csv file: %v\n", err)
			os.Exit(1)
		}
		if err := evalbench.WriteSummaryCSV(f, allResults, queryCases, k); err != nil {
			fmt.Fprintf(os.Stderr, "write summary csv: %v\n", err)
			os.Exit(1)
		}
		f.Close()
		fmt.Printf("wrote summary CSV to %s\n", summaryCSVPath)
	}

	// Legacy output (for backward compatibility)
	if outputPath != "" {
		f, err := os.Create(outputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create output file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		w := csv.NewWriter(f)
		defer w.Flush()

		if err := w.Write([]string{"arm", "query_id", "recall_at_k", "ndcg_at_k", "latency_ms"}); err != nil {
			fmt.Fprintf(os.Stderr, "write csv header: %v\n", err)
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
					fmt.Fprintf(os.Stderr, "write csv row: %v\n", err)
					os.Exit(1)
				}
			}
		}
	}
}

// loadQueryCases loads query cases from a CSV file.
// Expected format: query_id,query_text,relevant_post_ids (comma-separated)
// If file doesn't exist or path is empty, returns default test queries.

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

// loadQueryCases loads query cases from a CSV file.
// Expected format: query_id,query_text,relevant_post_ids (comma-separated)
// If file doesn't exist or path is empty, returns default test queries.
func loadQueryCases(path string) []evalbench.QueryCase {
	if path == "" {
		return []evalbench.QueryCase{
			{QueryID: 1, QueryText: "linux kernel", Relevant: map[int64]int{}},
			{QueryID: 2, QueryText: "golang programming", Relevant: map[int64]int{}},
		}
	}

	f, err := os.Open(path)
	if err != nil {
		panic(fmt.Errorf("open queries file: %w", err))
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		panic(fmt.Errorf("read CSV: %w", err))
	}

	if len(records) < 2 {
		panic(fmt.Errorf("CSV must have header and at least one data row"))
	}

	// Expect header: query_id,query_text,relevant_post_ids
	header := records[0]
	var qidIdx, textIdx, relIdx int
	for i, h := range header {
		switch h {
		case "query_id":
			qidIdx = i
		case "query_text":
			textIdx = i
		case "relevant_post_ids":
			relIdx = i
		}
	}

	var cases []evalbench.QueryCase
	for _, row := range records[1:] {
		if len(row) <= max(qidIdx, textIdx, relIdx) {
			continue
		}
		qid, _ := strconv.ParseInt(row[qidIdx], 10, 64)
		text := row[textIdx]
		rel := make(map[int64]int)
		if row[relIdx] != "" {
			for _, idStr := range strings.Split(row[relIdx], ",") {
				if id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64); err == nil {
					rel[id] = 1
				}
			}
		}
		cases = append(cases, evalbench.QueryCase{
			QueryID:   qid,
			QueryText: text,
			Relevant:  rel,
		})
	}
	return cases
}

func runModalEvalbench(ctx context.Context, cfg *config.Config, logger *slog.Logger, queryCases []evalbench.QueryCase, k int, modelVersion, rawCSVPath, summaryCSVPath, outputPath string) {
	modalEvalbench, err := evalbench.NewModalEvalbench(cfg.Modal, logger)
	if err != nil {
		logger.Error("create modal evalbench", "err", err)
		os.Exit(1)
	}
	defer modalEvalbench.Close()

	rawLocal, summaryLocal, err := modalEvalbench.RunEvalbench(ctx, queryCases, k, modelVersion)
	if err != nil {
		logger.Error("modal evalbench failed", "err", err)
		os.Exit(1)
	}

	if rawCSVPath != "" {
		if err := evalbench.WriteRawCSVFromModal(rawLocal, rawCSVPath, queryCases, k); err != nil {
			logger.Error("write raw csv from modal", "err", err)
			os.Exit(1)
		}
		logger.Info("wrote raw CSV", "path", rawCSVPath)
	}

	if summaryCSVPath != "" {
		if err := evalbench.WriteSummaryCSVFromModal(summaryLocal, summaryCSVPath, queryCases, k); err != nil {
			logger.Error("write summary csv from modal", "err", err)
			os.Exit(1)
		}
		logger.Info("wrote summary CSV", "path", summaryCSVPath)
	}

	if outputPath != "" {
		if err := evalbench.WriteRawCSVFromModal(rawLocal, outputPath, queryCases, k); err != nil {
			logger.Error("write output csv from modal", "err", err)
			os.Exit(1)
		}
		logger.Info("wrote output CSV", "path", outputPath)
	}

	logger.Info("modal evalbench completed successfully")
}
