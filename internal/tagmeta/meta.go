// Package tagmeta owns the on-disk tag-metadata store (Phase 32, TF-02).
//
// Tag metadata (color + description) is durable CONTENT, not operational data:
// it persists as one Markdown file per tag under a top-level Tags/ folder
// (Tags/{tag}.md, YAML frontmatter: tag, color, description) — the files-as-truth
// invariant. A SQLite tag_meta cache is a rebuildable derived cache: deleting
// the table and running RebuildTagMeta reproduces identical rows. SQLite is
// NEVER the source of truth (mirrors the 0009_graph.sql discipline).
//
// SetMeta writes Tags/{tag}.md through repo.Write + the SAME single-writer
// EnqueueCommit path every page save uses (no second commit path).
package tagmeta

// Meta is one parsed tag-metadata record: the tag name (canonical, normalized),
// an optional color (hex #RRGGBB/#RGB or a fixed palette token; "" = default),
// an optional single-line HTML-safe description ("" = none), and an optional
// parent tag name (normalized; "" = root, D-01/TF-03). The parent forms a
// cycle-safe hierarchy (checkCycle rejects a tag being its own ancestor).
type Meta struct {
	Tag         string
	Color       string
	Description string
	Parent      string
}