-- 0013_tag_op_journal: tag-operation undo journal (Phase 35, TF-05).
-- Operational/derived data ONLY — the invertible record of each tag-admin op
-- (rename/merge/delete/bulk), used to undo the LAST op. NOT content (not
-- user-authored; machine-generated operational state). Mirrors 0010_tag_suggestions
-- tone: SQLite is appropriate for operational state, not for content.
-- Append-only: a new row is inserted after every successful tag-admin apply.
-- Only the LAST row is undoable in v1.6 (last-op undo). A successful undo also
-- inserts a row for the undo op itself, so undo-of-undo (redo) works for free.
CREATE TABLE IF NOT EXISTS tag_op_journal (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    op_type    TEXT NOT NULL,          -- "rename" | "merge" | "delete" | "bulk" | "undo"
    op_detail  TEXT NOT NULL,          -- JSON: op-specific data
    pages      TEXT NOT NULL,          -- JSON: [{page_path, pre_op_tags, body_affected}]
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);