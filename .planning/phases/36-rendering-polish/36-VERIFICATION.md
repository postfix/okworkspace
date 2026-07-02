---
author: qa
responsible: qa
phase: 36
phase_type: implementation
verdict: pass
---

# Phase 36: Rendering Polish — Verification

**Goal:** Three independent read-surface wins — `#tag` badge in MarkdownProse,
copy-code button on CodeBlockWidget, description in search cards. Each verified
independently (red-team MIN-05: a subagent could ship 2 of 3).

**Requirements:** EP-01, EP-02, EP-04

**Commit:** 6d27b38

## Acceptance handshake

| Check | Result |
|---|---|
| Inputs complete (PLAN + SUMMARY present; EP-01/EP-02/EP-04 real) | PASS |
| PLAN ↔ SUMMARY consistency (SUMMARY claims match code reality — see per-feature below) | PASS |
| Goal verifiability (each feature is concrete + falsifiable) | PASS |
| Lenses engageable (XSS test, widget test, search test all runnable) | PASS |

No refusal. Inputs are consistent and verifiable.

---

## Feature 1 — EP-01: `#tag` badge in MarkdownProse

**Requirement (REQUIREMENTS.md:193-206):** Inline `#tag` renders as a badge on
the two `MarkdownProse` surfaces (HistoryPanel version preview + AgentAnswer),
matching the CM6 badge, without weakening the stored-XSS guard. Badge is a safe
element node (rehype-raw OFF, rehype-sanitize ON); false-`#` cases stay literal.

### Code reality

**Remark plugin exists and is wired:**
- `web/src/lib/remark-hashtag.ts` — a remark plugin walking mdast `text` nodes,
  replacing recognized `#tag` patterns with a typed hast `<span class="cm-hashtag">`
  element via `data.hName`/`data.hProperties`/`data.hChildren`. The tag text flows
  through as a hast `text` node — never parsed as markup.
- `web/src/components/MarkdownProse.tsx:9` imports `remarkHashtag`; `:90` wires it:
  `remarkPlugins={[remarkGfm, remarkHashtag]}`.

**XSS guard preserved:**
- `rehype-raw` is NOT imported and NOT in the plugins list (grep for `rehype-raw`
  in MarkdownProse.tsx returns no source match — only comments in other files
  reference it). `rehype-sanitize` IS present (`:6` import, `:91` wired with
  `headingIdSchema`).
- The badge is a typed hast element (span), NOT raw HTML. The `headingIdSchema`
  (`:27-34`) extends the default sanitize schema to allow `className: "cm-hashtag"`
  on `span` — value-locked to the literal `"cm-hashtag"` (a `["className",
  "cm-hashtag"]` tuple), not a free-form allow. `span` is already in the default
  tagNames; raw HTML stays disallowed.

**Both surfaces use MarkdownProse:**
- `HistoryPanel.tsx:11` imports MarkdownProse; `:126` renders
  `<MarkdownProse body={viewedPage.body} currentPath={path} />`.
- `AgentAnswer.tsx:4` imports MarkdownProse; `:46` renders
  `<MarkdownProse body={answer} currentPath={currentPath} />`.

