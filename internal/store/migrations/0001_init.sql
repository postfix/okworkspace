-- 0001_init: operational schema for OKF Workspace.
-- SQLite holds operational/derived data ONLY (users, sessions). It is NEVER the
-- source of truth for wiki content (files-as-truth invariant, SPEC §8.1).

CREATE TABLE IF NOT EXISTS users (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    username             TEXT    NOT NULL UNIQUE,
    display_name         TEXT    NOT NULL,
    role                 TEXT    NOT NULL,
    password_hash        TEXT    NOT NULL,
    must_change_password INTEGER NOT NULL DEFAULT 0,
    active               INTEGER NOT NULL DEFAULT 1,
    created_at           TEXT    NOT NULL
);

-- sessions table required by the alexedwards/scs SQLite store.
CREATE TABLE IF NOT EXISTS sessions (
    token  TEXT PRIMARY KEY,
    data   BLOB NOT NULL,
    expiry REAL NOT NULL
);

CREATE INDEX IF NOT EXISTS sessions_expiry_idx ON sessions (expiry);
