-- 0004_drafts: autosave drafts (D-02). Operational/derived data ONLY — this
-- table is NEVER canonical page content. The .md file on disk is truth; a draft
-- is an in-progress autosave buffer keyed by page path + user. The canonical
-- .md is written only when the batched CommitJob fires (D-01/D-03), so the
-- working tree stays byte-equal to the last Git commit.

CREATE TABLE IF NOT EXISTS drafts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    page_path   TEXT    NOT NULL,
    user_id     INTEGER NOT NULL,
    body        TEXT    NOT NULL DEFAULT '',
    frontmatter TEXT    NOT NULL DEFAULT '',
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(page_path, user_id)
);
