---
author: architect
responsible: architect
phase: 38
phase_type: implementation
status: Ready for planning
hard_bar: true
security_relevant: true
design_fork: false
parent_artifacts:
  - .planning/ROADMAP.md
  - .planning/REQUIREMENTS.md
  - .planning/codebase/MAP-cm-editor-subsystem.md
---

# Phase 38: Markdown Codec + Round-Trip Corpus (GO/NO-GO gate) - Context

**Gathered:** 2026-07-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Build a **headless** TipTap↔Markdown codec (`@tiptap/markdown`'s `MarkdownManager`, no
Editor/DOM) configured to the bounded slice, and a **round-trip corpus test written FIRST**
that is the **GO/NO-GO acceptance gate** for the entire v1.7 milestone. If the supported
corpus cannot pass after honest effort, the phase reports **NO-GO** with failing fixtures +
diffs and the milestone halts pending co-driver direction.

**This is a spike gate, not a formality.** The whole milestone is gated on this phase. No
TipTap UI, no editor host, no `#tag` node — ONLY the codec + the corpus.

**Requirements:** RE-01

**Non-goals (explicitly out of this phase):**
- The verbatim preservation tier (tables/math/raw-HTML byte-for-byte) — that is RE-02/Phase 39.
- Any React editor component, mounting, toolbar, or `@tiptap/react` — Phases 40–42.
- The `#tag` node, suggestions, slash commands — RE-03/Phase 41.
- `assertLazyChunks` wiring / code-split proof — RE-04/Phase 42 (the codec is only made
  *import-isolated* here so it CAN code-split later; the guard itself lands in 42).
- Reconciling mermaid's `marked@16` with TipTap's `marked@17` beyond noting it (see landmines).

**Parallelization:** independent of Phase 39; MAY run in parallel with it (39 extends this
codec but is a separate file set). No dependency on 40.
</domain>

<decisions>
## Implementation Decisions

### D-01: Headless codec via `MarkdownManager` (NOT an Editor)
- Use `@tiptap/markdown`'s **`MarkdownManager`** directly — it exposes `.parse(md): JSONContent`
  and `.serialize(json): string` and runs **fully headless (no Editor, no DOM)** (research-verified
  empirically under plain node). Do NOT construct an `Editor` for the codec (that needs a DOM).
- A shared **`buildExtensions()`** returns the bounded-slice extension array, consumed by BOTH the
  codec test now AND (later) the live editor — so parse/serialize behavior is identical in both
  surfaces (RE-01 crit-1). Bounded-slice config (research-verified):
  ```
  StarterKit.configure({ horizontalRule:false, underline:false, dropcursor:false,
    gapcursor:false, trailingNode:false, undoRedo:false })  // + TaskList, TaskItem
  ```
  StarterKit 3.x already bundles paragraphs, headings, bold/italic/strike/code, code block,
  blockquote, hard break, link, and the list nodes (via `@tiptap/extension-list`). **TaskList +
  TaskItem are NOT in StarterKit** — import them from `@tiptap/extension-list` separately.
- Module layout: new `web/src/lib/tiptap/` — `extensions.ts` (`buildExtensions()`),
  `codec.ts` (`parse`/`serialize`/`roundTrip` helpers over `MarkdownManager`), kept
  **import-isolated** (no static import reachable from the app entry) so RE-04 can code-split it.

### D-02: The gate is TREE-equality + idempotence, NOT byte-equality vs source
- The serializer **canonicalizes** markers (research-verified: bullets `*`/`+`→`-`, emphasis
  `__`→`**` / `_`→`*`, loose lists tightened). This is **allowed** Tier-1 normalization. So a
  naive byte-equality gate against arbitrary source WILL false-fail.
- **The gate (RE-01 crit-2):** for each fixture, `parse(serialize(src)) ≡ parse(src)` — TipTap
  **tree/JSON equality** — AND serialization is **idempotent**: `serialize(parse(src))` is stable
  on a second pass (`serialize(parse(x)) === serialize(parse(serialize(parse(x))))`). Marker
  normalization is fine as long as it's stable and predictable.

