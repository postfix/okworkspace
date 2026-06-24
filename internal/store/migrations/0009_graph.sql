-- 0009_graph: derived link/tag adjacency cache (Phase 8, LINK-01).
-- Operational/derived data ONLY — these tables are a REBUILDABLE CACHE of the
-- forward-link + tag adjacency that lives canonically in the Markdown files on
-- disk. Files remain the source of truth; deleting these tables and running
-- RebuildGraph from the .md files reproduces byte-identical rows. SQLite is
-- NEVER the source of truth (locked invariant — mirrors 0007_search.sql tone).
--
-- page_links holds one row per resolved internal forward edge (src links to a
-- dst .md that exists). Backlinks are the reverse query (SELECT src_path WHERE
-- dst_path = ?) — there is NO separate backlink table. External/absolute links
-- and dangling links to nonexistent .md files are not stored.
--
-- page_tags holds raw per-page frontmatter tag membership, matching exactly
-- what search.readTags reads from a page's frontmatter (so graph and search
-- agree on a page's tags).
--
-- graph_meta is a generic key/value table; its first key is `last_graphed_head`,
-- the Git HEAD SHA the graph was last built against. On startup the server
-- compares it to the current HEAD and rebuilds-from-files on a mismatch (drift
-- recovery — clones search_meta).

CREATE TABLE IF NOT EXISTS page_links (
    src_path TEXT NOT NULL,
    dst_path TEXT NOT NULL,
    PRIMARY KEY (src_path, dst_path)
);

-- Reverse lookup index for O(backlinks): SELECT src_path WHERE dst_path = ?.
CREATE INDEX IF NOT EXISTS idx_page_links_dst ON page_links (dst_path);

CREATE TABLE IF NOT EXISTS page_tags (
    page_path TEXT NOT NULL,
    tag       TEXT NOT NULL,
    PRIMARY KEY (page_path, tag)
);

-- Shared-tag join index (Phase 9 consumes; built now so the schema is stable).
CREATE INDEX IF NOT EXISTS idx_page_tags_tag ON page_tags (tag);

CREATE TABLE IF NOT EXISTS graph_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
