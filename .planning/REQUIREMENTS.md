---
author: architect
responsible: architect
milestone: v1.6
milestone_name: Tag System Completion & Authoring Polish
status: approved
last_updated: "2026-07-02"
parent_artifacts:
  - .planning/PROJECT.md
  - .planning/MILESTONES.md
  - .planning/codebase/MAP-tag-rewrite-subsystem.md
  - .planning/codebase/MAP-cm-editor-subsystem.md
---

# Milestone v1.6 Requirements — Tag System Completion & Authoring Polish

## Overview

A **feature milestone** (minor) on two tracks:

1. **Tag System Completion (T).** Finish the v1.3-original deferred tag set now that the
   tag subsystem is hardened (v1.5 shipped the server-authoritative body-rewrite +
   byte-stable batch apply): bulk multi-tag operations, tag color/description metadata,
   tag hierarchy, dedicated tag-op undo, and tag usage analytics.
2. **Authoring Polish (E).** Four small, mostly-frontend authoring wins deferred from v1.4:
   `#tag` badge rendering on the react-markdown surfaces, a copy-code button on read-mode
   fenced code, insert-at-cursor template snippets inside the open editor, and page
   description shown in search result cards.

**Rich-Block Editor (TipTap) split to v1.7.** The co-driver originally chose three tracks
including a TipTap WYSIWYG editor. The independent roadmap red-team (2026-07-02) returned
**SPLIT-REQUIRED**: the combined 11-phase draft was the largest milestone in project history,
and the TipTap track is a self-contained editor sub-project (net-new ProseMirror serialization
layer, its own GO/NO-GO fidelity gate, an 893-line `PageEditor` host refactor) that is
milestone-sized alone — and its opt-in-for-shared-content fidelity decision must not be rushed.
The co-driver **accepted the split**: v1.6 ships T + E; TipTap becomes **v1.7**. The TipTap
requirements + the red-team's TipTap-track findings are preserved below (§R — DEFERRED TO v1.7)
so v1.7 starts warm.

---

## Locked design decisions (from kickoff analysis + co-driver ratification)

### Global
- **Files-as-truth is inviolable (SPEC core value).** Every new durable datum (tag metadata,
  hierarchy) MUST live on disk in a committed, human-readable, portable form; SQLite may only
  hold a **rebuildable cache** of it. `internal/graph/store.go` states this invariant; a
  feature that puts tag metadata *only* in SQLite is rejected. (`MAP-tag-rewrite-subsystem`,
  `store.go:1-6`.)
- **Reuse the shipped byte-stable seams; no second implementation.** Tag writes go through
  `pages.ApplyTagsBatch` / `SetTagsFrontmatter` (N pages → 1 commit, TOCTOU-safe body channel);
  the single-recognizer `okf.FindHashtags` stays the only server-side `#tag` parser. New tag
  logic consumes these, never forks them.
- **Admin-only + CSRF + server-side re-validation** on every new mutating tag-admin endpoint,
  matching the v1.3/v1.5 surface (`router.go:302-343`).

### Rich-Block Editor (TipTap) — DEFERRED TO v1.7 (locked decisions preserved)
- **Coexist, do not replace.** CM6 `LivePreviewEditor` stays THE default editor and the
  **byte-exact** surface (its round-trip holds *by construction*: the CM6 doc IS the raw
  Markdown; all rendering is view-layer decoration — `MAP-cm-editor-subsystem`). TipTap is an
  **opt-in alternate mode** (per-user persisted preference, like the existing Live/Source
  toggle), never forced, never the default.
- **Two-tier fidelity contract (the acceptance gate).** TipTap serializes a ProseMirror tree
  back to Markdown, so it **cannot** be byte-stable by construction the way CM6 is. The gate is:
  - **Tier 1 — SUPPORTED syntax → semantic-lossless + idempotent.** `parse(serialize(md))`
    is tree-identical to `parse(md)`, and serialization is idempotent (one normalization pass,
    then byte-stable). Marker normalization (e.g. `*`→`-` bullets) is **allowed** but must be
    stable and predictable.
  - **Tier 2 — UNMODELED syntax → byte-for-byte.** Constructs the bounded slice does not model
    (tables, `$…$`/`$$…$$` math, raw HTML) survive an edit session **byte-identical** via a
    `PreservedBlock`/`PreservedInline` verbatim mechanism (`code:true` skips serializer
    escaping). Corpus-pinned.
  - **The corpus round-trip test is written FIRST as a GO/NO-GO spike gate.** If the supported
    corpus cannot pass after honest configuration effort, the track STOPS and reports NO-GO —
    it does not degrade the gate to ship anyway.
