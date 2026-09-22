package ambitious

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"akyuu/internal/store"
)

type mockStore struct {
	rows *mockRows
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

func TestStylometricWorker(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}
	worker := NewStylometricWorker(ms, logger, 5, 3, 5, 200, 0.7)

	ctx := context.Background()
	err := worker.RunOnce(ctx)
	if err != nil {
		t.Logf("Expected error (no DB): %v", err)
	}
}

func TestStylometricWorkerConfigDefaults(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}

	worker := NewStylometricWorker(ms, logger, 0, 0, 0, 0, 0)
	if worker == nil {
		t.Fatal("worker should not be nil")
	}
}

func TestExtractNGrams(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}
	worker := NewStylometricWorker(ms, logger, 5, 3, 5, 200, 0.7)

	tests := []struct {
		input    string
		minCount int
	}{
		{"hello world hello", 2},
		{"test test test test", 1},
		{"<p>html content</p>", 2},
		{"", 0},
		{"aaaa", 1},   // single n-gram "aaa" and "aaa" -> 1 unique
		{"abcabc", 2}, // "abc", "bca", "cab", "abc"
	}

	for _, tc := range tests {
		ngrams := worker.extractNGrams(tc.input)
		if len(ngrams) < tc.minCount && tc.input != "" {
			t.Errorf("extractNGrams(%q) = %v, want at least %d", tc.input, ngrams, tc.minCount)
		}
	}
}

func TestIsValidNGram(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}
	worker := NewStylometricWorker(ms, logger, 5, 3, 5, 200, 0.7)

	tests := []struct {
		input    string
		expected bool
	}{
		{"abc", true},
		{"123", true},
		{"a1b", true},
		{"   ", false},
		{"!!!", false},
		{"a!b", true}, // has alnum
	}

	for _, tc := range tests {
		got := worker.isValidNGram(tc.input)
		if got != tc.expected {
			t.Errorf("isValidNGram(%q) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

func TestCosineSimilarity(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}
	worker := NewStylometricWorker(ms, logger, 5, 3, 5, 200, 0.7)

	// Identical vectors
	a := map[string]float64{"test": 1.0, "hello": 0.5}
	b := map[string]float64{"test": 1.0, "hello": 0.5}
	sim := worker.cosineSimilarity(a, b)
	if sim < 0.99 {
		t.Errorf("identical vectors similarity = %f, want ~1.0", sim)
	}

	// Orthogonal vectors
	a = map[string]float64{"test": 1.0}
	b = map[string]float64{"hello": 1.0}
	sim = worker.cosineSimilarity(a, b)
	if sim > 0.01 {
		t.Errorf("orthogonal vectors similarity = %f, want ~0.0", sim)
	}

	// Empty vectors
	sim = worker.cosineSimilarity(map[string]float64{}, map[string]float64{"test": 1.0})
	if sim != 0 {
		t.Errorf("empty vs non-empty similarity = %f, want 0", sim)
	}

	// Partial overlap
	a = map[string]float64{"a": 1.0, "b": 1.0}
	b = map[string]float64{"b": 1.0, "c": 1.0}
	sim = worker.cosineSimilarity(a, b)
	if sim <= 0 || sim >= 1 {
		t.Errorf("partial overlap similarity = %f, want between 0 and 1", sim)
	}
}

func TestAnomalyWorker(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}
	worker := NewAnomalyWorker(ms, logger, 3.0, 5.0, 10, 168)

	ctx := context.Background()
	err := worker.RunOnce(ctx)
	if err != nil {
		t.Logf("Expected error (no DB): %v", err)
	}
}

func TestAnomalyWorkerConfigDefaults(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}

	worker := NewAnomalyWorker(ms, logger, 0, 0, 0, 0)
	if worker == nil {
		t.Fatal("worker should not be nil")
	}
}

func TestCalculateSeverity(t *testing.T) {
	tests := []struct {
		anomalyType string
		details     map[string]any
		expected    string
	}{
		{"volume_spike", map[string]any{"z_score": 6.0}, "critical"},
		{"volume_spike", map[string]any{"z_score": 3.5}, "high"},
		{"near_dup_burst", map[string]any{"burst_ratio": 15.0}, "critical"},
		{"near_dup_burst", map[string]any{"burst_ratio": 5.0}, "high"},
		{"coordinated_posting", map[string]any{"spread_count": 10}, "critical"},
		{"coordinated_posting", map[string]any{"spread_count": 3}, "high"},
		{"unknown", map[string]any{}, "medium"},
	}

	for _, tc := range tests {
		got := calculateSeverity(tc.anomalyType, tc.details)
		if got != tc.expected {
			t.Errorf("calculateSeverity(%q, %v) = %q, want %q", tc.anomalyType, tc.details, got, tc.expected)
		}
	}
}

func TestBoardStatsStruct(t *testing.T) {
	bs := boardStats{
		BoardID:   1,
		SiteID:    2,
		PostCount: 100,
		Hour:      time.Now(),
	}

	if bs.BoardID != 1 || bs.PostCount != 100 {
		t.Errorf("boardStats not initialized correctly")
	}
}

func TestCoordinator(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}

	cfg := CoordinatorConfig{
		PollInterval:       100 * time.Millisecond,
		StylometricEnabled: true,
		AnomalyEnabled:     true,

		StylometricMinPosts:     5,
		StylometricNGramMin:     3,
		StylometricNGramMax:     5,
		StylometricTopFeatures:  200,
		StylometricSimThreshold: 0.7,

		AnomalyVolumeZThreshold: 3.0,
		AnomalyBurstThreshold:   5.0,
		AnomalyBurstWindowMin:   10,
		AnomalyLookbackHours:    168,
	}

	coord, err := NewCoordinator(ms, cfg, logger)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	coord.Run(ctx)
}

func TestCoordinatorStylometricOnly(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}

	cfg := CoordinatorConfig{
		PollInterval:       100 * time.Millisecond,
		StylometricEnabled: true,
		AnomalyEnabled:     false,

		StylometricMinPosts:     5,
		StylometricNGramMin:     3,
		StylometricNGramMax:     5,
		StylometricTopFeatures:  200,
		StylometricSimThreshold: 0.7,
	}

	coord, err := NewCoordinator(ms, cfg, logger)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	coord.Run(ctx)
}

func TestCoordinatorAnomalyOnly(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}

	cfg := CoordinatorConfig{
		PollInterval:       100 * time.Millisecond,
		StylometricEnabled: false,
		AnomalyEnabled:     true,

		AnomalyVolumeZThreshold: 3.0,
		AnomalyBurstThreshold:   5.0,
		AnomalyBurstWindowMin:   10,
		AnomalyLookbackHours:    168,
	}

	coord, err := NewCoordinator(ms, cfg, logger)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	coord.Run(ctx)
}

func TestAuthorProfileStruct(t *testing.T) {
	ap := authorProfile{
		PosterID:  "test",
		ThreadID:  1,
		PostIDs:   []int64{1, 2, 3},
		Features:  map[string]float64{"test": 1.0},
		PostCount: 3,
	}

	if ap.PosterID != "test" || ap.PostCount != 3 {
		t.Errorf("authorProfile not initialized correctly")
	}
}
