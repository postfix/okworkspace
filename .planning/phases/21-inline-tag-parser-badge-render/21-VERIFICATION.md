---
author: qa
responsible: qa
phase: "21"
---

# Phase 21: Inline `#tag` Parser + Badge Render — VERIFICATION

**Plan file:** `.planning/phases/21-inline-tag-parser-badge-render/21-01-PLAN.md`
**SUMMARY file:** `.planning/phases/21-inline-tag-parser-badge-render/21-01-PLAN-SUMMARY.md`
**Requirement:** ED-02a
**Commits verified (on `main`):** `788aa89`, `a5bea08`, `a2d13d8`

## Verdict

```yaml
gate_result:
  record_author: qa
  id: GR-21-verification
  phase: "21"
  verdict: pass
  basis: "Body #tag tokens render as cm-hashtag badge widgets on BOTH CM6 surfaces (live editor + read/cleanRender) through ONE buildDecorations Hashtag case; decoration-only ⇒ zero document transactions ⇒ byte-stable (doc.toString() verbatim, literal #deploy kept). The full ED-02a negative corpus (`#code` inline, fenced #x, # Heading, /#frag, #1984, \\#esc, foo#bar) produces no badge and the positive corpus (#abc, #fff hex-like, #deploy, #a/b) produces exactly one — falsifiable fixtures over the real Lezer render path, not vacuous assertions. Source mode (no livePreviewExtension) and the active editing line (isActive guard) show raw #tag; HashtagWidget DOM is createElement+textContent only (no innerHTML, single inert text node asserted); the widget is NOT in atomicRanges (atomicCount==0 over 3 badges). parseInlineTags is genuinely unused in the render path (referenced only in tests + its own definition) and its two render-only divergences are documented + pinned for Phase 22 — a correct handoff, not a Phase-21 defect. src/lib/cm 147/147, full web suite 485/485 (49 files), npm run build clean."
  tiers:
    evidence: smtc_honest
    reasoning: manual_review
  provenance:
    - "Semantic/structural analysis: read hashtag.ts, widgets.ts, livePreview.ts, markdown.ts, theme.ts, mode.ts; import-graph grep of parseInlineTags / HashtagWidget / hashtagExtension across web/src (non-test render-path usage confirmed)"
    - "Test runs (from web/): npx vitest run src/lib/cm/ → 147/147; npx vitest run → 485/485 (49 files); npm run build (tsc -b && vite build) → clean"
    - "Read-mode wiring confirmed: LivePreviewEditor.tsx:138/156 (cleanRender.of(true) in the readOnly branch) ← PageView.tsx:125 (read-only LivePreviewEditor)"
    - "Git: all 3 commits are web/-only (no .go files) ⇒ Go TestGoldenRoundTrip untouched by construction"