- **Bounded slice.** v1.6 TipTap models: paragraphs, headings (1–6), bold/italic/strike/inline-code,
  bullet/ordered/task lists, blockquote, fenced code, links, hard breaks — the StarterKit +
  task-list set that `@tiptap/markdown` (TipTap 3.26+, MarkedJS-backed, CommonMark) round-trips.
  Everything else is Tier-2 preserved verbatim. Richer *editing* of tables/math is explicitly
  deferred.
- **Single-recognizer discipline extends to TipTap.** The TipTap `#tag` node reuses the same
  conceptual grammar as the Lezer/Go recognizers; it does not introduce a third divergent
  `#tag` parser semantics. (A TipTap tokenizer is a distinct mechanism, but its recognized
  `#tag` set must match the corpus — pinned by a shared fixture where feasible.)
- **Stack:** `@tiptap/core`, `@tiptap/pm`, `@tiptap/react`, `@tiptap/starter-kit`,
  `@tiptap/extension-list`, `@tiptap/extensions`, `@tiptap/suggestion`, `@tiptap/markdown`
  (all pinned exact, per the v1.4/v1.5 supply-chain discipline: no caret ranges, `npm audit`
  in CI). TipTap + its deps MUST be **code-split** (lazy chunk) and proven out of the entry
  bundle by the existing `assertLazyChunks` guard — CM6-only users never pay for TipTap.

### Tag metadata + hierarchy (the tag-track design fork)
- **Metadata/hierarchy need a NEW on-disk store — its exact shape is a design fork resolved in
  the phase (discuss-phase + `anvil-design-options`), NOT pre-decided here.** The constraint the
  store MUST satisfy: committed to git, human-readable, portable, and the source of truth for a
  rebuildable SQLite cache (candidates: a workspace-level `.okf-workspace/tags.json` beside the
  existing `manifest.json`, or a `Tags/` folder of per-tag `.md` files). Tags themselves remain
  plain strings in page frontmatter + body `#tags`; metadata is *adjunct*, and a page's tags are
  fully usable with the metadata store absent (graceful degradation).
- **Phase weight (red-team direct-answer B):** this is NOT a thin cache layer. Tag metadata has NO
  on-disk source today (`page_tags` migration 0009 rebuilds from *frontmatter*, not from any metadata
  store; grep for `tag_meta`/`tagmeta`/`tags.json` = empty). Phase 32 must invent the disk format, a
  cache table + **migration**, and the rebuild harness from scratch — plan it as a substantial phase.

---

## T — Tag System Completion

### TF-01: Bulk multi-tag operations
**Requirement:** An admin can add and/or remove **multiple tags** across a **selected set of
pages** in one preview→approve operation, reusing the byte-stable batch commit path.
**Acceptance criteria:**
- [ ] A new admin operation accepts a set of target pages (by explicit selection and/or by an
      existing-tag filter) and a set of tag adds and/or removes; it previews the exact per-page
      before/after tag set and the affected count before any write.
- [ ] Approve applies via `pages.ApplyTagsBatch` (N pages → **1** commit), byte-stable
      (only the frontmatter tags-region changes for a pure-frontmatter op; body `#tags` are
      only rewritten when the op is a rename/merge/delete, not an add/remove-from-set), with
      per-page stale-revision (409) that does not sink the batch, and audit-logged.
- [ ] Admin-only RBAC + CSRF; each added tag is server-side re-validated (`ValidateTagCharset`,
      and `ValidateInlineTag` if it will be spliced into a body — but add-to-frontmatter does not
      splice bodies, so charset gate suffices).
- [ ] Golden test: adding `[a, b]` and removing `[c]` across 3 selected pages yields the correct
      per-page frontmatter in one commit; a stale page is reported `stale` and excluded, batch
      completes.

