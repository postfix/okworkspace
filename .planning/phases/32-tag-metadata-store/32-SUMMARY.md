---
author: engineer
responsible: engineer
phase: 32
status: complete
---

# Phase 32: Tag Metadata Store (color/description) — Summary

## Goal

Introduce an on-disk tag-metadata store (`Tags/*.md` files, files-as-truth) with a
rebuildable SQLite cache + migration; let an admin set a tag's color + description,
surfaced in the admin tag list and on rendered CM6 badges.

## Requirements

TF-02 — tag metadata (color + description) persists as content (Tags/{tag}.md),
with a rebuildable SQLite cache and server-validated admin endpoint.

## Work completed

### Task 1: Backend — tagmeta package + migration + tree/graph/search skip (prior engineer, commit ef0c003)

- `internal/tagmeta/store.go` — `Store` with `GetMeta`/`AllMeta`/`SetMeta`/`ClearMeta`/
  `RebuildTagMeta` + single-writer commit path (reuses `pages.CommitEnqueuer` — the SAME
  path page saves use, never a second commit kind).
- `internal/tagmeta/meta.go` — `Meta{Tag, Color, Description}` struct.
- `internal/tagmeta/validate.go` — `ValidateColor` (hex #RRGGBB/#RGB or fixed palette
  token: red/blue/green/amber/purple/gray; empty = clear) + `ValidateDescription`
  (single-line, ≤200 chars, rejects HTML metacharacters `<>&"'`).
- `internal/store/migrations/0011_tag_meta.sql` — derived cache table (mirrors 0009_graph
  tone: operational data only, rebuildable from disk).
- `internal/pages/tree.go` — `Tags/` added to `isSkippedDir` (not a navigable folder).
- `internal/graph/rebuild.go` + `internal/search/rebuild.go` + `internal/graph/query.go` —
  `Tags/` skip in graph/search rebuilds (reserved folder, not page content).
- `internal/tagmeta/store_test.go` — 9 tests: TestSetGetMeta, TestRebuildTagMeta,
  TestRebuildParity, TestEmptyStore, TestValidateColor, TestValidateDescription,
  TestSetMetaWritesFile, TestClearMetaDeletesFile, TestRebuildSortsByTag.

### Task 2: Backend — admin endpoint + route + audit (prior engineer, commit ef0c003)

- `internal/audit/audit.go` — `ActionTagMeta = "tag_meta"` audit constant.
- `internal/server/handlers_tags_admin.go`:
  - `tagMetaRequest`/`tagMetaResponse` types.
  - `handleTagMeta` — POST /admin/tags/meta: validates tag (ValidateTagCharset), color
    (ValidateColor), description (ValidateDescription); calls `tagmeta.SetMeta` (writes
    Tags/{tag}.md + commits + upserts cache); records audit event; returns response.
    Empty color + description = clear (file deleted).
  - `handleTagList` — extended to merge per-tag color + description from `tagmeta.AllMeta`
    into the tag list response (one call; tags with no metadata get empty strings).
- `internal/server/router.go` — `admin.Post("/admin/tags/meta", h.handleTagMeta)` route
  registered in the admin subgroup (admin-only RBAC + CSRF inherited).
- `internal/server/handlers_auth.go` — `tagmeta *tagmeta.Store` field on `authHandlers`.
- `internal/server/router.go` — `Tagmeta *tagmeta.Store` field on `Deps`.

### Task 2: Server-side tests (this engineer)

- `internal/server/handlers_tags_admin_test.go`:
  - `newTagAdminServer` fixture extended to wire `tagmeta.NewStore(contentRepo, st.DB(), w, false)`
    into `Deps.Tagmeta` (and the fixture struct).
  - `TestTagMetaRBAC` — editor gets 403 on POST /admin/tags/meta.
  - `TestTagMetaSetGet` — admin sets color `#4caf50` + description `"Deploy ops"`; GET
    /admin/tags reflects them; Tags/deploy.md exists on disk with correct frontmatter.
  - `TestTagMetaValidation` — invalid color (`url(x)`, `var(--x)`), invalid description
    (newline, >200 chars, `<script>`), invalid tag (`a/b`), empty tag → all 400.
  - `TestTagMetaClear` — set metadata, then clear with empty color + description → file
    deleted, GET /admin/tags shows empty metadata.
  - `TestTagMetaAbsent` — GET /admin/tags with no Tags/ folder → tags render with empty
    color/description (graceful degradation).

