-- 0008_canonical_texts.sql
-- Canonical texts table for exact-match deduplication.
-- When the same cleaned text appears many times across posts, we can reuse
-- the same embedding vector instead of re-embedding.
-- Promotion threshold: only promote to canonical_texts after N occurrences
-- to avoid filling the table with one-off strings.

CREATE TABLE IF NOT EXISTS canonical_texts (
    normalized_text TEXT PRIMARY KEY,
    embedding        vector(384) NOT NULL,
    ref_count        INTEGER NOT NULL DEFAULT 0,
    first_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Candidates table for tracking occurrences before promotion
CREATE TABLE IF NOT EXISTS canonical_candidates (
    normalized_text TEXT PRIMARY KEY,
    seen_count      INTEGER NOT NULL DEFAULT 1,
    first_seen_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Index for fast lookup of canonical embeddings
CREATE INDEX IF NOT EXISTS canonical_texts_embedding_hnsw_idx
    ON canonical_texts USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- Function to get canonical embedding if it exists
-- Returns (embedding, found)
-- CREATE OR REPLACE FUNCTION get_canonical_embedding(normalized_text TEXT)
-- RETURNS vector(384) AS $$
--     SELECT embedding FROM canonical_texts WHERE normalized_text = $1;
-- $$ LANGUAGE sql STABLE;