package evalbench

import (
	"context"
	"time"
)

// QueryCase represents a single evaluation query with its relevance judgments.
type QueryCase struct {
	QueryID   int64
	QueryText string
	Relevant  map[int64]int // post_id -> grade (0=not relevant, 1=relevant, 2=highly relevant)
}

// RunResult contains the results of running one arm across all query cases.
type RunResult struct {
	ArmName  string
	PerQuery []QueryRunResult
}

// QueryRunResult contains the results for a single query.
type QueryRunResult struct {
	QueryID   int64
	Hits      []Hit
	LatencyMS float64
}

// Run executes an arm against all query cases and returns the results.
func Run(ctx context.Context, arm Arm, cases []QueryCase, k int) RunResult {
	res := RunResult{ArmName: arm.Name}
	for _, c := range cases {
		start := time.Now()
		hits, err := arm.Search(ctx, c.QueryText, k)
		elapsed := time.Since(start).Seconds() * 1000
		if err != nil {
			hits = nil // record as zero-hit rather than aborting the whole run
		}
		res.PerQuery = append(res.PerQuery, QueryRunResult{
			QueryID: c.QueryID, Hits: hits, LatencyMS: elapsed,
		})
	}
	return res
}
