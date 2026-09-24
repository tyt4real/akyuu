package evalbench

import (
	"math"
	"testing"
)

func TestRecallAtK(t *testing.T) {
	tests := []struct {
		name     string
		hits     []Hit
		relevant map[int64]int
		k        int
		expected float64
	}{
		{
			name:     "perfect recall at k",
			hits:     []Hit{{PostID: 1, Score: 0.9}, {PostID: 2, Score: 0.8}, {PostID: 3, Score: 0.7}},
			relevant: map[int64]int{1: 1, 2: 1, 3: 1},
			k:        3,
			expected: 1.0,
		},
		{
			name:     "partial recall at k",
			hits:     []Hit{{PostID: 1, Score: 0.9}, {PostID: 4, Score: 0.8}, {PostID: 5, Score: 0.7}},
			relevant: map[int64]int{1: 1, 2: 1, 3: 1},
			k:        3,
			expected: 1.0 / 3.0,
		},
		{
			name:     "zero recall at k",
			hits:     []Hit{{PostID: 4, Score: 0.9}, {PostID: 5, Score: 0.8}, {PostID: 6, Score: 0.7}},
			relevant: map[int64]int{1: 1, 2: 1, 3: 1},
			k:        3,
			expected: 0.0,
		},
		{
			name:     "k larger than hits",
			hits:     []Hit{{PostID: 1, Score: 0.9}, {PostID: 2, Score: 0.8}},
			relevant: map[int64]int{1: 1, 2: 1, 3: 1},
			k:        5,
			expected: 2.0 / 3.0,
		},
		{
			name:     "empty relevant set",
			hits:     []Hit{{PostID: 1, Score: 0.9}},
			relevant: map[int64]int{},
			k:        10,
			expected: 0.0,
		},
		{
			name:     "empty hits",
			hits:     []Hit{},
			relevant: map[int64]int{1: 1, 2: 1},
			k:        10,
			expected: 0.0,
		},
		{
			name:     "relevant grades don't affect recall",
			hits:     []Hit{{PostID: 1, Score: 0.9}},
			relevant: map[int64]int{1: 2},
			k:        1,
			expected: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RecallAtK(tt.hits, tt.relevant, tt.k)
			if math.Abs(got-tt.expected) > 1e-9 {
				t.Errorf("RecallAtK() = %f, want %f", got, tt.expected)
			}
		})
	}
}

func TestNDCGAtK(t *testing.T) {
	tests := []struct {
		name     string
		hits     []Hit
		relevant map[int64]int
		k        int
		expected float64
	}{
		{
			name:     "perfect ranking",
			hits:     []Hit{{PostID: 1, Score: 0.9}, {PostID: 2, Score: 0.8}, {PostID: 3, Score: 0.7}},
			relevant: map[int64]int{1: 2, 2: 1, 3: 1},
			k:        3,
			expected: 1.0,
		},
		{
			name:     "reversed ranking - all relevant but wrong order",
			hits:     []Hit{{PostID: 3, Score: 0.7}, {PostID: 2, Score: 0.8}, {PostID: 1, Score: 0.9}},
			relevant: map[int64]int{1: 2, 2: 1, 3: 1},
			k:        3,
			expected: 0.840303, // DCG: 1/1 + 1/1.585 + 2/2 = 2.631; IDCG: 2/1 + 1/1.585 + 1/2 = 3.131; NDCG = 0.8403
		},
		{
			name:     "partial relevant in top k",
			hits:     []Hit{{PostID: 4, Score: 0.9}, {PostID: 1, Score: 0.8}, {PostID: 5, Score: 0.7}},
			relevant: map[int64]int{1: 1, 2: 1},
			k:        3,
			expected: (1 / math.Log2(3)) / (1/math.Log2(2) + 1/math.Log2(3)),
		},
		{
			name:     "empty relevant set",
			hits:     []Hit{{PostID: 1, Score: 0.9}},
			relevant: map[int64]int{},
			k:        10,
			expected: 0.0,
		},
		{
			name:     "empty hits",
			hits:     []Hit{},
			relevant: map[int64]int{1: 1, 2: 1},
			k:        10,
			expected: 0.0,
		},
		{
			name:     "highly relevant at top",
			hits:     []Hit{{PostID: 1, Score: 0.9}, {PostID: 2, Score: 0.8}},
			relevant: map[int64]int{1: 2, 2: 1},
			k:        2,
			expected: 1.0,
		},
		{
			name:     "no relevant hits in top k",
			hits:     []Hit{{PostID: 4, Score: 0.9}, {PostID: 5, Score: 0.8}},
			relevant: map[int64]int{1: 1, 2: 1},
			k:        2,
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NDCGAtK(tt.hits, tt.relevant, tt.k)
			if math.Abs(got-tt.expected) > 1e-6 {
				t.Errorf("NDCGAtK() = %f, want %f", got, tt.expected)
			}
		})
	}
}

func TestPercentile(t *testing.T) {
	// Percentile uses nearest-rank method (not linear interpolation)
	// idx = int(p/100 * (len-1))
	tests := []struct {
		name     string
		data     []float64
		p        float64
		expected float64
	}{
		{
			name:     "median",
			data:     []float64{1, 2, 3, 4, 5},
			p:        50,
			expected: 3, // idx = int(0.5 * 4) = 2 -> data[2] = 3
		},
		{
			name:     "p25",
			data:     []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			p:        25,
			expected: 3, // idx = int(0.25 * 9) = 2 -> data[2] = 3
		},
		{
			name:     "p75",
			data:     []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			p:        75,
			expected: 7, // idx = int(0.75 * 9) = 6 -> data[6] = 7 (0-indexed)
		},
		{
			name:     "p0",
			data:     []float64{1, 2, 3},
			p:        0,
			expected: 1,
		},
		{
			name:     "p100",
			data:     []float64{1, 2, 3},
			p:        100,
			expected: 3,
		},
		{
			name:     "single element",
			data:     []float64{42},
			p:        50,
			expected: 42,
		},
		{
			name:     "empty slice",
			data:     []float64{},
			p:        50,
			expected: 0,
		},
		{
			name:     "duplicate values",
			data:     []float64{1, 1, 1, 2, 2, 3},
			p:        50,
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Percentile(tt.data, tt.p)
			if math.Abs(got-tt.expected) > 1e-9 {
				t.Errorf("Percentile() = %f, want %f", got, tt.expected)
			}
		})
	}
}

func TestSortDesc(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		expected []int
	}{
		{
			name:     "already sorted",
			input:    []int{5, 4, 3, 2, 1},
			expected: []int{5, 4, 3, 2, 1},
		},
		{
			name:     "reverse sorted",
			input:    []int{1, 2, 3, 4, 5},
			expected: []int{5, 4, 3, 2, 1},
		},
		{
			name:     "random order",
			input:    []int{3, 1, 4, 1, 5, 9, 2, 6},
			expected: []int{9, 6, 5, 4, 3, 2, 1, 1},
		},
		{
			name:     "all same",
			input:    []int{5, 5, 5, 5},
			expected: []int{5, 5, 5, 5},
		},
		{
			name:     "single element",
			input:    []int{42},
			expected: []int{42},
		},
		{
			name:     "empty",
			input:    []int{},
			expected: []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := append([]int(nil), tt.input...)
			sortDesc(input)
			if len(input) != len(tt.expected) {
				t.Errorf("sortDesc() length = %d, want %d", len(input), len(tt.expected))
				return
			}
			for i, v := range input {
				if v != tt.expected[i] {
					t.Errorf("sortDesc() at index %d = %d, want %d", i, v, tt.expected[i])
				}
			}
		})
	}
}