### TF-02: Tag color/description metadata
**Requirement:** A tag can carry a **color** and a **short description**, stored on disk
(files-as-truth), surfaced in the admin tag list and on rendered tag badges.
**Acceptance criteria:**
- [ ] Tag metadata persists in a committed, human-readable on-disk store (shape decided in the
      phase per the locked design fork); a SQLite cache of it is fully rebuildable from that store
      (delete-cache → rebuild reproduces identical metadata), matching the `page_tags` derived-cache
      discipline.
- [ ] Admin UI (`TagManagement.tsx`) can set/clear a tag's color + description via an admin-only,
      CSRF-protected, server-validated endpoint (color constrained to a safe palette/hex subset;
      description single-line length-capped, HTML-metachar-safe — no injection into badge DOM).
- [ ] Rendered `#tag` badges (CM6 `HashtagWidget`) reflect the tag's color when set; the badge DOM
      stays `createElement`+`textContent` only (color applied via class/style attribute, never
      `innerHTML`) — the no-innerHTML XSS invariant holds.
- [ ] Absent metadata degrades gracefully: a tag with no metadata renders exactly as today; the
      feature is additive and never blocks tag use.

### TF-03: Tag hierarchy (parent/child)
**Requirement:** Tags can form a **single-level-or-nested parent/child hierarchy** for grouping
and navigation, stored in the same on-disk metadata store, with cycle prevention.
**Acceptance criteria:**
- [ ] A tag may declare a parent (or none); the store rejects cycles and self-parenting
      server-side (a tag cannot be its own ancestor); orphan/root tags are valid.
- [ ] The admin tag list can display tags grouped by hierarchy (parent → children), and hierarchy
      edits go through the same admin-only + CSRF + validated endpoint as TF-02 metadata.
- [ ] Hierarchy is **display/organization only** in v1.6 — it does NOT auto-apply parent tags to
      pages, auto-expand tag search to descendants, or rewrite any page (that scope is deferred and
      stated as such). No page bytes change from a hierarchy edit.
- [ ] Rebuildable-from-disk parity holds (hierarchy is part of the TF-02 store).

### TF-04: Tag usage analytics
**Requirement:** An admin can see **per-tag usage** — page counts, most/least used, and orphan
tags — building on the existing `TagPageCounts` query.
**Acceptance criteria:**
- [ ] An admin analytics view surfaces per-tag page counts (sortable), most-used and unused/orphan
      tags, and total tag/tagged-page counts, sourced from `graph.Store.TagPageCounts`
      (`query.go:449`) — no new heavy scan; reuses the existing `GROUP BY` query.
- [ ] Counts reflect frontmatter tags (the `page_tags` derived cache); the view states this scope
      (body-only `#tags` not yet in frontmatter are counted only after materialization, consistent
      with the graph model) so the number is not misread.
- [ ] Admin-only; read-only (analytics never mutates); efficient at ~5-user, modest-page scale.

### TF-05: Tag-operation undo
**Requirement:** An admin can **undo the last tag-admin operation** (rename/merge/delete/bulk),
reverting affected pages to their pre-operation tag state.
**Acceptance criteria:**
- [ ] Each tag-admin operation records a **minimum invertible record** (op type, old/new names, the
      affected pages, each page's pre-op frontmatter tags + body-affected flag — enough to reconstruct
      the inverse), stored durably in a **dedicated op-journal**. NOTE (red-team MAJ-01): the audit log
      (`audit.Event{…, Detail string}`) CANNOT carry this — it holds only free-text `Detail`. The
      journal is either a new small table/store (default; keeps the phase dependent on TF-01 only) or
      the TF-02 metadata store (adds a dependency on Phase 32) — decided in discuss-phase. "Reuse the
      audit log" is dropped as an option.
- [ ] "Undo last tag operation" replays the inverse through `ApplyTagsBatch` (byte-stable, one
      commit), guarded by per-page stale-revision (409): a page edited since the original op is
      reported and skipped from the undo (never clobbered), not silently overwritten.
