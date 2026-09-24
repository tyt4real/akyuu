-- 0007_dot_product.sql
-- Add inner product (dot product) HNSW index for dot product similarity.
-- Since ONNX embedder outputs L2-normalized vectors (internal/embedder/pool.go:normalize()),
-- dot product and cosine distance are mathematically identical (cosine = 1 - dot).
-- This index is for benchmarking compute cost difference, not ranking differences.
-- Uses vector_ip_ops for inner product (pgvector's <#> operator returns negative inner product).

CREATE INDEX IF NOT EXISTS post_embeddings_ip_hnsw_idx
    ON post_embeddings USING hnsw (embedding vector_ip_ops)
    WITH (m = 16, ef_construction = 64);

-- For halfvec dot product (if halfvec is populated)
CREATE INDEX IF NOT EXISTS post_embeddings_halfvec_ip_hnsw_idx
    ON post_embeddings USING hnsw (embedding_half vector_ip_ops)
    WITH (m = 16, ef_construction = 64);