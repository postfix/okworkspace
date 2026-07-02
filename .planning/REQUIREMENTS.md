---
author: architect
responsible: architect
milestone: v1.7
milestone_name: Rich-Block Editor (TipTap)
status: approved
last_updated: "2026-07-02"
parent_artifacts:
  - .planning/PROJECT.md
  - .planning/MILESTONES.md
  - .planning/codebase/MAP-cm-editor-subsystem.md
  - .planning/milestones/v1.6-REQUIREMENTS.md
---

# Milestone v1.7 Requirements — Rich-Block Editor (TipTap)

## Overview

A **self-contained editor sub-project**: add a TipTap (ProseMirror) WYSIWYG editor as an
**opt-in** editing mode beside the existing CodeMirror-6 `LivePreviewEditor`, behind a
**two-tier Markdown-fidelity gate**. CM6 stays THE default and the byte-exact surface; TipTap is
never forced, never the default.

This milestone was **split out of v1.6** by the independent roadmap red-team (2026-07-02,
SPLIT-REQUIRED, co-driver-accepted): the combined 11-phase draft was the largest in project
history, and TipTap is milestone-sized alone — a net-new ProseMirror↔Markdown serialization layer,
its own GO/NO-GO fidelity gate, and a ~949-line `PageEditor` host refactor. Its requirements + the
red-team's R-track findings were preserved in the v1.6 REQUIREMENTS §R so this milestone starts
warm; they are now promoted here to **active** with the red-team findings **folded in** (sequencing
resolved, acceptance tightened).

**Scope:** 5 requirements (RE-01…05) → **5 phases (38–42)**, strict risk-first ordering — the
round-trip corpus is the GO/NO-GO gate and comes FIRST; no TipTap UI is exposed to users until the
fidelity contract passes.

---

## Locked design decisions (promoted from v1.6 §R; co-driver-ratified)

### Global (unchanged, inherited)
- **Files-as-truth is inviolable (SPEC core value).** Every byte stays plain Markdown on disk,
  versioned in Git. TipTap must not fork the wiki's on-disk bytes silently (see the shared-content
  fidelity DESIGN FORK below).
- **Reuse the shipped byte-stable seams; no second implementation.** Saving from TipTap goes through
  the existing revision-checked byte-stable save (`pages.Save(baseRevision)` → single-writer commit);
  body `#tags` materialize through the SAME save path CM6 uses, never a divergent one.
- **Single-recognizer discipline.** The TipTap `#tag` recognizer matches the existing Lezer/Go
  corpus (`web/src/lib/cm/hashtag.ts` `inlineTagsFromBody` + `internal/okf/testdata/hashtag_corpus.json`);
  it does NOT introduce a third divergent `#tag` semantics.
- **Supply-chain discipline (v1.4/v1.5).** All TipTap deps pinned exact (no caret ranges); `npm audit`
  clean; the whole TipTap dep tree is **code-split** and proven out of the entry chunk by the existing
  `assertLazyChunks` guard (`web/vite-plugins/assertLazyChunks.ts`) — CM6-only users pay zero TipTap cost.

### TipTap-specific (the acceptance contract)
- **Coexist, do not replace.** CM6 `LivePreviewEditor` (`web/src/components/LivePreviewEditor.tsx`)
  stays THE default and the **byte-exact** surface — its round-trip holds *by construction* because the
  CM6 doc IS the raw Markdown and all rendering is view-layer decoration
  (`MAP-cm-editor-subsystem`). TipTap is an **opt-in alternate mode** (persisted per-user preference,
  like the existing Live/Source toggle).
