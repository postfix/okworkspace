---
author: architect
responsible: architect
milestone: v1.6
status: shipped
last_updated: "2026-07-02"
parent_artifacts:
  - .planning/REQUIREMENTS.md
  - .planning/PROJECT.md
red_team: folded (agent RoadmapRedTeam-2, 2026-07-02 — SPLIT-REQUIRED accepted; TipTap→v1.7)
---

# Roadmap: OKF Workspace

Milestone **v1.6 — Tag System Completion & Authoring Polish**. Two tracks, seven phases (31–37):

- **T — Tag System Completion** (Phases 31–35): the v1.3-original deferred tag set, on the
  hardened v1.5 tag subsystem.
- **E — Authoring Polish** (Phases 36–37): four small v1.4-deferred authoring wins.

> Phases continue from v1.5's last (Phase 30) → v1.6 spans **Phases 31–37** (7 phases — matching
> v1.4's proven size).
>
> **TipTap (rich-block editor) split to v1.7.** The independent roadmap red-team (2026-07-02)
> returned **SPLIT-REQUIRED**: the original 11-phase draft was the largest milestone in project
> history, and track R (TipTap, phases 38–41) is a self-contained editor sub-project — a net-new
> ProseMirror serialization layer, its own GO/NO-GO fidelity gate, and an 893-line `PageEditor`
> host refactor — that is milestone-sized on its own. A second reason: TipTap opt-in for **shared**
> content needs a design decision (Tier-1 marker normalization silently rewrites modeled-syntax
> bytes for all viewers — it passes the "lossless" load guard yet forks the wiki's on-disk bytes by
> editor) that must not be rushed inside an oversized minor. The R requirements + the red-team's
> R-track findings are preserved in `REQUIREMENTS.md` (§R, deferred to v1.7) so v1.7 starts warm.

## Milestones

- ✅ **v0.9.9 MVP** — Phases 0–7 (2026-06-23)
- ✅ **v1.0 Knowledge Graph & LLM Auto-Tagging** — Phases 8–12 (2026-06-24)
- ✅ **v1.1 Hardening & Polish** — Phases 13–17 (2026-06-27)
- ✅ **v1.2 Graph Schema Extension** — Phase 18 (2026-06-28)
- ✅ **v1.3 Tag Management UX** — built `9de5077` (2026-06-28); closed retroactively in v1.5
- ✅ **v1.4 Editor Enhancements & Authoring Polish** — Phases 21–27 (2026-07-01)
- ✅ **v1.5 Tag Regression Fix, Reading-Shell Fold-In & v1.3 Closure** — Phases 28–30 (2026-07-01)
- ⏭️ **v1.7 Rich-Block Editor (TipTap)** — deferred here; requirements locked (`REQUIREMENTS.md` §R)

**Coverage:** every v1.6 requirement (TF-01…05, EP-01…04) maps to exactly one phase. 7 phases · all covered.

## Phases

| #  | Track | Phase | Goal | Requirements | Depends on |
|----|-------|-------|------|--------------|------------|
| 31 | T | Bulk Multi-Tag Operations | Add/remove multiple tags across a selected page set in one byte-stable preview→approve batch commit | TF-01 | — |
| 32 | T | Tag Metadata Store (color/description) | Net-new committed on-disk tag-metadata store (files-as-truth) + rebuildable cache + migration; color/description in admin UI + badges | TF-02 | — |
| 33 | T | Tag Hierarchy | Parent/child hierarchy (cycle-safe, display-only) in the metadata store; grouped admin list | TF-03 | 32 |
| 34 | T | Tag Usage Analytics | Admin analytics view over `TagPageCounts` — counts, most/least used, orphans | TF-04 | — |
| 35 | T | Tag-Operation Undo | Undo the last tag-admin op via inverse `ApplyTagsBatch` (byte-stable, 409-guarded), backed by a dedicated op-journal | TF-05 | 31 (+ op-journal decision) |
| 36 | E | Rendering Polish | `#tag` badge in MarkdownProse; copy-code button; description in search cards | EP-01, EP-02, EP-04 | — |
| 37 | E | Insert-at-Cursor Templates | Insert a `Templates/` snippet at the cursor via a normal CM6 transaction | EP-03 | — |

