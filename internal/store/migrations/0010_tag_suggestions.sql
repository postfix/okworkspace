-- 0010_tag_suggestions: bulk-sweep staging queue (Phase 12, TAG-05).
-- Operational/derived STAGING data ONLY — this table holds PENDING tag
-- suggestions produced by a bulk sweep awaiting an explicit human approve. It is
-- NOT page content and is NEVER the source of truth: the Markdown files on disk
-- remain truth. Deleting this table and re-running a sweep reproduces the rows
-- (each row is a re-derivable proposal, not a fact). A tag is NEVER written to a
-- file FROM this table without an explicit human-approved apply — the sweep only
-- PRODUCES pending rows; the write happens solely via the Phase-11 byte-stable
-- apply path (mirrors the 0009_graph.sql "rebuildable cache, never truth" tone).
--
-- Lifecycle: status starts 'pending' and a later batch approve marks the row
-- 'resolved' (Wave 2). suggestions is a JSON array of {tag, existing} objects —
-- the staged proposal exactly as Phase-11 SuggestTags returned it (tags + the
-- existing-vs-new flags). base_revision is the page's optimistic-concurrency
-- token captured AT suggestion time, re-checked on apply (a stale page 409s).

CREATE TABLE IF NOT EXISTS tag_suggestions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    page_path     TEXT NOT NULL,
    suggestions   TEXT NOT NULL,                         -- JSON: [{"tag":..,"existing":..}]
    base_revision TEXT NOT NULL,                         -- captured at suggestion time
    status        TEXT NOT NULL DEFAULT 'pending',       -- pending -> resolved
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

-- At most ONE pending row per page: re-staging a page supersedes its prior
-- pending suggestion (the StagePending DELETE-then-INSERT relies on this).
-- A partial unique index so resolved rows (history) do not block re-staging.
CREATE UNIQUE INDEX IF NOT EXISTS idx_tag_suggestions_pending_page
    ON tag_suggestions (page_path) WHERE status = 'pending';

-- The pending-list read filters on status.
CREATE INDEX IF NOT EXISTS idx_tag_suggestions_status ON tag_suggestions (status);
