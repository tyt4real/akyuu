package evalbench

import (
	"context"

	"akyuu/internal/embedder"
	"akyuu/internal/store"
)

// HalfvecConfig holds the dependencies needed for the halfvec arm.
type HalfvecConfig struct {
	Store        *store.Store
	Embedder     embedder.Embedder
	ModelVersion string
}

// HalfvecSearch returns an Arm that uses half-precision vectors for search.
// Requires pgvector 0.7+ with halfvec support.
func HalfvecSearch(cfg HalfvecConfig) Arm {
	return Arm{
		Name: "halfvec",
		Search: func(ctx context.Context, query string, k int) ([]Hit, error) {
			cleaned, err := embedder.CleanText(query)
			if err != nil {
				return nil, err
			}
			vecs, err := cfg.Embedder.EmbedBatch(ctx, []string{cleaned})
			if err != nil {
				return nil, err
			}
			opts := store.SearchOpts{Limit: k}
			results, err := cfg.Store.SearchEmbeddingsHalfvec(ctx, vecs[0], cfg.ModelVersion, opts)
			if err != nil {
				return nil, err
			}
			hits := make([]Hit, len(results))
			for i, r := range results {
				hits[i] = Hit{PostID: r.PostID, Score: r.Score}
			}
			return hits, nil
		},
	}
}