```

## Verification method

Goal-backward verification from the ED-02a acceptance criteria to the actual render path, reading the code (not trusting the SUMMARY) and re-running the build/tests as independent evidence. Each criterion is confirmed by examining the real implementation and the falsifiability of its fixtures.

---

## 1. Verification acceptance handshake

| Check | Status |
|-------|--------|
| Inputs complete (CONTEXT, PLAN, SUMMARY, REQUIREMENTS §ED-02a) | PASS |
| PLAN ↔ SUMMARY consistency (claimed symbols exist + wired) | PASS — `hashtagExtension` (markdown.ts:24), `HashtagWidget` (widgets.ts:136 → used livePreview.ts:294), `case "Hashtag"` (livePreview.ts:282), `.cm-hashtag` (theme.ts:112) all present and composed |
| Goal verifiability: "renders as a badge", "byte-stable", "no badge for negative corpus", "no innerHTML", "non-atomic" are concrete, falsifiable, testable claims | PASS |
| Lenses engageable (every criterion checkable by test or code read) | PASS |

No refusal: inputs are consistent and verifiable.

---

## 2. PLAN-SUMMARY consistency check

Every `must_haves.truths` claim from the PLAN checked against the code:

| PLAN truth | Code location | Status |
|---|---|---|
| `@lezer/markdown` inline parser defines `Hashtag` node composed into `markdownExtension` | `hashtag.ts:114-117`; `markdown.ts:24` | CONFIRMED |
| Grammar: start-of-section/whitespace start; body `[A-Za-z0-9_/-]` ≥1 non-digit; nested `#a/b` | `hashtag.ts:48-92` (`isWs`/`isBody`/`isNonDigit`/`scanBody`), `:97-108` (`parseHashtag`) | CONFIRMED |
| Negative corpus excluded (fenced, inline code, ATX marker, `/#frag`, `#1984`, `\#esc`, `foo#bar`) | structural + boundary; pinned `livePreview.test.ts:312-325` | CONFIRMED |
| Positive corpus badges (`#abc`, `#fff`, `#deploy`, `#a/b`) | pinned `livePreview.test.ts:292-307` | CONFIRMED |
| `#tag` renders as badge widget in live preview AND read (cleanRender) mode | `livePreview.ts:282-299` (one case); read-mode wiring §4 | CONFIRMED |
| Active editing line + source mode show raw `#tag` | `isActive` guard `livePreview.ts:290`; `sourceExtensions` excludes plugin `mode.ts:37` | CONFIRMED |
| `HashtagWidget` DOM createElement + textContent only (no innerHTML); class `cm-hashtag` | `widgets.ts:146-153` | CONFIRMED |
| Decoration-only: zero transactions; `doc.toString()` keeps literal `#deploy` | ViewPlugin provides decorations only; pinned `livePreview.test.ts:345-361, 598-619` | CONFIRMED |
| Hashtag widget NOT added to `atomicRanges` | not pushed to `widgetRanges` (livePreview.ts:282-299 vs Image :313); pinned `:567-594` | CONFIRMED |
| `.cm-hashtag` pill styling in theme.ts using tokens; layout-neutral | `theme.ts:112-118` (token vars, no font-size/line-height override) | CONFIRMED |

No PLAN↔SUMMARY divergence found. Every claimed symbol exists in the code and is wired into the render path.

---

## 3. Per-criterion verdict (ED-02a)

### Criterion 1 — `#deploy` badges on live preview AND read mode; saved `.md` keeps literal `#deploy` (byte-stable, decoration-only, zero transactions)

**Truth:** PASS.

**Evidence:**
- **One code path, both surfaces.** `buildDecorations` (`livePreview.ts:282-299`) emits `Decoration.replace({ widget: new HashtagWidget(tag) })` over `[node.from, node.to)` unless `isActive(node.from)`. In **read mode** the `cleanRender` facet keeps `activeLines` empty (`livePreview.ts:190-197`), so the badge always shows; in **live edit mode** the same case runs via `liveExtensions` (mode.ts:28-32). Read-mode wiring confirmed independently: `PageView.tsx:125` mounts a read-only `LivePreviewEditor`, and `LivePreviewEditor.tsx:138/156` provides `cleanRender.of(true)` in the `readOnly` branch. The single Hashtag case therefore serves both surfaces — matching the criterion's "live preview AND read mode".
- **Byte-stable / zero transactions.** The `livePreview` ViewPlugin (`livePreview.ts:383-416`) only provides `decorations` + `atomicRanges`; it never dispatches a `changes` transaction. Pinned by `livePreview.test.ts:345-361` (clean render forces a badge, then asserts `view.state.doc.toString() === doc` and `.toContain("#deploy")`) and the tag-dense churn test `:598-619` (byte-verbatim in BOTH edit and clean mode, `#deploy` and `#h/i/` kept).
- **Go golden round-trip untouched.** All 3 commits are `web/`-only (git `--stat` shows no `.go` files), so `TestGoldenRoundTrip` is untouched by construction — consistent with the decoration-only design.

### Criterion 2 — Negative corpus produces NO badge; positive corpus badges; fixtures are falsifiable

**Truth:** PASS.

