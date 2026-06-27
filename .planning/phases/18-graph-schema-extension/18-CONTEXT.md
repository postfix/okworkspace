---
author: architect
---

# Phase 18: Graph Schema Extension - Context

**Gathered:** 2026-06-28
**Status:** Ready for planning

<domain>
## Phase Boundary

Extend the backend `GraphNode` struct and graph store to populate `aliases`, `tags`, `exists`, and `type: "attachment"` — activating the 3 inert Phase 17 filter toggles. Pure backend data plumbing. No new UI.

**In scope:** GraphNode struct extension, rebuild/extract changes, TypeScript type update, removal of type assertions in filter.ts.

**Out of scope:** Graph group CRUD, attachment text extraction for graph, mobile layout.
</domain>

<decisions>
- **D-01:** `GraphNode` gains `Aliases []string` (from YAML frontmatter `aliases` key), `Tags []string` (from frontmatter `tags`), `Exists bool` (true for live pages, false for unresolved link targets).
- **D-02:** `GraphNode.Type` extended to `"attachment"` for non-.md files (images, PDFs, etc.). Attachment nodes created during rebuild from repo Tree items that aren't .md and aren't trashed.
- **D-03:** `pageAliases()` extractor added to `extract.go`, mirroring `pageTags()` pattern — reads `aliases` key from frontmatter yaml.Node.
- **D-04:** Unresolved link targets (pages referenced in page_links but not on disk) get `Exists: false` — these become filterable by the "Existing files only" toggle.
- **D-05:** TypeScript `GraphNode` type in `client.ts` updated to match extended Go struct. Filter.ts removes all `as` type assertions.
</decisions>

<canonical_refs>
- `internal/graph/query.go` — GraphNode struct, GraphData, Neighborhood (node construction points)
- `internal/graph/rebuild.go` — RebuildGraph (link/tag extraction + bulk insert)
- `internal/graph/extract.go` — pageTags, outboundLinks (extraction helpers to mirror)
- `web/src/api/client.ts` — TypeScript GraphNode type
- `web/src/lib/graph/filter.ts` — matchesSearch, filterNodes (type assertions to remove)
- `.planning/REQUIREMENTS.md` §GF-02
</canonical_refs>
