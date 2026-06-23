# Phase 8: Derived Link/Tag Store & Maintenance - Context

**Gathered:** 2026-06-24
**Status:** Ready for planning
**Mode:** Auto-generated (infrastructure foundation phase — smart-discuss grey-area questioning skipped; design pre-resolved by v1.0 ARCHITECTURE research)

<domain>
## Phase Boundary

Deliver a derived, rebuildable-from-files adjacency store — page→page links, the reverse backlink edges, and per-page tag membership — that stays fresh on every page mutation. This is the **foundation layer** both the graph UI (Phases 9–10) and LLM tag-vocabulary prompting (Phases 11–12) depend on. The store is a *cache*: SQLite holds the adjacency, but the Markdown files on disk remain the only source of truth, and deleting + rebuilding the store from files must reproduce identical adjacency.

**In scope:** the link/backlink/tag SQLite tables; the incremental maintenance job wired into every page mutation; a full rebuild-from-files path; an admin "rebuild" affordance consistent with the existing "Rebuild search index" button; queryable tag membership.

**Out of scope (later phases):** HTTP graph endpoints + backlinks panel (Phase 9), graph visualization UI (Phase 10), any LLM tagging (Phases 11–12), shared-tag *edge* computation/thresholding (Phase 9 owns the edge query; this phase only stores raw `page_tags` membership).

</domain>

<decisions>
## Implementation Decisions

### Storage model (from v1.0 ARCHITECTURE research — verified against source)
- New SQLite tables in the existing operational DB: `page_links` (forward edges: src_path → dst_path) and `page_tags` (page_path → tag). Backlinks are the reverse query on `page_links` — no separate table.
- Both tables are **derived caches**, never source of truth — rebuildable from the Markdown files on disk (mirrors how the Bleve index relates to files today).
- Use a schema migration consistent with the existing migration pattern (the v0.9.9 code already uses numbered migrations, e.g. `0008` for `delete_group_id`).

### Freshness mechanism (mirror the existing Bleve incremental-index pattern exactly)
- Add a new `KindGraph` job kind to the existing single-drain `internal/jobs` worker, mirroring the existing `KindIndex` search-index job.
- Enqueue it fire-and-forget right beside the existing search `enqueueIndexUpsert`/index-enqueue call in EVERY `pages` mutation: create, save, create-folder, rename, move, delete-to-trash, restore (and folder rename/move/delete from Phase 7). On rename/move, re-scan all affected linker pages so inbound edges are rewritten — same discipline the existing structural link-rewrite + Bleve re-index already follow.
- A page delete/trash removes its rows; restore re-adds them. No app restart required for any mutation to be reflected.

### Link & tag extraction (reuse existing parsers — do NOT re-parse with a new scanner)
- Forward links: reuse the existing structural link finder in `internal/okf` (`okf.FindLinks` / the scanner used by `rewriteFolderInboundLinks`) that already skips fenced/inline code and resolves relative `.md` links via `path.Clean(path.Join(...))`. Unresolved/dangling links are naturally dropped by the resolver (do not surface them as a feature this phase).
- Tag membership: reuse the existing sequence-aware frontmatter tag reader used by search (`search.readTags`) so `page_tags` exactly matches what search already understands as a page's tags.

### Rebuild-from-files (mirror existing search rebuild)
- Provide a full `RebuildGraph` path that walks all pages on disk and repopulates `page_links` + `page_tags`, mirroring the existing `search` rebuild (`internal/search/rebuild.go` / `RebuildIndex`).
- Run a startup-drift rebuild consistent with existing startup behavior so the store self-heals if it diverges from files.
- Admin affordance: an endpoint + button that mirrors the existing "Rebuild search index" admin control (same RBAC gating, same UI placement pattern). Reuse, don't reinvent.

### Claude's Discretion
- Exact table/column names, index choices, migration number, and package layout (`internal/graph` is the research's suggested home but the planner may choose) are at Claude's discretion, provided the above contracts hold.
- Whether `page_links`/`page_tags` maintenance share one `KindGraph` job or split is at Claude's discretion — single job preferred for simplicity.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets (verified present in repo)
- `internal/search/indexjob.go` — the `KindIndex` job + `enqueueIndexUpsert` enqueue pattern to mirror for `KindGraph`.
- `internal/search/rebuild.go` (`RebuildIndex`) — the rebuild-from-files pattern to mirror for `RebuildGraph`.
- `internal/search/service.go` (`readTags`) — sequence-aware frontmatter tag reader; reuse for `page_tags`.
- `internal/okf/links.go` (`FindLinks`) + the resolver in `pages` `rewriteFolderInboundLinks` — structural link extraction + relative-link resolution; reuse for `page_links`.
- `internal/jobs/` single-drain worker (retry/backoff, fire-and-forget) — host the new `KindGraph` job here.
- The existing "Rebuild search index" admin endpoint + button — clone its shape (RBAC, route group, UI control) for the graph rebuild.

### Established Patterns
- Operational metadata only in SQLite (`modernc.org/sqlite`, CGO-free); content stays in files.
- Numbered idempotent migrations (latest seen: `0008`).
- Mutations enqueue fire-and-forget jobs on the single worker; reads are separate.
- A named regression/concurrency test (`-race`) proving the re-index/job path doesn't deadlock the drain goroutine (e.g. the existing `CR-01` search test) — replicate for `KindGraph`.

### Integration Points
- Every `pages.Service` mutation method (create/save/createfolder/rename/move/delete/restore + folder ops) — add the graph enqueue beside the existing search enqueue.
- App startup (`cmd/okf-workspace` / server wiring) — register `KindGraph` handler + startup-drift rebuild.
- Admin handlers/router group — add the rebuild endpoint mirroring search rebuild.

</code_context>

<specifics>
## Specific Ideas

- The derived-only invariant is a hard success criterion: include a test that deletes the store, rebuilds from files, and asserts identical adjacency (SQLite never source of truth).
- Mutation-freshness must be proven for ALL mutation kinds (create/save/rename/move/delete-to-trash/restore + folder rename/move/delete), since stale edges on rename/move are the #1 pitfall flagged by research (PITFALLS.md pitfall 2).
- Keep `page_tags` raw membership only — shared-tag edge thresholding/clique-capping is Phase 9's concern, not this phase's.

</specifics>

<deferred>
## Deferred Ideas

- Surfacing dangling/unresolved internal links as a distinct UI affordance — out of v1.0 scope (resolver drops them silently).
- Shared-tag edge computation + popular-tag cap — Phase 9.
- Graph payload shaping / endpoints — Phase 9.

</deferred>
