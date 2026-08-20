package embedder

import (
	"math"
	"strings"
	"testing"
)

func TestCleanTextStripsQuotesAndTags(t *testing.T) {
	html := `<span class="quote">>>12345</span> &gt;&gt;&gt;/b/999 <b>dubs</b> <br>`
	got, err := CleanText(html)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, ">>") {
		t.Errorf("quote links not stripped: %q", got)
	}
	if strings.Contains(got, "<") || strings.Contains(got, "&gt;") {
		t.Errorf("HTML not decoded: %q", got)
	}
	if !strings.Contains(got, "dubs") {
		t.Errorf("post text missing: %q", got)
	}
}

func TestCleanTextEmpty(t *testing.T) {
	got, err := CleanText("<br><br>")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("expected empty text, got %q", got)
	}
}

func TestMeanPoolNormalizes(t *testing.T) {
	// 2 sequences, 2 tokens each, hidden 3; mask keeps both tokens.
	hidden := []float32{
		1, 0, 0,
		0, 1, 0,
		0, 0, 2,
		3, 0, 0,
	}
	masks := [][]int64{{1, 1}, {1, 1}}
	out, err := meanPool(hidden, []int64{2, 2, 3}, masks)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(out))
	}
	// Row 0 mean = (0.5, 0.5, 0) then normalized.
	norm := math.Sqrt(0.5*0.5 + 0.5*0.5)
	if math.Abs(float64(out[0][0])-0.5/norm) > 1e-5 ||
		math.Abs(float64(out[0][1])-0.5/norm) > 1e-5 ||
		out[0][2] != 0 {
		t.Errorf("row 0 wrong: %v", out[0])
	}
	// Row 1 mean = (1.5, 0, 1), then normalized.
	norm1 := math.Sqrt(1.5*1.5 + 1)
	if math.Abs(float64(out[1][0])-1.5/norm1) > 1e-5 || out[1][1] != 0 ||
		math.Abs(float64(out[1][2])-1/norm1) > 1e-5 {
		t.Errorf("row 1 wrong: %v", out[1])
	}
}

func TestMeanPoolMasked(t *testing.T) {
	// Second token masked out: row 0 is just token 0.
	hidden := []float32{1, 0, 0, 5, 5, 5}
	masks := [][]int64{{1, 0}}
	out, err := meanPool(hidden, []int64{1, 2, 3}, masks)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(out[0][0])-1) > 1e-5 || out[0][1] != 0 || out[0][2] != 0 {
		t.Errorf("masked token leaked into pooling: %v", out[0])
	}
}

func TestFakeEmbedderStableAndUnitLength(t *testing.T) {
	f := NewFake()
	if f.Dimensions() != 384 {
		t.Fatalf("fake dims: %d", f.Dimensions())
	}
	a, err := f.EmbedBatch(t.Context(), []string{"linux distro debates"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.EmbedBatch(t.Context(), []string{"linux distro debates"})
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("expected one vector per input")
	}
	for i := range a[0] {
		if a[0][i] != b[0][i] {
			t.Fatalf("fake vectors not deterministic at dim %d", i)
		}
	}
	norm := 0.0
	for _, x := range a[0] {
		norm += float64(x) * float64(x)
	}
	if math.Abs(norm-1) > 1e-5 {
		t.Errorf("fake vector not unit length: %f", norm)
	}
}

func TestHashText(t *testing.T) {
	if hashText("same") != hashText("same") {
		t.Error("hash not stable")
	}
	if hashText("same") == hashText("different") {
		t.Error("hash collision on different texts")
	}
}