**False-`#` cases:** `remark-hashtag.ts` excludes inline code (text nodes only
walked — `code`/`inlineCode` never visited), escapes (vfile source-position check
for `\` before `#`), ATX headings (block markers, not inline text), and URL
fragments (link URLs are in `link` node `url` props, not `text` children). All-digit
runs excluded (≥1 non-digit required). In-word excluded (start-boundary rule).

### Tests run

| Command | Result |
|---|---|
| `cd web && npx vitest run src/components/AgentAnswer.test.tsx` | **4/4 PASS** — the stored-XSS test (`:34-44`) confirms `<img src=x onerror="alert(1)">` does NOT render an `<img>` (rehype-raw OFF). |
| `cd web && npx vitest run src/components/MarkdownProse.test.tsx` | **10/10 PASS** — badge rendering, multiple tags, inline-code exclusion, all-digit exclusion, in-word exclusion, ATX-heading exclusion, escape exclusion, URL-fragment exclusion, line-start badge, and the stored-XSS guard (raw HTML does not render, badge still renders). |

### Criterion check

| Criterion (EP-01) | Evidence | Verdict |
|---|---|---|
| Remark inline transform renders body `#tag` as badge `<span>` in MarkdownProse (HistoryPanel + AgentAnswer), matching CM6 badge + recognized set | `remark-hashtag.ts` plugin wired at `MarkdownProse.tsx:90`; both surfaces import + render MarkdownProse; 10 MarkdownProse tests pass | PASS |
| Badge is a safe element node, NOT raw HTML; rehype-raw OFF, rehype-sanitize ON; AgentAnswer stored-XSS test passes | rehype-raw absent from MarkdownProse.tsx; rehype-sanitize at `:91`; badge is typed hast element via `data.hName`; `className` value-locked to `"cm-hashtag"`; AgentAnswer XSS test 4/4 pass | PASS |
| False-`#` cases (fenced/inline code, escaped, heading, URL fragment) stay literal | MarkdownProse.test.tsx: inline-code (0 badges, code has literal `#tag`), all-digit (0), in-word (0), ATX heading (0 + h1 renders), escape `\#` (0), URL fragment (0) | PASS |

**Feature 1 verdict: PASS.**

---

## Feature 2 — EP-02: Copy-code button on CodeBlockWidget

**Requirement (REQUIREMENTS.md:208-218):** Read-mode fenced code blocks
(CodeBlockWidget) have a copy-to-clipboard button copying the exact code text.
Button DOM built with `createElement` (no innerHTML); does not perturb the
async highlight swap or `destroyed`/`isConnected` teardown. Clipboard write uses
`navigator.clipboard` with graceful fallback.

### Code reality

**Copy button exists in CodeBlockWidget.toDOM:**
- `web/src/lib/cm/widgets.ts:149-170` — a `<button class="cm-copy-btn">` is
  created via `document.createElement("button")`, `btn.className = "cm-copy-btn"`,
  `btn.type = "button"`, `btn.textContent = "Copy"`, `btn.setAttribute("aria-label",
  "Copy code to clipboard")`. **No `innerHTML`** — `textContent` only.
- The button is a SIBLING of `<code>` inside `<pre>` (`:170` `pre.appendChild(btn)`).
  The async `highlight()` (`:181-233`) operates on `codeEl`'s children and
  `pre.classList` — never touches the button. The `destroyed`/`codeEl.isConnected`
  teardown guards (`:196`) guard `codeEl`, not the button.

**Clipboard write with fallback:**
- `:156-168` — uses `navigator.clipboard?.writeText?.bind(navigator.clipboard)`.
  If `write` is unavailable → "Failed" state + reset to "Copy" after 1.5s. On
  success → "Copied" + reset to "Copy" after 1.5s. On rejection → "Failed" + reset.
  No exception escapes toDOM (the `.catch` swallows).

**No innerHTML in the file:** All `innerHTML` occurrences in `widgets.ts` are in
comments documenting the no-innerHTML invariant — none in executable code.

### Tests run

| Command | Result |
|---|---|
| `cd web && npx vitest run src/lib/cm/widgets.test.ts` | **32/32 PASS** — including the EP-02 copy-button test (`:218-243`): button exists as `button.cm-copy-btn`, `textContent === "Copy"`, is a sibling of `<code>` (not inside it), clicking calls `clipboard.writeText` with `this.code`, shows "Copied" then resets to "Copy". |

### Criterion check

| Criterion (EP-02) | Evidence | Verdict |
|---|---|---|
| Copy button on read-mode CodeBlockWidget; copies `this.code`; brief confirmation | `widgets.ts:149-170` — button built createElement-only, click calls `navigator.clipboard.writeText(this.code)`, "Copied"/"Copy" cycle; test `:218-243` verifies | PASS |
| Button DOM built createElement + explicit attrs (no innerHTML); does not perturb async highlight swap or destroyed/isConnected teardown | Button is sibling of `<code>`; `highlight()` operates on `codeEl` children only; teardown guards guard `codeEl`; no `innerHTML` in code; 32 widget tests pass (incl. async re-highlight + teardown-safe) | PASS |
| Clipboard write uses `navigator.clipboard` with graceful no-op/fallback | `:156-168` — optional-chain `navigator.clipboard?.writeText?.`; absent → "Failed" + reset; `.catch` swallows rejection | PASS |

**Feature 2 verdict: PASS.**

---

## Feature 3 — EP-04: Description in search result cards

**Requirement (REQUIREMENTS.md:233-244):** Search backend indexes + returns the
frontmatter description as an additive optional field on `search.Result`;
`SearchResultRow.tsx` renders it for page results; absent description falls back
to today's card; no relevance regression.

### Code reality

**Backend — indexed + returned:**
- `internal/search/doc.go:22` — `pageDoc` signature includes `description string`;
  `:29` stores `"description": description` in the indexable map.
- `internal/search/mapping.go:61-67` — a keyword field mapping for `description`
  (`descFM := bleve.NewKeywordFieldMapping()`, `descFM.Store = true`). Keyword
  mapping = whole-token stored + retrievable, NOT analyzed — no relevance change.
- `internal/search/rebuild.go:91` — `description := okf.Field(doc, okf.FieldDescription)`,
  passed to `pageDoc` at `:92-94`.
- `internal/search/indexjob.go:138` — `description := okf.Field(doc, okf.FieldDescription)`,
  passed to `pageDoc` at `:139`.
- `internal/search/query.go:21` — `Result` struct has
  `Description string \`json:"description,omitempty"\`` (additive, omitempty).
- `internal/search/query.go:72` — `req.Fields` includes `"description"` so Bleve
  returns it in hit documents.
- `internal/search/query.go:146` — `mapHit` populates
  `Description: fieldString(hit.Fields, "description")`.
- `okf.FieldDescription = "description"` (`okf.go:26`) — the frontmatter field
  constant exists.

**Handler — pass-through (no change needed):**
- `internal/server/handlers_search.go:50-56` — `handleSearch` calls
  `h.search.Query(...)` and `writeJSON(w, http.StatusOK, results)`. The
  `json:"description,omitempty"` struct tag automatically serializes the new field.
  No handler change required — purely additive.

**Frontend — type + render:**
- `web/src/api/client.ts:1214` — `SearchResult` interface has `description?: string`.
- `web/src/components/search/SearchResultRow.tsx:77-79` — renders description for
  page results only (`result.description && result.kind === "page"`) as
  `<span className="search-result-description">{result.description}</span>`,
  between the title row and the snippet. Absent description → the conditional
  short-circuits, falling back to today's card.

### Tests run

| Command | Result |
|---|---|
| `go build ./...` | **exit 0** |
| `go test ./internal/search/ -count=1` | **PASS** (1 package ok) — including `TestQuery_Description` (seeds a page with frontmatter description, rebuilds, queries → verifies Description surfaces) and `TestQuery_DescriptionAbsent` (no description → empty Description field, no regression). |

### Criterion check

| Criterion (EP-04) | Evidence | Verdict |
|---|---|---|
| Backend indexes + returns description as additive optional field on search.Result; existing snippet/anchor/page_title unchanged | `doc.go:22,29` (pageDoc), `mapping.go:61-67` (keyword, not analyzed), `query.go:21` (Description field, omitempty), `query.go:72` (Fields includes description), `query.go:146` (mapHit populates); existing fields untouched | PASS |
| SearchResultRow renders description for page results (distinct from snippet); absent description falls back to today's card | `SearchResultRow.tsx:77-79` — `result.description && result.kind === "page"` guard; rendered between title row and snippet; absent → conditional skips | PASS |
| Search response stays lean (single additive field); no relevance regression | `mapping.go:61-67` — keyword mapping (not analyzed), display-only; `TestQuery_DescriptionAbsent` confirms absent description yields empty Description (no shape change); all search tests pass | PASS |

**Feature 3 verdict: PASS.**

---

## Build + type checks

| Command | Result |
|---|---|
| `go build ./...` | **exit 0** |
| `cd web && npx tsc --noEmit` | **exit 0** |
| `cd web && npx vitest run src/components/AgentAnswer.test.tsx` | **4/4 PASS** |
| `cd web && npx vitest run src/lib/cm/widgets.test.ts` | **32/32 PASS** |
| `cd web && npx vitest run src/components/MarkdownProse.test.tsx` | **10/10 PASS** |
| `go test ./internal/search/ -count=1` | **PASS** |

---

## Security (EP-01)

The badge is a typed hast element (via `data.hName`/`data.hProperties`), not raw
HTML. `rehype-raw` is NOT enabled; `rehype-sanitize` IS ON. The tag text flows
through as a hast `text` node (via `data.hChildren`) — never parsed as markup. The
`className` allow-list entry is value-locked to the literal `"cm-hashtag"` (a
`["className", "cm-hashtag"]` tuple). The AgentAnswer stored-XSS test confirms
raw HTML in content does not render. The MarkdownProse stored-XSS test confirms
the same with a badge present. **The stored-XSS guard holds.**

---

## Red-team MIN-05: independent verification of each feature

All three features verified independently against code reality:

1. **EP-01** — remark hashtag plugin + XSS guard: **PASS** (plugin wired, rehype-raw OFF, rehype-sanitize ON, badge is typed element, XSS tests pass, false-`#` cases literal).
2. **EP-02** — copy button on CodeBlockWidget: **PASS** (button exists, createElement-only, no innerHTML, navigator.clipboard + fallback, doesn't perturb async highlight/teardown, 32 tests pass).
3. **EP-04** — description in search cards: **PASS** (indexed as keyword, returned as additive omitempty field, rendered for page results, absent falls back, no relevance regression, search tests pass).

No feature was shipped as 2-of-3. All three are present and verified.

---

## Verdict

**PASS.** Phase 36 delivers all three independent read-surface wins goal-backward
against code reality. Each feature is present, wired, tested, and meets its
acceptance criteria. The stored-XSS guard (EP-01) holds. No gaps found.