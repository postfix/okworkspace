---
author: qa
responsible: qa
phase: "25"
---

# Phase 25: Code Syntax Highlighting (ED-04a) — VERIFICATION

**Plan file:** `.planning/phases/25-code-syntax-highlighting/25-01-PLAN.md`
**Context file:** `.planning/phases/25-code-syntax-highlighting/25-CONTEXT.md`
**SUMMARY file:** `.planning/phases/25-code-syntax-highlighting/25-01-PLAN-SUMMARY.md` (claims checked against code reality, not trusted)
**Requirements:** ED-04a
**Commits verified (on `main`):** `98eb4ca` `93ad433` `2734fe6` `9bbb88c` `ff6b226`
**Substrate note:** SMTC MCP disconnected for this run — evidence gathered via Bash/Read/grep + live execution (tsc/vitest/build + a live plugin teeth-harness). Adequate for a frontend/TS phase.

## Verdict

```yaml
gate_result:
  record_author: qa
  id: GR-25-verification
  phase: "25"
  verdict: pass
  basis: >
    All nine must-have truths hold against the real code (read, not trusted from the SUMMARY)
    and full re-run evidence (tsc 0, vitest 685/685, build exit 0 with the assertion active,
    plus a live plugin teeth-harness). ONE exported codeHighlightStyle object
    (highlight.ts:34) drives BOTH surfaces — live via syntaxHighlighting(codeHighlightStyle)
    (mode.ts:19,38) and the read CodeBlockWidget via highlightCode(..., codeHighlightStyle, ...)
    (widgets.ts:27,174-177); it is the same import, not two. Colors are var(--syntax-*) only
    (tokens.css:103-108 → LivePreviewEditor.css:160-175), zero inline hex in the highlight path.
    Highlighter spans are createElement + textContent / createTextNode only — no innerHTML
    anywhere in the highlight path (widgets.ts:178-194; every innerHTML occurrence in the file
    is a comment). Grammars code-split via LanguageDescription.of({ load: () => import(...) })
    (highlight.ts:87-156, no static grammar import — pinned by highlight.test.ts:94-105 ?raw
    assertion); the read widget renders plain first then re-highlights, teardown-safe via a
    destroyed flag + codeEl.isConnected guard + atomic detached-fragment swap (widgets.ts:120,
    152-210). Unknown lang and ```mermaid stay plain via matchLanguageName fuzzy=false
    (widgets.ts:157-158). The vite assertLazyChunks plugin is wired (vite.config.ts:35-38),
    unit-tested (10 tests), and PROVEN to have teeth live: lazy grammars + legacy-modes throw
    in an entry chunk, the 4 core grammars correctly pass, oversize throws, and prefix-evasion
    (lang-cssinjs) stays forbidden via anchoring. The four-grammar exclusion is JUSTIFIED, not
    scope-cheating (see bundle-budget ruling). GFM-only parser contract preserved (markdown.ts
    keeps base: commonmarkLanguage + tagParserExtensions; codeLanguages additive). Frontend-only
    — Go/internal/cmd diff is empty over the range.
  remediation_owner: null
  evidence:
    - "tsc -b → exit 0"
    - "vitest run → 685 passed / 59 files, 0 fail"
    - "npm run build → exit 0 with assertLazyChunks active; entry 1,180,616 B < 1,420,000 budget"
    - "live plugin teeth-harness: forbidden→throw, core→pass, oversize→throw, prefix-evasion→throw"
    - "git diff --stat 98eb4ca^..ff6b226 -- '*.go' internal/ cmd/ → empty"
