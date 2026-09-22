-- 0009_structure.sql
-- Structure-aware features: reply graph resolution, thread continuity.

-- Materialized view: resolved reply graph edges
-- Resolves post_quotes (native IDs) to internal post IDs
-- Includes cross-board and cross-site references where resolvable
CREATE MATERIALIZED VIEW IF NOT EXISTS reply_graph_edges AS
SELECT
    pq.post_id          AS source_post_id,
    p.id                AS target_post_id,
    pq.board            AS target_board_code,   -- '' = same board, otherwise board code
    p.thread_id         AS target_thread_id,
    t.board_id          AS target_board_id,
    b.site_id           AS target_site_id,
    st.name             AS target_site_name,
    CASE
        WHEN pq.board = '' THEN 'same_board'
        WHEN pq.board IS NOT NULL AND pq.board <> '' THEN 'cross_board'
        ELSE 'unresolved'
    END                 AS reference_type,
    pq.quoted_post_native_id AS target_native_id
FROM post_quotes pq
LEFT JOIN posts p
    ON p.post_native_id = pq.quoted_post_native_id
    AND (
        (pq.board = '' AND p.thread_id IN (SELECT t1.id FROM threads t1 WHERE t1.board_id = (SELECT t2.board_id FROM threads t2 JOIN posts p2 ON p2.thread_id = t2.id WHERE p2.id = pq.post_id)))
        OR (pq.board <> '' AND p.thread_id IN (SELECT t3.id FROM threads t3 JOIN boards b3 ON b3.id = t3.board_id WHERE b3.code = pq.board))
    )
LEFT JOIN threads t ON t.id = p.thread_id
LEFT JOIN boards b ON b.id = t.board_id
LEFT JOIN sites st ON st.id = b.site_id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_reply_graph_edges_unique ON reply_graph_edges (source_post_id, target_post_id);
CREATE INDEX IF NOT EXISTS idx_reply_graph_edges_source ON reply_graph_edges (source_post_id);
CREATE INDEX IF NOT EXISTS idx_reply_graph_edges_target ON reply_graph_edges (target_post_id);
CREATE INDEX IF NOT EXISTS idx_reply_graph_edges_thread ON reply_graph_edges (target_thread_id);

-- Function to refresh the materialized view (call after batch ingest)
-- REFRESH MATERIALIZED VIEW CONCURRENTLY reply_graph_edges;

-- Thread continuity links: "General" threads that continue across recreations
CREATE TABLE IF NOT EXISTS continuity_links (
    id              BIGSERIAL PRIMARY KEY,
    prev_thread_id  BIGINT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    next_thread_id  BIGINT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    confidence      REAL NOT NULL,                -- 0.0-1.0
    method          TEXT NOT NULL,                -- 'title_similarity', 'op_similarity', 'title+op', 'manual'
    title_similarity REAL,                        -- Levenshtein ratio or prefix match
    op_similarity   REAL,                         -- cosine similarity of OP embeddings
    time_gap_hours  REAL,                         -- hours between prev last bump and next creation
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (prev_thread_id, next_thread_id)
);

CREATE INDEX IF NOT EXISTS idx_continuity_prev ON continuity_links (prev_thread_id);
CREATE INDEX IF NOT EXISTS idx_continuity_next ON continuity_links (next_thread_id);
CREATE INDEX IF NOT EXISTS idx_continuity_confidence ON continuity_links (confidence DESC);

-- Add continuity pointer to threads for easy traversal
ALTER TABLE threads ADD COLUMN IF NOT EXISTS continuity_prev_id BIGINT REFERENCES threads(id) ON DELETE SET NULL;
ALTER TABLE threads ADD COLUMN IF NOT EXISTS continuity_next_id BIGINT REFERENCES threads(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_threads_continuity_prev ON threads (continuity_prev_id);
CREATE INDEX IF NOT EXISTS idx_threads_continuity_next ON threads (continuity_next_id);

-- Cross-board/cross-site crosspost detection (populated by pHash/SHA256 match jobs)
CREATE TABLE IF NOT EXISTS crosspost_matches (
    id              BIGSERIAL PRIMARY KEY,
    blob_id         TEXT NOT NULL REFERENCES blobs(file_hash) ON DELETE CASCADE,  -- the shared content
    post_ids        BIGINT[] NOT NULL,                  -- all posts sharing this content
    site_ids        BIGINT[] NOT NULL,                  -- sites involved
    board_ids       BIGINT[] NOT NULL,                  -- boards involved
    match_type      TEXT NOT NULL,                      -- 'exact_sha256', 'near_phash', 'ocr_text'
    first_seen_at   TIMESTAMPTZ NOT NULL,
    last_seen_at    TIMESTAMPTZ NOT NULL,
    spread_count    INTEGER NOT NULL DEFAULT 1,         -- number of distinct posts
    UNIQUE (blob_id, match_type)
);

CREATE INDEX IF NOT EXISTS idx_crosspost_blob ON crosspost_matches (blob_id);
CREATE INDEX IF NOT EXISTS idx_crosspost_first_seen ON crosspost_matches (first_seen_at);
CREATE INDEX IF NOT EXISTS idx_crosspost_sites ON crosspost_matches USING GIN (site_ids);