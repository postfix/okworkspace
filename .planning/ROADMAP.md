# Roadmap: OKF Workspace

Milestone **v1.4 — Editor Enhancements & Authoring Polish**. Seven phases (21–27), risk-first and
dependency-ordered per the research synthesis (`.planning/research/SUMMARY.md`) and the roadmap
red-team pre-mortem (which split the original Phase 21 and hardened several criteria).

> Phases 19–20 belong to the **deferred v1.3 Tag Management UX** milestone (roadmapped, not built;
> archived to `.planning/milestones/v1.3-ROADMAP.md`). v1.4 continues numbering at 21.
>
> **Tag-model label note:** this roadmap/REQUIREMENTS/SUMMARY call the chosen model "Model B =
> frontmatter-canonical, sync-at-save". `PITFALLS.md` uses swapped A/B labels — the
> *frontmatter-canonical* model is the chosen one regardless of label.

**Coverage:** every v1.4 requirement (ED-01…ED-06) maps to exactly one phase. 7 phases · all covered.

**Shared surfaces (merge-coordination note — not separate deps):** `PageEditor.tsx` is touched by
21/22 (tags) and 24 (description); `markdown.ts` by 21 (Hashtag) and 25 (codeLanguages);
`livePreview.ts`+`widgets.ts` by 21/22, 25, 26. Logically additive (different functions), but
sequence 21 first and coordinate merges across these files.

## Phases

| #  | Phase | Goal | Requirements | Depends on |
|----|-------|------|--------------|------------|
| 21 | Inline `#tag` Parser + Badge Render | Body `#tag` renders as a badge on both CM6 surfaces; byte-stable; false-positive corpus | ED-02a | — |
| 22 | Model-B Materialization + Tag UI | On save, reconcile body `#tags` ↔ frontmatter (add+remove); replace chip field with badge bar | ED-02b, ED-02c | 21 |
| 23 | LLM Tag Suggestions | Agent suggests tags into frontmatter, de-duped vs the union | ED-03a | 22 |
| 24 | LLM-Generated Description | Drop the Description field; agent generates on demand, review→approve, autosave-safe | ED-01a, ED-01b | — |
| 25 | Code Syntax Highlighting | Language-aware highlighting on both surfaces, code-split + bundle-budgeted | ED-04a | — |
| 26 | Mermaid Diagrams | ` ```mermaid ` renders sandboxed + lazy; build-asserted out of main bundle | ED-05a | 25 |
| 27 | Page Templates | New pages seed from `Templates/` files with `{{…}}` vars + robust frontmatter merge | ED-06a | — |

---

## Phase 21: Inline `#tag` Parser + Badge Render

**Goal:** A `@lezer/markdown` `Hashtag` inline parser + a badge decoration render `#tag` on the
live + read surfaces, decoration-only (byte-stable). This is the *pure, demoable* foundation
Phase 22 builds on — no frontmatter writes here. The known correctness landmine (which `#` is a
tag) is pinned by a fixture corpus in this phase.

**Requirements:** ED-02a.

**Success criteria (observable):**
1. Typing `#deploy` shows a "deploy" badge in live preview and read mode; the saved `.md` still contains the literal `#deploy` (byte-stable round-trip holds — `TestGoldenRoundTrip` + corpus).
2. `#` inside fenced code, inline code, an ATX heading, a URL fragment (`/#frag`), `#1984`, `\#esc`, and mid-word `foo#bar` does NOT badge; `#abc` and `#a/b` (nested) DO badge. All fixture-pinned in `livePreview.test.ts`.
3. Source mode and the active editing line show the raw `#tag`; the badge DOM is `createElement`+`textContent` only.

**Depends on:** — (builds on the existing CM6 surface).

---

## Phase 22: Model-B Materialization + Tag UI

**Goal:** On save, reconcile body `#tags` into the canonical frontmatter `tags:` **client-side**
(reuse the Lezer parse; persist through the normal save — byte-stable on `Emit`; no second Go
parser). Reconciliation is a diff, not a monotonic union, so removals work. Replace the chip Tags
field with a minimal badge bar whose ✕ removes only non-inline (LLM/imported) tags. Carries the
milestone's load-bearing decision; unblocks Phase 23.

**Requirements:** ED-02b (materialize/reconcile on save), ED-02c (badge bar + removal).

**Success criteria (observable):**
1. Save computes `removedInline = inline(lastSavedBody) − inline(currentBody)`, `tags = (priorFrontmatter − removedInline) ∪ inline(currentBody)` — verified by a golden test doing **add + remove + LLM-tag-persists** in one save.
2. After saving a body with `#deploy`, the file's frontmatter `tags:` contains `deploy`; the page appears in tag search / graph shared-tag for `deploy` with **no indexer change**.
3. Deleting `#deploy` from the body and saving removes it from frontmatter; an LLM-applied tag never written inline persists and shows an ✕ in the badge bar; an inline-origin tag has no ✕.
4. The old chip Tags field is gone; the editor has no regression; round-trip stays byte-stable.

