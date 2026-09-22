package lineage

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"akyuu/internal/store"
)

type mockStore struct{}

func (m *mockStore) Query(ctx context.Context, query string, args ...any) (store.Rows, error) {
	return &mockRows{}, nil
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

func TestCrosspostWorker(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}
	worker := NewCrosspostWorker(ms, logger, 8)

	ctx := context.Background()
	err := worker.RunOnce(ctx)
	if err != nil {
		t.Logf("Expected error (no DB): %v", err)
	}
}

func TestCrosspostWorkerHammingDistance(t *testing.T) {
	tests := []struct {
		a, b string
		exp  int
	}{
		{"0000", "0000", 0},
		{"ffff", "0000", 16},
		{"abcd", "abcd", 0},
		{"abcd", "abce", 2}, // d(13) ^ e(14) = 3 (0011) = 2 bits
		{"", "", 0},
		{"0", "1", 1},
		{"f", "0", 4}, // 15 ^ 0 = 15 (1111) = 4 bits
	}

	for _, tc := range tests {
		got := hammingDistance(tc.a, tc.b)
		if got != tc.exp {
			t.Errorf("hammingDistance(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.exp)
		}
	}
}

func TestDecodeHex(t *testing.T) {
	tests := []struct {
		c   byte
		exp byte
	}{
		{'0', 0}, {'9', 9},
		{'a', 10}, {'f', 15},
		{'A', 10}, {'F', 15},
		{'x', 0},
	}

	for _, tc := range tests {
		got := decodeHex(tc.c)
		if got != tc.exp {
			t.Errorf("decodeHex(%c) = %d, want %d", tc.c, got, tc.exp)
		}
	}
}

func TestLineageWorker(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}
	worker := NewLineageWorker(ms, logger, "phash", 8, 100)

	ctx := context.Background()
	err := worker.RunOnce(ctx)
	if err != nil {
		t.Logf("Expected error (no DB): %v", err)
	}
}

func TestLineageWorkerConfigDefaults(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}

	// Test default algo
	worker := NewLineageWorker(ms, logger, "", 0, 0)
	if worker == nil {
		t.Fatal("worker should not be nil")
	}
}

func TestContinuityWorker(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}
	worker := NewContinuityWorker(ms, logger, 48, 0.8, 0.75)

	ctx := context.Background()
	err := worker.RunOnce(ctx)
	if err != nil {
		t.Logf("Expected error (no DB): %v", err)
	}
}

func TestContinuityWorkerConfigDefaults(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}

	worker := NewContinuityWorker(ms, logger, 0, 0, 0)
	if worker == nil {
		t.Fatal("worker should not be nil")
	}
}

func TestReplyGraphWorker(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}
	worker := NewReplyGraphWorker(ms, logger)

	ctx := context.Background()
	err := worker.RunOnce(ctx)
	if err != nil {
		t.Logf("Expected error (no DB): %v", err)
	}
}

func TestTitleSimilarity(t *testing.T) {
	tests := []struct {
		a, b string
		min  float64
	}{
		{"General", "General #2", 0.5},
		{"General #1", "General #2", 0.8},
		{"Random", "Random", 1.0},
		{"", "test", 0.0},
		{"hello world", "hello world", 1.0},
		{"Test", "test", 1.0},
		{"long title here", "long title there", 0.5},
	}

	for _, tc := range tests {
		got := titleSimilarity(tc.a, tc.b)
		if got < tc.min {
			t.Errorf("titleSimilarity(%q, %q) = %f, want >= %f", tc.a, tc.b, got, tc.min)
		}
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		exp  int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"a", "a", 0},
		{"ab", "ab", 0},
		{"ab", "ac", 1},
		{"kitten", "sitting", 3},
		{"general", "general #2", 3},
	}

	for _, tc := range tests {
		got := levenshtein(tc.a, tc.b)
		if got != tc.exp {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.exp)
		}
	}
}

func TestTextSimilarity(t *testing.T) {
	tests := []struct {
		a, b string
		min  float64
	}{
		{"hello world", "hello world", 1.0},
		{"hello", "world", 0.0},
		{"the quick brown", "quick brown fox", 0.5},
		{"", "test", 0.0},
	}

	for _, tc := range tests {
		got := textSimilarity(tc.a, tc.b)
		if got < tc.min {
			t.Errorf("textSimilarity(%q, %q) = %f, want >= %f", tc.a, tc.b, got, tc.min)
		}
	}
}

func TestCosineSimilarity(t *testing.T) {
	// Identical vectors
	a := []float32{1.0, 2.0, 3.0}
	b := []float32{1.0, 2.0, 3.0}
	sim := cosineSimilarity(a, b)
	if sim < 0.99 {
		t.Errorf("identical vectors similarity = %f, want ~1.0", sim)
	}

	// Orthogonal vectors
	a = []float32{1.0, 0.0}
	b = []float32{0.0, 1.0}
	sim = cosineSimilarity(a, b)
	if sim > 0.01 {
		t.Errorf("orthogonal vectors similarity = %f, want ~0.0", sim)
	}

	// Different lengths
	a = []float32{1.0}
	b = []float32{1.0, 2.0}
	sim = cosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("different lengths similarity = %f, want 0", sim)
	}

	// Empty vectors
	sim = cosineSimilarity([]float32{}, []float32{1.0})
	if sim != 0 {
		t.Errorf("empty vs non-empty similarity = %f, want 0", sim)
	}
}

func TestCoordinator(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}

	cfg := CoordinatorConfig{
		PollInterval:      100 * time.Millisecond,
		CrosspostEnabled:  true,
		LineageEnabled:    true,
		ContinuityEnabled: true,
		ReplyGraphEnabled: true,

		CrosspostPHASHThreshold: 8,
		LineageAlgo:             "phash",
		LineageThreshold:        8,
		LineageBatchSize:        100,

		ContinuityTimeGapHours:   48,
		ContinuityTitleThreshold: 0.8,
		ContinuityOpSimThreshold: 0.75,
	}

	coord, err := NewCoordinator(ms, cfg, logger)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	coord.Run(ctx)
}