**Risk-first ordering rationale:**
- **Track T's design fork is front-loaded.** Phase 32 (the on-disk metadata store) is the tag
  track's load-bearing unknown — and it is **heavier than a thin cache layer**: tag metadata has
  NO on-disk source today (`page_tags`, migration 0009, is rebuildable from *frontmatter*, not from
  any metadata store; grep for `tag_meta`/`tagmeta`/`tags.json` = empty). Phase 32 must invent the
  disk format, a cache table + **migration**, and the rebuild harness from scratch. It is sequenced
  early because Phase 33 (hierarchy) builds on the same store. TF-01/04 do NOT depend on it.
- **Track E is cheap and independent of T's *backend*** — but see the shared-file correction below;
  Phase 36 shares CM6 widget files with Phase 32.

**Cross-track / shared-file coordination (corrected — the earlier "fully independent" claim was
false, per red-team MAJ-02):**
- **Phases 32 and 36 BOTH touch `web/src/lib/cm/widgets.ts` and `web/src/lib/cm/theme.ts`.** Phase 32
  adds tag-color plumbing to `HashtagWidget` (+ theme tokens); Phase 36 adds the copy-code button to
  `CodeBlockWidget` (+ the EP-01 badge, which is `MarkdownProse.tsx`, not `widgets.ts`). These are
  logically additive (different widget classes) but **collide as edits** — sequence 32 before 36, or
  assign explicit file-ownership if parallelized. **Invariant coupling:** both widget classes depend
  on the no-`innerHTML` / `textContent`-only DOM invariant (`MAP-cm-editor-subsystem` #8); Phase 32's
  color plumbing MUST apply color via class/style attribute, never `innerHTML`, or it regresses the
  XSS invariant that Phase 36's copy button also relies on.
- **`PageEditor.tsx` (at `web/src/routes/PageEditor.tsx` — NOT `components/`)** is touched by Phase 37
  (insert affordance). No v1.6 collision (the TipTap host work that would also touch it is in v1.7).

**Verification & security posture (per config: ASVS L1, block-on-high, code_review on):**
- Every phase: engineer builds → independent qa goal-backward verification; gaps route UP to the architect.
- **Security-relevant phases** (in-phase security lens + milestone-close review scope): **32** (new
  on-disk store + color/description injection surface into badge DOM), **35** (undo write surface),
  **36** (EP-01 stored-XSS boundary in `MarkdownProse` — the badge MUST be a sanitized element node,
  never raw HTML; `rehype-raw` OFF / `rehype-sanitize` ON preserved). **31** is a bulk write surface
  (RBAC/CSRF/validation) but reuses the reviewed batch path.
- **Byte-stability** (all tag writes through `ApplyTagsBatch`/`SetTagsFrontmatter`) is the load-bearing
  invariant qa verifies against code reality, not summaries.

---

## Phase 31: Bulk Multi-Tag Operations

**Goal:** An admin adds and/or removes multiple tags across a selected set of pages in ONE
byte-stable preview→approve batch commit, reusing `pages.ApplyTagsBatch`.

**Requirements:** TF-01

**Success criteria (observable):**
1. An admin operation takes a page set (explicit selection and/or existing-tag filter) + a set of
   tag adds and/or removes; the preview shows exact per-page before/after tag sets + affected count
   before any write.
2. Approve applies via `ApplyTagsBatch` — N pages → **1** commit, byte-stable (only the frontmatter
   tags-region changes; add/remove-from-set does NOT rewrite bodies); per-page stale (409) excludes
   that page without sinking the batch; audit-logged.
3. Admin-only RBAC + CSRF; each added tag server-side re-validated (`ValidateTagCharset`).
4. Golden test: add `[a,b]` + remove `[c]` across 3 pages → correct per-page frontmatter in 1 commit;
   a stale page reported `stale` and excluded.

**Key design decisions:** reuse `buildTagApplyItems` + `ApplyTagsBatch` (seam ready — no new write
path); add/remove is frontmatter-only (no body `#tag` rewrite → no `RewriteHashtags`, no
`ValidateInlineTag`).

**Key files:** `internal/server/handlers_tags_admin.go` (bulk handler + preview), `internal/pages/batch.go`
(reuse `ApplyTagsBatch`), `web/src/components/admin/TagManagement.tsx` + `web/src/api/client.ts`,
`internal/audit/audit.go` (bulk action if added).

**Depends on:** — (the batch seam already exists).

---

## Phase 32: Tag Metadata Store (color/description)

**Goal:** Introduce a **net-new**, committed, human-readable, on-disk tag-metadata store
(files-as-truth) with a rebuildable SQLite cache **and its migration**; let an admin set a tag's
color + description, surfaced in the admin list and on rendered badges.

**Requirements:** TF-02

**Scope acknowledgement (red-team direct-answer B):** this is NOT a thin layer. Tag metadata has no
on-disk source today. This phase creates: (1) the on-disk format, (2) a cache table + migration,
(3) the rebuild-from-disk harness, (4) the admin endpoint + UI, (5) the badge color plumbing. Plan it
as a substantial phase.

**Success criteria (observable):**
1. Tag metadata persists on disk in a committed, portable form (shape chosen in discuss-phase via
   `anvil-design-options`: workspace `.okf-workspace/tags.json` vs a `Tags/` folder of `.md` files);
   a SQLite cache is fully rebuildable from it (delete-cache → rebuild = identical metadata), matching
   the `page_tags` derived-cache discipline; a migration creates the cache table.
2. Admin UI sets/clears a tag's color + description via an admin-only, CSRF-protected, server-validated
   endpoint (color = safe palette/hex subset; description single-line, length-capped, HTML-metachar-safe).
3. `#tag` badges (`HashtagWidget`) reflect a set color via **class/style attribute** — DOM stays
   `createElement`+`textContent` (no `innerHTML`); the XSS invariant holds (shared with Phase 36).
4. Absent metadata degrades gracefully (tag renders as today; feature is additive; a page's tags are
   fully usable with the store empty).

**Key design decisions:** THE tag-track design fork — on-disk store shape decided in-phase (constraint:
on-disk + committed + rebuildable cache), not pre-committed. Metadata is adjunct to the plain-string tag
model. The shape must extend trivially to Phase 33's parent field.

**Key files:** new store package (e.g. `internal/tagmeta/`) + a rebuildable cache table (new migration
`internal/store/migrations/00NN_*.sql`), a metadata admin endpoint (`handlers_tags_admin.go`),
`TagManagement.tsx` + `client.ts`, badge color plumbing in `web/src/lib/cm/widgets.ts` (`HashtagWidget`)
+ `web/src/lib/cm/theme.ts`. **Shared-file note:** `widgets.ts` + `theme.ts` are also touched by
Phase 36 — sequence 32 first.

**Depends on:** — (net-new store; independent of TF-01).

---

## Phase 33: Tag Hierarchy

**Goal:** Tags form a cycle-safe parent/child hierarchy in the Phase-32 store, shown grouped in the
admin list — display/organization only (no page rewrites, no search expansion) in v1.6.

**Requirements:** TF-03

**Success criteria (observable):**
1. A tag may declare a parent (or none); server rejects cycles + self-parenting (no tag is its own
   ancestor); root/orphan tags valid.
2. The admin tag list can display tags grouped parent→children; hierarchy edits use the same admin-only
   + CSRF + validated endpoint as TF-02.
3. Hierarchy is display-only: no auto-apply of parent tags, no descendant-expanding search, no page
   bytes changed by a hierarchy edit.
4. Rebuildable-from-disk parity holds (hierarchy lives in the TF-02 store).

**Key design decisions:** hierarchy is metadata, not a page operation — zero page bytes change;
cycle-detection is server-side and total. Scope deliberately narrow (organize/display); hierarchy-aware
behavior is a stated Future item.

**Key files:** the `internal/tagmeta/` store (parent field + cycle check), `handlers_tags_admin.go`,
`TagManagement.tsx` (grouped rendering).

**Depends on:** Phase 32 (the metadata store).

---

## Phase 34: Tag Usage Analytics

**Goal:** An admin analytics view over the existing `TagPageCounts` query — per-tag page counts,
most/least used, orphan tags, totals — read-only, no new heavy scan.

**Requirements:** TF-04

**Success criteria (observable):**
1. An admin view shows per-tag page counts (sortable), most-used + unused/orphan tags, and totals,
   sourced from `graph.Store.TagPageCounts` (`query.go:449`) — reuses the existing `GROUP BY`.
2. The view states its scope (frontmatter `page_tags` cache; body-only `#tags` counted after
   materialization) so numbers aren't misread.
3. Admin-only, read-only, efficient at ~5-user scale.

**Key design decisions:** a thin read over an already-shipped query — the cheapest tag feature; no
mutation, no new index.

**Key files:** a read endpoint (reuse/extend `handleTagList` or a new analytics handler), `client.ts`,
a `TagManagement.tsx` analytics panel/tab.

**Depends on:** — (query exists).

---

## Phase 35: Tag-Operation Undo

**Goal:** Undo the last tag-admin operation (rename/merge/delete/bulk) by replaying its inverse
through `ApplyTagsBatch` — byte-stable, one commit, per-page 409-guarded, forward-commit (never a
history rewrite) — backed by a dedicated durable op-journal.

**Requirements:** TF-05

**Design constraint (red-team MAJ-01):** the audit log (`audit.Event{…, Detail string}`, `audit.go:73-90`)
**cannot** carry the inverse-op data (it only stores a free-text `Detail` like `"N pages"`). So undo needs
a **dedicated op-journal**, decided in discuss-phase. Two options: (a) a new small journal table/store
(default — keeps Phase 35 dependent on Phase 31 only), or (b) the Phase-32 metadata store (which would
add a **`depends_on: 32`** edge). "Reuse the audit log" is a dead end and is dropped as an option.

**Success criteria (observable):**
1. Each tag-admin op records a **minimum invertible record** durably: op type, old/new names,
   the affected pages, each page's **pre-op frontmatter tags + body-affected flag** (enough to
   reconstruct the inverse). Stored in the dedicated op-journal.
