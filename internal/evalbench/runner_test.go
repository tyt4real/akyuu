package evalbench

import (
	"context"
	"testing"
	"time"
)

func TestRun(t *testing.T) {
	arm := Arm{
		Name: "test_arm",
		Search: func(ctx context.Context, query string, k int) ([]Hit, error) {
			// Simulate some latency
			time.Sleep(1 * time.Millisecond)
			return []Hit{
				{PostID: 1, Score: 0.9},
				{PostID: 2, Score: 0.8},
			}, nil
		},
	}

	cases := []QueryCase{
		{QueryID: 1, QueryText: "test query 1", Relevant: map[int64]int{1: 1, 2: 1}},
		{QueryID: 2, QueryText: "test query 2", Relevant: map[int64]int{1: 1, 3: 1}},
	}

	ctx := context.Background()
	result := Run(ctx, arm, cases, 10)

	if result.ArmName != "test_arm" {
		t.Errorf("expected arm name 'test_arm', got '%s'", result.ArmName)
	}
	if len(result.PerQuery) != 2 {
		t.Errorf("expected 2 query results, got %d", len(result.PerQuery))
	}
	for _, qr := range result.PerQuery {
		if qr.LatencyMS <= 0 {
			t.Errorf("expected positive latency, got %f", qr.LatencyMS)
		}
		if len(qr.Hits) != 2 {
			t.Errorf("expected 2 hits, got %d", len(qr.Hits))
		}
	}
}

func TestRunWithError(t *testing.T) {
	arm := Arm{
		Name: "error_arm",
		Search: func(ctx context.Context, query string, k int) ([]Hit, error) {
			return nil, ErrSearchFailed
		},
	}

	cases := []QueryCase{
		{QueryID: 1, QueryText: "test", Relevant: map[int64]int{1: 1}},
	}

	ctx := context.Background()
	result := Run(ctx, arm, cases, 10)

	if result.ArmName != "error_arm" {
		t.Errorf("expected arm name 'error_arm', got '%s'", result.ArmName)
	}
	if len(result.PerQuery) != 1 {
		t.Errorf("expected 1 query result, got %d", len(result.PerQuery))
	}
	// Error should result in nil hits, not aborting the run
	if result.PerQuery[0].Hits != nil {
		t.Errorf("expected nil hits on error, got %v", result.PerQuery[0].Hits)
	}
}

func TestRunEmptyCases(t *testing.T) {
	arm := Arm{
		Name: "test_arm",
		Search: func(ctx context.Context, query string, k int) ([]Hit, error) {
			return []Hit{{PostID: 1, Score: 0.9}}, nil
		},
	}

	ctx := context.Background()
	result := Run(ctx, arm, []QueryCase{}, 10)

	if result.ArmName != "test_arm" {
		t.Errorf("expected arm name 'test_arm', got '%s'", result.ArmName)
	}
	if len(result.PerQuery) != 0 {
		t.Errorf("expected 0 query results, got %d", len(result.PerQuery))
	}
}

func TestRunRespectsK(t *testing.T) {
	arm := Arm{
		Name: "test_arm",
		Search: func(ctx context.Context, query string, k int) ([]Hit, error) {
			// Return k hits
			hits := make([]Hit, k)
			for i := 0; i < k; i++ {
				hits[i] = Hit{PostID: int64(i + 1), Score: 0.9 - float32(i)*0.1}
			}
			return hits, nil
		},
	}

	cases := []QueryCase{
		{QueryID: 1, QueryText: "test", Relevant: map[int64]int{1: 1, 2: 1, 3: 1}},
	}

	ctx := context.Background()
	result := Run(ctx, arm, cases, 2)

	if len(result.PerQuery) != 1 {
		t.Fatalf("expected 1 query result")
	}
	if len(result.PerQuery[0].Hits) != 2 {
		t.Errorf("expected 2 hits (k=2), got %d", len(result.PerQuery[0].Hits))
	}
}
