package embedder

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"akyuu/internal/parser"
)

// quoteRef strips 4chan-style quote links: same-board ">>12345" and
// cross-board ">>>/b/12345".
var quoteRef = regexp.MustCompile(`>>(?:/[a-zA-Z0-9_-]+/)?\d+`)

// CleanText reduces a post's sanitized comment HTML to the text that gets
// embedded: plain text, quote links removed, whitespace collapsed. The same
// function is used at ingestion time (worker) and at query time (search), so
// the query text and post bodies go through identical cleaning.
func CleanText(commentHTML string) (string, error) {
	text, err := parser.TextContent(commentHTML)
	if err != nil {
		return "", fmt.Errorf("embedder: extract text: %w", err)
	}
	text = quoteRef.ReplaceAllString(text, " ")
	text = strings.Join(strings.Fields(text), " ")
	return text, nil
}

// Fake is a deterministic, dependency-free Embedder for tests and for running
// the pipeline without a model. Vectors are a stable hash of the cleaned text
// (words bucketed by a small rolling hash), so equal texts always produce
// equal vectors and similar word sets land nearby.
type Fake struct {
	Version string
}

// NewFake builds a Fake embedder.
func NewFake() *Fake { return &Fake{Version: "fake"} }

// EmbedBatch implements Embedder.
func (f *Fake) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		out[i] = fakeVector(text)
	}
	return out, nil
}

// Dimensions implements Embedder.
func (f *Fake) Dimensions() int { return 384 }

// ModelVersion implements Embedder.
func (f *Fake) ModelVersion() string { return f.Version }

// fakeVector hashes each word into a fixed dimension bucket, biasing a random
// projection so the vector is unit-length.
func fakeVector(text string) []float32 {
	const dim = 384
	v := make([]float32, dim)
	words := strings.Fields(text)
	for _, w := range words {
		h := fnvHash(strings.ToLower(w))
		bucket := int(h % uint32(dim))
		v[bucket] += float32(int(h%3) - 1) // -1, 0, +1
	}
	return normalize(v)
}

func fnvHash(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}
