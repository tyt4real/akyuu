-- 0008_context_embeddings.sql
-- Context-augmented embeddings column for parent-post concatenation.
-- When a post quotes a parent, we concatenate parent_text + " [SEP] " + post_text
-- and embed the concatenation. This captures thread context for better retrieval.

ALTER TABLE post_embeddings ADD COLUMN embedding_ctx vector(384);

CREATE INDEX IF NOT EXISTS post_embeddings_ctx_hnsw_idx
    ON post_embeddings USING hnsw (embedding_ctx vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);