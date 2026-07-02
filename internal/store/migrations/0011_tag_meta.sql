-- 0011_tag_meta: tag metadata cache (Phase 32, TF-02).
-- Operational/derived data ONLY — the canonical tag metadata lives on disk as
-- Tags/{tag}.md files (files-as-truth). Deleting this table and running
-- RebuildTagMeta reproduces identical rows. SQLite is NEVER the source of truth
-- (mirrors 0009_graph.sql tone).

CREATE TABLE IF NOT EXISTS tag_meta (
    tag         TEXT PRIMARY KEY,
    color       TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT ''
);