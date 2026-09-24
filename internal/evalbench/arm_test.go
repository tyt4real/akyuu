package evalbench

import (
	"context"
	"testing"
)

func TestArmSearch(t *testing.T) {
	arm := Arm{
		Name: "test_arm",
		Search: func(ctx context.Context, query string, k int) ([]Hit, error) {
			return []Hit{
				{PostID: 1, Score: 0.9},
				{PostID: 2, Score: 0.8},
				{PostID: 3, Score: 0.7},
			}, nil
		},
	}

	ctx := context.Background()
	hits, err := arm.Search(ctx, "test query", 3)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(hits) != 3 {
		t.Errorf("expected 3 hits, got %d", len(hits))
	}
	if hits[0].PostID != 1 || hits[0].Score != 0.9 {
		t.Errorf("unexpected first hit: %+v", hits[0])
	}
}

func TestArmSearchError(t *testing.T) {
	arm := Arm{
		Name: "error_arm",
		Search: func(ctx context.Context, query string, k int) ([]Hit, error) {
			return nil, ErrSearchFailed
		},
	}

	ctx := context.Background()
	_, err := arm.Search(ctx, "test", 10)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if err != ErrSearchFailed {
		t.Errorf("expected ErrSearchFailed, got %v", err)
	}
}

func TestArmEmptyResults(t *testing.T) {
	arm := Arm{
		Name: "empty_arm",
		Search: func(ctx context.Context, query string, k int) ([]Hit, error) {
			return []Hit{}, nil
		},
	}

	ctx := context.Background()
	hits, err := arm.Search(ctx, "test", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("expected empty results, got %d hits", len(hits))
	}
}