- **Two-tier fidelity contract (the GO/NO-GO gate).** TipTap serializes a ProseMirror tree back to
  Markdown, so it **cannot** be byte-stable by construction the way CM6 is. The gate:
  - **Tier 1 — SUPPORTED syntax → semantic-lossless + idempotent.** `parse(serialize(md))` is
    tree-identical to `parse(md)`, and serialization is idempotent (one normalization pass, then
    byte-stable). Marker normalization (`*`→`-` bullets, etc.) is *allowed* but must be stable and
    predictable.
  - **Tier 2 — UNMODELED syntax → byte-for-byte.** Constructs the bounded slice does not model
    (tables, `$…$`/`$$…$$` math, raw HTML) survive an edit session **byte-identical** via a
    `PreservedBlock`/`PreservedInline` verbatim mechanism (`code:true` skips serializer escaping).
  - **Corpus round-trip test written FIRST as a spike gate.** If the supported corpus cannot pass
    after honest configuration effort, the track reports **NO-GO** with failing fixtures + diffs and
    STOPS — the gate is NOT degraded to force a ship.
- **Bounded slice.** TipTap models: paragraphs, headings (1–6), bold/italic/strike/inline-code,
  bullet/ordered/task lists, blockquote, fenced code, links, hard breaks — the StarterKit + task-list
  set `@tiptap/markdown` round-trips. Everything else is Tier-2 preserved verbatim. Richer *editing*
  of tables/math is explicitly deferred (Future).
- **Stack:** `@tiptap/core`, `@tiptap/pm`, `@tiptap/react`, `@tiptap/starter-kit`,
  `@tiptap/extension-list`, `@tiptap/extensions`, `@tiptap/suggestion`, `@tiptap/markdown` (pinned
  exact). Exact versions verified at RE-01 plan-time (verify-don't-recall — the version currency is a
  research item, not a memory claim).

---

## Requirements (active — v1.7)

> **Strict risk-first ordering.** RE-01 is the GO/NO-GO gate and comes first; if it reports NO-GO the
> milestone STOPS and reports to the co-driver (the whole track is gated on it). RE-05 (editor-host
> abstraction) precedes RE-03/RE-04 to resolve the red-team's BLK-01 ordering dependency and isolate
> the ~949-line `PageEditor` refactor (MAJ-03) from TipTap-feature risk.

### RE-01: Markdown codec + round-trip corpus (GO/NO-GO spike gate) — Phase 38
**Requirement:** A headless TipTap Markdown codec (`@tiptap/markdown`) with the bounded-slice
extension set, proven by a round-trip corpus test that is the **acceptance gate** for the whole track.
**Acceptance criteria:**
- [ ] A `buildExtensions()` schema set (StarterKit configured to the bounded slice + task lists +
      `Markdown`) and headless `parse`/`serialize`/`roundTrip` helpers exist, shared by the codec
      test and (later) the live editor so parse/serialize behavior is identical in both.
- [ ] **Tier-1 corpus:** for each fixture (basic marks, headings, ordered/unordered/nested/task
      lists, blockquotes, fenced code incl. a `mermaid` fence kept as code, CJK+emoji,
      blockquote-mixed), `parse(serialize(src)) ≡ parse(src)` (tree equality) AND serialization is
      idempotent (stable after one normalization pass). All pass.