- [ ] Undo is admin-only + CSRF + audited; it is a **forward commit** (never a git history
      rewrite — consistent with `pages.RestoreVersion`); undo of an undo is possible or explicitly
      scoped out (stated in the phase).
- [ ] Golden test: rename `#ops`→`#devops`, then undo → affected pages' frontmatter + body `#tags`
      byte-match the pre-rename state (via the same body-rewrite channel); a page changed in between
      is skipped with a clear report.

---

## E — Authoring Polish

### EP-01: `#tag` badge on the react-markdown surfaces
**Requirement:** Inline `#tag` renders as a **badge** (not literal `#tag` text) on the two
`MarkdownProse` (react-markdown) surfaces — the history version preview and the agent answer —
matching the CM6 badge, without weakening the stored-XSS guard.
**Acceptance criteria:**
- [ ] A remark inline transform (or equivalent react-markdown plugin) renders body `#tag` as a
      badge `<span>` in `MarkdownProse` (`HistoryPanel` version preview + `AgentAnswer`), matching
      the CM6 `HashtagWidget` visual + the same recognized-`#tag` set (fenced/inline-code/escape/
      heading exclusions honored).
- [ ] The badge is produced as a **safe element node** (react element / sanitized span), NOT raw
      HTML injection: `rehype-raw` stays OFF and `rehype-sanitize` stays ON; the `AgentAnswer`
      stored-XSS test (`AgentAnswer.test.tsx:42-44`) still passes.
- [ ] A `#` that should not badge (in fenced/inline code, escaped, heading, URL fragment) renders
      as literal text on these surfaces too — no false badges.

### EP-02: Copy-code button on read-mode fenced code
**Requirement:** Read-mode fenced code blocks (`CodeBlockWidget`) have a **copy-to-clipboard**
button that copies the exact code text.
**Acceptance criteria:**
- [ ] A copy button appears on the read-mode `CodeBlockWidget` (`widgets.ts`); clicking it writes
      the block's exact source (`this.code`) to the clipboard and gives brief visual confirmation.
- [ ] The button DOM is built with `createElement` + explicit attributes (no `innerHTML`), and does
      NOT perturb the plain→highlighted async swap or its `destroyed`/`isConnected` teardown guards
      (`widgets.ts:165-178`). Edit mode (raw fence revealed) is unaffected.
- [ ] Clipboard write uses `navigator.clipboard` with a graceful no-op/fallback where unavailable;
      no exception escapes to break the widget.

### EP-03: Insert-at-cursor template snippets
**Requirement:** Inside an open editor, a user can **insert a template snippet at the cursor**
(distinct from Phase-27 new-page seeding), reusing the existing `Templates/` source.
**Acceptance criteria:**
- [ ] An editor affordance lists templates via the existing `listTemplates` endpoint and inserts
      the chosen template's body (with `{{title}}`/`{{date}}`/`{{time}}` substituted) at the current
      cursor position in the `LivePreviewEditor`.
- [ ] Insertion dispatches a **normal CM6 transaction** on the live `EditorView` — it is an ordinary
      user edit (the byte-stable EDIT-03 `onChange` fires verbatim on `docChanged`); the
      `externalSeed` reset annotation is NOT used (that is for prop-driven resets only).
- [ ] Insertion respects the safe-path resolver for the template read; no placeholder executes code;
      inserting into a non-empty document does not corrupt surrounding text (cursor/selection correct).

### EP-04: Page description in search result cards
**Requirement:** Search result cards for **page** results show the page's frontmatter
**description** (when present), in addition to the body-match snippet.
**Acceptance criteria:**
- [ ] The search backend indexes the frontmatter description (`pageDoc`/`rebuild.go`/`indexPage`)
      and returns it as an **additive optional** field on `search.Result` (`query.go:14-21`) — the
      existing snippet/anchor/page_title fields are unchanged.
- [ ] `SearchResultRow.tsx` renders the description for page results (distinct from the body-match
      snippet), truncated/styled to the card; absent description falls back to today's behavior.
- [ ] The search response stays lean (description is a single additive field); no regression to
      existing search relevance or the heading/attachment result shapes.

---

## R — Rich-Block Editor (TipTap) — DEFERRED TO v1.7

