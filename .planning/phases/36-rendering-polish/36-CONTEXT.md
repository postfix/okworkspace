---
author: architect
responsible: architect
phase: 36
phase_type: implementation
status: Ready for planning
hard_bar: false
security_relevant: true
design_fork: false
parent_artifacts:
  - .planning/ROADMAP.md
  - .planning/REQUIREMENTS.md
---

# Phase 36: Rendering Polish (#tag badge in MarkdownProse · copy-code · description in search) - Context

**Gathered:** 2026-07-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Three independent read-surface wins: (1) `#tag` renders as a badge in the
react-markdown `MarkdownProse` surfaces (history preview + agent answer) without
weakening the XSS guard; (2) read-mode fenced code gets a copy button; (3) page
search cards show the frontmatter description. **Red-team MIN-05: qa MUST verify
each independently — a subagent could ship 2 of 3 and mark the phase done.**

**Requirements:** EP-01, EP-02, EP-04

**Non-goals:** the CM6 editor badge color (Phase 32); insert-at-cursor templates
(Phase 37); changes to search relevance or heading/attachment result shapes.

**Shared-file coordination:** `widgets.ts` + `theme.ts` are also touched by Phase 32
(`HashtagWidget` color). **Sequence 32 before 36** (or assign explicit file-ownership
if parallelized). The no-`innerHTML` DOM invariant is shared.
</domain>

<decisions>
## Implementation Decisions

### EP-01: `#tag` badge in MarkdownProse (D-01) — remark inline transform
- **D-01:** A remark inline transform plugin renders body `#tag` as a safe badge
  `<span>` in `MarkdownProse.tsx` — matching the CM6 `HashtagWidget` visual + the
  recognized `#tag` set (fenced/inline-code/escape/heading exclusions honored). The
  transform runs as a `remarkPlugins` entry on the `ReactMarkdown` component
  (`MarkdownProse.tsx:78`). It walks text nodes, finds `#tag` patterns using the same
  boundary rules as the Lezer grammar (word-start boundary, body alphabet
  `[A-Za-z0-9_/-]`, ≥1 non-digit, no space), and replaces them with a React element
  `<span className="cm-hashtag">#tag</span>`. Code spans (`code`), escaped `\#`, and
  heading `#` are excluded.
- **D-01a (XSS guard):** `rehype-raw` stays OFF, `rehype-sanitize` stays ON
  (`MarkdownProse.tsx:79`). The badge is a React element (not raw HTML), so it never
  touches the rehype-sanitize pipeline as raw HTML. The `AgentAnswer` stored-XSS test
  (`AgentAnswer.test.tsx:42-44`) must still pass. The transform runs BEFORE rehype, so
  the badge `<span>` is a hast element, not a string — sanitize keeps it (span is in
  the default schema).
- **D-01b (recognized-`#tag` set):** the remark transform must honor the same
  exclusions as the CM6 grammar: fenced code (```` ``` ````), inline code (`` ` ``),
  `\#` escapes, ATX heading `#`, URL fragments. A `#` in any of these stays literal.
  Reuse the recognition logic from `hashtag.ts` (the `isWs`/`isBody`/`isNonDigit`
  predicates) — port them to the remark plugin (or use a regex that approximates them
  for the remark text-node walk).

### EP-02: Copy-code button (D-02) — CodeBlockWidget
- **D-02:** A copy-to-clipboard button on the read-mode `CodeBlockWidget`
  (`widgets.ts:121`). The button:
  - Appears on the widget DOM (a small button element appended to the widget
    container).
  - On click: copies `this.code` to clipboard via `navigator.clipboard.writeText`
    with a graceful no-op fallback (try/catch — if clipboard is unavailable, the
    button does nothing or shows a brief "copy failed" state).
  - Brief visual confirmation (button text changes to "Copied" for 1-2 seconds).
  - DOM built with `createElement` + explicit attributes — NO `innerHTML`.
  - Does NOT perturb the async highlight swap or `destroyed`/`isConnected` teardown
    (`widgets.ts:165-178`).

### EP-04: Description in search (D-03) — additive field
- **D-03:** The search backend indexes the frontmatter description and returns it as
  an additive optional field on `search.Result`. Changes:
  - `internal/search/doc.go`: add `Description` to the page document struct.
  - `internal/search/rebuild.go`: index the description (from `okf.Parse` frontmatter).
  - `internal/search/query.go`: add `Description string json:"description,omitempty"`
    to `Result` (`:14-21`).
  - `internal/server/handlers_search.go`: pass the description through in the response.
  - `web/src/api/client.ts`: add `description` to the search result type.
  - `web/src/components/search/SearchResultRow.tsx`: render the description for page
    results (distinct from the body snippet); absent description falls back to
    today's card.
  - No relevance change (description is additive, not queried — or it IS indexed for
    search; the planner decides whether the description is searchable or just
    displayed. Recommend: index it for search too — a description match is a strong
    signal — but this is a search-relevance change, so state it clearly. If the
    requirement only says "show in cards," display-only is safer).

