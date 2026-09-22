-- 0011_discovery.sql
-- Search & discovery: saved searches, alerts, trend snapshots.

-- Saved searches with DSL AST storage
CREATE TABLE IF NOT EXISTS saved_searches (
    id              BIGSERIAL PRIMARY KEY,
    name            TEXT NOT NULL,
    description     TEXT,
    dsl_query       TEXT NOT NULL,              -- human-readable DSL string
    dsl_ast         JSONB NOT NULL,             -- parsed AST for execution
    sql_where       TEXT,                       -- compiled SQL WHERE clause (cached)
    vector_params   JSONB,                      -- vector search params (cached)
    site_filter     TEXT,                       -- optional site name
    board_filter    TEXT,                       -- optional board code
    is_public       BOOLEAN NOT NULL DEFAULT FALSE,
    created_by      TEXT,                       -- user identifier (for multi-user future)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_evaluated  TIMESTAMPTZ,
    evaluation_count INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_saved_searches_public ON saved_searches (is_public) WHERE is_public = TRUE;
CREATE INDEX IF NOT EXISTS idx_saved_searches_eval ON saved_searches (last_evaluated);

-- Alert configurations for saved searches
CREATE TABLE IF NOT EXISTS search_alerts (
    id              BIGSERIAL PRIMARY KEY,
    saved_search_id BIGINT NOT NULL REFERENCES saved_searches(id) ON DELETE CASCADE,
    channel         TEXT NOT NULL,              -- 'webhook', 'email', 'log'
    target          TEXT NOT NULL,              -- webhook URL, email address, or 'stdout'
    threshold       INTEGER NOT NULL DEFAULT 1, -- min new hits to trigger
    cooldown_minutes INTEGER NOT NULL DEFAULT 60, -- min minutes between alerts
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    last_triggered  TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_search_alerts_search ON search_alerts (saved_search_id);
CREATE INDEX IF NOT EXISTS idx_search_alerts_enabled ON search_alerts (enabled) WHERE enabled = TRUE;

-- Alert firing history (audit trail)
CREATE TABLE IF NOT EXISTS alert_history (
    id              BIGSERIAL PRIMARY KEY,
    alert_id        BIGINT NOT NULL REFERENCES search_alerts(id) ON DELETE CASCADE,
    hit_count       INTEGER NOT NULL,
    new_post_ids    BIGINT[] NOT NULL,          -- post IDs that triggered this alert
    payload         JSONB NOT NULL,             -- what was sent to the channel
    status          TEXT NOT NULL,              -- 'sent', 'failed', 'skipped'
    error           TEXT,
    triggered_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_alert_history_alert ON alert_history (alert_id);
CREATE INDEX IF NOT EXISTS idx_alert_history_time ON alert_history (triggered_at);

-- Trend snapshots: hourly aggregates for volume charts
CREATE TABLE IF NOT EXISTS trend_snapshots (
    id              BIGSERIAL PRIMARY KEY,
    board_id        BIGINT NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    site_id         BIGINT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    term            TEXT NOT NULL,              -- keyword, n-gram, or 'total_posts'
    count           BIGINT NOT NULL,            -- occurrences in this window
    window_start    TIMESTAMPTZ NOT NULL,       -- start of aggregation window
    window_end      TIMESTAMPTZ NOT NULL,       -- end of aggregation window
    interval        TEXT NOT NULL,              -- 'hour', 'day', 'week'
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (board_id, term, window_start, interval)
);

CREATE INDEX IF NOT EXISTS idx_trends_board_term ON trend_snapshots (board_id, term);
CREATE INDEX IF NOT EXISTS idx_trends_time ON trend_snapshots (window_start);
CREATE INDEX IF NOT EXISTS idx_trends_interval ON trend_snapshots (interval);

-- Search hit logging (for saved search evaluation tracking)
CREATE TABLE IF NOT EXISTS search_hits (
    id              BIGSERIAL PRIMARY KEY,
    saved_search_id BIGINT NOT NULL REFERENCES saved_searches(id) ON DELETE CASCADE,
    post_id         BIGINT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    score           REAL,                       -- relevance score
    matched_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (saved_search_id, post_id)
);

CREATE INDEX IF NOT EXISTS idx_search_hits_search ON search_hits (saved_search_id);
CREATE INDEX IF NOT EXISTS idx_search_hits_post ON search_hits (post_id);
CREATE INDEX IF NOT EXISTS idx_search_hits_time ON search_hits (matched_at);