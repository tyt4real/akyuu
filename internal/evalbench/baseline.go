package evalbench

import (
	"context"

	"akyuu/internal/embedder"
	"akyuu/internal/store"
)

// BaselineConfig holds the dependencies needed for the baseline arm.
type BaselineConfig struct {
	Store        *store.Store
	Embedder     embedder.Embedder
	ModelVersion string
}

// BaselineSearch returns an Arm that implements the baseline retrieval strategy:
// clean query text -> embed -> search embeddings.
func BaselineSearch(cfg BaselineConfig) Arm {
	return Arm{
		Name: "baseline",
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
			results, err := cfg.Store.SearchEmbeddings(ctx, vecs[0], cfg.ModelVersion, opts)
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