2. "Undo last tag operation" replays the inverse via `ApplyTagsBatch` (byte-stable, 1 commit); a page
   edited since the op is reported + skipped (409-guarded), never clobbered.
3. Admin-only + CSRF + audited; forward-commit (consistent with `pages.RestoreVersion`); undo-of-undo
   possible or explicitly scoped out (stated in-phase).
4. Golden test: rename `#ops`→`#devops`, then undo → affected pages' frontmatter + body `#tags`
   byte-match pre-rename (via the body-rewrite channel); an intervening-edit page skipped with a report.

**Key design decisions:** undo reuses the v1.5 body-rewrite channel (`BodyOld`/`BodyNew`) to revert body
`#tags`, so it inverts rename/merge/delete AND bulk; the 409-guard is what stops it clobbering newer
edits — the invariant the server-authoritative model already depends on.

**Key files:** a dedicated op-journal (new table/store — decided in discuss-phase), an undo handler
(`handlers_tags_admin.go`), `ApplyTagsBatch` (reused), `TagManagement.tsx` undo control.

**Depends on:** Phase 31 (bulk is an undoable op + validates the inverse-apply pattern). Gains a
`depends_on: 32` edge IF discuss-phase picks the metadata-store journal option.

---

## Phase 36: Rendering Polish (`#tag` badge in MarkdownProse · copy-code · description in search)

