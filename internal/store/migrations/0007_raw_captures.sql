-- 0007_raw_captures.sql
-- Raw capture storage for archival integrity and reprocessing.
-- Stores the exact HTTP response (body + headers + metadata) before parsing.

CREATE TABLE IF NOT EXISTS raw_captures (
    id              BIGSERIAL PRIMARY KEY,
    site_id         BIGINT      NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    board_id        BIGINT      REFERENCES boards(id) ON DELETE SET NULL,
    thread_id       BIGINT      REFERENCES threads(id) ON DELETE SET NULL,
    url             TEXT        NOT NULL,
    method          TEXT        NOT NULL DEFAULT 'GET',
    status_code     INTEGER     NOT NULL,
    request_headers JSONB       NOT NULL DEFAULT '{}',
    response_headers JSONB      NOT NULL DEFAULT '{}',
    body            BYTEA       NOT NULL,               -- raw response body (HTML, JSON, etc.)
    body_sha256     TEXT        NOT NULL,               -- sha256 hex of body for dedup
    content_type    TEXT,                               -- from response Content-Type header
    content_length  BIGINT,
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    fetch_duration_ms INTEGER,                          -- total fetch time
    parser_version  TEXT        NOT NULL DEFAULT 'v1',  -- which parser version processed this
    processed       BOOLEAN     NOT NULL DEFAULT FALSE, -- whether parser has run
    processed_at    TIMESTAMPTZ,
    error           TEXT                                -- parser error if processing failed
);

-- Index for finding captures by site/board/thread for reprocessing
CREATE INDEX IF NOT EXISTS idx_raw_captures_site_board ON raw_captures (site_id, board_id);
CREATE INDEX IF NOT EXISTS idx_raw_captures_thread ON raw_captures (thread_id);
CREATE INDEX IF NOT EXISTS idx_raw_captures_url ON raw_captures (url);
CREATE INDEX IF NOT EXISTS idx_raw_captures_fetched ON raw_captures (fetched_at);
CREATE INDEX IF NOT EXISTS idx_raw_captures_unprocessed ON raw_captures (site_id, fetched_at) WHERE processed = FALSE;
CREATE INDEX IF NOT EXISTS idx_raw_captures_body_hash ON raw_captures (body_sha256);

-- Partition by month for retention management (optional, requires pg_partman or manual)
-- ALTER TABLE raw_captures SET (autovacuum_enabled = true);