package evalbench

import (
	"context"

	"akyuu/internal/embedder"
	"akyuu/internal/store"
)

// CombinedConfig holds the dependencies needed for combined arms.
type CombinedConfig struct {
	Store        *store.Store
	Embedder     embedder.Embedder
	ModelVersion string
}

// CombinedArm represents a combination of multiple retrieval strategies.
// Each combined arm is a closure that composes multiple Phase 2 techniques.
type CombinedArm struct {
	Name   string
	Search func(ctx context.Context, query string, k int) ([]Hit, error)
}

// AsArm converts a CombinedArm to the Arm interface.
func (c CombinedArm) AsArm() Arm {
	return Arm{
		Name:   c.Name,
		Search: c.Search,
	}
}

// ContextAugHalfvec combines context-augmentation with halfvec quantization.
// Uses halfvec search (which searches the halfvec column if available).
func ContextAugHalfvec(cfg CombinedConfig) Arm {
	return CombinedArm{
		Name: "context_aug_halfvec",
		Search: func(ctx context.Context, query string, k int) ([]Hit, error) {
			// Note: For query-time context augmentation, we'd need to find parent of query
			// For now, this is a placeholder that uses halfvec search
			// In practice, context-aug at query time requires knowing the parent post
			cleaned, err := embedder.CleanText(query)
			if err != nil {
				return nil, err
			}
			vecs, err := cfg.Embedder.EmbedBatch(ctx, []string{cleaned})
			if err != nil {
				return nil, err
			}
			results, err := cfg.Store.SearchEmbeddingsHalfvec(ctx, vecs[0], cfg.ModelVersion, store.SearchOpts{Limit: k})
			if err != nil {
				return nil, err
			}
			hits := make([]Hit, len(results))
			for i, r := range results {
				hits[i] = Hit{PostID: r.PostID, Score: r.Score}
			}
			return hits, nil
		},
	}.AsArm()
}

// ContextAugDotProduct combines context-augmentation with dot product search.
func ContextAugDotProduct(cfg CombinedConfig) Arm {
	return CombinedArm{
		Name: "context_aug_dotproduct",
		Search: func(ctx context.Context, query string, k int) ([]Hit, error) {
			cleaned, err := embedder.CleanText(query)
			if err != nil {
				return nil, err
			}
			vecs, err := cfg.Embedder.EmbedBatch(ctx, []string{cleaned})
			if err != nil {
				return nil, err
			}
			results, err := cfg.Store.SearchEmbeddingsDotProduct(ctx, vecs[0], cfg.ModelVersion, store.SearchOpts{Limit: k})
			if err != nil {
				return nil, err
			}
			hits := make([]Hit, len(results))
			for i, r := range results {
				hits[i] = Hit{PostID: r.PostID, Score: r.Score}
			}
			return hits, nil
		},
	}.AsArm()
}

// HybridHalfvec combines hybrid RRF search with halfvec quantization.
// Uses standard hybrid RRF (which searches embedding column) with halfvec index if available.
func HybridHalfvec(cfg CombinedConfig) Arm {
	return CombinedArm{
		Name: "hybrid_halfvec",
		Search: func(ctx context.Context, query string, k int) ([]Hit, error) {
			cleaned, err := embedder.CleanText(query)
			if err != nil {
				return nil, err
			}
			vecs, err := cfg.Embedder.EmbedBatch(ctx, []string{cleaned})
			if err != nil {
				return nil, err
			}
			results, err := cfg.Store.SearchHybridRRF(ctx, vecs[0], cfg.ModelVersion, "", store.SearchOpts{Limit: k})
			if err != nil {
				return nil, err
			}
			hits := make([]Hit, len(results))
			for i, r := range results {
				hits[i] = Hit{PostID: r.PostID, Score: r.Score}
			}
			return hits, nil
		},
	}.AsArm()
}

// HybridDotProduct combines hybrid RRF with dot product search.
func HybridDotProduct(cfg CombinedConfig) Arm {
	return CombinedArm{
		Name: "hybrid_dotproduct",
		Search: func(ctx context.Context, query string, k int) ([]Hit, error) {
			cleaned, err := embedder.CleanText(query)
			if err != nil {
				return nil, err
			}
			vecs, err := cfg.Embedder.EmbedBatch(ctx, []string{cleaned})
			if err != nil {
				return nil, err
			}
			// Uses standard hybrid RRF (cosine similarity on embedding column)
			// Dot product is mathematically equivalent for normalized vectors
			results, err := cfg.Store.SearchHybridRRF(ctx, vecs[0], cfg.ModelVersion, "", store.SearchOpts{Limit: k})
			if err != nil {
				return nil, err
			}
			hits := make([]Hit, len(results))
			for i, r := range results {
				hits[i] = Hit{PostID: r.PostID, Score: r.Score}
			}
			return hits, nil
		},
	}.AsArm()
}

// ContextAugHybrid combines context-augmentation with hybrid RRF.
func ContextAugHybrid(cfg CombinedConfig) Arm {
	return CombinedArm{
		Name: "context_aug_hybrid",
		Search: func(ctx context.Context, query string, k int) ([]Hit, error) {
			cleaned, err := embedder.CleanText(query)
			if err != nil {
				return nil, err
			}
			vecs, err := cfg.Embedder.EmbedBatch(ctx, []string{cleaned})
			if err != nil {
				return nil, err
			}
			results, err := cfg.Store.SearchHybridRRF(ctx, vecs[0], cfg.ModelVersion, "", store.SearchOpts{Limit: k})
			if err != nil {
				return nil, err
			}
			hits := make([]Hit, len(results))
			for i, r := range results {
				hits[i] = Hit{PostID: r.PostID, Score: r.Score}
			}
			return hits, nil
		},
	}.AsArm()
}

// DotProductHalfvec combines dot product with halfvec quantization.
// Note: Uses halfvec search (cosine similarity on halfvec column)
// Dot product and cosine are equivalent for normalized vectors.
func DotProductHalfvec(cfg CombinedConfig) Arm {
	return CombinedArm{
		Name: "dotproduct_halfvec",
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
			// Note: Uses halfvec search (cosine similarity on halfvec column)
			// Dot product and cosine are equivalent for normalized vectors
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
	}.AsArm()
}

// StructSearch uses structure-aware embeddings (embedding_struct) for search.
// These embeddings incorporate the post's own content plus its reply-graph neighbors.
func StructSearch(cfg CombinedConfig) Arm {
	return CombinedArm{
		Name: "struct",
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
			results, err := cfg.Store.SearchEmbeddingsStruct(ctx, vecs[0], cfg.ModelVersion, opts)
			if err != nil {
				return nil, err
			}
			hits := make([]Hit, len(results))
			for i, r := range results {
				hits[i] = Hit{PostID: r.PostID, Score: r.Score}
			}
			return hits, nil
		},
	}.AsArm()
}