**Goal:** Three independent read-surface wins: `#tag` renders as a badge in the react-markdown
`MarkdownProse` surfaces (history preview + agent answer) without weakening the XSS guard; read-mode
fenced code gets a copy button; page search cards show the frontmatter description.

**Requirements:** EP-01, EP-02, EP-04

**Verification note (red-team MIN-05):** three unrelated features in one phase — qa MUST verify each
independently (a subagent could ship 2 of 3 and mark the phase done). EP-01 is the security-flagged one.

**Success criteria (observable):**
1. **EP-01:** a remark inline transform renders body `#tag` as a **safe badge element** (sanitized
   span, NOT raw HTML) in `MarkdownProse` (`HistoryPanel` + `AgentAnswer`), matching the CM6 badge +
   recognized-`#tag` set; `rehype-raw` OFF / `rehype-sanitize` ON preserved; the `AgentAnswer`
   stored-XSS test (`AgentAnswer.test.tsx:42-44`) still passes; false-`#` cases
   (code/escape/heading/URL-fragment) stay literal.
2. **EP-02:** a copy-to-clipboard button on the read-mode `CodeBlockWidget` (`widgets.ts`) copies
   `this.code` with brief confirmation; built `createElement`-only (no `innerHTML`); does not perturb
   the async highlight swap / `destroyed`/`isConnected` teardown (`widgets.ts:165-178`);
   `navigator.clipboard` with graceful fallback.
