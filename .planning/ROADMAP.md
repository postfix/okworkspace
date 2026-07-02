---
author: architect
responsible: architect
milestone: v1.7
status: approved
last_updated: "2026-07-02"
parent_artifacts:
  - .planning/REQUIREMENTS.md
  - .planning/PROJECT.md
  - .planning/codebase/MAP-cm-editor-subsystem.md
red_team: folded (RoadmapRedTeam3, 2026-07-02 — GO-WITH-REVISIONS; MAJ-01…04 + MED-05…07 + MIN-08…10 folded into REQUIREMENTS.md + this roadmap)
---

# Roadmap: OKF Workspace

Milestone **v1.7 — Rich-Block Editor (TipTap)**. One track, five phases (38–42): an opt-in TipTap
(ProseMirror) WYSIWYG editor beside CM6, behind a two-tier Markdown-fidelity gate. CM6 stays THE
default and byte-exact surface.

> Phases continue from v1.6's last (Phase 37) → v1.7 spans **Phases 38–42** (5 phases).
>
> This is a **self-contained editor sub-project**, split out of v1.6 by the independent roadmap
> red-team (SPLIT-REQUIRED, co-driver-accepted). Its requirements + the red-team's R-track findings
> are locked in `REQUIREMENTS.md` (RE-01…05), with the findings folded (BLK-01 ordering resolved via
> RE-05, BLK-02/03 tightened, MAJ-03 extracted to its own phase, MAJ-04/05 to security review, the
> shared-content fidelity question elevated to a Phase-42 design fork).

## Milestones

- ✅ **v0.9.9 MVP** — Phases 0–7 (2026-06-23)
- ✅ **v1.0 Knowledge Graph & LLM Auto-Tagging** — Phases 8–12 (2026-06-24)
- ✅ **v1.1 Hardening & Polish** — Phases 13–17 (2026-06-27)
- ✅ **v1.2 Graph Schema Extension** — Phase 18 (2026-06-28)
- ✅ **v1.3 Tag Management UX** — built `9de5077` (2026-06-28); closed in v1.5
- ✅ **v1.4 Editor Enhancements & Authoring Polish** — Phases 21–27 (2026-07-01)
- ✅ **v1.5 Tag Regression Fix, Reading-Shell Fold-In & v1.3 Closure** — Phases 28–30 (2026-07-01)
- ✅ **v1.6 Tag System Completion & Authoring Polish** — Phases 31–37 (2026-07-02)
- 🔨 **v1.7 Rich-Block Editor (TipTap)** — Phases 38–42 (this milestone)

**Coverage:** every v1.7 requirement (RE-01…05) maps to exactly one phase. 5 phases · all covered.

## Phases

| #  | Phase | Goal | Requirements | Depends on | Security |
|----|-------|------|--------------|-----------|----------|
| 38 | Markdown Codec + Round-Trip Corpus (GO/NO-GO gate) | Headless TipTap↔Markdown codec (bounded slice) proven by a Tier-1 round-trip corpus written FIRST; NO-GO halts the milestone | RE-01 | — | — |
| 39 | Verbatim Preservation Tier | `PreservedBlock`/`PreservedInline` so tables/math/raw-HTML round-trip byte-for-byte (Tier 2) | RE-02 | 38 | **MAJ-04** raw-HTML `toDOM` |
| 40 | Editor-Host Abstraction (safe refactor) | Extract the save/reconcile/lock/409 host seam; mount CM6 through it as default with ZERO behavior change; extend `editorMode` to 3-way | RE-05 | — | — |
| 41 | TipTap Editor: `#tag` + Suggestions + Slash + Save | Mount the TipTap surface via the host; `#tag` node, `#`-suggest, `/`-slash; materialize tags through the REAL save | RE-03 | 38, 39, 40 | **MAJ-05** paste-XSS |
| 42 | Toggle + Load Guard + Code-Split (user-facing) | Expose the persisted 3-way toggle; warn-AND-fall-back load guard; code-split proven by `assertLazyChunks`; resolve the shared-content fidelity design fork | RE-04 | 40, 41 | load-guard boundary |

