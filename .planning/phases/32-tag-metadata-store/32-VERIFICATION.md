---
author: qa
responsible: qa
phase: 32
phase_type: implementation
status: verified
verdict: passed
gate_result:
  id: gate-32-1
  verdict: pass
  evidence_tier: high
  remediation_owner: ""
---

# Phase 32: Tag Metadata Store — Verification

**Verifier:** qa (independent judge — separate from the engineer).
**Method:** goal-backward against CODE REALITY (not the SUMMARY). Every claim below is
grounded in a file read or a test run performed by qa in this session.

## Acceptance handshake

- **Inputs complete:** PLAN, CONTEXT, SUMMARY present. Requirement TF-02 real
  (`REQUIREMENTS.md` §TF-02, lines 126-141). ✓
- **PLAN ↔ SUMMARY consistency:** SUMMARY claims match the PLAN's tasks (tagmeta package,
  migration, tree skip, admin endpoint, route, audit, widgets, client, TagManagement).
  SMTC/reads confirm the claimed symbols exist with real bodies (not stubs). ✓
- **Goal verifiability:** the goal (on-disk metadata store + rebuildable cache + admin
  set/clear + badge color via style.color + graceful absent) is fully verifiable by
  test + grep + build. ✓

**Handshake: PASS** — inputs verifiable, proceeding to goal-backward verify.

## Goal-backward verification against code reality

### TF-02-1: On-disk store + rebuildable cache + migration

