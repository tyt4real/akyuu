-- 0004_embeddings.sql
-- Semantic search over post bodies: a per-post embedding queue plus the
-- pgvector table the vectors land in. Requires the vector extension (the
-- compose image is pgvector/pgvector:pg16).

CREATE EXTENSION IF NOT EXISTS vector;

-- Embedding queue. The worker consumes posts with pending_embedding = TRUE;
-- it either writes a post_embeddings row or marks the post non-searchable
-- (too short / empty body). The upsert sets this for new posts and for edits
-- whose text actually changed, so steady-state re-polls do not re-embed.
ALTER TABLE posts ADD COLUMN IF NOT EXISTS pending_embedding BOOLEAN NOT NULL DEFAULT FALSE;

-- One-time backfill: every existing text post becomes eligible so the worker
-- walks the whole archive on first run.
UPDATE posts SET pending_embedding = TRUE WHERE comment_parsed <> '';

CREATE TABLE IF NOT EXISTS post_embeddings (
    post_id       BIGINT PRIMARY KEY REFERENCES posts(id) ON DELETE CASCADE,
    embedding     vector(384) NOT NULL,
    model_version TEXT        NOT NULL,   -- which model produced the vector
    text_hash     TEXT        NOT NULL,   -- hash of the embedded text (staleness check)
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS post_embeddings_hnsw_idx
    ON post_embeddings USING hnsw (embedding vector_cosine_ops);