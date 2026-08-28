-- 0005_tags.sql
-- Tagging system for auto-tagging and user-defined tags.

-- Tags table: lightweight user-defined or auto-generated tags.
CREATE TABLE IF NOT EXISTS tags (
    id       BIGSERIAL PRIMARY KEY,
    name     TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Tag post links: many-to-many between tags and posts.
CREATE TABLE IF NOT EXISTS taggings (
    tag_id   BIGINT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    post_id  BIGINT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    PRIMARY KEY (tag_id, post_id)
);

-- Tag thread links: many-to-many between tags and threads.
CREATE TABLE IF NOT EXISTS taggings_threads (
    tag_id     BIGINT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    thread_id BIG BIGINT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    PRIMARY KEY (tag_id, thread_id)
);