```

## Per-truth verdicts

| # | Must-have truth | Verdict | Key evidence (file:line) |
|---|---|---|---|
| 1 | ONE shared HighlightStyle drives BOTH surfaces | **PASS** | `highlight.ts:34` exports `codeHighlightStyle`; consumed live in `mode.ts:19,38` and read in `widgets.ts:27,174-177` — same object. `markdown.test.ts:59-65` (live `highlightingFor`→tok-*) + `widgets.test.ts:202-225` (read tok-* spans) |
| 2 | ```go highlights both surfaces; var(--syntax-*) tokens-only; curated set wired | **PASS** | 5 `--syntax-*` vars `tokens.css:103-108`; `.tok-*{color:var(--syntax-*)}` `LivePreviewEditor.css:160-175`; grep: zero inline hex in highlight path. Curated 12 langs `highlight.ts:86-156`. go in live `markdown.test.ts:50-53`, read `widgets.test.ts:202-225` |
| 3 | Spans createElement + textContent ONLY, no innerHTML | **PASS** | `widgets.ts:178-194` (span via createElement + textContent; text nodes via createTextNode). grep: all `innerHTML` mentions are comments. `widgets.test.ts:211-220` asserts nodeType/tagName/children; markup-as-text `:231-248` |
| 4 | Code-split via load:()=>import; plain-first then re-highlight; teardown-safe | **PASS** | dynamic imports `highlight.ts:87-156`; no-static-import `highlight.test.ts:94-105`. Plain-first sync `widgets.ts:133-145`; destroyed flag `:120,208-210` + isConnected `:167` + atomic fragment swap `:173-197`. Tests `widgets.test.ts:191-200,278-304` |
| 5 | Unknown → plain no throw; mermaid not highlighted | **PASS** | `matchLanguageName(..., false)` `widgets.ts:157`; no-match early return `:158`; whole pass in try/catch `:163-203`. Tests `widgets.test.ts:250-263` (unknown), `:265-276` (mermaid) |
| 6 | Automated bundle assertion (teeth) + size cap; reusable | **PASS** | Plugin `assertLazyChunks.ts`; wired `vite.config.ts:35-38`; 10 unit tests + live teeth-harness (see ruling). Build exit 0, entry 1,180,616 B < 1,420,000. See bundle-budget ruling below |
| 7 | Zero-layout-shift (color-only) + subtle fade-in | **PASS** | `LivePreviewEditor.css:160-175` color is the only per-role property; fade `:181-194` animates `opacity` only, gated by `prefers-reduced-motion`. Read surface carries `.livepreview-editor` (`LivePreviewEditor.tsx:260`) → fade on both surfaces |
| 8 | GFM-only parser contract preserved; codeLanguages additive | **PASS** | `markdown.ts:33-37` keeps `base: commonmarkLanguage` + `extensions: tagParserExtensions`; `codeLanguages` additive. `markdown.test.ts:30-47` (4 GFM + Hashtag parse; no Emoji/Superscript superset) |
| 9 | Frontend-only; --syntax-* dark + structured for light swap | **PASS** | `git diff --stat 98eb4ca^..ff6b226 -- '*.go' internal/ cmd/` empty. `tokens.css:97-108` dark values, named/grouped for pure value swap |

## Build / test evidence (re-run by qa)

```
# from web/
$ npx tsc -b            → exit 0 (clean)
$ npx vitest run        → 59 files passed, 685 tests passed, 0 fail
                          (incl. highlight 12, markdown 6, widgets +8, assertLazyChunks 10,
                           widgets.adversarial 45 — both fix regression-guards green)
$ npm run build         → exit 0 WITH assertLazyChunks active
                          entry index-pG1X61Dq.js = 1,180.61 kB (1,180,616 B) < 1,420,000 budget
                          lazy chunks emitted: diff (0.30 kB), shell (2.44 kB), dist-*.js (go/python/
                          rust/json/yaml/sql); 4 INEFFECTIVE_DYNAMIC_IMPORT warnings = the expected
                          markdown→html→css/js core chain (warnings, not errors)
$ grep lazy-grammar markers in built entry chunk → 0 hits (lang-go/python/rust/sql/yaml absent)
$ git diff --stat 98eb4ca^..ff6b226 -- '*.go' internal/ cmd/ → empty (frontend-only)
```

