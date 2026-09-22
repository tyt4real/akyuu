-- 0010_lineage.sql
-- Meme lineage: perceptual hash clusters and genealogy graphs.

-- Perceptual hash index (separate table for flexible algo support)
-- Migration 0006 already added platform_phash column to blobs
-- This table stores multiple algorithms per blob
CREATE TABLE IF NOT EXISTS blob_phash (
    blob_id     TEXT NOT NULL REFERENCES blobs(file_hash) ON DELETE CASCADE,
    algo        TEXT NOT NULL,              -- 'phash', 'dhash', 'ahash'
    hash        TEXT NOT NULL,              -- hex string (64-bit = 16 hex chars)
    width       INTEGER,                    -- image width at hash time
    height      INTEGER,                    -- image height at hash time
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (blob_id, algo)
);

CREATE INDEX IF NOT EXISTS idx_blob_phash_hash ON blob_phash (hash);
CREATE INDEX IF NOT EXISTS idx_blob_phash_algo ON blob_phash (algo);

-- Meme clusters: groups of near-duplicate blobs (Hamming distance <= threshold)
CREATE TABLE IF NOT EXISTS meme_clusters (
    id              BIGSERIAL PRIMARY KEY,
    algo            TEXT NOT NULL,          -- 'phash', 'dhash'
    threshold       INTEGER NOT NULL,       -- Hamming distance threshold used
    representative  TEXT NOT NULL REFERENCES blobs(file_hash) ON DELETE CASCADE,  -- first-seen blob
    blob_count      INTEGER NOT NULL DEFAULT 1,
    first_seen_at   TIMESTAMPTZ NOT NULL,
    last_seen_at    TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_meme_clusters_algo ON meme_clusters (algo);
CREATE INDEX IF NOT EXISTS idx_meme_clusters_rep ON meme_clusters (representative);
CREATE INDEX IF NOT EXISTS idx_meme_clusters_time ON meme_clusters (first_seen_at);

-- Cluster membership: which blobs belong to which cluster
CREATE TABLE IF NOT EXISTS meme_cluster_blobs (
    cluster_id  BIGINT NOT NULL REFERENCES meme_clusters(id) ON DELETE CASCADE,
    blob_id     TEXT NOT NULL REFERENCES blobs(file_hash) ON DELETE CASCADE,
    distance    INTEGER NOT NULL,           -- Hamming distance from representative
    generation  INTEGER NOT NULL DEFAULT 0, -- 0 = representative, 1 = direct child, etc.
    parent_blob_id TEXT REFERENCES blobs(file_hash) ON DELETE SET NULL,  -- immediate ancestor
    first_seen_post_id BIGINT REFERENCES posts(id) ON DELETE SET NULL,
    first_seen_at  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (cluster_id, blob_id)
);

CREATE INDEX IF NOT EXISTS idx_meme_cluster_blobs_blob ON meme_cluster_blobs (blob_id);
CREATE INDEX IF NOT EXISTS idx_meme_cluster_blobs_gen ON meme_cluster_blobs (generation);
CREATE INDEX IF NOT EXISTS idx_meme_cluster_blobs_parent ON meme_cluster_blobs (parent_blob_id);

-- Meme spread tracking: how a cluster propagates across boards/sites over time
CREATE TABLE IF NOT EXISTS meme_spread (
    id              BIGSERIAL PRIMARY KEY,
    cluster_id      BIGINT NOT NULL REFERENCES meme_clusters(id) ON DELETE CASCADE,
    board_id        BIGINT NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    site_id         BIGINT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    post_id         BIGINT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    blob_id         TEXT NOT NULL REFERENCES blobs(file_hash) ON DELETE CASCADE,
    first_seen_at   TIMESTAMPTZ NOT NULL,
    UNIQUE (cluster_id, post_id)
);

CREATE INDEX IF NOT EXISTS idx_meme_spread_cluster ON meme_spread (cluster_id);
CREATE INDEX IF NOT EXISTS idx_meme_spread_board ON meme_spread (board_id);
CREATE INDEX IF NOT EXISTS idx_meme_spread_time ON meme_spread (first_seen_at);

-- Lineage job state (for incremental cluster building)
CREATE TABLE IF NOT EXISTS meme_lineage_state (
    algo            TEXT PRIMARY KEY,         -- 'phash', 'dhash'
    last_blob_id    TEXT,                     -- last processed blob for incremental scan
    last_updated    TIMESTAMPTZ NOT NULL DEFAULT now()
);