> **This whole track ships as v1.7, not v1.6** (co-driver-accepted red-team split, 2026-07-02).
> The requirements + the red-team's R-track findings below are preserved verbatim so v1.7 starts
> warm. Phase numbers (38–41) are provisional and re-confirmed when v1.7 is roadmapped.
> Track ordering when built is strict/risk-first: the **codec round-trip corpus is the GO/NO-GO
> gate and comes FIRST** (RE-01); no TipTap UI is wired until the fidelity contract passes.

### RE-01: Markdown codec + round-trip corpus (GO/NO-GO spike gate)
**Requirement:** A headless TipTap Markdown codec (`@tiptap/markdown`) with the bounded-slice
extension set, proven by a round-trip corpus test that is the **acceptance gate** for the whole
track.
**Acceptance criteria:**
- [ ] A `buildExtensions()` schema set (StarterKit configured to the bounded slice + task lists +
      `Markdown`) and headless `parse`/`serialize`/`roundTrip` helpers exist, shared by the codec
      test and (later) the live editor so parse/serialize behavior is identical in both.
- [ ] **Tier-1 corpus (supported syntax):** for each fixture (basic marks, headings, ordered/
      unordered/nested/task lists, blockquotes, fenced code incl. a `mermaid` fence kept as code,
      CJK+emoji, blockquote-mixed), `parse(serialize(src)) ≡ parse(src)` (tree equality) AND
      serialization is idempotent (stable after one normalization pass). All pass.
- [ ] The test is **written first and fails for the right reason** (missing module) before the codec
      exists; if the supported corpus cannot be made to pass after honest config/custom-serializer
      effort, the phase reports **NO-GO** with the failing fixtures + diffs and the track STOPS
      (does not degrade the gate).
- [ ] TipTap + deps are pinned exact; `npm audit` clean; the codec module is import-isolated so it
      code-splits (proven later by `assertLazyChunks`).

### RE-02: Verbatim preservation tier (PreservedBlock / PreservedInline)
**Requirement:** Unmodeled constructs (tables, `$…$`/`$$…$$` math, raw HTML) round-trip
**byte-for-byte** through a preserved-node/mark mechanism.
**Acceptance criteria:**
- [ ] `PreservedBlock` (block node, `code:true`, editable literal mono text) and `PreservedInline`
      (inline mark, `code:true`) capture the raw Markdown source of unmodeled constructs at parse
      time and re-emit it unchanged on serialize; marked "bridge" tokenizers/handlers route tables,
      math, and raw HTML into them.
- [ ] **Tier-2 corpus (byte equality):** fixtures containing only preserved constructs + inert prose
      round-trip byte-identical (modulo outer trim); serialization is idempotent. Inline math
      `$x_1$` does NOT get its `_` escaped; inline HTML `<em>x</em>` stays literal (not converted to
      a mark); autolinks are not swallowed.
- [ ] Unit tests assert the preserved constructs parse to a single `preservedBlock`/marked inline,
      not a fallback that reflows them.

### RE-03: `#tag` + suggestions + slash commands in TipTap
**Requirement:** The TipTap editor supports the app's authoring affordances — inline `#tag`
atoms, `#`-triggered tag suggestions, and `/`-triggered slash commands — consistent with the
existing tag model.
**Acceptance criteria:**
- [ ] A TipTap `Tag` inline node renders `#tag` as a badge and serializes back to literal `#tag`
      text (round-trips); its recognized-`#tag` set matches the corpus/grammar (no third divergent
      semantics) — pinned by a shared fixture where feasible.
- [ ] `#`-suggestion popup offers existing-vocabulary tags (reusing the tag vocabulary endpoint);
      `/`-slash-command popup inserts bounded-slice blocks (heading, list, code, quote). Both are
      UI-only extensions that do not alter the serialized schema.
- [ ] Body `#tags` authored in TipTap materialize into frontmatter through the SAME
      `reconcileFrontmatterTags` save path (ED-02b) — no divergent tag-materialization for the
      TipTap surface.