### D-03: Cross-engine parity is REQUIRED, not optional (MAJ-02 — the locked invariant)
- TipTap's own round-trip proves only TipTap-internal idempotence. The project's **locked
  invariant** (`markdown.ts:1-8`, MAP landmine #13) is that the editor's parse set MUST agree with
  server Goldmark GFM + the read view — and `@tiptap/markdown` is **MarkedJS-backed** (`marked@17`),
  a **4th** Markdown parser beside Goldmark, the CM6 `@lezer/markdown` GFM config, and read-mode
  `remark-gfm`.
- **The corpus MUST also assert cross-engine parity for supported constructs:** re-parse
  `serialize(src)` through the **same** `@lezer/markdown` GFM config the CM6 editor uses
  (`[Table, Strikethrough, TaskList, Autolink]` over `commonmarkLanguage`, per `markdown.ts:33-37`;
  `@lezer/markdown` 1.6.4 is already installed and headless) and diff the parse trees for the
  bounded-slice constructs. A construct where TipTap's serialization parses to a DIFFERENT Lezer
  structure than the source did is a **divergence** — either fix it, or explicitly scope+document it
  as accepted TipTap-mode divergence. **An un-cross-checked "round-trip passes" is NOT a GO.**

### D-04: Security — the codec MUST NOT parse raw HTML→DOM (MAJ-04-b), enforced by a `node` test env
- Research-verified: `MarkdownManager` only turns embedded HTML into DOM nodes when
  `window.DOMParser` EXISTS; in a pure-node env it returns HTML as entity-escaped **literal text**
  (safe, no live DOM). Confirmed: `<div onclick><script>…` → escaped literal text.
- **BUT the app's `web/vitest.config.ts` sets `environment: "jsdom"` globally** — jsdom provides
  `window.DOMParser`, which would RE-ENABLE HTML→DOM parsing in the codec test and defeat the
  property. **Decision:** the codec test MUST run under a **`node`** environment (per-file
  `// @vitest-environment node` docblock, or a dedicated project/config entry), NOT the global jsdom.
  A test asserting an `<img onerror=…>`/`<script>` fixture stays inert literal text is part of the
  gate. This is the concrete Phase-38 slice of MAJ-04 (the full preserved-tier HTML handling is
  Phase 39).

### D-05: Exact pinned versions (research-verified; supply-chain discipline)
- All `@tiptap/*` on the unified **3.27.1** line (peerDeps pin core+pm to exactly 3.27.1 — the set
  moves in lockstep). Phase 38 needs ONLY the codec deps (NO `@tiptap/react` — that's editor UI,
  Phase 40+):
  ```
  "@tiptap/core": "3.27.1", "@tiptap/pm": "3.27.1",
  "@tiptap/starter-kit": "3.27.1", "@tiptap/extension-list": "3.27.1",
  "@tiptap/markdown": "3.27.1"
  ```
- Pinned EXACT (no caret — v1.4/v1.5 supply-chain discipline); `npm audit` clean; re-run the corpus
  on every bump (the package is young — first release 2025-10-14, ~weekly patches).

### The agent's discretion
- The exact tree-equality comparison (deep-equal on `MarkdownManager.parse` JSON vs a normalized
  form) and how the Lezer parity diff is structured (full-tree vs node-type sequence).
- Corpus fixture organization (one file of fixtures vs per-construct).
- Whether the Lezer parity check is a hard assertion per fixture or a reported diff for the
  supported subset (planner decides; a divergence must at minimum be surfaced, not swallowed).
- Commit slicing.
</decisions>

<disciplines>
## Lenses surfaced for this phase

~~~yaml
disciplines:
  - section: "D-02/D-03 the GO/NO-GO gate"
    name: "gate-is-not-degraded-to-ship"
    scope: ["the round-trip corpus test", "web/src/lib/tiptap/codec.ts"]
    guidance: "The corpus is the acceptance contract for the WHOLE milestone. If the bounded
      slice cannot pass tree-equality + idempotence + cross-engine parity after honest
      configuration/custom-serializer effort, the phase reports NO-GO with the failing fixtures +
      diffs. It does NOT relax the assertion, skip a fixture, or widen 'normalization' to hide a
      real divergence to force a GO. A NO-GO is a correct, honest outcome — not a phase failure."
    rationale: "RE-01 exists precisely because TipTap cannot be byte-stable by construction. The
      value of the gate is that it can say NO. Degrading it to ship defeats the milestone's reason
      for being risk-first."
    override_signals:
      - "None. A failing supported-corpus fixture is either fixed honestly or reported as NO-GO."
  - section: "D-04 no HTML->DOM"
    name: "codec-runs-headless-node-never-jsdom"
    scope: ["the codec test", "web/vitest env for tiptap"]
    guidance: "The no-HTML->DOM security property holds ONLY when window.DOMParser is absent. The
      codec test MUST run under a `node` environment, never the app's global jsdom. Do not import
      or construct anything that provides a DOM into the codec path. Assert embedded HTML stays
      inert literal text."
    rationale: "MAJ-04-b: ProseMirror's HTML parsing is an untrusted-input surface; the CM6
      no-innerHTML invariant does not auto-apply. jsdom in the test env would silently re-open the
      HTML->DOM path and give a false-safe pass."
    override_signals:
      - "None for v1.7 — the codec has no reason to parse HTML to DOM."
~~~

</disciplines>

<canonical_refs>
## Canonical References

### Phase spec & requirements
- `.planning/ROADMAP.md` §"Phase 38: Markdown Codec + Round-Trip Corpus" — goal, 7 success criteria, GO/NO-GO.
- `.planning/REQUIREMENTS.md` §RE-01 (+ the folded MAJ-02 cross-engine parity, MAJ-04-b codec-HTML, MIN-09 likely-fail fixtures).

### The cross-engine parity target (D-03 / MAJ-02)
- `web/src/lib/cm/markdown.ts:33-37` — the CM6 Lezer GFM config `markdown({ base: commonmarkLanguage, extensions: tagParserExtensions })` and the parity invariant comment (`:1-8`).
- `web/src/lib/cm/hashtag.ts` — `tagParserExtensions = [Table, Strikethrough, TaskList, Autolink, hashtagExtension]` (the shared Lezer extension set; for parity use the GFM subset).
- `@lezer/markdown` 1.6.4 (installed, headless) — `parser.configure([GFM]).parse(src): Tree`.

### Test/build environment
- `web/vitest.config.ts` — global `environment: "jsdom"` (D-04: the codec test must OVERRIDE to `node`).
- `web/package.json` — scripts `test: vitest run`, `build: tsc -b && vite build`; vitest ^3.2.4, React 19.2.7.
- `web/package-lock.json` — current `marked` = **16.4.2** (via mermaid); TipTap needs `marked ^17` (landmine).

### External (research-verified — see `agent://TipTapCodecResearch`)
- `@tiptap/markdown` 3.27.1 `MarkdownManager` — headless `.parse`/`.serialize`; HTML→literal-text in node.
- Per-extension `renderMarkdown`/`parseMarkdown`/`markdownTokenizer` hooks (for later phases).
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable assets
- `@lezer/markdown` (already installed via `@codemirror/lang-markdown`) — the parity parser, headless.
- The CM6 Lezer GFM config in `markdown.ts` — the exact parity target (do not re-derive a different GFM set).
- vitest 3.2.4 harness — add a `node`-env codec test alongside the existing jsdom suites.

### Established patterns
- Exact-pinned deps, `npm audit` in CI (v1.4/v1.5 supply-chain discipline).
- Import-isolation for code-split-later modules (the mermaid/lang-grammar pattern — dynamic import, out of entry).
- TDD sequence visible in the phase (test written first, fails for the right reason).

### Landmines
- **`marked` major mismatch:** app carries `marked@16.4.2` (mermaid); TipTap needs `marked@17`. npm dedupes only within a major → expect TWO `marked` copies. Both MUST stay in lazy chunks (never entry); `marked` is already in the `assertLazyChunks` forbidden-entry set (`vite.config.ts:48`). Full reconciliation is Phase 42's concern; Phase 38 only must not pull `marked` into the entry chunk (it won't — the codec is import-isolated and unused by the app entry until Phase 42).
- **jsdom test env defeats the HTML→DOM guard (D-04)** — the single biggest security foot-gun here. Global jsdom provides `window.DOMParser`. The codec test MUST be `node`-env.
- **Byte-equality is the wrong gate (D-02)** — the serializer canonicalizes; assert tree-equality + idempotence, not source byte-match.
- **NO-GO is a real exit** — nested/task lists + blockquote-mixed are the classic MarkedJS/ProseMirror serialization pain points (research + MIN-09). If one genuinely can't pass, report NO-GO or re-spec that construct to Tier-2 — never fudge the assertion.
- **Young package** — 3.27.1, ~weekly patches; pin exact and re-run the corpus on any bump.
</code_context>

<deferred>
## Deferred Ideas

- **marked@16↔@17 dedup / single-copy reconciliation** — surfaced here, owned by Phase 42 (code-split + forbidden-set work). Phase 38 only avoids pulling marked into entry.
- **`#tag` node markdown tokenizer** — the `markdownTokenizer` hook exists (research Q7); RE-03/Phase 41.
- **Preserved-tier custom `renderMarkdown`/`parseMarkdown`** — RE-02/Phase 39.
</deferred>

---

*Phase: 38-markdown-codec-roundtrip-corpus*
*Context gathered: 2026-07-02*