### The agent's Discretion
- The exact remark plugin shape (a custom plugin function vs a published package —
  recommend a small custom plugin matching the CM6 recognition).
- Whether the description is indexed for search (relevance) or display-only
  (recommend: display-only per the requirement's "show in cards" language).
- The copy button visual style (icon + text vs icon-only).
- Commit slicing.
</decisions>

<disciplines>
## Lenses surfaced for this phase

~~~yaml
disciplines:
  - section: "EP-01 XSS guard"
    name: "badge-is-element-not-html"
    scope: ["the remark hashtag plugin", "MarkdownProse.tsx"]
    guidance: "The #tag badge MUST be a React element (hast span node), NOT raw HTML
      injection. rehype-raw stays OFF; rehype-sanitize stays ON. The remark transform
      produces a typed hast element, which rehype-sanitize keeps (span is in the
      default schema). Injecting the badge as a raw HTML string would bypass
      rehype-sanitize and reopen the stored-XSS surface."
    rationale: "EP-01 is the security-sharpest of the three. MarkdownProse renders
      untrusted page content; the XSS guard is the no-raw-HTML invariant. A badge
      injected as raw HTML would be a stored-XSS vector."
    override_signals:
      - "None — raw HTML injection in a surface rendering untrusted content is
        always wrong. Use React elements / hast nodes."
~~~

</disciplines>

<canonical_refs>
## Canonical References

### Phase spec & requirements
- `.planning/ROADMAP.md` §"Phase 36: Rendering Polish" — goal, success criteria, shared-file note.
- `.planning/REQUIREMENTS.md` §EP-01 (193-207), §EP-02 (208-219), §EP-04 (233-244).

### EP-01 — MarkdownProse badge
- `web/src/components/MarkdownProse.tsx` (the ReactMarkdown setup at :77-84, remark/rehype plugins at :78-79).
- `web/src/components/AgentAnswer.tsx` (uses MarkdownProse; the stored-XSS test at :42-44).
- `web/src/components/HistoryPanel.tsx` (uses MarkdownProse for version preview).
- `web/src/lib/cm/hashtag.ts:44-68` (the recognition predicates to port: isWs/isBody/isNonDigit).
- `web/src/lib/cm/widgets.ts:335-359` (HashtagWidget — the visual to match).

### EP-02 — Copy-code button
- `web/src/lib/cm/widgets.ts:121-178` (CodeBlockWidget — the widget + async highlight + teardown).

### EP-04 — Description in search
- `internal/search/query.go:14-21` (Result struct — add Description field).
- `internal/search/doc.go` (page document struct — add Description).
- `internal/search/rebuild.go` (indexPage — index the description from frontmatter).
- `internal/server/handlers_search.go` (pass description through).
- `web/src/components/search/SearchResultRow.tsx` (render description for page results).
- `web/src/api/client.ts` (search result type — add description).

### No external specs
- No third-party specs/ADRs.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `MarkdownProse.tsx` — the react-markdown component; add a remark plugin for the badge.
- `hashtag.ts` — the CM6 recognition predicates (port to remark).
- `CodeBlockWidget` — the code block widget; add a copy button.
- `search.Result` — the search result DTO; add an additive Description field.

### Established Patterns
- `createElement` + `textContent` DOM invariant (no innerHTML) — shared with Phase 32.
- `rehype-sanitize` ON / `rehype-raw` OFF — the stored-XSS guard.
- Search Result is additive (heading/attachment fields are `omitempty`).

### Landmines
- **EP-01 is the security-flagged one** — the badge MUST be a React element, never raw HTML. `rehype-raw` OFF / `rehype-sanitize` ON preserved. The AgentAnswer XSS test must still pass.
- **Shared file `widgets.ts`** — Phase 32 also touches it (HashtagWidget color). Sequence 32 first.
- **qa MUST verify each independently** (red-team MIN-05) — three unrelated features in one phase; a subagent could ship 2 of 3.
</code_context>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.
</deferred>

---

*Phase: 36-rendering-polish*
*Context gathered: 2026-07-02*