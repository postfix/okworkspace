---
author: engineer
responsible: engineer
phase: 36
phase_type: implementation
status: complete
---

# Phase 36: Rendering Polish — Summary

**Goal:** Three independent read-surface wins — `#tag` badge in MarkdownProse,
copy-code button on CodeBlockWidget, and description in search cards.

**Requirements:** EP-01, EP-02, EP-04

## What was built

### Task 1 — EP-01: `#tag` badge in MarkdownProse

- Created `web/src/lib/remark-hashtag.ts`: a remark plugin that walks mdast
  `text` nodes and replaces recognized `#tag` patterns with a typed hast
  `<span class="cm-hashtag">` element (via `data.hName`/`data.hProperties`/
  `data.hChildren`). The badge is a React/hast element — NOT raw HTML — so
  rehype-sanitize keeps it (`span` is in the default schema).
- Recognition mirrors `hashtag.ts`: body `[A-Za-z0-9_/-]`, ≥1 non-digit,
  trailing `/` trimmed, word-start boundary. Code/heading/URL exclusions are
  structural (text nodes only walked; inline code and fenced code are `code`
  nodes, never visited; ATX heading markers are block marks).
- Escape detection: remark consumes `\#` as an escape, merging the `#` into
  the adjacent text node. The plugin uses the text node's position to check
  the original source (vfile) — if the source char before the `#`'s offset is
  `\`, the `#` was escaped and stays literal.
- Wired into `MarkdownProse.tsx` via `remarkPlugins={[remarkGfm, remarkHashtag]}`.
- Extended `headingIdSchema` to allow `className: "cm-hashtag"` on `span` —
  the default rehype-sanitize schema allows `span` in tagNames but not
  `className` on it. The allowed value is restricted to the literal
  `"cm-hashtag"` (a `["className", "cm-hashtag"]` tuple — value-locked, not
  a free-form allow).
- Added `.cm-hashtag` CSS to `MarkdownProse.css` (mirrors the CM6
  `HashtagWidget` visual from `theme.ts`).
- `rehype-raw` stays OFF; `rehype-sanitize` stays ON. The AgentAnswer
  stored-XSS test passes.
- Created `web/src/components/MarkdownProse.test.tsx` (10 tests): badge
  rendering, multiple tags, inline-code exclusion, all-digit exclusion,
  in-word exclusion, ATX heading exclusion, escape exclusion, URL-fragment
  exclusion, line-start badge, and the stored-XSS guard (raw HTML does not
  render, badge still renders).

### Task 2 — EP-02: Copy-code button on CodeBlockWidget

- Added a copy-to-clipboard `<button class="cm-copy-btn">` to
  `CodeBlockWidget.toDOM` in `widgets.ts`. Built with `createElement` +
  `textContent` only (no `innerHTML`). The button is a sibling of `<code>`
  inside `<pre>`; it does not interfere with the async highlight swap (which
  operates on `codeEl`'s children) or the `destroyed`/`isConnected` teardown
  guards (which guard `codeEl`).
- Uses `navigator.clipboard.writeText` with a graceful fallback (no-op +
  "Failed" state). Shows "Copied" for 1.5s on success.
- Added `.cm-copy-btn` CSS to `LivePreviewEditor.css` (subtle, top-right,
  appears on hover).
- Added a test in `widgets.test.ts` verifying the button exists, is a
  sibling of `<code>`, copies `this.code` via clipboard, and shows
  "Copied"/"Copy" confirmation cycle.

### Task 3 — EP-04: Description in search

- `internal/search/doc.go`: added `description` parameter to `pageDoc`.
- `internal/search/rebuild.go` + `internal/search/indexjob.go`: extract
  `okf.Field(doc, okf.FieldDescription)` and pass to `pageDoc`.
- `internal/search/query.go`: added `Description` to `Result` struct
  (`json:"description,omitempty"`); populated in `mapHit` via
  `fieldString(hit.Fields, "description")`; added `"description"` to
  `req.Fields` so Bleve returns it in hit documents.
- `internal/search/mapping.go`: added a keyword field mapping for
  `description` (Store=true, not analyzed — display-only, no relevance
  change).
- `web/src/api/client.ts`: added `description?: string` to `SearchResult`.
- `web/src/components/search/SearchResultRow.tsx`: renders the description
  for page results (between title row and snippet) when present; absent
  description falls back to today's card.
- `web/src/components/search/SearchResultRow.css`: added
  `.search-result-description` style (muted, clamped to 2 lines).
- Added `TestQuery_Description` and `TestQuery_DescriptionAbsent` in
  `query_test.go`: seed a page with a frontmatter description, rebuild,
  query → verify the result carries the description; and a page with no
  description → empty Description field (no regression).

## Verification

- `go build ./...` — exits 0 ✓
- `go test ./internal/search/ -count=1` — exits 0 (all search tests pass) ✓
- `cd web && npx vitest run src/components/AgentAnswer.test.tsx` — 4/4 pass ✓
- `cd web && npx vitest run src/lib/cm/widgets.test.ts` — 32/32 pass ✓
- `cd web && npx vitest run src/components/MarkdownProse.test.tsx` — 10/10 pass ✓
- `cd web && npx tsc --noEmit` — exits 0 ✓

## Security (EP-01)

The badge is a typed hast element (via `data.hName`/`data.hProperties`), not
raw HTML. `rehype-raw` is NOT enabled; `rehype-sanitize` is ON. The tag text
flows through as a hast `text` node (via `data.hChildren`) — never parsed as
markup. The `className` allow-list entry is value-locked to `"cm-hashtag"`.
The AgentAnswer stored-XSS test confirms raw HTML in content does not render.

## Files modified

- `web/src/lib/remark-hashtag.ts` (NEW)
- `web/src/components/MarkdownProse.tsx`
- `web/src/components/MarkdownProse.css`
- `web/src/components/MarkdownProse.test.tsx` (NEW)
- `web/src/lib/cm/widgets.ts`
- `web/src/lib/cm/widgets.test.ts`
- `web/src/components/LivePreviewEditor.css`
- `internal/search/doc.go`
- `internal/search/rebuild.go`
- `internal/search/indexjob.go`
- `internal/search/query.go`
- `internal/search/mapping.go`
- `internal/search/query_test.go`
- `web/src/api/client.ts`
- `web/src/components/search/SearchResultRow.tsx`
- `web/src/components/search/SearchResultRow.css`