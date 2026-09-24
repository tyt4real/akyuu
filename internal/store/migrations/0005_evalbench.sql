-- 0005_evalbench.sql
-- Hand-labeled + proxy relevance judgments for the retrieval benchmark.
-- Lives alongside the archive schema but is never touched by the scraper
-- or embed worker.

CREATE TABLE IF NOT EXISTS eval_queries (
    id          BIGSERIAL PRIMARY KEY,
    query_text  TEXT        NOT NULL,
    source      TEXT        NOT NULL DEFAULT 'hand_labeled', -- hand_labeled | proxy_quote
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS eval_relevance (
    query_id    BIGINT NOT NULL REFERENCES eval_queries(id) ON DELETE CASCADE,
    post_id     BIGINT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    grade       SMALLINT NOT NULL DEFAULT 1, -- 0 = not relevant, 1 = relevant, 2 = highly relevant
    PRIMARY KEY (query_id, post_id)
);

CREATE TABLE IF NOT EXISTS eval_runs (
    id          BIGSERIAL PRIMARY KEY,
    arm_name    TEXT        NOT NULL,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    notes       TEXT
);

CREATE TABLE IF NOT EXISTS eval_results (
    run_id       BIGINT NOT NULL REFERENCES eval_runs(id) ON DELETE CASCADE,
    query_id     BIGINT NOT NULL REFERENCES eval_queries(id) ON DELETE CASCADE,
    post_id      BIGINT NOT NULL,
    rank         INT    NOT NULL,
    score        REAL,
    latency_ms   REAL   NOT NULL,
    PRIMARY KEY (run_id, query_id, rank)
);