### RE-04: Editor-mode toggle + host integration
**Requirement:** TipTap is selectable as an **opt-in editing mode** beside CM6, behind a persisted
per-user preference, wired so the byte-exact CM6 path remains the default and the fidelity gate is
enforced at the boundary.
**Acceptance criteria:**
- [ ] A shared editor-host abstraction lets `PageEditor` mount either the CM6 `LivePreviewEditor`
      (default) or the TipTap editor based on a persisted preference (localStorage, like the
      Live/Source toggle); switching modes preserves the current Markdown content losslessly per the
      two-tier contract.
- [ ] On opening a page in TipTap mode, a **load guard** checks `isLosslessRoundTrip` for that page;
      if the page contains content the bounded slice cannot round-trip losslessly (and not covered by
      the preserved tier), the UI warns and/or falls back to CM6 for that page rather than silently
      normalizing — the byte-exact default is never violated without user awareness.
- [ ] TipTap + deps are **code-split** and proven absent from the entry chunk by `assertLazyChunks`
      (the `FORBIDDEN_IN_ENTRY` list updated in lockstep); CM6-only users incur no TipTap bundle cost.
- [ ] Saving from TipTap mode goes through the existing revision-checked byte-stable save
      (`pages.Save(baseRevision)` → single-writer commit); a stale-revision save 409s as usual.

### Red-team findings for the R track (fold into v1.7 planning — do NOT lose these)

The 2026-07-02 roadmap red-team (agent `RoadmapRedTeam-2`, all HIGH/direct-read) found these in the
original R draft. They are unresolved because R was deferred; resolve them when v1.7 is planned:
- **BLK-01 (ordering):** RE-03 criterion 3 (materialize via the real `reconcileFrontmatterTags` save
  path) secretly depends on RE-04 (TipTap isn't mounted into `PageEditor` until then), NOT RE-02.
  Either reorder so the host-mount precedes the materialization criterion, or extract the editor-host
  abstraction before RE-03. Criterion 3 must be tested through the real revision-checked save, not a
  headless harness alone.
- **BLK-02 (existing file):** `editorMode.ts` ALREADY EXISTS at `web/src/stores/editorMode.ts`
  (`EditorMode='live'|'source'`, persisted key `okf.editor.mode`, consumed by `PageEditor.tsx` +
  `mode.ts` `toggleKeymap`). RE-04 must **extend** it to `'live'|'source'|'tiptap'` — NOT create a new
  `web/src/lib/editorMode.ts`. Decide the three-way `Mod-e` toggle semantics.
- **BLK-03 (load guard):** RE-04's "warns **and/or** falls back to CM6" is non-deterministic — it
  permits a warning-only impl that still lets the user normalize. Tighten to "warn **AND** fall back"
  (TipTap unavailable for that page, not merely discouraged). The load guard is a guard, not a toast.
- **Shared-content fidelity (direct-answer C) — the deferred DESIGN DECISION:** Tier-1 marker
  normalization is *lossless per the contract*, so `isLosslessRoundTrip` does NOT catch it. In a
  5-person team, one user editing a shared page in TipTap silently rewrites modeled-syntax bytes
  (bullets, heading style, spacing) for ALL viewers — forking the wiki's on-disk bytes by editor and
  surprising the CM6 author who owns the page. It is still Markdown (core value not violated) but
  breaks byte-stability's spirit for shared content. **v1.7 must decide:** (i) document that TipTap
  edits normalize modeled syntax for all viewers (informed consent), OR (ii) tighten the load guard to
  warn on ANY non-preserved byte delta, not just unmodeled-content fallback.
- **MAJ-03 (host refactor):** RE-04 bundles a large `PageEditor.tsx` (~893 ln, at `web/src/routes/`,
  NOT `components/`) host refactor with the toggle + load guard + code-split + save wiring. Consider a
  dedicated "editor-host abstraction" phase extracting the save/reconcile/lock machinery first.
- **MAJ-04 (security — flag RE-02):** `PreservedBlock`/`PreservedInline` `toDOM` for raw HTML is an
  untrusted-input surface. The CM6 no-`innerHTML` invariant does NOT apply to ProseMirror. Add: "raw
  HTML preserved as inert text (`textContent`) or via a sanitized DOM path — never `innerHTML` of page
  bytes; an `<img onerror>` fixture does not execute in the editor view." Flag RE-02 for security review.
- **MAJ-05 (security — TipTap paste is the SHARP surface):** the novel untrusted-input path is TipTap
  PASTE/serialization, not EP-01. Add an RE-03/04 criterion: "pasted HTML/Markdown parses through the
  bounded-slice schema only; dangerous nodes (script, event-handler attrs, iframe) are dropped or
  preserved-as-text, never rendered live; a paste-XSS fixture proves it." Scope it into the v1.7
  security review.
- **MIN-02:** a `mermaid` fence renders as a diagram in CM6 but as raw code in TipTap (bytes preserved,
  but TipTap is second-class for mermaid pages) — acceptable, note it.
- **MIN-04:** RE-04's `FORBIDDEN_IN_ENTRY` regex must cover the WHOLE TipTap dep tree (`@tiptap/core`,
  `/pm`, `/react`, `/starter-kit`, `/markdown`, `/extension-list`, `/extensions`, `/suggestion`) or
  `assertLazyChunks` won't catch a stray static import.