**Depends on:** Phase 21 (the parser + badge surface).

---

## Phase 23: LLM Tag Suggestions

**Goal:** Extend the shipped `SuggestTags` flow so agent suggestions feed the frontmatter-canonical
tag set — review→approve, landing in frontmatter, de-duped vs the inline+frontmatter union. Never
injected into prose.

**Requirements:** ED-03a.

**Success criteria (observable):**
1. "Suggest tags" returns vocabulary-biased, capped suggestions for review; nothing writes until approved.
2. Approving adds the tag to frontmatter `tags:` (byte-stable); it renders in the badge bar.
3. A suggestion equal (case-insensitively) to an existing inline `#tag` or frontmatter tag is not offered/added.
4. A suggestion failing tag-charset validation is rejected server-side.

**Depends on:** Phase 22 (tag model + badge bar).

---

## Phase 24: LLM-Generated Description

**Goal:** Remove the Description field; an editor generates/refreshes the description on demand,
reviews it, and approves it into byte-stable frontmatter — never auto-runs, and the apply is
**autosave-safe** (no 409 thrash, no revert).

**Requirements:** ED-01a (remove field), ED-01b (generate on demand).

**Success criteria (observable):**
1. The editor has no Description input; existing descriptions are untouched; every page still has a `description` frontmatter key (repair unchanged).
2. "Generate description" returns a ~1-sentence draft for review; editable + regenerable before apply.
3. Approve writes `description` via byte-stable `okf.SetField` through the revision-checked save.
4. **Autosave-safe:** approve description → keep typing the body → the next autosave neither 409s into the conflict dialog nor reverts the description (apply reseeds `description` + advances `baseRevision` atomically).
5. No description is ever written without explicit approval.

**Depends on:** — (independent; the other frontmatter-field deletion + suggest→apply mode).

---

## Phase 25: Code Syntax Highlighting

**Goal:** Language-aware highlighting on both CM6 surfaces via one shared, theme-aware
`HighlightStyle` and the native `highlightCode` path — languages stay code-split, and the main
bundle stays within budget.

**Requirements:** ED-04a.

**Success criteria (observable):**
1. A ` ```go ` block shows highlighted Go tokens in the live editor and read mode; colors follow the dark theme tokens; spans are `textContent`-only.
2. Languages are code-split; the read widget renders plain then **re-highlights when the language parser resolves** (no static-import of grammars into the main chunk).
3. The curated set (js/ts, python, go, rust, json, yaml, sql, html, css, bash, markdown, diff) highlights; an unknown language falls back to plain monospace with no error; a ` ```mermaid ` fence is NOT highlighted.
4. **Bundle budget:** the main editor chunk grows only by the highlight core + style (not the per-language grammars) — checked against the build output.

**Depends on:** — (settles the widget-highlight + lazy-language pattern Phase 26 reuses).

---

## Phase 26: Mermaid Diagrams

**Goal:** ` ```mermaid ` blocks render as diagrams via a lazy-loaded, sandboxed `MermaidWidget`,
with a safe raw-code fallback. Mermaid is build-asserted out of the main bundle.

**Requirements:** ED-05a.

**Success criteria (observable):**
1. A valid ` ```mermaid ` flowchart/sequence/class/state/ER renders as an inline SVG on read + live, themed to dark mode.
2. `mermaid` loads only when a mermaid block is first displayed; an **automated build assertion** confirms `mermaid` is absent from the main editor chunk.
3. An invalid diagram falls back to the raw fenced code block (no blank, no crash).
4. Rendering uses `securityLevel:'strict'` and injects SVG without `innerHTML` of page content.

**Depends on:** Phase 25 (shares `widgets.ts`/`livePreview.ts`; heaviest/riskiest).

---

## Phase 27: Page Templates

**Goal:** New pages seed from Obsidian-style templates stored as `.md` files in a `Templates/`
folder, with `{{title}}`/`{{date}}`/`{{time}}` substitution — files-as-truth, no templating
engine, no executable templates, and robust frontmatter merge.

**Requirements:** ED-06a.

**Success criteria (observable):**
1. Templates placed under `Templates/` appear in a picker on new-page (with a blank default).
2. Choosing a template seeds the new page's frontmatter + body; `{{title}}`/`{{date}}`/`{{time}}` resolve.
3. **Robust merge:** a template carrying its own `---` frontmatter yields a single valid frontmatter block (no duplicate keys / malformed YAML), then required-field repaired — adversarial test included.
4. Template reads resolve through the safe path resolver; no `{{…}}` placeholder executes code.

**Depends on:** — (fully independent; could parallelize).
