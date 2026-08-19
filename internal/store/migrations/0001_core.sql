-- 0001_core.sql
-- Core metadata schema for the multi-site imageboard archiver.

CREATE TABLE IF NOT EXISTS sites (
    id           BIGSERIAL PRIMARY KEY,
    name         TEXT        NOT NULL UNIQUE,
    base_url     TEXT        NOT NULL,
    platform_type TEXT       NOT NULL,   -- adapter platform: lynxchan | vichan | fourchan
    api_available BOOLEAN    NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Per-site health / observability. Kept here rather than a separate table so
-- it stays in sync with site lifecycle.
ALTER TABLE sites ADD COLUMN IF NOT EXISTS last_successful_poll TIMESTAMPTZ;
ALTER TABLE sites ADD COLUMN IF NOT EXISTS consecutive_failures INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sites ADD COLUMN IF NOT EXISTS last_error TEXT;
ALTER TABLE sites ADD COLUMN IF NOT EXISTS circuit_open BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE sites ADD COLUMN IF NOT EXISTS circuit_until TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS boards (
    id         BIGSERIAL PRIMARY KEY,
    site_id    BIGINT      NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    code       TEXT        NOT NULL,
    title      TEXT,
    nsfw       BOOLEAN     NOT NULL DEFAULT FALSE,
    worksafe   BOOLEAN     NOT NULL DEFAULT TRUE,
    bump_limit INTEGER,
    max_pages  INTEGER,
    UNIQUE (site_id, code)
);

CREATE TABLE IF NOT EXISTS threads (
    id              BIGSERIAL PRIMARY KEY,
    board_id        BIGINT      NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    thread_native_id TEXT       NOT NULL,          -- platform thread number, as text
    subject         TEXT,
    sticky          BOOLEAN     NOT NULL DEFAULT FALSE,
    locked          BOOLEAN     NOT NULL DEFAULT FALSE,
    archived        BOOLEAN     NOT NULL DEFAULT FALSE,
    last_bump_time  TIMESTAMPTZ,
    reply_count     INTEGER     NOT NULL DEFAULT 0,
    file_count      INTEGER     NOT NULL DEFAULT 0,
    -- lifecycle: active -> archived (explicit 404 or missing from catalog)
    status          TEXT        NOT NULL DEFAULT 'active',  -- active | archived
    missing_count   INTEGER     NOT NULL DEFAULT 0,         -- consecutive misses
    first_seen_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at    TIMESTAMPTZ,
    UNIQUE (board_id, thread_native_id)
);

CREATE TABLE IF NOT EXISTS posts (
    id              BIGSERIAL PRIMARY KEY,
    thread_id       BIGINT      NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    post_native_id  TEXT        NOT NULL,
    "timestamp"     TIMESTAMPTZ,
    author_name     TEXT,
    tripcode        TEXT,
    capcode         TEXT,
    poster_id       TEXT,          -- 4chan/LynxChan poster hash; unavailable on plain vichan
    comment_raw     TEXT,          -- verbatim comment markup as served
    comment_parsed  TEXT,          -- sanitized, display-safe HTML
    sage            BOOLEAN       NOT NULL DEFAULT FALSE,
    country         TEXT,
    flag            TEXT,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (thread_id, post_native_id)
);

-- Full-text search over the parsed comment. Tags are stripped at index time.
ALTER TABLE posts ADD COLUMN IF NOT EXISTS comment_tsv tsvector
    GENERATED ALWAYS AS (to_tsvector('simple', regexp_replace(coalesce(comment_parsed, ''), '<[^>]+>', ' ', 'g'))) STORED;
CREATE INDEX IF NOT EXISTS idx_posts_comment_tsv ON posts USING GIN (comment_tsv);

-- Quote graph. quoted_post_native_id is stored verbatim even when the target
-- post has never been seen (dead/never-seen quotes) -- resolution happens at
-- query time, not write time. board is '' (empty) for same-board references;
-- it cannot be NULL because it is part of the composite primary key.
CREATE TABLE IF NOT EXISTS post_quotes (
    post_id               BIGINT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    quoted_post_native_id TEXT   NOT NULL,
    board                 TEXT   NOT NULL DEFAULT '',  -- '' = same board (cross-board refs carry board)
    PRIMARY KEY (post_id, quoted_post_native_id, board)
);

-- Content-addressable blobs. One row per unique file content across ALL sites;
-- this is where cross-site dedup happens.
CREATE TABLE IF NOT EXISTS blobs (
    file_hash           TEXT PRIMARY KEY,     -- sha256 hex of the content
    mime_type           TEXT,
    size_bytes          BIGINT,
    storage_path        TEXT,                 -- relative to storage root, e.g. full/ab/cd/<hash>.jpg
    thumb_storage_path  TEXT,                 -- relative to storage root
    platform_md5        TEXT,                 -- md5 as reported by a platform, for cheap dedup lookups
    platform_sha1       TEXT,
    first_seen_post_id  BIGINT REFERENCES posts(id),
    ref_count           INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_blobs_md5  ON blobs (platform_md5)  WHERE platform_md5  IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_blobs_sha1 ON blobs (platform_sha1) WHERE platform_sha1 IS NOT NULL;

CREATE TABLE IF NOT EXISTS files (
    id                BIGSERIAL PRIMARY KEY,
    post_id           BIGINT  NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    file_hash         TEXT    REFERENCES blobs(file_hash),
    original_filename TEXT,
    server_filename   TEXT,
    width             INTEGER,                  -- null for non-image mime types
    height            INTEGER,
    thumbnail_url     TEXT,
    full_url          TEXT,
    spoiler           BOOLEAN NOT NULL DEFAULT FALSE,
    downloaded_full   BOOLEAN NOT NULL DEFAULT FALSE,
    downloaded_thumb  BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE (post_id, full_url)
);
CREATE INDEX IF NOT EXISTS idx_files_hash ON files (file_hash) WHERE file_hash IS NOT NULL;