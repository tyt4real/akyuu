-- 0009_binary_quant.sql
-- Binary quantization for two-stage retrieval.
-- Stores binary vectors (bit(384)) for fast Hamming distance pre-filtering.
-- Reranks top candidates with full-precision vectors.
-- Requires pgvector 0.7+ with binary_quantize() support.

ALTER TABLE post_embeddings ADD COLUMN embedding_bits bit(384);

-- Populate binary vectors using pgvector's binary_quantize() (preferred)
-- Falls back to manual quantization if not available
DO $$
DECLARE
    ver text;
BEGIN
    SELECT extversion INTO ver FROM pg_extension WHERE extname = 'vector';
    -- pgvector 0.7.0+ has binary_quantize()
    IF ver >= '0.7.0' THEN
        UPDATE post_embeddings SET embedding_bits = binary_quantize(embedding)::bit(384);
    ELSE
        -- Manual fallback: sign bit per dimension
        UPDATE post_embeddings SET embedding_bits = (
            SELECT string_agg(CASE WHEN v > 0 THEN '1' ELSE '0' END, '')::bit(384)
            FROM unnest(embedding::float4[]) AS v
        )::bit(384);
    END IF;
END $$;

-- Optional: HNSW index for Hamming distance (requires pgvector 0.7+)
-- CREATE INDEX IF NOT EXISTS post_embeddings_bits_hnsw_idx
--     ON post_embeddings USING hnsw (embedding_bits bit_hamming_ops);