-- 0005_trash: deleted-page trash records (D-08/D-10). Operational/derived data
-- only — the canonical page content lives on disk under .okf-workspace/trash/ as
-- a real Git commit (delete = a git mv, never a git rm), NOT in this table. Each
-- row remembers where a deleted page came from (original_path), where it now
-- lives in trash (trash_path), its display title, and who deleted it + when, so
-- Restore knows the target folder and the trash view can show provenance.

CREATE TABLE IF NOT EXISTS trash (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    original_path TEXT NOT NULL,
    trash_path    TEXT NOT NULL,
    title         TEXT NOT NULL DEFAULT '',
    deleted_by    TEXT NOT NULL,
    deleted_at    TEXT NOT NULL DEFAULT (datetime('now'))
);