- [ ] **(MAJ-02 — cross-engine parity, the LOCKED invariant):** TipTap-internal round-trip
      (`parse(serialize(src)) ≡ parse(src)`) proves TipTap idempotence ONLY — it does NOT prove parity
      with the app's other parsers. `@tiptap/markdown` is **MarkedJS-backed** (a 4th Markdown parser
      beside server Goldmark-GFM, the CM6 Lezer recognizer, and read-mode `remark-gfm`); the project's
      locked invariant (`MAP-cm-editor-subsystem` landmine #13, `markdown.ts:1-8`) is that the editor's
      parse set MUST match server Goldmark GFM. The corpus MUST therefore ALSO assert cross-engine
      parity for supported constructs: feed `serialize(src)` back through the existing CM6 Lezer
      recognizer (`inlineTagsFromBody`/`tagParserExtensions`) and/or a Goldmark reference and diff — OR
      the milestone explicitly scopes+documents accepted TipTap-mode divergence for supported syntax.
      An un-cross-checked "round-trip passes" is NOT a GO.
- [ ] **(MAJ-04 — no HTML→DOM path before Phase 39):** the bounded-slice codec MUST NOT parse raw
      embedded HTML into ProseMirror DOM (`@tiptap/markdown` can, via TipTap's HTML parsing). Raw HTML
      is dropped or deferred to the Tier-2 preserved tier (Phase 39) — so no HTML→DOM execution path
      exists in Phase 38 before Phase 39's inert-DOM guard lands.
- [ ] The test is **written first and fails for the right reason** (missing module) before the codec
      exists; if the supported corpus cannot be made to pass after honest config/custom-serializer
      effort, the phase reports **NO-GO** with the failing fixtures + diffs and the track STOPS.
      **(MIN-09):** nested lists, task lists, and blockquote-mixed are the classic ProseMirror/MarkedJS
      serialization pain points (tight/loose lists, indent ambiguity) — flag them at plan-time; a
      failure there is a correct NO-GO or a re-spec to Tier-2, never a fudge.
- [ ] TipTap + deps pinned exact (**versions verified at plan-time, not recalled**); `npm audit` clean;
      the codec module is import-isolated so it code-splits (proven later by `assertLazyChunks`).

### RE-02: Verbatim preservation tier (PreservedBlock / PreservedInline) — Phase 39 · SECURITY (MAJ-04)
**Requirement:** Unmodeled constructs (tables, `$…$`/`$$…$$` math, raw HTML) round-trip
**byte-for-byte** through a preserved-node/mark mechanism.
**Acceptance criteria:**
- [ ] `PreservedBlock` (block node, `code:true`, editable literal mono text) and `PreservedInline`
      (inline mark, `code:true`) capture the raw Markdown source of unmodeled constructs at parse time
      and re-emit it unchanged on serialize; bridge tokenizers/handlers route tables, math, and raw
      HTML into them.
- [ ] **Tier-2 corpus (byte equality):** fixtures containing only preserved constructs + inert prose
      round-trip byte-identical (modulo outer trim); serialization is idempotent. Inline math `$x_1$`
      does NOT get its `_` escaped; inline HTML `<em>x</em>` stays literal (not converted to a mark);
      autolinks are not swallowed.
- [ ] Unit tests assert preserved constructs parse to a single `preservedBlock`/marked inline, not a
      fallback that reflows them.
- [ ] **SECURITY (MAJ-04):** raw HTML is preserved as **inert text (`textContent`) or via a sanitized
      DOM path — never `innerHTML` of page bytes**. The CM6 no-`innerHTML` invariant does NOT
      auto-apply to ProseMirror `toDOM`; this must be enforced explicitly. An `<img onerror=…>` fixture
      does NOT execute in the editor view. Flagged for the milestone security review.

### RE-05: Editor-host abstraction (the safe refactor, no behavior change) — Phase 40
**Requirement:** Extract a shared editor-host seam so `PageEditor` can mount either editor, with CM6
mounted through it as the default and **zero behavior change** — isolating the large host refactor
(MAJ-03) from TipTap-feature risk and resolving the BLK-01 ordering dependency (RE-03's materialization
needs the host in place).
**Acceptance criteria:**
- [ ] A host abstraction extracts `PageEditor.tsx`'s save / tag-reconcile / soft-lock / revision (409)
      machinery so an editor surface is mounted behind a stable interface; **CM6 `LivePreviewEditor`
      is mounted through it as the default** with byte-identical behavior.
- [ ] **(MED-06 — the interface must include imperative insert-at-cursor):** the current insert paths
      are CM6-specific — `insertTemplateAtCursor` (EP-03) reaches `.cm-editor` `cmView.view` and calls
      `view.dispatch(view.state.replaceSelection(...))` (`PageEditor.tsx:529-542`), `insertLink` appends
      via `setBody` (`PageEditor.tsx:507`). The host interface EITHER exposes a surface-agnostic
      `insertAtCursor(text)` each surface implements, OR the phase explicitly scopes template/link
      insertion as **CM6-only for v1.7** (TipTap mode omits those affordances) — stated, not left implicit.
- [ ] **(MED-05 — the 3-way toggle is code, not just docs):** `mode.ts` `setMode(view, mode)` +
      `toggle()` + the `Mod-e` `toggleKeymap` encode 2-way CM6-internal logic and are inactive when
      TipTap is mounted. So: `editorMode.ts` extends to `EditorMode = "live" | "source" | "tiptap"`
      (BLK-02 — extend the store + key `okf.editor.mode`, NO new file); the CM6 `toggle()` + `Mod-e`
      keymap keep cycling **`live`↔`source` ONLY**; `"tiptap"` is a **React-toolbar-only** selection
      that remounts the surface; `setMode` is narrowed so it can never receive `"tiptap"` (no type break
      leaks into the CM6 seam). These are acceptance items, not just "documented".
- [ ] **Zero regression:** the full existing editor test suites (`LivePreviewEditor.test.tsx`, CM6
      `mode.ts` / keymap tests, `PageEditor.test.tsx` + `.conflict` + `.reconcile.adversarial`) show
      byte-identical save/lock/409/tag-reconcile behavior. **(MIN-10):** those `PageEditor` suites
      `vi.mock('../components/LivePreviewEditor')`; extracting the host MAY require re-pointing the mock
      to the new host surface — that is an allowed change (behavior byte-identical, mock seam moved),
      not a regression.

### RE-03: `#tag` + suggestions + slash commands in TipTap — Phase 41 · SECURITY (MAJ-05)
**Requirement:** The TipTap editor supports the app's authoring affordances — inline `#tag` atoms,
`#`-triggered tag suggestions, `/`-triggered slash commands — consistent with the existing tag model;
mounted via the RE-05 host so it saves through the real path.
**Acceptance criteria:**
- [ ] **(MAJ-01 clarified — data-integrity uses the EXISTING Lezer path, not the Tag node)** A TipTap
      `Tag` inline node renders `#tag` as a badge and serializes back to literal `#tag` text
      (round-trips). Its recognized set is a **display concern only** — tag DATA integrity does not
      depend on it, because materialization (next criterion) runs the serialized body through the
      EXISTING Lezer recognizer. Match the display set to the corpus where feasible (pinned fixture); a
      display mismatch is cosmetic, not a data fork.
- [ ] `#`-suggestion popup offers existing-vocabulary tags (reusing the tag vocabulary endpoint);
      `/`-slash-command popup inserts bounded-slice blocks (heading, list, code, quote). Both are
      UI-only extensions that do not alter the serialized schema.
- [ ] **(BLK-01 resolved — the seam EXISTS and is wired; MAJ-01)** Body `#tags` authored in TipTap
      materialize into frontmatter through the SAME save path CM6 uses: `reconcileFrontmatterTags`
      (`web/src/lib/tag-reconcile.ts:51`, imported + called in `runSaver` at `PageEditor.tsx:21,278`),
      which runs `inlineTagsFromBody` (`hashtag.ts:140`, the single Lezer recognizer) over the body
      STRING. TipTap only serializes its tree to a Markdown body string and routes it through the RE-05
      host's `runSaver` — **do NOT build a second materializer.** Tested through the real
      revision-checked save, not a headless harness alone.
- [ ] **SECURITY (MAJ-05 — paste, the sharpest surface):** pasted HTML/Markdown parses through the
      bounded-slice schema only; dangerous nodes (`script`, event-handler attrs, `iframe`) are dropped
      or preserved-as-text, **never rendered live**; a paste-XSS fixture proves it.
- [ ] **SECURITY (MAJ-03 — the suggestion/slash POPUP is a second untrusted render surface):** the
      `#`-suggest and `/`-slash popups are custom-rendered (a `@tiptap/suggestion` popup is raw DOM and
      does NOT inherit React's auto-escaping the way CM6's `TagSuggest` does — `TagSuggest.tsx` renders
      tag names as React text children, NEVER `dangerouslySetInnerHTML`). The TipTap popups MUST render
      vocabulary tags + command labels as `textContent` only, never `innerHTML`; add a fixture with a
      tag/label bearing markup (`<img onerror>`/`</script>`) proving it stays inert. Both MAJ-05 and
      MAJ-03 are scoped into the milestone security review.

### RE-04: Editor-mode toggle + host integration (user-facing opt-in) — Phase 42 · DESIGN FORK
**Requirement:** Expose TipTap as a user-selectable opt-in editing mode, behind the persisted
preference, with the fidelity gate enforced at the load boundary and the byte-exact CM6 default never
violated without user awareness.
**Acceptance criteria:**
- [ ] The persisted three-way toggle (RE-05's `editorMode`) is exposed in the `PageEditor` UI; the
      CM6 path stays the default; switching modes preserves current Markdown content losslessly per the
      two-tier contract.
- [ ] **(BLK-03 resolved — guard, not toast; MED-07 — guard BOTH entries, on the LIVE buffer)** The
      load guard checks `isLosslessRoundTrip` on **both** entries into TipTap mode: opening a page in
      TipTap mode AND switching CM6→TipTap mid-session — and it checks the **current live (possibly
      unsaved) buffer**, not just the saved page bytes. If the content can't round-trip losslessly (and
      isn't covered by the preserved tier), the UI **warns AND falls back to CM6** for that content —
      TipTap made unavailable there, not merely discouraged. The byte-exact default is never silently
      normalized.
- [ ] **(MIN-04 + MIN-08 — the forbidden set is dual-rooted now)** TipTap + deps are code-split and
      proven absent from the entry chunk by `assertLazyChunks`. Add a **shared** forbidden set covering
      the WHOLE TipTap tree: `@tiptap/*` (`/core`, `/pm`, `/react`, `/starter-kit`, `/markdown`,
      `/extension-list`, `/extensions`, `/suggestion`), **`prosemirror-*`** (pulled via `@tiptap/pm`),
      and **`marked`**. Note `marked` is ALREADY in `MERMAID_FORBIDDEN` labeled "mermaid-UNIQUE"
      (`vite.config.ts:41-52`) — `@tiptap/markdown`→`marked` makes it **dual-rooted**, so move it to a
      shared forbidden constant and fix the stale "mermaid-unique" comment. `maxEntryBytes` stays the backstop.
- [ ] Saving from TipTap mode goes through the existing revision-checked byte-stable save
      (`pages.Save(baseRevision)` → single-writer commit); a stale-revision save 409s as usual.
- [ ] **DESIGN FORK resolved in-phase (see below):** the shared-content Tier-1-normalization decision
      is made and implemented (informed-consent documentation OR a tightened any-byte-delta load guard).

---

## The DESIGN FORK (RE-04 discuss-phase; co-driver decision — red-team direct-answer C)

**Tier-1 marker normalization is *lossless per the contract*, so `isLosslessRoundTrip` does NOT catch
it.** In a 5-person team, one user editing a **shared** page in TipTap silently rewrites modeled-syntax
bytes (bullets `*`→`-`, heading style, spacing) for ALL viewers — forking the wiki's on-disk bytes by
editor and surprising the CM6 author who owns the page. It is still Markdown (core value intact) but
breaks byte-stability's *spirit* for shared content. **v1.7 (Phase 42 discuss) must decide:**
- **(i) Informed consent** — document + surface that TipTap edits normalize modeled syntax for all
  viewers (a one-time notice / mode-switch confirmation), accepting the normalization openly; OR
- **(ii) Tighten the load guard** — warn on ANY non-preserved byte delta (not just unmodeled-content
  fallback), so a page whose modeled syntax would normalize also warns/falls back.

This is a genuine >1-defensible-answer fork → **`anvil-design-options` at Phase 42 discuss + a co-driver
pause** (per the autonomous workflow). It does NOT block Phases 38–41.

---

## Red-team findings — disposition (all folded)

The 2026-07-02 roadmap red-team (agent `RoadmapRedTeam-2`, HIGH/direct-read) findings, now resolved:

| Finding | Disposition |
|---------|-------------|
| **BLK-01** (RE-03 materialization depends on host mount, not RE-02) | RESOLVED — RE-05 (host, Phase 40) precedes RE-03 (Phase 41); RE-03 crit-3 tests through the real save. |
| **BLK-02** (`editorMode.ts` already exists) | FOLDED — RE-05 **extends** `web/src/stores/editorMode.ts` to 3-way; no new file. Verified on disk. |
| **BLK-03** (load guard "and/or" is non-deterministic) | FOLDED — RE-04 tightened to "warn **AND** fall back" (guard, not toast). |
| **Shared-content fidelity (direct-answer C)** | ELEVATED to the DESIGN FORK above → `anvil-design-options` + co-driver pause at Phase 42. |
| **MAJ-03** (bundled ~949-line host refactor) | RESOLVED — extracted into its own phase (RE-05, Phase 40), no-behavior-change, verified for zero regression. |
| **MAJ-04** (raw-HTML `toDOM` untrusted surface) | FOLDED into RE-02 acceptance (inert `textContent`/sanitized DOM, `<img onerror>` fixture) + security review. |
| **MAJ-05** (TipTap paste is the sharp XSS surface) | FOLDED into RE-03 acceptance (bounded-slice-only paste, paste-XSS fixture) + security review. |
| **MIN-02** (`mermaid` renders in CM6, raw code in TipTap) | ACCEPTED — bytes preserved; TipTap second-class for mermaid pages; noted. |
| **MIN-04** (`FORBIDDEN_IN_ENTRY` must cover whole dep tree) | FOLDED into RE-04 code-split acceptance. |

### RoadmapRedTeam3 gate (2026-07-02, independent — attacked the v1.7 roadmap; verdict GO-WITH-REVISIONS)

A second independent gate ran against THIS v1.7 roadmap (not the v1.6 draft). Verdict **GO-WITH-REVISIONS**
— architecture sound (risk-first correct, GO/NO-GO genuine, BLK-01 resolution sufficient, 39‖40 truly
disjoint, design-fork deferral correct); no blocker forces re-split. All findings folded above:

| Finding | Disposition |
|---------|-------------|
| **MAJ-01** (the ED-02b seam EXISTS + is wired — roadmap wrongly said it doesn't) | FOLDED into RE-03 + the roadmap corrected. `reconcileFrontmatterTags` (`tag-reconcile.ts:51`, wired `PageEditor.tsx:21,278`) runs `inlineTagsFromBody` (Lezer). TipTap serializes to a body string → routes through the SAME `runSaver`; NO second materializer. Favorable (less work). |
| **MAJ-02** (RE-01 round-trip is TipTap-self-referential, not cross-engine parity) | FOLDED into RE-01 — the corpus MUST also assert parity vs the CM6 Lezer/Goldmark set (the locked invariant, MAP landmine #13); MarkedJS is a 4th parser. Un-cross-checked round-trip is not a GO. |
| **MAJ-03** (the `#`/`/` suggestion popup is a 2nd untrusted render surface, not just paste) | FOLDED into RE-03 security — popups render `textContent` only, never `innerHTML`; markup-bearing fixture; into security review. |
| **MAJ-04-b** (raw HTML is a Phase-38 CODEC concern, not only Phase 39) | FOLDED into RE-01 — the codec MUST NOT parse raw HTML→DOM; drop/defer to Tier-2, so no HTML→DOM path exists before Phase 39's guard. |
| **MED-05** (RE-05 3-way is code, not docs — `setMode`/`Mod-e` are CM6-internal 2-way) | FOLDED into RE-05 — CM6 `toggle()`/`Mod-e` cycle `live↔source` only; `"tiptap"` is a React-toolbar remount; `setMode` narrowed to never receive `"tiptap"`. |
| **MED-06** (RE-05 interface omits imperative insert-at-cursor — template/link are CM6-specific) | FOLDED into RE-05 — expose a surface-agnostic `insertAtCursor(text)` OR scope template/link insertion CM6-only for v1.7 (stated). |
| **MED-07** (BLK-03 guard fires on open but not on mid-session switch) | FOLDED into RE-04 — guard BOTH open + switch, on the current live buffer. |
| **MIN-08** (`marked` is dual-rooted via `@tiptap/markdown`; `prosemirror-*` uncovered) | FOLDED into RE-04 — shared forbidden set incl. `prosemirror-*` + `marked`; fix the "mermaid-unique" comment. |
| **MIN-09** (nested lists / task lists / blockquote-mixed likely gate failures) | FOLDED into RE-01 — flagged at plan-time; a failure is a correct NO-GO or Tier-2 re-spec. |
| **MIN-10** (RE-05 "tests pass unchanged" optimistic — `PageEditor` suites mock `LivePreviewEditor`) | FOLDED into RE-05 — behavior byte-identical; the mock seam MAY move to the host. |

**Roadmap facts CONFIRMED correct by the gate:** `PageEditor.tsx` = 949 lines exact; `editorMode` consumer
set low-blast; `assertLazyChunks` takes `forbidden: RegExp[]` (extensible); the `#tag` recognizer internals
are module-private (only `inlineTagsFromBody`/`inlineTagSpansFromBody`/`tagParserExtensions` exported);
39‖40 truly disjoint (39=`web/src/lib/tiptap/*`, 40=`PageEditor.tsx`+host+`editorMode.ts`; `PageView.tsx`
read path untouched); BLK-01 resolution sufficient; design-fork deferral correct (compose
`isLosslessRoundTrip` at Phase 42 from Phase-38 `roundTrip` + Phase-39 preserved tier, wired per MED-07).

---

## Future Requirements (deferred)

- **TipTap rich editing of tables/math** — v1.7 preserves them verbatim (Tier 2); structured WYSIWYG
  editing of tables and math is a follow-on.
- **TipTap as a possible default / CM6 retirement** — a v2.0 question, not this minor. CM6 stays the
  byte-exact default here.

## Out of Scope (explicit)

- **Replacing CM6 with TipTap.** CM6 stays the default, byte-exact surface; TipTap is opt-in only.
- **Degrading the fidelity gate to ship TipTap.** If the round-trip corpus cannot pass, the track is
  NO-GO — the gate is not relaxed to force a ship.
- **A third `#tag` parser semantics.** TipTap tag recognition matches the existing corpus/grammar.
- **Notion-style databases, realtime CRDT collaboration, comments, public publishing, SSO** — remain
  SPEC §4 non-goals (unchanged).

## Traceability

| REQ-ID | Phase | Depends on | Security | Status |
|--------|-------|-----------|----------|--------|
| RE-01 (codec + corpus GO/NO-GO gate) | 38 | — | — | planned |
| RE-02 (verbatim preserved tier) | 39 | 38 | MAJ-04 (raw HTML `toDOM`) | planned |
| RE-05 (editor-host abstraction) | 40 | — (independent of 38/39) | — | planned |
| RE-03 (`#tag`/suggest/slash in TipTap) | 41 | 38, 39, 40 | MAJ-05 (paste-XSS) | planned |
| RE-04 (toggle + host integration) | 42 | 40, 41 | (load-guard boundary) | planned |

**v1.7 = RE-01…05 = 5 phases (38–42).** Phase numbers continue from v1.6's last (Phase 37).

> **GO/NO-GO:** Phase 38 (RE-01) is the milestone-gating spike. NO-GO there halts the milestone and
> returns to the co-driver. Phases 39 + 40 are independent of each other (39 extends the codec; 40 is
> a self-contained CM6 refactor) and MAY be parallelized; 41 depends on 38+39+40; 42 depends on 40+41.
