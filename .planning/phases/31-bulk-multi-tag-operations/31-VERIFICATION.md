---
author: qa
responsible: qa
phase: 31
phase_type: implementation
status: verified
hard_bar: false
security_relevant: true
design_fork: false
wave: 1
depends_on: []
requirements: [TF-01]
autonomous: true
---

# Phase 31: Bulk Multi-Tag Operations — Verification

**Verdict: passed**

**Verifier:** qa (independent — judge ≠ doer)
**Method:** goal-backward against CODE REALITY, not the SUMMARY. Read the
committed source, ran the tests, ran the builds. SMTC daemon offline — evidence
is grep/read/run-grounded; confidence stated per criterion.

**Requirement bar:** TF-01 (`.planning/REQUIREMENTS.md` lines 108-124) — the
authoritative acceptance criteria.

---

## Acceptance handshake

- **Inputs complete:** CONTEXT, PLAN, SUMMARY all present; TF-01 is a real
  requirement ID in REQUIREMENTS.md.
- **PLAN ↔ SUMMARY consistency:** SUMMARY claims `handleTagBulk`,
  `computeBulkTags`, `ActionTagBulk`, route registration, 6 tests, `bulkTag`
  client fn, `BulkTagDialog`. All confirmed present in code (evidence below).
  No SUMMARY-claims-X-but-code-does-Y divergence found.
- **Goal verifiable:** "admin adds/removes multiple tags across a page set in
  one byte-stable preview→approve batch commit" — fully verifiable by test +
  code read.
- **Lenses engageable:** no-new-write-path and server-re-resolves-page-set are
  both checkable by reading the handler body + test assertions.

Handshake: **accepted** — inputs are verifiable.

---

## Criterion-by-criterion verification

### TF-01 Criterion 1 — page set (explicit + filter) + add/remove; preview per-page before/after + count

**Status: PASS (HIGH confidence — code read + test run)**

Evidence:
- `tagBulkRequest` struct (`handlers_tags_admin.go:55-61`) carries
  `PagePaths` (explicit selection), `FilterTags` (existing-tag filter union),
  `AddTags`, `RemoveTags`, `Preview` — exactly the request shape the criterion
  requires.
- Handler resolves the page set server-side: explicit paths seeded into a map,
  then each filter-tag resolved via `h.graph.PagesByTag(ctx, ft)` and unioned
  (`handlers_tags_admin.go:641-654`), deduped via the map, sorted for
  determinism (`:655-659`).
- Preview path returns `tagPreviewResponse{Tag: "bulk", AffectedPages:
  preview, TotalAffected: len(preview)}` (`:669-675`). Each `tagPreviewPage`
  (`:69-76`) carries `PagePath`, `CurrentTags`, `NewTags` — the per-page
  before/after the criterion demands.
- `TestBulkTagPreview` (`handlers_tags_admin_test.go:572-618`): seeds 3 pages
  (`a.md` [c,x], `b.md` [c,y], `d.md` [c,z]), previews add [a,b] + remove [c],
  asserts `TotalAffected == 3` and per-page `NewTags` == {x,a,b}/{y,a,b}/{z,a,b}.
  **PASS** (test run confirmed).

### TF-01 Criterion 2 — ApplyTagsBatch (N→1 commit), byte-stable (frontmatter only), per-page 409, audit-logged

**Status: PASS (HIGH confidence — code read + test run)**

Evidence:
- Apply path calls `h.pages.ApplyTagsBatch(ctx, items, actor)`
  (`handlers_tags_admin.go:685`) — the proven single-writer batched commit.
  No parallel write path; `buildTagApplyItems` is the reused compute seam
  (`:663`).
- **Byte-stable / frontmatter-only:** `buildTagApplyItems` is called with
  `bodyOld=""`, `bodyNew=""` (`:663`) — `BodyOld==""` disables the body channel
  (no `RewriteHashtags`), so only the frontmatter tags-region changes. This is
  the D-02 decision implemented exactly.
- **N→1 commit:** `TestBulkTagGolden` (`:622-675`) asserts `commit delta == 1`
  for a 3-page batch apply (`:645-647`). **PASS** (test run confirmed).
- **Per-page stale (409) exclusion:** `TestBulkTagStale` (`:679-769`) feeds
  `ApplyTagsBatch` a stale `BaseRevision` for p2 (the deterministic form of the
  mid-apply race), asserts `p2 status == "stale"`, `p1 status == "applied"`,
  `commit delta == 1`, and the stale page is NOT clobbered (no "new" tag
  written). **PASS** (test run confirmed).
- **Audit-logged:** `audit.ActionTagBulk` recorded non-fatally
  (`handlers_tags_admin.go:690-695`); constant exists at `audit.go:68`
  (`ActionTagBulk = "tag_bulk"`). Detail carries only non-secret provenance
  (`"%d pages, +%v -%v"`).

### TF-01 Criterion 3 — Admin-only RBAC + CSRF; added tags server-side re-validated (ValidateTagCharset)

**Status: PASS (HIGH confidence — code read + test run)**

Evidence:
- **Admin-only RBAC:** route registered inside the admin subgroup:
  `admin.Post("/admin/tags/bulk", h.handleTagBulk)` (`router.go:350`), which is
  inside `authed.Group(func(admin chi.Router) { admin.Use(auth.RequireRole
  (auth.RoleAdmin)) ... })` (`router.go:302-303`). Role read from session, never
  client input.