---

## Future Requirements (deferred)

- **TipTap rich editing of tables/math** — v1.6 preserves them verbatim (Tier 2); structured WYSIWYG
  editing of tables and math is a follow-on.
- **Hierarchy-aware tag behavior** — auto-applying parent tags, descendant-expanding tag search, and
  hierarchy-driven graph grouping (TF-03 is display-only in v1.6).
- **Tag-op undo history (multi-level)** — TF-05 is last-op undo; a full undo stack is deferred.
- **Re-introduce a per-page local graph** / **in-place file-tree filter** — the v1.5 removals; still
  gated on proven miss.
- Prior v1.4/v1.5 carried follow-ups already CLOSED post-v1.5 (F-01/F-02/govulncheck/P26 UAT — see
  `milestones/v1.5-FOLLOWUPS.md`); none carry into v1.6.

## Out of Scope (explicit)

- **Replacing CM6 with TipTap.** CM6 stays the default, byte-exact surface; TipTap is opt-in only.
  A full editor replacement would be a v2.0, not this minor.
- **Degrading the fidelity gate to ship TipTap.** If the round-trip corpus cannot pass, the track is
  NO-GO — the gate is not relaxed to force a ship.
- **A third `#tag` parser semantics.** The TipTap tag recognition matches the existing corpus/grammar;
  it does not fork the recognized-`#tag` set.
- **Tag metadata living only in SQLite.** Metadata/hierarchy MUST be on-disk (files-as-truth); a
  cache-only store is rejected.
- **Notion-style databases, realtime CRDT collaboration, comments, public publishing, SSO** — remain
  SPEC §4 non-goals (unchanged).

## Traceability

| REQ-ID | Track | Phase | Status |
|--------|-------|-------|--------|
| TF-01 (bulk multi-tag) | T | 31 | planned |
| TF-02 (tag metadata) | T | 32 | planned |
| TF-03 (tag hierarchy) | T | 33 | planned |
| TF-04 (usage analytics) | T | 34 | planned |
| TF-05 (tag-op undo) | T | 35 | planned |
| EP-01 (`#tag` in MarkdownProse) | E | 36 | planned |
| EP-02 (copy-code button) | E | 36 | planned |
| EP-04 (description in search) | E | 36 | planned |
| EP-03 (insert-at-cursor templates) | E | 37 | planned |
| RE-01 (codec + corpus gate) | R | v1.7 | deferred |
| RE-02 (preserved verbatim tier) | R | v1.7 | deferred |
| RE-03 (tag/suggest/slash in TipTap) | R | v1.7 | deferred |
| RE-04 (toggle + host integration) | R | v1.7 | deferred |

**v1.6 = TF-01…05 (Phases 31–35, track T) + EP-01…04 (Phases 36–37, track E) = 7 phases.** The R
rows are the v1.7 backlog, preserved here so v1.7 starts warm.

> Phase numbers continue from v1.5's last (Phase 30). v1.6 spans Phases 31–37; the EP consolidation
> (36 = EP-01/02/04) and the TipTap split to v1.7 were decided by the co-driver-accepted red-team
> pre-mortem (2026-07-02).
