# Milestone v1.2 Requirements — Graph Schema Extension

## Overview

Close the Phase 17 gap: extend the backend graph data model so the 3 inert filter toggles (Attachments, Existing files only, search by aliases/tags) become functional. Pure backend data plumbing — no new UI, no new endpoints.

---

## Graph Schema Extension

### GF-02: Backend Graph Schema Extension

**Requirement:** Extend the backend `GraphNode` struct and graph store to include `aliases`, `tags`, `exists`, and `type: "attachment"` fields so the Phase 17 filter sidebar toggles operate on real data instead of dead code.

**Acceptance criteria:**
- [ ] `GraphNode` struct extended with `Aliases []string`, `Tags []string`, `Exists bool` fields (Go)
- [ ] `GraphNode.Type` extended to include `"attachment"` for attachment file nodes
- [ ] Graph store (rebuild/incremental) populates `Aliases` and `Tags` from page frontmatter
- [ ] Graph store populates `Exists: true` for pages that exist on disk, `false` for unresolved links
- [ ] Graph store creates attachment nodes for uploaded files (PDF, DOCX, images, etc.) with `type: "attachment"`
- [ ] `web/src/api/client.ts` `GraphNode` TypeScript type updated to include `aliases?`, `tags?`, `exists?`, `type: "page" | "tag" | "attachment"`
- [ ] `web/src/lib/graph/filter.ts` `matchesSearch` uses real `node.aliases` and `node.tags` (remove type assertions)
- [ ] `web/src/lib/graph/filter.ts` `filterNodes` uses real `node.type` and `node.exists` (remove type assertions)
- [ ] Attachments toggle filters attachment nodes in/out of the graph
- [ ] Existing files only toggle hides nodes where `exists === false`
- [ ] Search input matches against aliases and tags (not just label + path)
- [ ] All 17 GF-01 criteria from v1.1 pass (currently 12/15 — this closes the last 3)
- [ ] `go test ./internal/graph/...` and `npx tsc --noEmit` clean

**Notes:**
- This is the gap-closure milestone for Phase 17's accepted tech debt
- No new UI components needed — FilterControls, filter.ts, and graphFilters store are already built
- Attachment nodes should only appear when the Attachments toggle is ON (OFF by default per GF-01)

---

## Traceability

| REQ-ID | Phase | Status |
|--------|-------|--------|
| GF-02 | Phase 18 | Not started |

---

## Out of Scope

- **Graph group CRUD (edit/delete/reorder)** — Deferred, "New group" button exists
- **Attachment text extraction for graph** — Attachments appear as nodes, their text content is not indexed in the graph
- **Mobile-specific filter layout** — Current layout works on desktop; mobile is future polish
- **Real-time collaboration (CRDT)** — Out of MVP scope per v0.9.9
