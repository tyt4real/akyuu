package search

import (
	"context"

	"akyuu/internal/store"
)

// StoreAPI is the interface the search workers need from the store.
type StoreAPI interface {
	Query(ctx context.Context, query string, args ...any) (store.Rows, error)
	QueryRow(ctx context.Context, query string, args ...any) store.RowScanner
	Exec(ctx context.Context, query string, args ...any) error
}

// Embedder is the interface for embedding text.
type Embedder interface {
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	ModelVersion() string
	Dimensions() int
}
