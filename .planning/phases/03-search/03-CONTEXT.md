# Phase 3: Search - Context

**Gathered:** 2026-06-21
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous) — 4 grey areas, all recommendations accepted by user

<domain>
## Phase Boundary

A user can quickly find any knowledge in the workspace — across page titles, body, tags,
attachment filenames, and extracted attachment text — with typed results (page / heading /
attachment). In scope: SRCH-01..SRCH-06. Built on Bleve v2.6.0 (LOCKED, pure-Go), indexing
page content (Phase 1) + extracted attachment text (Phase 2).

Out of scope: the Eino agent (Phase 4), collaboration (Phase 5).
</domain>

<decisions>
## Implementation Decisions

### Index architecture & lifecycle (Area 1 — accepted)
- **Bleve index on disk under the data dir** (e.g. `<data_dir>/index/`), **NOT in Git** — it
  is a derived, rebuildable artifact (files remain the source of truth).
- **Incremental `IndexJob`** on the EXISTING `internal/jobs` worker (new `KindIndex`),
  triggered on page-save (CommitJob done) and extraction-done, **plus a full
  rebuild-from-files** path.
- **Drift recovery:** persist the last-indexed Git HEAD; on startup, if HEAD differs from the
  last-indexed HEAD, trigger a rebuild (defense against SQLite/Bleve/disk drift). Also expose
  an **admin "Reindex" action**.
- **Indexed documents:** pages (title / body / tags / headings) and attachments (original
  filename + extracted `.txt` text), each as a TYPED Bleve document.

### Query & relevance (Area 2 — accepted)
- **Per-field index mapping:** title (boosted for relevance), body, tags (keyword/faceted),
  filename, extracted-text. Headings indexed for deep-link results.
- **Match query with prefix + fuzzy (typo tolerance) + phrase support**; title boosted.
- **Facet by result type** (page / heading / attachment) — the richer faceting that motivated
  choosing Bleve over SQLite FTS5.
- **Bleve fragment highlighting** of matched terms for result snippets.

### Result types & UX (Area 3 — accepted)
- **Typed results (SRCH-06):** page / heading / attachment. A heading result deep-links to the
  page section; an attachment result links to its **owning page** (SRCH-05).
- **Obsidian-style ⌘K quick-switcher** (top-bar / keyboard-triggered command palette) opening a
  results panel — fits the "mimic Obsidian" UI direction (team are ex-Obsidian users).
- **Result row:** type badge + title + highlighted snippet; click navigates in-app. No Git
  vocabulary anywhere.

### Scope & access (Area 4 — accepted)
- **Any authenticated user** may search (matches the page-read authorization model).
- **Trashed pages are EXCLUDED** from results (live pages only).
- **Reindex triggers:** page create / edit / rename / move / delete + attachment upload /
  replace / remove + extraction-done all keep the index live (incremental).
- **Clear empty + "no results" states**, no Git vocabulary.

### Claude's Discretion
- Exact Bleve index mapping/analyzer config, query builder shape, snippet length, ⌘K component
  layout, and the index-version/HEAD bookkeeping mechanism are at Claude's discretion,
  consistent with Phase 0–2 patterns.
</decisions>

<code_context>
## Existing Code Insights

- **Job worker** to extend: `internal/jobs` — register a new `KindIndex` handler (mirrors
  `KindCommit` from Phase 1 and `KindExtract` from Phase 2). Reuse the fire-and-forget
  `Enqueue` (NOT `EnqueueAndWait` from inside the worker — see Phase 2 CR-01).
- **Index sources:** page content via `internal/okf` (parsed Doc: frontmatter title/tags +
  body + headings) and `internal/pages` (tree, paths); attachment extracted text + meta via
  `internal/attachments` (the `<id>.txt` sidecar + `<id>.json` meta, operational `attachments`
  table). Trash state via `internal/pages/trash.go`.
- **Config:** add a `search` section (already a parsed-but-unused placeholder per Phase 0
  `config.go` — `Search.Enabled`, `engine: bleve`); index dir derives from `storage.data_dir`.
- **HTTP/RBAC:** search endpoint mounts under the `authed` group (`internal/server/router.go`);
  admin reindex under the admin subgroup. Safe-path resolver (SEC-01) on any file access.
- **Frontend:** `web/src/api/client.ts` query patterns; existing components (Dialog, LeftTree,
  AutosaveStatus) + tokens.css for the ⌘K palette; react-query for the search query.
- **Hidden-Git discipline** and the design system established in Phases 0–2 must carry over.
</code_context>

<specifics>
## Specific Ideas

- The rebuild-from-files reindex job is the primary engineering concern (the defense against
  index drift) — make it robust and idempotent.
- Search must find an attachment by its extracted text and surface the PAGE it belongs to.
- Keep results fast and typed; ⌘K should feel instant for a 5-user workspace.
</specifics>

<deferred>
## Deferred Ideas

- Search analytics / ranking tuning beyond Bleve defaults + title boost.
- Cross-workspace or saved searches.
</deferred>
