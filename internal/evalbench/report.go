package evalbench

import (
	"encoding/csv"
	"io"
	"strconv"
)

// WriteRawCSV writes one row per (arm, query): the finest-grained record
// of a harness run. This is the file to re-derive anything from later —
// the summary file is a convenience view, this is the source of truth.
func WriteRawCSV(w io.Writer, results []RunResult, cases []QueryCase, k int) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	// Write header
	if err := cw.Write([]string{"arm", "query_id", "recall_at_k", "ndcg_at_k", "latency_ms"}); err != nil {
		return err
	}

	// Build relevance map for quick lookup
	relevanceByID := make(map[int64]map[int64]int, len(cases))
	for _, c := range cases {
		relevanceByID[c.QueryID] = c.Relevant
	}

	for _, r := range results {
		rel := relevanceByID[r.PerQuery[0].QueryID]
		for _, qr := range r.PerQuery {
			recall := RecallAtK(qr.Hits, rel, k)
			ndcg := NDCGAtK(qr.Hits, rel, k)
			recallStr := strconv.FormatFloat(recall, 'f', 4, 64)
			ndcgStr := strconv.FormatFloat(ndcg, 'f', 4, 64)
			latencyStr := strconv.FormatFloat(qr.LatencyMS, 'f', 2, 64)
			if err := cw.Write([]string{
				r.ArmName,
				strconv.FormatInt(qr.QueryID, 10),
				recallStr,
				ndcgStr,
				latencyStr,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// WriteSummaryCSV writes one row per arm: aggregate stats across all
// queries. This is the file that goes straight into a table or a chart —
// no pivoting required after opening it.
func WriteSummaryCSV(w io.Writer, results []RunResult, cases []QueryCase, k int) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	// Write header
	if err := cw.Write([]string{"arm", "n_queries", "mean_recall_at_k", "mean_ndcg_at_k", "p50_latency_ms", "p95_latency_ms", "p99_latency_ms"}); err != nil {
		return err
	}

	// Build relevance map for quick lookup
	relevanceByID := make(map[int64]map[int64]int, len(cases))
	for _, c := range cases {
		relevanceByID[c.QueryID] = c.Relevant
	}

	for _, r := range results {
		var totalRecall, totalNDCG float64
		var latencies []float64
		for _, qr := range r.PerQuery {
			rel := relevanceByID[qr.QueryID]
			recall := RecallAtK(qr.Hits, rel, k)
			ndcg := NDCGAtK(qr.Hits, rel, k)
			totalRecall += recall
			totalNDCG += ndcg
			latencies = append(latencies, qr.LatencyMS)
		}

		n := float64(len(r.PerQuery))
		if n == 0 {
			n = 1 // avoid division by zero
		}

		meanRecall := totalRecall / n
		meanNDCG := totalNDCG / n

		p50 := Percentile(latencies, 50)
		p95 := Percentile(latencies, 95)
		p99 := Percentile(latencies, 99)

		if err := cw.Write([]string{
			r.ArmName,
			strconv.FormatInt(int64(len(r.PerQuery)), 10),
			strconv.FormatFloat(meanRecall, 'f', 4, 64),
			strconv.FormatFloat(meanNDCG, 'f', 4, 64),
			strconv.FormatFloat(p50, 'f', 2, 64),
			strconv.FormatFloat(p95, 'f', 2, 64),
			strconv.FormatFloat(p99, 'f', 2, 64),
		}); err != nil {
			return err
		}
	}
	return nil
}