**Evidence:**
- **Negative render corpus** (`livePreview.test.ts:312-325`): `` `#code` ``, fenced `#x`, `# Heading`, `see /#frag`, `#1984`, `\#esc`, `foo#bar` — each asserts `badges(doc).length === 0`, where `badges` filters real plugin decorations by `d.widget instanceof HashtagWidget` (`:283-288`). These are **not vacuous**: if the grammar/precedence regressed and produced a `Hashtag` node, the count would be ≥1 and the assertion would fail. I confirmed the exclusion mechanism in code:
  - `foo#bar`, `/#frag`, `\#esc` → start-boundary rejects (`before` is a letter/`/`/`\`, not whitespace; `hashtag.ts:103-104`).
  - `#1984` → `scanBody` returns -1 on `!hasNonDigit` (`hashtag.ts:90`).
  - `` `#code` `` / fenced `#x` / `# Heading` → structural: `before: "Link"` places Hashtag after Escape/InlineCode so code spans are consumed first, and an ATX leading `#` is a block `HeaderMark` (never inline content). The adversarial corpus additionally pins `` `x`#tag `` and `\t#deploy` (indented code) negatives (`:527-535`).
- **Positive render corpus** (`livePreview.test.ts:292-307`): `#deploy`, `#abc`, `#fff` (hex-like, not special-cased), `#a/b` — each asserts exactly 1 badge with the correct `tag` label AND the exact `from`/`to` span. Falsifiable on both presence and span.
- **Pure-grammar mirror** (`hashtag.test.ts`, 41 tests): the same positive/negative corpus plus body/slash, start-boundary, unicode/surrogate edge cases — locks the grammar in isolation. Original-case display vs lower-cased materialization is pinned (`#0xFF` → render label `0xFF`, `parseInlineTags` → `0xff`; `livePreview.test.ts:504-508`).

### Criterion 3 — Source mode + active line show raw `#tag`; badge DOM is createElement+textContent (no innerHTML); widget not atomic

**Truth:** PASS.

**Evidence:**
- **Source mode raw.** `sourceExtensions = [markdownExtension, theme, sourceTheme]` (`mode.ts:37`) — it does **not** include `livePreviewExtension`, so no Hashtag decoration is emitted at all in source mode; the raw `#tag` text shows. Structural, by construction.
- **Active-line reveal.** The Hashtag case emits the widget only `if (!isActive(node.from))` (`livePreview.ts:290`). Pinned by `livePreview.test.ts:327-337` (cursor off the tag line → 1 badge; cursor on the tag line → 0, raw revealed) and `:339-343` (clean mode keeps `activeLines` empty so it always badges even with selection on the line).
- **No innerHTML.** `HashtagWidget.toDOM` (`widgets.ts:146-153`) is `createElement("span")` + `textContent = "#" + this.tag` only — no `innerHTML` anywhere in the file. Pinned by `widgets.test.ts:159-169`: input `a<b>c` yields `textContent === "#a<b>c"`, `children.length === 0`, `childNodes.length === 1`, `childNodes[0].nodeType === 3` (TEXT_NODE) — the structural no-innerHTML guard. Untrusted tag bytes can never become live markup.
- **Non-atomic.** The Hashtag range is NOT pushed into `widgetRanges` (only the `Image` case does, `livePreview.ts:313`); `atomicRanges = Decoration.set(widgetRanges, true)` (`:379`). Pinned by `livePreview.test.ts:567-594`: a doc of 3 badges produces 3 widgets yet `plugin.atomicRanges` iterates to `atomicCount === 0`. Cursor is never trapped (Pitfall 1).

---

## 4. Deferred item — `parseInlineTags` (Phase-22 handoff, NOT a Phase-21 gap)

The lead flagged that the pure `parseInlineTags(text)` helper is intentionally NOT code-region/block-boundary aware and is unused in Phase-21 render. Verified:

- **Genuinely unused in render.** Import-graph grep across `web/src`: `parseInlineTags` is referenced ONLY in test files (`hashtag.test.ts`, `livePreview.test.ts`) and its own definition (`hashtag.ts`). No non-test render-path source imports it. The render path uses the Lezer `Hashtag` node → `HashtagWidget`, never the string scanner. Confirmed: zero Phase-21 runtime effect.
- **Divergence documented.** `hashtag.ts:14-25` (file SCOPE header) and `:119-128` (function SCOPE NOTE) state the two render-only behaviors out of scope: (1) code-region exclusion, (2) block-section start boundary (`cx.offset`) — both reconciled in Phase 22 (ED-02b).
- **Divergence pinned (not hidden).** `hashtag.test.ts:179-198` ("KNOWN scope gap" — extracts from code regions) and `livePreview.test.ts:634-656` ("render⇄scanner divergence" — `>#deploy` renders `["deploy"]` but `parseInlineTags` returns `[]`; with a space `> #deploy` both agree). These are pinned as observed truth so Phase 22 inherits a concrete failing target and cannot regress it silently.

This is a correct Phase-22 handoff, not a Phase-21 defect.

---

## 5. ED-02a acceptance criteria (REQUIREMENTS.md)

| # | Criterion | Status |
|---|---|---|
| 1 | `@lezer/markdown` inline parser defines `Hashtag` node, composed with GFM extensions in `markdown.ts` | PASS (`hashtag.ts:114`; `markdown.ts:24`) |
| 2 | `#tag` badges in live preview + read mode; raw `.md` keeps literal `#tag`; source mode + active line show raw | PASS (§3 C1 + C3) |
| 3 | Tag syntax: alnum + `_`/`-`/`/` (nested `#a/b`), ≥1 non-digit, no spaces, case-insensitive match / original-case display | PASS (`hashtag.ts:48-92`; case pinned `livePreview.test.ts:504-508`) |
| 4 | NOT a tag inside fenced/inline code, ATX heading, `/#frag`, `#1984`, `\#`, `foo#bar`; hex `#abc`/`#fff` ARE tags | PASS (§3 C2) |
| 5 | Badge DOM built with createElement + textContent only (no innerHTML) | PASS (`widgets.ts:146-153`; `widgets.test.ts:159-169`) |
| Security | Decoration/widget only — no document transaction; no HTML injection from tag text | PASS (zero transactions §3 C1; no-innerHTML §3 C3) |

---

## 6. Build/test verification (re-run independently from `web/`)

| Command | Result |
|---|---|
| `npx vitest run src/lib/cm/` | PASS — 147/147 (7 files: hashtag 41, livePreview 54, widgets 14, + sibling suites) |
| `npx vitest run` (full web suite) | PASS — 485/485, 49 files (no consumer regression) |
| `npm run build` (tsc -b && vite build) | PASS — clean. The `>500 kB` chunk warning targets the pre-existing main `index-*.js` (1.45 MB) app bundle; this phase added ~200 LOC of TS and did not introduce it. Non-blocking, generic advisory. |

Numbers match the SUMMARY exactly (147/147, 485/485, build clean).

---

## 7. Confidence note

Verdict rests on HIGH-confidence evidence: I re-ran all three commands myself (focused suite, full suite, build), read every changed file, and confirmed the import graph so the render path / scanner-unused claims are grounded in real structure, not the SUMMARY's prose. The corpus tests exercise the real Lezer grammar through a headless `EditorView` under jsdom (the cwd-from-`web/` gotcha was respected — the suite is green with jsdom), so the negative/positive/reveal/byte-stable/non-atomic assertions are observed behavior. SMTC's LSP semantic tier is Go/Rust/Java; for this TS phase the strongest available grounding is test execution + structural reads, which is why the evidence tier is `smtc_honest` rather than a Decision-grade cert.

---

## Conclusion

**Verdict: PASS.** Phase 21 delivers the ED-02a goal: body `#tag` tokens render as byte-stable `cm-hashtag` badge widgets on both CM6 surfaces through a single decoration path; the negative corpus is excluded and the positive corpus badged by falsifiable fixtures over the real render; source mode and the active line reveal raw `#tag`; the widget DOM is no-innerHTML and non-atomic. The `parseInlineTags` render⇄scanner divergence is genuinely unused in render, documented, and pinned as a correct Phase-22 handoff. Build, focused suite (147/147), and full suite (485/485) all clean.

No gaps found. No refusal needed.