### Assertion-has-teeth proof (live harness against synthetic bundles, real production forbidden-regex)

Invoked the shipped `assertLazyChunks` with the exact `vite.config.ts` `FORBIDDEN_IN_ENTRY` list:

| Scenario | Expected | Result |
|---|---|---|
| lazy `lang-go` in an entry chunk | throw | **THREW** ✓ |
| `legacy-modes/mode/shell` in an entry chunk | throw | **THREW** ✓ |
| the 4 core grammars (markdown/html/css/javascript) in entry | pass | **PASS** ✓ |
| `lang-go` in a non-entry (lazy) chunk | pass | **PASS** ✓ |
| oversize entry (> maxEntryBytes) | throw | **THREW** ✓ |
| prefix-evasion `lang-cssinjs` in entry | throw (anchoring) | **THREW** ✓ |

Both arms (forbidden-regex + byte-budget) confirmed live, not merely asserted by the SUMMARY.

## Bundle-budget ruling (truth #6 — the critical judgment call)

**RULING: the four-grammar exclusion is JUSTIFIED. ED-04a's no-bloat intent is satisfied in SPIRIT. No gap.**

The engineer excluded `lang-markdown|html|css|javascript` from the forbidden list while forbidding `python/go/rust/json/yaml/sql` + `legacy-modes`. I verified this independently against ED-04a's "main chunk grows by no more than core+style, NOT per-language grammars":

- **(a) The four are genuinely pre-existing / unavoidable transitive deps of the base editor.**
  `git show 98eb4ca^:web/package.json` lists `@codemirror/lang-markdown ^6.5.0` as a **pre-phase** direct dependency. Its `node_modules/.../package.json` hard-depends on `@codemirror/lang-html ^6.0.0`, which in turn hard-depends on **both** `@codemirror/lang-css ^6.0.0` and `@codemirror/lang-javascript ^6.0.0`. `markdown.ts` statically imports lang-markdown (the editor's base language), so html→css→js are unavoidably in the entry chunk and were there **before** this phase. The build's 4 `INEFFECTIVE_DYNAMIC_IMPORT` warnings name exactly this chain — expected, harmless.
- **(b) Every NON-pre-existing per-fence grammar is truly absent from the entry chunk.**
  python/go/rust/json/yaml/sql + legacy-modes shell/diff are all forbidden by the regex AND proven absent: the build passes with the **teeth-proven** assertion active (it would `this.error` if any landed in entry), and a direct grep of the built entry chunk finds 0 lazy-grammar markers. They emit their own `dist-*.js` / `shell-*.js` / `diff-*.js` chunks.
- **(c) Zero phase-induced grammar bloat.** Entry measured 1,180,616 B; the SUMMARY's pre-gate figure was 1,180,566 B — the ~50-byte delta is core+style wiring, not a grammar. js/ts highlighting riding the pre-existing `lang-javascript` is sensible no-bloat reuse, NOT an ED-04a violation: the phase adds the highlight core (`@lezer/highlight`) + style + the lazy `LanguageDescription` entries, and **no** new grammar to the entry chunk.

The exclusion narrows a pre-existing core fact out of the forbidden set without weakening the guard on any code-fence grammar (anchoring keeps `lang-cssinjs`-style evasions forbidden, proven live). This is correct engineering, not scope-cheating.

## Gaps routed UP

**None.** All nine truths PASS; the bundle-budget judgment clears. No code change required.

## Minor observations (non-blocking, FYI for the architect — not gaps)

- The entry chunk is ~1.18 MB (Vite's 500 kB warning fires). This is **pre-existing** (the CM6 editor + app bundle), not phase-induced; the phase added only core+style. No action implied by ED-04a.
- A future legitimate entry-chunk growth requires bumping `maxEntryBytes` in `vite.config.ts:37` — the comment already instructs this. Working as designed.

---

*Verified 2026-06-30 by qa. Goal-backward against commit range + code reality; SMTC unavailable, evidence via Bash/Read + live execution.*
