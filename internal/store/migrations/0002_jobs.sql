-- 0002_jobs.sql
-- Postgres-backed job queue. pending -> in_progress -> done/failed.
-- Per-site isolation: every job is scoped to site_id and workers claim jobs
-- per site, so one site's problems cannot starve others.

CREATE TABLE IF NOT EXISTS jobs (
    id          BIGSERIAL PRIMARY KEY,
    site_id     BIGINT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    board_id    BIGINT REFERENCES boards(id) ON DELETE CASCADE,   -- NULL for site-wide jobs
    thread_id   BIGINT REFERENCES threads(id) ON DELETE CASCADE,  -- NULL for catalog jobs
    kind        TEXT   NOT NULL,      -- catalog | thread | download | backfill
    status      TEXT   NOT NULL DEFAULT 'pending',  -- pending | in_progress | done | failed
    payload     JSONB,
    attempts    INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 10,
    run_after   TIMESTAMPTZ NOT NULL DEFAULT now(),  -- backoff delay target
    last_error  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Claimable jobs: due and not claimed. SKIP LOCKED lets concurrent workers
-- claim different rows without blocking.
CREATE INDEX IF NOT EXISTS idx_jobs_claim ON jobs (site_id, status, run_after)
    WHERE status IN ('pending', 'in_progress');

-- One active job per (site, board, thread, kind): prevents double-enqueuing
-- the same poll. A completed job can be re-queued later.
CREATE UNIQUE INDEX IF NOT EXISTS uq_jobs_active
    ON jobs (site_id, board_id, thread_id, kind)
    WHERE status IN ('pending', 'in_progress');