**Risk-first ordering rationale:**
- **The GO/NO-GO gate is FIRST (Phase 38).** TipTap cannot be byte-stable by construction (it
  serializes a ProseMirror tree, unlike CM6 whose doc IS the Markdown). Whether the bounded slice can
  round-trip losslessly is the milestone's load-bearing unknown. The corpus test is written first and
  fails for the right reason; if it can't pass after honest effort, the phase reports **NO-GO** and the
  **milestone STOPS** — no editor UI is built against a failing codec. This is a genuine spike gate,
  not a formality.
- **The scary refactor is isolated and de-risked (Phase 40, RE-05).** `PageEditor.tsx` is **949 lines**
  (verified on disk; red-team MAJ-03 estimated ~893). Extracting its save/reconcile/soft-lock/409
  machinery into a host seam is the milestone's biggest blast-radius change — so it is done as its OWN
  phase, mounting **CM6 through the new host as the default with zero behavior change**. qa verifies
  exactly that: byte-identical save/lock/409/tag-reconcile behavior. TipTap risk is added only AFTER
  the host is proven neutral.
- **Materialization ordering resolved (BLK-01).** RE-03's "materialize body `#tags` through the real
  save path" secretly depends on TipTap being mounted into a real host — NOT on RE-02. RE-05 (Phase 40)
  precedes RE-03 (Phase 41) so RE-03 crit-3 is tested through the real revision-checked save, not a
  headless harness.
- **Parallelization opportunity:** Phases 39 and 40 are independent of each other (39 extends the
  Phase-38 codec; 40 is a self-contained CM6-host refactor touching different files). They MAY run in
  parallel with explicit file-ownership. 41 depends on 38+39+40; 42 depends on 40+41.

**Security posture (per config: ASVS L1, block-on-high, code_review on):**
- **The novel untrusted-input surface is TipTap PASTE/serialization (Phase 41, MAJ-05)** — pasted
  HTML/Markdown must parse through the bounded-slice schema only; `script`/event-handler-attrs/`iframe`
  dropped or preserved-as-text, never rendered live (paste-XSS fixture). This is sharper than any v1.6
  surface.
- **Raw-HTML preservation `toDOM` (Phase 39, MAJ-04)** — the CM6 no-`innerHTML` invariant does NOT
  auto-apply to ProseMirror; raw HTML preserved as inert `textContent`/sanitized DOM, never `innerHTML`
  of page bytes (`<img onerror>` fixture). Both 39 + 41 are in the milestone-close security-review scope.
- **Byte-stability spirit for shared content (Phase 42 design fork)** — see below; a security-adjacent
  correctness property, resolved by co-driver decision.

**The design fork (Phase 42, co-driver pause):** Tier-1 marker normalization is lossless *per the
contract* but silently rewrites modeled-syntax bytes for ALL viewers of a shared page. Phase 42 discuss
runs **`anvil-design-options`** and **pauses for the co-driver** to choose: (i) informed-consent
documentation/notice, or (ii) a tightened any-byte-delta load guard. Does not block Phases 38–41.

**Verification posture:**
- Every phase: engineer builds → independent qa goal-backward verification; gaps route UP to the architect.
- **Phase 38 has a NO-GO exit** — qa verifies the corpus genuinely passes (tree-equality + idempotence),
  not that a test file merely exists. A degraded/skipped fixture set is a gap, not a pass.
- **Phase 40 is a no-behavior-change refactor** — qa verifies byte-identical editor behavior (the full
  existing editor suites pass unchanged), the hardest thing to fake and the whole point of isolating it.
- **Lesson from v1.6 (banked):** a phase touching a shared component runs that component's FULL existing
  suite, not just its new tests. Phases 40–42 all touch `PageEditor.tsx` / `editorMode.ts` — run the
  full editor + PageEditor suites, and the architect's close-time full smoke remains the backstop.

---

## Phase 38: Markdown Codec + Round-Trip Corpus (GO/NO-GO gate)

**Goal:** A headless TipTap↔Markdown codec (`@tiptap/markdown` + StarterKit configured to the bounded
slice + task lists) with shared `parse`/`serialize`/`roundTrip` helpers, proven by a Tier-1 round-trip
corpus test **written first as the acceptance gate**. If the supported corpus cannot pass, report NO-GO.

**Requirements:** RE-01

