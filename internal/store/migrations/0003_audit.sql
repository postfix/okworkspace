-- 0003_audit.sql — the SEC-05 audit log mirror.
-- SQLite is operational metadata ONLY (never wiki content); an audit trail of
-- who-did-what is operational metadata, so mirroring it here is in policy.
-- audit.Record writes one row here AND emits one structured slog line per event.
CREATE TABLE IF NOT EXISTS audit_log (
    id         INTEGER PRIMARY KEY,
    action     TEXT NOT NULL,
    actor      TEXT,
    target     TEXT,
    detail     TEXT,
    source     TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log (created_at);
