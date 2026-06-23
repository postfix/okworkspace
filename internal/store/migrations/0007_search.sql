-- 0007_search: search operational metadata (Phase 3, SRCH-01..06).
-- Operational/derived data ONLY — the canonical search index is the Bleve scorch
-- index on disk under <data_dir>/index/ (a derived, rebuildable artifact, NOT in
-- Git; files remain the source of truth). These tables hold the small bookkeeping
-- the index lifecycle needs.
--
-- search_meta is a generic key/value table; its first key is `last_indexed_head`,
-- the Git HEAD SHA the index was last built against. On startup the server
-- compares it to the current HEAD and rebuilds-from-files on a mismatch (drift
-- recovery — the phase's primary correctness backstop).
--
-- page_headings tracks the heading-doc IDs a page currently contributes to the
-- index, so that on a body edit the stale heading sub-documents (whose IDs encode
-- the old anchor) can be deleted before re-indexing the new set. Created now so
-- the migration sequence is stable; it is POPULATED by 03-03 (heading deep-links),
-- not by this plan (pages-only scope).

CREATE TABLE IF NOT EXISTS search_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS page_headings (
    page_path  TEXT NOT NULL,
    heading_id TEXT NOT NULL,
    PRIMARY KEY (page_path, heading_id)
);
