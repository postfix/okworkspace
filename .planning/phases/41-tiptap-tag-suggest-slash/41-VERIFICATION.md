---
author: qa
responsible: architect
phase: 41
phase_type: implementation
verdict: PASS
requirements: [RE-03]
security_relevant: true
hard_bar: true
---

# Phase 41 Verification — TipTap #tag + Suggest + Slash + Real Save (RE-03)

**VERDICT: PASS.** The security keystone is REAL (paste = faithful two-layer; popup =
textContent-only), BLK-01 materializes through the ONE Lezer path (no second
materializer), the Tag node is display-only + serializes to literal `#tag`, the
recognizer regex mirrors the Lezer grammar, and TipTap is code-split out of the entry
chunk. Independently re-run against code reality — the engineer's SUMMARY is honest.

## Evidence (re-run on disk)

- `npx vitest run src/components/TipTapEditor.test.tsx` → **7/7 pass**.
- `npx vitest run --environment node src/lib/tiptap/` → **115/115 pass** (2 files).
- `npx vitest run` (full) → **69 files / 937 tests pass** (matches the headline claim;
  stderr `act()` warnings are pre-existing, in untouched files, not failures).
- `npm run build` → **exit 0**; `tsc -b` clean; `assertLazyChunks` GREEN (entry
  991,508 B < 1,420,000 B budget).

## Checks

### 1. MAJ-05 paste — the keystone (RE-03-4) — PASS
- Wiring REAL: `TipTapEditor.tsx:107` `editorProps.transformPastedHTML: sanitizePastedHtml`.
- `sanitizePaste.ts:20-25` strips `<script>/<iframe>/<object>/<embed>` (paired + stray),
  `on\w+=` handler attrs, and `javascript:` URLs.
- Test drives BOTH layers, not a stub (`TipTapEditor.test.tsx`):
  - Layer 2: `sanitizePastedHtml(PAYLOAD)` asserted `not.toMatch` `<script`, `onerror`,
    `onclick`, `javascript:` (payload = `<img onerror>` + `<script>` + `<div onclick>` +
    `<a href="javascript:">`).
  - Layer 1: `getSchema([...buildExtensions(), Markdown, Tag])` (schema-identical to the
    live editor — the suggestion extensions add no schema nodes) parsed via
    `PMDOMParser.fromSchema(schema).parse(holder)` over the sanitized HTML →
    asserts `types` has no `script/iframe/image/div` AND `attrKeys` has no
    `onerror/onclick/onload/onmouseover`. No live node, no executable attr. Faithful.

### 2. MAJ-03 popup (RE-03-3) — PASS
- `suggestion.ts:66` `row.textContent = labelOf(item)` — the only item-render path.
- `grep '\.innerHTML\s*=' ` over suggestion.ts / tagNode.ts / TipTapEditor.tsx → **no
  match**. The `innerHTML` token appears ONLY in guard comments (as expected).
- Test: a markup vocab entry (`<img src=x onerror=alert(1)>`) → popup `textContent`
  contains the literal string, `querySelectorAll("img").length === 0`, markup row
  `children.length === 0`. Stays inert text.

### 3. BLK-01 materialization (RE-03-5) — PASS · no second materializer
- Test imports the REAL `reconcileFrontmatterTags` (`../lib/tag-reconcile`) and REAL
  `inlineTagsFromBody` (`../lib/cm/hashtag`); only `../api/client` is mocked.
- Types `#deploy ` in the live editor → takes `onChange`/`getMarkdown()` body → feeds it
  through the real `reconcileFrontmatterTags({lastSavedBody:"", currentBody, frontmatter:""})`
  → asserts `changed` + frontmatter contains `deploy`; and
  `inlineTagsFromBody(tiptapBody) === inlineTagsFromBody("#deploy")` (CM6 parity).
- `tag-reconcile.ts:51-53` runs `inlineTagsFromBody` (the ONE Lezer recognizer). No
  divergent materializer exists.

### 4. Tag node display-only + regex ↔ grammar (RE-03-1) — PASS
- `tagNode.ts:96` `renderMarkdown:(node)=>` `#${name}` (literal). renderHTML 3rd element
  is a text hole → textContent, never innerHTML.
- `TAG_BODY = "#(?=[A-Za-z0-9_/-]*[A-Za-z_-])[A-Za-z0-9_/-]+"` — shared by INPUT_RE
  (`(?:^|\s)(TAG_BODY)\s$`), PASTE_RE (`(?<=^|\s)TAG_BODY`), TAG_RE (`^TAG_BODY$`);
  mirrors `hashtag.ts` (body `[A-Za-z0-9_/-]`, ≥1 non-digit, `/` separator, ws/BOS
  boundary). codec.test.ts cross-checks TAG_RE vs `inlineTagsFromBody` on
  accept `#a/#a/b/#-1/#_/#fff/#deploy/#a1` and reject `#1984/#12/34/#/#123`, plus
  `foo#bar` boundary → no tag. Registered ONLY in the live surface (never
  buildExtensions) → headless codec is tag-free (TAG_CORPUS proves `#tag` stays literal).

### 5. Isolation (RE-03-2) — PASS
- `PageEditor.tsx:48` `lazy(() => import("../components/TipTapEditor"))`; mounted in
  `<Suspense>` at the `mode === "tiptap"` branch.
- Built entry chunk `index-*.js` (991 KB): grep `useEditor|@tiptap/react|EditorContent`
  → **none**; the sole `tiptap` token is the `l === "tiptap"` mode-branch string.
- `TipTapEditor-*.js` lazy chunk (510 KB) holds ProseMirror×56 + EditorContent×2.
- NOTE (not a defect): `FORBIDDEN_IN_ENTRY` does not yet list a `@tiptap` regex — the
  structural FORBIDDEN assertion for the TipTap tree is deferred to Phase 42 (per
  CONTEXT). Isolation is nonetheless independently proven here via the grep + byte
  budget.

### 6. P3 fold (RE-03-6) — PASS
- `preserved.ts:200` `inlineMathTokenizer` regex `^\$(?!\$)(?![\s])(?:[^$\n]*[^$\n\s])?\$(?!\d)`.
- `PROSE_CURRENCY_CORPUS` (`It costs $5 and $10 today.` / `$5$10` / `$3, $50, $100`):
  parsed JSON has NO `preservedInline`; byte-exact round-trip. Real `inlineMath` /
  `inlineMathTwice` still capture `preservedInline`.

## Known / expected (not defects)
- `"tiptap"` not user-selectable (Phase 42 adds the UI).
- Repo-wide react-hooks 7.x eslint findings in untouched files (not this phase).
- `innerHTML` string in suggestion.ts / test = comments + the faithful clipboard-parse
  reconstruction; no `el.innerHTML=` in production code.

*Verified: 2026-07-05 · qa (independent, code-reality, goal-backward)*