**Success criteria (observable):**
1. `buildExtensions()` + headless `parse`/`serialize`/`roundTrip` helpers exist, import-isolated (so
   the codec code-splits — proven later by `assertLazyChunks`), shared by the test and (later) the editor.
2. Tier-1 corpus fixtures (basic marks, headings, ordered/unordered/nested/task lists, blockquotes,
   fenced code incl. a `mermaid` fence kept as code, CJK+emoji, blockquote-mixed): each satisfies
   `parse(serialize(src)) ≡ parse(src)` (tree equality) AND idempotent serialization. All pass.
3. **(MAJ-02 — cross-engine parity, THE locked invariant)** The corpus ALSO asserts that
   `serialize(src)` matches the app's OTHER parsers for supported constructs — TipTap round-trip alone
   proves only TipTap-internal idempotence, and `@tiptap/markdown` is MarkedJS-backed (a 4th parser
   beside Goldmark / CM6 Lezer / `remark-gfm`). The locked invariant (MAP landmine #13, `markdown.ts:1-8`)
   is that the editor parse set MUST match server Goldmark GFM. Re-parse `serialize(src)` through the CM6
   Lezer set and/or Goldmark and diff — OR explicitly scope+document accepted divergence. Un-cross-checked
   round-trip is NOT a GO.
4. **(MAJ-04-b — no HTML→DOM path yet)** The codec MUST NOT parse raw embedded HTML into ProseMirror DOM
   (`@tiptap/markdown` can); raw HTML is dropped or deferred to Tier-2 (Phase 39) so no HTML→DOM path
   exists before Phase 39's inert-DOM guard.
5. The test was written first and failed for the right reason (missing module) before the codec existed
   (visible in the phase's TDD sequence). **(MIN-09)** nested/task lists + blockquote-mixed are the
   classic MarkedJS/ProseMirror serialization pain points — flag them at plan-time; a failure is a
   correct NO-GO or a Tier-2 re-spec, never a fudge.
6. TipTap deps pinned exact (**versions verified at plan-time, not recalled**); `npm audit` clean.
7. **GO/NO-GO:** on failure after honest config/custom-serializer effort, the phase reports NO-GO with
   failing fixtures + diffs, and the milestone halts pending co-driver direction.

**Key design decisions:** verify current `@tiptap/*` versions at plan-time (verify-don't-recall — a
research item). The corpus is the contract; marker normalization (`*`→`-`) is allowed if stable/idempotent.

**Key files:** new `web/src/lib/tiptap/` (codec + `buildExtensions` + helpers), a codec corpus test,
`web/package.json` (pinned deps).

**Depends on:** — (first phase; gates all others).

---

## Phase 39: Verbatim Preservation Tier

**Goal:** A `PreservedBlock`/`PreservedInline` mechanism so unmodeled constructs (tables, `$…$`/`$$…$$`
math, raw HTML) round-trip **byte-for-byte** through an edit session (Tier 2), with raw HTML kept inert.

**Requirements:** RE-02 · **SECURITY: MAJ-04**

**Success criteria (observable):**
1. `PreservedBlock` (block, `code:true`) + `PreservedInline` (inline mark, `code:true`) capture raw
   Markdown source at parse and re-emit unchanged; bridge tokenizers route tables/math/raw-HTML in.
2. Tier-2 corpus: preserved-construct + inert-prose fixtures round-trip byte-identical (modulo outer
   trim); idempotent. Inline math `$x_1$` `_` not escaped; inline HTML `<em>x</em>` stays literal;
   autolinks not swallowed.
3. Unit tests assert preserved constructs parse to a single preserved node/marked inline (not a reflow).
4. **SECURITY (MAJ-04):** raw HTML preserved as inert `textContent`/sanitized DOM — never `innerHTML`
   of page bytes; an `<img onerror=…>` fixture does NOT execute in the editor view.

**Key design decisions:** `code:true` skips serializer escaping — the crux of byte-for-byte. The
no-`innerHTML` invariant must be re-established explicitly for ProseMirror `toDOM` (it does not inherit
from CM6's `widgets.ts`).

**Key files:** `web/src/lib/tiptap/` (preserved nodes/marks + bridge handlers), a Tier-2 corpus test.

**Depends on:** Phase 38 (extends the codec/extension set).

---

## Phase 40: Editor-Host Abstraction (safe refactor, no behavior change)

**Goal:** Extract a shared editor-host seam from `PageEditor.tsx` (save / tag-reconcile / soft-lock /
revision-409 machinery), mount the CM6 `LivePreviewEditor` through it as the default with **zero
behavior change**, and extend `editorMode` to three-way — isolating the ~949-line refactor from
TipTap-feature risk and unblocking RE-03's real-save materialization.

**Requirements:** RE-05

**Success criteria (observable):**
1. A host abstraction mounts an editor surface behind a stable interface; CM6 is mounted through it as
   the default with byte-identical save/lock/409/tag-reconcile behavior.
2. **(MED-06 — the interface includes imperative insert-at-cursor)** the current insert paths are
   CM6-specific (`insertTemplateAtCursor` reaches `.cm-editor` `cmView.view.dispatch(replaceSelection)`,
   `PageEditor.tsx:529-542`; `insertLink` via `setBody`, `:507`). The host EITHER exposes a
   surface-agnostic `insertAtCursor(text)` OR scopes template/link insertion **CM6-only for v1.7**
   (stated, not implicit).
3. **(MED-05 — 3-way is code, not docs)** `editorMode.ts` extended to `"live" | "source" | "tiptap"`
   (extend store + key `okf.editor.mode`; NO new file — BLK-02). CM6 `toggle()` + `Mod-e` keymap keep
   cycling `live↔source` ONLY; `"tiptap"` is a **React-toolbar-only** selection that remounts the
   surface; `setMode` narrowed so it never receives `"tiptap"`. `"tiptap"` not yet user-selectable.
4. **Zero regression:** the full existing editor suites (`LivePreviewEditor.test.tsx`, CM6 `mode.ts` /
   keymap, `PageEditor.test.tsx` + `.conflict` + `.reconcile.adversarial`) show byte-identical behavior.
   **(MIN-10)** those `PageEditor` suites `vi.mock` `LivePreviewEditor`; re-pointing the mock to the new
   host surface is allowed (behavior byte-identical, mock seam moved), not a regression.

**Key design decisions:** structure-only change; the seam mirrors the existing `mode.ts`
`modeCompartment` toggle discipline (reconfigure without perturbing bytes). This phase adds NO TipTap
imports (keeps it out of the bundle until RE-04 code-splits it).

**Key files:** `web/src/routes/PageEditor.tsx` (extract host), a new editor-host module,
`web/src/stores/editorMode.ts` (3-way extension).

**Depends on:** — (independent of 38/39; MAY parallelize with 39).

---

## Phase 41: TipTap Editor — `#tag` + Suggestions + Slash + Real Save

**Goal:** Build the TipTap editor surface, mount it via the Phase-40 host, and add the authoring
affordances — inline `#tag` node, `#`-suggestions, `/`-slash commands — with body `#tags` materializing
through the REAL revision-checked save; harden the paste surface.

**Requirements:** RE-03 · **SECURITY: MAJ-05 (the sharp surface)**

**Success criteria (observable):**
1. A TipTap `Tag` inline node renders `#tag` as a badge and serializes to literal `#tag` (round-trips).
   Its recognized set is **display-only** (data integrity uses the Lezer path in crit-3, not this node),
   so a display mismatch is cosmetic — match the corpus where feasible (MAJ-01).
2. `#`-suggest popup offers vocabulary tags (reuse the tag vocabulary endpoint); `/`-slash inserts
   bounded-slice blocks. Both UI-only (schema unchanged).
3. **(BLK-01 — seam EXISTS + is wired; MAJ-01)** Body `#tags` materialize into frontmatter through the
   SAME save path CM6 uses — `reconcileFrontmatterTags` (`tag-reconcile.ts:51`, called in `runSaver` at
   `PageEditor.tsx:278`) running `inlineTagsFromBody` (Lezer, `hashtag.ts:140`). TipTap serializes to a
   body string and routes through the RE-05 host's `runSaver`; NO second materializer. Tested through
   the real revision-checked save, not a headless harness alone.
4. **SECURITY (MAJ-05 — paste):** pasted HTML/Markdown parses through the bounded-slice schema only;
   dangerous nodes (`script`, event-handler attrs, `iframe`) dropped or preserved-as-text, never live;
   paste-XSS fixture proves it.
5. **SECURITY (MAJ-03 — the suggest/slash POPUP is a 2nd untrusted render surface):** the popups render
   vocabulary tags + labels as `textContent` only, never `innerHTML` (a `@tiptap/suggestion` popup is
   raw DOM — unlike CM6's `TagSuggest`, which renders React text, it does NOT auto-escape); a
   markup-bearing fixture proves inertness. Both MAJ-05 + MAJ-03 → milestone security review.

**Key design decisions:** the ED-02b materialization seam ALREADY EXISTS and is wired
(`reconcileFrontmatterTags`, `web/src/lib/tag-reconcile.ts:51` ← `PageEditor.tsx:21,278`) — do NOT build
a second materializer. The `Tag` node is display; data integrity flows through the existing Lezer
recognizer over the serialized body string. Suggestions via `@tiptap/suggestion`.

**Key files:** `web/src/lib/tiptap/` (Tag node, suggestion + slash extensions, the React editor
component), the RE-05 host (mount TipTap), the real tag-materialization save seam.

**Depends on:** Phases 38, 39, 40.

---

## Phase 42: Toggle + Load Guard + Code-Split (user-facing opt-in)

**Goal:** Expose TipTap as a user-selectable opt-in mode behind the persisted preference, gate it at the
load boundary (warn AND fall back), prove it code-splits, and resolve the shared-content fidelity design
fork.

**Requirements:** RE-04 · **DESIGN FORK (co-driver pause)**

**Success criteria (observable):**
1. The persisted 3-way toggle is exposed in `PageEditor` UI; CM6 stays default; mode switch preserves
   content losslessly per the two-tier contract.
2. **(BLK-03 + MED-07)** Load guard checks `isLosslessRoundTrip` on **both** entries — opening a page in
   TipTap mode AND switching CM6→TipTap mid-session — against the **current live (possibly unsaved)
   buffer**, not just saved bytes. Non-round-trippable content → **warn AND fall back to CM6** (TipTap
   unavailable there, not merely discouraged). Byte-exact default never silently normalized.
3. **(MIN-04 + MIN-08 — dual-rooted forbidden set)** `assertLazyChunks` proves TipTap + WHOLE dep tree
   absent from the entry chunk: a shared forbidden set covering `@tiptap/*`, **`prosemirror-*`** (via
   `@tiptap/pm`), and **`marked`** (already in `MERMAID_FORBIDDEN` as "mermaid-UNIQUE",
   `vite.config.ts:41-52` — now dual-rooted via `@tiptap/markdown`; move to a shared constant + fix the
   stale comment).
4. TipTap-mode save goes through the revision-checked byte-stable save; stale-revision 409s as usual.
5. **DESIGN FORK resolved:** the Tier-1 shared-content normalization decision (informed-consent notice
   OR tightened any-byte-delta guard) is chosen by the co-driver and implemented.

**Key design decisions:** the design fork is decided at THIS phase's discuss (`anvil-design-options` +
co-driver pause). The load guard is a guard, not a toast (BLK-03).

**Key files:** `web/src/routes/PageEditor.tsx` (toggle UI + load guard), `web/src/stores/editorMode.ts`,
`web/vite-plugins/assertLazyChunks.ts` (`FORBIDDEN_IN_ENTRY`), `web/vite.config.ts`, the lazy import seam.

**Depends on:** Phases 40, 41.

---

## Milestone close (required steps)

- **Security review** (security-engineer) — scope centers on the two flagged surfaces: TipTap **paste**
  (MAJ-05, Phase 41) and raw-HTML **preservation** (MAJ-04, Phase 39). Cert engine on the keystone.
- **Documentation** (codebase-archaeologist) — a new `docs/` section + `MAP-tiptap-editor-subsystem.md`
  covering the codec, the two-tier contract, the host seam, and how it coexists with CM6; refresh the
  CM-editor map. Doc-verifier drift check.
- **MILESTONE-AUDIT** (architect) — refinement check grounded in a re-read of the real system: does
  opt-in TipTap satisfy the two-tier fidelity contract without violating the byte-exact CM6 default?
- **If Phase 38 returned NO-GO:** the milestone closes as a documented NO-GO (the fidelity gate is the
  contract; not shipping a failing codec is a *correct* outcome, not a failure to hide).
