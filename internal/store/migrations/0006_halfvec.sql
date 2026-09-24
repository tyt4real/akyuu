-- 0006_halfvec.sql
-- Add half-precision vector column for scalar quantization (pgvector 0.7+)
-- halfvec uses 16-bit floats (2 bytes per dimension) vs 4 bytes for full vector
-- 50% storage reduction with minimal recall loss for most use cases

ALTER TABLE post_embeddings ADD COLUMN embedding_half halfvec(384);

-- Populate from existing embeddings
UPDATE post_embeddings SET embedding_half = embedding::halfvec(384);

-- HNSW index for halfvec cosine similarity
CREATE INDEX IF NOT EXISTS post_embeddings_halfvec_hnsw_idx
    ON post_embeddings USING hnsw (embedding_half halfvec_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- Verify the column was populated correctly
-- SELECT count(*) FROM post_embeddings WHERE embedding_half IS NOT NULL;