| Check | Reality (qa-observed) | Verdict |
|---|---|---|
| `internal/tagmeta/store.go` has `SetMeta`, `GetMeta`, `AllMeta`, `RebuildTagMeta` | `store.go:118` SetMeta, `:52-69` GetMeta, `:71-95` AllMeta, `:227-291` RebuildTagMeta (walks `Tags/*.md` via `repo.Tree()` + prefix filter + `okf.Parse`, DELETE-all + re-insert in one tx) | ✓ |
| `internal/tagmeta/meta.go` has `Meta` struct | `meta.go:17` `type Meta struct { Tag, Color, Description string }` | ✓ |
| `internal/tagmeta/validate.go` has `ValidateColor`, `ValidateDescription` | `validate.go:25` ValidateColor (hex #RGB/#RRGGBB + 6-token palette map; empty=clear), `validate.go:57` ValidateDescription (single-line, ≤200, rejects `<>&"'`) | ✓ |
| `internal/store/migrations/0011_tag_meta.sql` with `CREATE TABLE tag_meta` | `0011_tag_meta.sql:7` `CREATE TABLE IF NOT EXISTS tag_meta (tag TEXT PRIMARY KEY, color TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '')`; tone mirrors 0009_graph | ✓ |
| `internal/pages/tree.go` has `Tags/` in isSkippedDir | `tree.go:133` `if slashRel == "Tags" \|\| strings.HasPrefix(slashRel, "Tags/") { return true }` | ✓ |
| Graph/search rebuilds skip `Tags/` | `graph/rebuild.go:29` + `:58,:74` `isTagMetaFile` skip; `search/rebuild.go:32` + `:71` `isTagMetaFile` skip | ✓ |
| Files-as-truth parity (delete-cache → rebuild = identical) | `store_test.go:133` `TestRebuildParity`: seeds Tags/deploy.md + Tags/ops.md, rebuilds, snapshots; `:149` nukes `DELETE FROM tag_meta`; `:152` rebuilds; `:154` asserts `reflect.DeepEqual` identical. **PASS** (qa ran it) | ✓ |
| Absent store degrades gracefully | `store_test.go:182` TestEmptyStore: rebuild on empty store → empty snapshot, no error; `store.go:73` AllMeta nil-db/absent-folder → empty slice, nil error | ✓ |

### TF-02-2: Admin-only + CSRF + server-validated endpoint

| Check | Reality (qa-observed) | Verdict |
|---|---|---|
| `handleTagMeta` handler exists | `handlers_tags_admin.go:781` `func (h *authHandlers) handleTagMeta` — decodes `tagMetaRequest`, validates tag (`normalizeTag` + `okf.ValidateTagCharset`), color (`tagmeta.ValidateColor`), description (`tagmeta.ValidateDescription`); calls `h.tagmeta.SetMeta`; audits; returns `tagMetaResponse` | ✓ |
| Route registered, admin subgroup (RBAC + CSRF inherited) | `router.go:359` `admin.Post("/admin/tags/meta", h.handleTagMeta)` inside the `admin` subgroup (admin-only + nosurf CSRF) | ✓ |
| `tagmeta.Store` wired into `authHandlers` + `Deps` + main | `handlers_auth.go` `tagmeta *tagmeta.Store`; `router.go` `Deps.Tagmeta`; `main.go:340` `tagmetaStore := tagmeta.NewStore(contentRepo, st.DB(), worker, cfg.Git.PushOnCommit)`; `main.go:418` `Tagmeta: tagmetaStore` | ✓ |
| `handleTagList` extended with color + description | `handlers_tags_admin.go:719` reads `h.tagmeta.AllMeta(ctx)` once, merges into `metaByTag`; `:752` emits `tagEntry{Tag, Count, Color, Description}`; absent metadata → zero-value `Meta{}` → empty fields (graceful) | ✓ |
| Audit constant | `audit.go:71` `ActionTagMeta = "tag_meta"` (after `ActionTagBulk`); `:816` recorded with Actor/Target/Detail/Source | ✓ |
| RBAC enforced | `TestTagMetaRBAC` PASS — editor gets 403 on POST /admin/tags/meta | ✓ |
| Color validation rejects unsafe CSS | `TestTagMetaValidation` PASS — `url(x)`, `var(--x)`, `<script>` → 400 | ✓ |
| Description validation (single-line, ≤200, HTML-safe) | `TestTagMetaValidation` PASS — newline, >200 chars, `<script>` → 400 | ✓ |
| Clear = empty color + description → file deleted | `TestTagMetaClear` PASS — file gone, GET /admin/tags shows empty | ✓ |

### TF-02-3: HashtagWidget color via style.color, no innerHTML

| Check | Reality (qa-observed) | Verdict |
|---|---|---|
| HashtagWidget has `color?` param | `widgets.ts:336` `constructor(readonly tag: string, readonly color?: string)` | ✓ |
| `toDOM()` applies `span.style.color` | `widgets.ts:354-355` `if (this.color) { span.style.color = this.color }` | ✓ |
| DOM is createElement + textContent, NO innerHTML | `widgets.ts:346` `document.createElement("span")`; `:350` `span.textContent = "#" + this.tag`; no `innerHTML` in the HashtagWidget range (:335-365). The file-level `innerHTML` mentions are all in *comments* asserting the invariant (security doc) — no assignment of page content. | ✓ |
| `eq()` compares color (re-render on change) | `widgets.ts:342` `other.tag === this.tag && other.color === this.color` | ✓ |
| Color flows from cache → livePreview → widget | `livePreview.ts:77` `tagColors` Facet (Record<string,string>, combined via Object.assign); `:197` read in buildDecorations; `:304` `new HashtagWidget(tag, colors[tag])` | ✓ |
| TypeScript compiles | `cd web && npx tsc --noEmit` — exit 0 (qa ran it) | ✓ |

### TF-02-4: Absent metadata degrades gracefully

| Check | Reality (qa-observed) | Verdict |
|---|---|---|
| No `Tags/` folder → empty metadata, no error | `TestEmptyStore` PASS; `TestTagMetaAbsent` PASS (GET /admin/tags renders tags with empty color/description) | ✓ |
| AllMeta nil-db → empty slice, nil error | `store.go:73` documented + implemented | ✓ |
| GetMeta absent → `(Meta{}, false)` | `store.go:52-69` | ✓ |
| Feature is additive (no page bytes change) | SetMeta writes ONLY `Tags/{tag}.md`; no page frontmatter/body touched; graph/search rebuilds skip `Tags/` | ✓ |

## Test runs (qa-executed)

```
go test ./internal/tagmeta/ -count=1 -v
=== RUN   TestSetGetMeta          --- PASS
=== RUN   TestRebuildTagMeta      --- PASS
=== RUN   TestRebuildParity       --- PASS  (delete-cache → rebuild = identical)
=== RUN   TestEmptyStore          --- PASS
=== RUN   TestValidateColor       --- PASS
=== RUN   TestValidateDescription --- PASS
=== RUN   TestSetMetaWritesFile   --- PASS
=== RUN   TestClearMetaDeletesFile --- PASS
=== RUN   TestRebuildSortsByTag   --- PASS
ok  github.com/postfix/okworkspace/internal/tagmeta  0.280s

go test ./internal/server/ -run TestTagMeta -count=1 -v
=== RUN   TestTagMetaRBAC         --- PASS
=== RUN   TestTagMetaSetGet       --- PASS
=== RUN   TestTagMetaValidation   --- PASS
=== RUN   TestTagMetaClear        --- PASS
=== RUN   TestTagMetaAbsent       --- PASS
ok  github.com/postfix/okworkspace/internal/server  0.436s

go build ./...           — exit 0
cd web && npx tsc --noEmit — exit 0
```

## TF-02 acceptance criteria (the authoritative bar)

- [x] Tag metadata persists in a committed, human-readable on-disk store
      (`Tags/{tag}.md`, YAML frontmatter); SQLite cache fully rebuildable
      (delete-cache → rebuild = identical — `TestRebuildParity` proves it).
- [x] Admin UI (`TagManagement.tsx`) sets/clears color + description via admin-only,
      CSRF-protected, server-validated endpoint; color = hex/palette subset;
      description single-line ≤200 HTML-safe.
- [x] CM6 `HashtagWidget` reflects color via `style.color`; DOM stays
      createElement + textContent (no innerHTML; XSS invariant holds).
- [x] Absent metadata degrades gracefully (TestEmptyStore + TestTagMetaAbsent).

## Security (security-relevant phase)

- Color validated to hex (#RGB/#RRGGBB) or 6-token palette — `url()`, `var()`,
  expressions, arbitrary CSS rejected at the server gate (`ValidateColor`).
- Description single-line + rejects `<>&"'` (`ValidateDescription`).
- Badge DOM: `span.style.color` only — NO `innerHTML` in the HashtagWidget path.
- Admin-only RBAC + nosurf CSRF inherited from the admin route subgroup.
- Single-writer commit path reused (`pages.CommitEnqueuer` — no second commit kind).

## Gaps

None. Every TF-02 criterion is met against code reality; all tests pass; builds
(Go + TS) are green.

## Verdict

**PASSED** — Phase 32 delivers the TF-02 goal: a committed on-disk tag-metadata
store with a rebuildable SQLite cache, an admin set/clear endpoint (validated +
admin-only + CSRF), CM6 badge color via `style.color` (no-innerHTML invariant
holds), and graceful degradation when metadata is absent. The verification is
grounded in qa's own reads of the real code + qa-executed test/build runs.