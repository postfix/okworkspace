-- 0006_attachments: attachment operational metadata (Phase 2, ATT-01..10).
-- Operational/derived data ONLY — the canonical attachment lives on disk under
-- attachments/<id>.<ext> (byte-exact original) + attachments/<id>.json (the meta
-- sidecar, the source of truth for the original filename) + attachments/<id>.txt
-- (extracted text), each a real hidden-Git commit. This table mirrors the meta so
-- the per-page list and the extraction-status chip can be served without walking
-- the working tree. id is the server-generated opaque ULID (never the original
-- filename — SEC-02). extract_status is one of pending/done/empty/failed.

CREATE TABLE IF NOT EXISTS attachments (
    id            TEXT PRIMARY KEY,
    page_path     TEXT NOT NULL,
    original_name TEXT NOT NULL,
    mime_type     TEXT NOT NULL,
    size_bytes    INTEGER NOT NULL DEFAULT 0,
    uploader_name TEXT NOT NULL DEFAULT '',
    uploaded_at   TEXT NOT NULL DEFAULT (datetime('now')),
    extract_status TEXT NOT NULL DEFAULT 'pending',
    extract_error TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_attachments_page_path ON attachments (page_path);
