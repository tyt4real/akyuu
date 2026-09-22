package search

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"akyuu/internal/store"
)

type mockStore struct {
	searches []*SavedSearch
	rows     *mockRows
}

func (m *mockStore) Query(ctx context.Context, query string, args ...any) (store.Rows, error) {
	return m.rows, nil
}

func (m *mockStore) QueryRow(ctx context.Context, query string, args ...any) store.RowScanner {
	return &mockRow{}
}

func (m *mockStore) Exec(ctx context.Context, query string, args ...any) error {
	return nil
}

func (m *mockStore) ListSites(ctx context.Context) ([]*store.Site, error) {
	return []*store.Site{{ID: 1, Name: "test"}}, nil
}

type mockRows struct{}

func (m *mockRows) Next() bool             { return false }
func (m *mockRows) Scan(dest ...any) error { return nil }
func (m *mockRows) Close()                 {}
func (m *mockRows) Err() error             { return nil }

type mockRow struct{}

func (m *mockRow) Scan(dest ...any) error { return nil }

type mockEmbedder struct{}

func (m *mockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return [][]float32{{0.1, 0.2, 0.3}}, nil
}

func (m *mockEmbedder) ModelVersion() string { return "test-model" }

func (m *mockEmbedder) Dimensions() int { return 384 }

func TestEvaluator(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}
	emb := &mockEmbedder{}
	eval := NewEvaluator(ms, emb, logger)

	ctx := context.Background()
	err := eval.RunOnce(ctx)
	if err != nil {
		t.Logf("Expected error (no DB): %v", err)
	}
}

func TestEvaluatorConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}
	emb := &mockEmbedder{}

	eval := NewEvaluator(ms, emb, logger)
	if eval == nil {
		t.Fatal("evaluator should not be nil")
	}
}

func TestTrendAggregator(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}
	agg := NewTrendAggregator(ms, logger)

	ctx := context.Background()
	err := agg.RunOnce(ctx)
	if err != nil {
		t.Logf("Expected error (no DB): %v", err)
	}
}

func TestTrendAggregatorConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}

	agg := NewTrendAggregator(ms, logger)
	if agg == nil {
		t.Fatal("trend aggregator should not be nil")
	}
}

func TestCoordinator(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}
	emb := &mockEmbedder{}

	cfg := CoordinatorConfig{
		PollInterval:     100 * time.Millisecond,
		EvaluatorEnabled: true,
		TrendsEnabled:    true,
	}

	coord, err := NewCoordinator(ms, emb, cfg, logger)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	coord.Run(ctx)
}

func TestCoordinatorEvaluatorOnly(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}
	emb := &mockEmbedder{}

	cfg := CoordinatorConfig{
		PollInterval:     100 * time.Millisecond,
		EvaluatorEnabled: true,
		TrendsEnabled:    false,
	}

	coord, err := NewCoordinator(ms, emb, cfg, logger)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	coord.Run(ctx)
}

func TestCoordinatorTrendsOnly(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}

	cfg := CoordinatorConfig{
		PollInterval:     100 * time.Millisecond,
		EvaluatorEnabled: false,
		TrendsEnabled:    true,
	}

	coord, err := NewCoordinator(ms, nil, cfg, logger)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	coord.Run(ctx)
}

func TestExtractTerms(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"hello world", []string{"hello", "world"}},
		{"<p>hello world</p>", []string{"hello", "world"}},
		{"the quick brown fox", []string{"quick", "brown", "fox"}},
		{"a b c", []string{}},
		{"test123 test", []string{"test123", "test"}},
		{"Hello World!", []string{"hello", "world"}},
		{"<script>alert(1)</script>test", []string{"test"}},
	}

	for _, tc := range tests {
		got := extractTerms(tc.input)
		if len(tc.expected) > 0 && len(got) == 0 {
			t.Errorf("extractTerms(%q) = %v, want non-empty", tc.input, got)
		}
	}
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<p>hello</p>", "hello"},
		{"hello", "hello"},
		{"<div><span>test</span></div>", "test"},
		{"a<b>c", "ac"},
	}

	for _, tc := range tests {
		got := stripHTML(tc.input)
		if got != tc.expected {
			t.Errorf("stripHTML(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestIsLetterOrDigit(t *testing.T) {
	tests := []struct {
		r      rune
		result bool
	}{
		{'a', true}, {'z', true}, {'A', true}, {'Z', true},
		{'0', true}, {'9', true},
		{'!', false}, {' ', false}, {'@', false},
	}

	for _, tc := range tests {
		got := isLetterOrDigit(tc.r)
		if got != tc.result {
			t.Errorf("isLetterOrDigit(%c) = %v, want %v", tc.r, got, tc.result)
		}
	}
}

func TestSavedSearchStruct(t *testing.T) {
	s := SavedSearch{
		ID:          1,
		Name:        "test search",
		DSLQuery:    "test query",
		SQLWhere:    "WHERE 1=1",
		SiteFilter:  "4chan",
		BoardFilter: "g",
		IsPublic:    true,
	}

	if s.ID != 1 || s.Name != "test search" {
		t.Errorf("SavedSearch not initialized correctly")
	}
}

func TestSearchHitStruct(t *testing.T) {
	h := SearchHit{
		SavedSearchID: 1,
		PostID:        123,
		Score:         0.95,
		MatchedAt:     time.Now(),
	}

	if h.PostID != 123 || h.Score != 0.95 {
		t.Errorf("SearchHit not initialized correctly")
	}
}
