# Requirements: OKF Workspace — v1.0 Knowledge Graph & LLM Auto-Tagging

**Defined:** 2026-06-24
**Core Value:** A non-technical teammate can create, edit, and find knowledge — and get useful AI help on it — while every byte stays as plain Markdown + original files on disk, versioned in Git, with no proprietary store to escape.

## v1.0 Requirements

Requirements for this milestone. Each maps to exactly one roadmap phase (see Traceability).

### Link & Backlink Index (LINK)

Derived, rebuildable-from-files adjacency that powers both the graph and the backlinks panel. SQLite cache, never source of truth.

- [x] **LINK-01**: System maintains a derived page-link / backlink index, built from existing Markdown link parsing, kept fresh on every page mutation (create / save / rename / move / delete-to-trash / restore)
- [x] **LINK-02**: User can see a "Referenced by" (backlinks) panel on a page that lists every page linking to it, each entry click-to-navigate
- [x] **LINK-03**: Admin can rebuild the link/graph index from files (recovery backstop), consistent with the existing "rebuild search index" affordance

### Knowledge Graph (GRAPH)

Obsidian-style force-directed visualization. 2D Canvas renderer (`react-force-graph-2d`), lazy-loaded route; no WebGL/three.js.

- [x] **GRAPH-01**: User can open a global graph view showing all pages as nodes and links as edges, with zoom/pan and click-a-node-to-open-its-page
- [x] **GRAPH-02**: Node size reflects connection count (degree); orphan (unlinked) pages are visible and visually distinguishable
- [x] **GRAPH-03**: User can open a per-page local graph panel showing the current page plus its direct neighbors, auto-updating when the active page changes, with a depth control (default 1 hop)
- [x] **GRAPH-04**: User can toggle edge types on/off in the graph UI (page links / backlinks / shared tags), Obsidian-style; shared-tag edges default OFF and are thresholded (≥2 shared tags, popular tags capped) to avoid a hairball
- [x] **GRAPH-05**: Hovering a node highlights that node and its immediate neighbors/edges

### LLM Auto-Tagging (TAG)

Suggest→approve only — no silent frontmatter writes. Reuses the existing Eino single-shot path + propose→approve→apply→commit safety flow.

- [ ] **TAG-01**: User can request LLM tag suggestions on demand for the page they are viewing/editing
- [ ] **TAG-02**: Suggested tags are presented for human review and approved/rejected per tag before anything is written; new vs. already-existing tags are distinguished and newly-invented tags default to unchecked
- [ ] **TAG-03**: Approved tags are merged into the page's YAML frontmatter `tags` field byte-stably (only the `tags` lines change; body and other frontmatter untouched) and committed through the single-writer Git path; a stale page revision 409s rather than clobbering
- [ ] **TAG-04**: Tag suggestions are biased toward the existing workspace tag vocabulary (existing tags fed into the prompt) and capped in count (max 5 per page), normalized on write (lowercase, trimmed, deduped) to prevent tag explosion
- [ ] **TAG-05**: Admin can run a bulk tagging sweep over untagged (or all) pages that enqueues per-page suggestion jobs on the existing async worker and produces a queue of pending suggestions — writing nothing automatically
- [ ] **TAG-06**: User can review the bulk-sweep suggestion queue and approve/reject suggestions per page, with approvals routed through the same byte-stable TAG-03 apply path and batched commits

## v2 Requirements

Deferred to a future release. Tracked, not in this roadmap.

### Graph (GRAPH)

- **GRAPH-F1**: Tag/group coloring in the graph (color nodes by tag or folder)
- **GRAPH-F2**: In-graph search/filter (highlight or isolate matching nodes)
- **GRAPH-F3**: Local-graph "include neighbors-of-neighbors links" toggle

### Backlinks (LINK)

- **LINK-F1**: Backlink context lines (show the linking sentence/snippet in the backlinks panel)

### Tagging (TAG)

- **TAG-F1**: Embedding-based near-synonym tag merge/suggestion

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Embedding/vector store for tagging | Breaks the single CGO-free binary + files-as-truth simplicity; deferred to v2 |
| Nested / hierarchical tags | Undefined storage + UX model; not needed for v1.0 value |
| Attachment tagging | Attachments have no frontmatter `tags` storage model defined |
| 3D / WebGL / VR graph (`react-force-graph` umbrella, three.js) | Bloats the embedded binary; 2D Canvas is sufficient at this scale |
| Silent / automatic tag application (no review) | Violates the locked agent safety model — all agent writes require human approval |
| Saved graph filters / named color-group presets | Polish beyond v1.0 table stakes |
| Graph DB (Neo4j/Dgraph/etc.) | Violates single-binary + files-as-truth; SQLite-derived adjacency suffices |

## Traceability

Which phases cover which requirements. Filled in during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| LINK-01 | Phase 8 | Complete |
| LINK-02 | Phase 9 | Complete |
| LINK-03 | Phase 8 | Complete |
| GRAPH-01 | Phase 10 | Complete |
| GRAPH-02 | Phase 10 | Complete |
| GRAPH-03 | Phase 10 | Complete |
| GRAPH-04 | Phase 10 | Complete |
| GRAPH-05 | Phase 10 | Complete |
| TAG-01 | Phase 11 | Pending |
| TAG-02 | Phase 11 | Pending |
| TAG-03 | Phase 11 | Pending |
| TAG-04 | Phase 11 | Pending |
| TAG-05 | Phase 12 | Pending |
| TAG-06 | Phase 12 | Pending |