### Task 3: Frontend — badge color + client API + admin UI (this engineer)

- `web/src/lib/cm/widgets.ts` — `HashtagWidget` extended with optional `color?: string`
  constructor parameter; `toDOM()` applies `span.style.color = this.color` when set (NO
  innerHTML — the no-innerHTML XSS invariant holds; color flows through `style.color` only).
  `eq()` compares `other.color === this.color` so color changes re-render the widget.
- `web/src/lib/cm/livePreview.ts` — new `tagColors` Facet (Record<string, string> tag→color,
  combined via Object.assign). `buildDecorations` reads the facet and passes `colors[tag]`
  to `new HashtagWidget(tag, colors[tag])`. Tags with no color entry fall back to the
  .cm-hashtag CSS default.
- `web/src/api/client.ts` — `TagEntry` extended with `color?: string` + `description?: string`.
  New `setTagMeta({tag, color, description})` function (POST /admin/tags/meta via `mutate`).
- `web/src/components/admin/TagManagement.tsx`:
  - Color swatch (small colored dot) beside each tag name in the table.
  - "Edit metadata" (Palette icon) button per tag row → opens `TagMetaDialog`.
  - `TagMetaDialog` — color picker (`<input type="color">` for hex + `<select>` for palette
    tokens) + description text input (maxLength=200). Save calls `setTagMeta`; invalidate
    TAGS_KEY. Clear button resets color to empty.
- `web/src/components/admin/TagManagement.css` — CSS for `.tagmgmt-color-dot`,
  `.tagmgmt-color-row`, `.tagmgmt-color-picker`, `.tagmgmt-color-palette`.

### main.go wiring (this engineer)

- `cmd/okf-workspace/main.go` — `tagmeta` import; `tagmetaStore` constructed after
  `tagsweepStore` (reuses the SAME worker for the single-writer commit path);
  `Tagmeta: tagmetaStore` passed to `server.Deps`.

## Verification

- `go build ./...` — exits 0.
- `go test ./internal/server/ -run TestTagMeta -count=1 -v` — all 5 tests pass.
- `go test ./internal/tagmeta/ -count=1 -v` — all 9 prior tests pass (unchanged).
- `cd web && npx tsc --noEmit` — exits 0.
- `grep 'style.color' web/src/lib/cm/widgets.ts` — match found.
- `grep 'export async function setTagMeta' web/src/api/client.ts` — match found.
- `grep 'TagMetaDialog' web/src/components/admin/TagManagement.tsx` — match found.

## Security

- Admin-only RBAC + CSRF inherited from the admin route subgroup.
- Color validated to hex/palette subset (no CSS injection — `url()`, `var()`, expressions
  rejected at the server gate).
- Description single-line + HTML-safe (rejects `<>&"'`).
- Badge DOM stays createElement + textContent (no innerHTML) — color via `style.color` only.
- The no-innerHTML XSS invariant holds: `grep 'innerHTML' web/src/lib/cm/widgets.ts` in the
  HashtagWidget section returns no match.

## Files modified

Prior (commit ef0c003):
- internal/tagmeta/ (new package: store.go, meta.go, validate.go, store_test.go)
- internal/store/migrations/0011_tag_meta.sql
- internal/pages/tree.go
- internal/pages/commitjob.go
- internal/graph/rebuild.go, query.go
- internal/search/rebuild.go
- internal/server/handlers_tags_admin.go
- internal/server/handlers_auth.go
- internal/server/router.go
- internal/audit/audit.go

This engineer:
- internal/server/handlers_tags_admin_test.go (5 tests + fixture wiring)
- web/src/lib/cm/widgets.ts
- web/src/lib/cm/livePreview.ts
- web/src/api/client.ts
- web/src/components/admin/TagManagement.tsx
- web/src/components/admin/TagManagement.css
- cmd/okf-workspace/main.go (tagmeta wiring)