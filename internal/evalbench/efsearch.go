package evalbench

import (
	"context"

	"akyuu/internal/embedder"
	"akyuu/internal/store"
)

// EfSearchConfig holds the dependencies needed for the ef_search arm.
type EfSearchConfig struct {
	Store        *store.Store
	Embedder     embedder.Embedder
	ModelVersion string
	EfSearch     int
}

// EfSearchSearch returns an Arm that uses a specific ef_search value.
// Wraps the baseline search with SET LOCAL hnsw.ef_search.
func EfSearchSearch(cfg EfSearchConfig) Arm {
	return Arm{
		Name: "efsearch_" + itoa(cfg.EfSearch),
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

			var results []store.SearchResult
			var ferr error
			ferr = cfg.Store.WithEfSearch(ctx, cfg.EfSearch, func(ctx context.Context) error {
				var err2 error
				results, err2 = cfg.Store.SearchEmbeddings(ctx, vecs[0], cfg.ModelVersion, opts)
				return err2
			})
			if ferr != nil {
				return nil, ferr
			}
			hits := make([]Hit, len(results))
			for i, r := range results {
				hits[i] = Hit{PostID: r.PostID, Score: r.Score}
			}
			return hits, nil
		},
	}
}

// itoa converts int to string
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	n := len(buf)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		n--
		buf[n] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		n--
		buf[n] = '-'
	}
	return string(buf[n:])
}
