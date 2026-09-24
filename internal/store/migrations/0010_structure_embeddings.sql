-- 0010_structure_embeddings.sql
-- Structure-aware embeddings column for reply-graph aware retrieval.
-- embedding_struct stores the average of a post's own embedding and its neighbors' embeddings.
-- Neighbors are defined by the reply graph: parent post + direct children (replies).
-- This enables structure-aware retrieval that respects thread topology.

ALTER TABLE post_embeddings ADD COLUMN embedding_struct vector(384);

CREATE INDEX IF NOT EXISTS post_embeddings_struct_hnsw_idx
    ON post_embeddings USING hnsw (embedding_struct vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);