3. **EP-04:** the search backend indexes + returns the frontmatter description as an **additive optional**
   field (`internal/search/{doc.go,rebuild.go,query.go}` + `handlers_search.go`); `SearchResultRow`
   renders it for page results (distinct from the body snippet); absent description falls back to today's
   card; no relevance/shape regression.

**Key design decisions:** EP-01 is the security-sharpest of the three — the badge MUST be a sanitized
element node, never raw HTML injection. The three share a "read surface" theme but are otherwise
independent; grouped to keep the phase count at v1.4's size.

**Key files:** `web/src/components/MarkdownProse.tsx` (+ a remark hashtag plugin), `web/src/lib/cm/widgets.ts`
(`CodeBlockWidget` copy button — **shared with Phase 32; sequence 32 first**),
`internal/search/{doc.go,rebuild.go,query.go}` + `internal/server/handlers_search.go` +
`web/src/api/client.ts` + `web/src/components/search/SearchResultRow.tsx`.

**Depends on:** — (independent of T's backend; shares `widgets.ts` with Phase 32 — coordinate).

---

## Phase 37: Insert-at-Cursor Templates

**Goal:** Insert a `Templates/` snippet at the cursor inside the open `LivePreviewEditor` via a normal
CM6 transaction — distinct from Phase-27 new-page seeding.

**Requirements:** EP-03

**Success criteria (observable):**
1. An editor affordance lists templates (existing `listTemplates` endpoint) and inserts the chosen
   template's substituted body (`{{title}}`/`{{date}}`/`{{time}}`) at the cursor.
2. Insertion is a normal CM6 transaction on the live `EditorView` (verbatim `onChange` on `docChanged`);
   the `externalSeed` reset annotation is NOT used.
3. Safe-path resolver for the template read; no placeholder executes code; inserting into a non-empty
   doc keeps surrounding text + selection correct.

**Key design decisions:** reuse the Phase-27 `Templates/` source + `listTemplates`; insertion is an
ordinary edit (not a document reset), so byte-stability + autosave are unaffected.

**Key files:** `web/src/lib/templates.ts` (substitution, reused), a CM6 insert command + affordance in
`web/src/routes/PageEditor.tsx` (**path corrected — routes/, not components/**) / `LivePreviewEditor.tsx`,
`web/src/api/client.ts` (`listTemplates`).

**Depends on:** — (independent).

---

## Progress

| Phase | Track | Milestone | Plans | Status | Completed |
| ----- | ----- | --------- | ----- | ------ | --------- |
| 28–30 | — | v1.5 | 3/3 | Complete | 2026-07-01 |
| **31. Bulk Multi-Tag** | **T** | **v1.6** | **0** | **Planned** | — |
| **32. Tag Metadata Store** | **T** | **v1.6** | **0** | **Planned** | — |
| **33. Tag Hierarchy** | **T** | **v1.6** | **0** | **Planned** | — |
| **34. Tag Usage Analytics** | **T** | **v1.6** | **0** | **Planned** | — |
| **35. Tag-Operation Undo** | **T** | **v1.6** | **0** | **Planned** | — |
| **36. Rendering Polish** | **E** | **v1.6** | **0** | **Planned** | — |
| **37. Insert-at-Cursor Templates** | **E** | **v1.6** | **0** | **Planned** | — |

---

## Execution Order

- **Track T** (31–35): 31 (bulk), 34 (analytics), 32 (metadata store) are independent starts; 33
  (hierarchy) needs 32; 35 (undo) needs 31 (+ 32 if the journal picks the metadata store). Suggested:
  31 → 32 → 33, 34 parallelizable, then 35.
- **Track E** (36–37): both independent; either order. **36 shares `widgets.ts`+`theme.ts` with
  Phase 32 — sequence 32 before 36.**
- **Cross-track merge coordination:** 32 + 36 (`widgets.ts`, `theme.ts`); no `PageEditor.tsx` collision
  in v1.6 (the TipTap host work moved to v1.7).
- **Close steps (not numbered phases):** independent qa per phase; milestone-close security review
  (scope: 32, 35, 36); MILESTONE-AUDIT; archaeologist **docs + codebase-map refresh** (`MAP-tag-rewrite`
  gains the metadata/hierarchy/undo surface; `MAP-cm-editor` gains the badge-color + copy-button
  changes) — the map refresh is a required close step (red-team MIN-03).