- **CSRF:** the admin subgroup inherits the nosurf CSRF middleware from the
  outer `/api/v1` route group (same as rename/merge/delete — the bulk endpoint
  is in the identical subgroup).
- **RBAC test:** `TestBulkTagRBAC` (`:551-568`): editor gets 403 on both
  preview and apply. **PASS** (test run confirmed).
- **Added-tag re-validation:** each add_tag is validated via
  `okf.ValidateTagCharset(t)` (`handlers_tags_admin.go:629-633`), 400 on
  invalid. Removed/filter tags are matched, not written, so no charset gate
  needed (correct per TF-01: "charset gate suffices" for frontmatter-only).
- **Validation test:** `TestBulkTagValidation` (`:815-836`): `<script>` in
  add_tags → 400; empty page set + empty add/remove → 400. **PASS** (test run
  confirmed).

### TF-01 Criterion 4 — Golden test: add [a,b] + remove [c] across 3 pages → correct frontmatter, 1 commit; stale page excluded

**Status: PASS (HIGH confidence — test run)**

Evidence:
- `TestBulkTagGolden` (`:622-675`): seeds 3 pages (`a.md` [c,x], `b.md` [c,y],
  `d.md` [c,z]), applies add [a,b] + remove [c], verifies:
  - `commit delta == 1` (`:645-647`) — batch atomicity.
  - Per-page frontmatter via raw YAML parse (`pageTags` helper, `:523-548`):
    a.md → [x,a,b], b.md → [y,a,b], d.md → [z,a,b] (`:650-660`).
  - Graph rebuild: tag `c` gone from all pages (`:666-669`), tag `a` on all 3
    (`:671-674`).
  **PASS** (test run confirmed).
- Stale exclusion: `TestBulkTagStale` (`:679-769`) — stale page reported
  "stale", excluded, batch completes in 1 commit, stale page not clobbered.
  **PASS** (test run confirmed).

---

## Build + test commands (run by qa, not trusted from SUMMARY)

| Command | Result |
|---|---|
| `go test ./internal/server/ -run TestBulkTag -count=1 -v` | **PASS** — all 6 tests pass (0.659s) |
| `go build ./...` | **exit 0** |
| `cd web && npx tsc --noEmit` | **exit 0** |

Test output (run by qa):
```
=== RUN   TestBulkTagRBAC
--- PASS: TestBulkTagRBAC (0.07s)
=== RUN   TestBulkTagPreview
--- PASS: TestBulkTagPreview (0.08s)
=== RUN   TestBulkTagGolden
--- PASS: TestBulkTagGolden (0.20s)
=== RUN   TestBulkTagStale
--- PASS: TestBulkTagStale (0.13s)
=== RUN   TestBulkTagFilterSelection
--- PASS: TestBulkTagFilterSelection (0.08s)
=== RUN   TestBulkTagValidation
--- PASS: TestBulkTagValidation (0.06s)
PASS
ok  	github.com/postfix/okworkspace/internal/server	0.659s
```

---

## Frontend verification

- `bulkTag` API function exists (`web/src/api/client.ts:1423-1434`): mirrors
  `renameTag`/`mergeTag`/`deleteTag`, reuses `TagPreview`/`TagApplyResponse`
  types, POSTs to `/api/v1/admin/tags/bulk`.
- `BulkTagDialog` component exists
  (`web/src/components/admin/TagManagement.tsx:339-490`):
  - Page-set selector: "Filter by tag" comma-separated input (union,
    `:414-425`) + "Or select pages" checkbox list over the flattened page tree
    (`:427-445`). Both arrays sent to the server (server unions them).
  - Tag operations: "Tags to add" + "Tags to remove" comma-separated inputs
    (`:447-470`).
  - Preview button (`handlePreview`, `:373-390`) → per-page
    `current_tags → new_tags` preview table (`:472-485`).
  - Apply button (`handleApply`, `:392-405`) → calls `bulkTag` apply,
    invalidates `TAGS_KEY` query (`:397`), closes dialog.
  - Cancel button: closes dialog (`onClose`).
- "Bulk add/remove" button in `TagTable` (`:66-68`) opens `BulkTagDialog` via
  `bulkOpen` state slot (`:49`, `:105-107`).
- `npx tsc --noEmit` exits 0 — frontend compiles cleanly.

---

## Invariant checks

| Invariant | Evidence |
|---|---|
| No new write path | `handleTagBulk` calls `buildTagApplyItems` + `ApplyTagsBatch` (`:663`, `:685`) — the existing seam, never a parallel commit. |
| Byte-stability (frontmatter only) | `bodyOld=""`, `bodyNew=""` passed to `buildTagApplyItems` (`:663`) — body channel disabled. |
| Server-side page-set re-resolution | Page set re-derived from live store at both preview and apply (`:641-659`) — client list is advisory, not trusted. |
| Per-page stale exclusion | `ApplyTagsBatch` 409 floor; `TestBulkTagStale` proves exclusion without sinking batch. |
| Admin-only RBAC + CSRF | Route in admin subgroup (`router.go:350`), `RequireRole(admin)` (`:303`), nosurf inherited. `TestBulkTagRBAC` proves 403. |
| Added-tag server-side re-validation | `ValidateTagCharset` per add_tag (`:629-633`); `TestBulkTagValidation` proves 400 on `<script>`. |
| Audit-logged | `audit.ActionTagBulk` (`:690-695`); constant at `audit.go:68`. |

---

## Verdict

**passed** — all 4 TF-01 acceptance criteria verified against code reality
with test-run + code-read evidence. No gaps found. No human input needed.