-- 0003_archive.sql
-- Archive-source support: per-board pagination cursor for FoolFuuka-style
-- paginated catalog crawls, plus an index to resolve 4chan thread mirrors.

ALTER TABLE boards ADD COLUMN IF NOT EXISTS archive_page INTEGER NOT NULL DEFAULT 1;

-- Mirror dedup: 4chan thread numbers are globally unique, so a thread pulled
-- from live 4chan or from a desuarchive mirror can be found across boards.
CREATE INDEX IF NOT EXISTS idx_threads_mirror
    ON threads (thread_native_id, board_id);
