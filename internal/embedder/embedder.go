// Package embedder turns post bodies into vectors for semantic search. It
// defines the Embedder interface (in-process ONNX, or anything else), the
// text-cleaning rules shared by ingestion and query time, and the worker loop
// that drains the store's pending_embedding queue.
package embedder

import "context"

// Embedder produces normalized embedding vectors for texts.
type Embedder interface {
	// EmbedBatch embeds several texts in one call. len(out) == len(texts).
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Dimensions is the width of every returned vector; it must match the
	// post_embeddings.embedding column width (vector(384) in v1).
	Dimensions() int

	// ModelVersion names the model that produced the vectors. It is stored
	// alongside every vector so search never mixes models.
	ModelVersion() string
}

// Closer is an optional interface for embedders that need cleanup.
type Closer interface {
	Close() error
}
