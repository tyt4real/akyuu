package embedder

import (
	"context"
	"math"
	"os"
	"testing"
)

// TestONNXModelEmbedding exercises the real ONNX Runtime path. It needs the
// shared library (libonnxruntime.so) available to dlopen and a directory with
// model.onnx + tokenizer.json; without either it skips. Run with:
//
//	AKYUU_TEST_MODEL_DIR=/path/to/all-MiniLM-L6-v2 \
//	  go test -count=1 ./internal/embedder/ -run TestONNX -v
func TestONNXModelEmbedding(t *testing.T) {
	dir := os.Getenv("AKYUU_TEST_MODEL_DIR")
	if dir == "" {
		t.Skip("set AKYUU_TEST_MODEL_DIR to run real-model ONNX tests")
	}
	m, err := NewONNX(dir, "all-MiniLM-L6-v2", 384)
	if err != nil {
		t.Fatalf("load model: %v", err)
	}
	defer m.Close()

	ctx := context.Background()
	texts := []string{"best linux distro debates", "the same words again"}
	vecs, err := m.EmbedBatch(ctx, texts)
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vecs))
	}
	for i, v := range vecs {
		if len(v) != 384 {
			t.Errorf("vector %d width %d, want 384", i, len(v))
		}
		var sum float64
		for _, x := range v {
			sum += float64(x) * float64(x)
		}
		if math.Abs(sum-1) > 1e-3 {
			t.Errorf("vector %d not unit length (sum=%f)", i, sum)
		}
	}

	// Determinism: same input, same vector.
	again, err := m.EmbedBatch(ctx, texts)
	if err != nil {
		t.Fatalf("re-embed: %v", err)
	}
	for i := range vecs {
		for j := range vecs[i] {
			if vecs[i][j] != again[i][j] {
				t.Fatalf("embedding not deterministic at %d,%d", i, j)
			}
		}
	}

	// Similar texts land near each other; disjoint texts land apart.
	a, _ := m.EmbedBatch(ctx, []string{"linux distro recommendations"})
	b, _ := m.EmbedBatch(ctx, []string{"pasta sauce recipes"})
	closeScore := dot(vecs[0], a[0])
	farScore := dot(vecs[0], b[0])
	if closeScore <= farScore {
		t.Errorf("expected related texts closer (%.3f) than unrelated (%.3f)", closeScore, farScore)
	}
}

func dot(a, b []float32) float64 {
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}
