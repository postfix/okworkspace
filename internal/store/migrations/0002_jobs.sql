-- 0002_jobs: async job queue (SPEC §16.1 Job service). Operational/derived data
-- only — job payloads reference repo paths, never store wiki content.
-- The single-writer worker drains due jobs FIFO and applies retry-with-backoff.

CREATE TABLE IF NOT EXISTS jobs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    kind       TEXT    NOT NULL,
    payload    TEXT    NOT NULL DEFAULT '',
    status     TEXT    NOT NULL DEFAULT 'pending', -- pending | running | done | failed
    attempts   INTEGER NOT NULL DEFAULT 0,
    last_error TEXT    NOT NULL DEFAULT '',
    run_after  TEXT    NOT NULL DEFAULT (datetime('now')),
    created_at TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- Index supporting the worker's "due pending jobs, oldest first" drain query.
CREATE INDEX IF NOT EXISTS jobs_status_run_after_idx ON jobs (status, run_after);
