package evalbench

import (
	"context"

	"akyuu/internal/embedder"
	"akyuu/internal/store"
)

// DotProductConfig holds the dependencies needed for the dot product arm.
type DotProductConfig struct {
	Store        *store.Store
	Embedder     embedder.Embedder
	ModelVersion string
}

// DotProductSearch returns an Arm that uses dot product (inner product) similarity.
// Since ONNX embedder outputs L2-normalized vectors, dot product is mathematically
// equivalent to cosine similarity. Uses pgvector's <#> operator with vector_ip_ops index.
func DotProductSearch(cfg DotProductConfig) Arm {
	return Arm{
		Name: "dotproduct",
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
			results, err := cfg.Store.SearchEmbeddingsDotProduct(ctx, vecs[0], cfg.ModelVersion, opts)
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
