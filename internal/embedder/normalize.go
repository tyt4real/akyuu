package embedder

import (
	"context"
)

// TextNormalizer normalizes text before embedding. This is useful for
// cleaning up noisy text (e.g., LLM-generated reformulations).
type TextNormalizer interface {
	// Normalize rewrites the input text to a cleaner form suitable for embedding.
	// Returns the normalized text and an error if normalization fails.
	Normalize(ctx context.Context, text string) (string, error)
}

// FakeNormalizer is a no-op normalizer for tests and when LLM is not available.
type FakeNormalizer struct{}

// NewFakeNormalizer creates a no-op normalizer.
func NewFakeNormalizer() *FakeNormalizer { return &FakeNormalizer{} }

// Normalize implements TextNormalizer (no-op).
func (f *FakeNormalizer) Normalize(_ context.Context, text string) (string, error) {
	return text, nil
}
