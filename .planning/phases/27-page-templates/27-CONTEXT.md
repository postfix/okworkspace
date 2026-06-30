---
author: architect
---

# Phase 27: Page Templates — Context

**Gathered:** 2026-06-30
**Status:** Ready for planning

<domain>
## Phase Boundary

Creating a page can seed it from an Obsidian-style **template** — a plain `.md` file in a
`Templates/` folder. The new-page flow offers a picker (blank default); the chosen template's
frontmatter + body seed the page with `{{title}}`/`{{date}}`/`{{time}}` substituted, and the
template's frontmatter is merged into the new page's SINGLE valid frontmatter block (no double
`---`, no duplicate keys), then required-field repaired. **Backend + frontend.** No templating
engine, no executable templates. Requirement: **ED-06a**. Fully independent (no phase dependency).

**In scope (ED-06a):**
- Templates = plain `.md` files in a top-level `Templates/` folder (files-as-truth, git-versioned,
  editable as normal pages, visible in the tree).
- A server **list endpoint** enumerating templates; the create-page flow offers a picker with a
  **blank/no-template default** (the existing title-only create unchanged).
- The chosen template seeds the new page; `{{title}}`/`{{date}}`/`{{time}}` substituted via
  `String.replaceAll` + `Intl.DateTimeFormat` (client; user-local date/time) — NO templating engine,
  only these three placeholders.
- **Robust frontmatter merge (the load-bearing risk):** a template carrying its OWN `---`
  frontmatter yields a single valid block — no two blocks, no duplicate keys, no malformed YAML —
  via the server's `okf` engine (yaml.Node), then `okf.Repair`. An adversarial test is required.
- Template reads (list + the chosen template) resolve through the existing fuzz-tested safe path
  resolver (`repo.Resolve`), constrained to the `Templates/` subtree.

**Out of scope / unchanged:**
- Insert-at-cursor into an existing page (deferred — Future).
- Templater-style per-folder default templates / template functions (Future).
- Nested/recursive `Templates/` subfolders — flat `Templates/` for v1.4.
- Any placeholder beyond the three; any executable/JS/Templater template (code-execution surface).
- The existing title-only create path (blank default) — kept exactly.
</domain>

<decisions>
## Implementation Decisions

### Storage + listing (D-01, D-02)
- **D-01 — Templates are plain `.md` files in a top-level `Templates/` folder.** Files-as-truth,
  git-versioned, editable as ordinary pages, visible in the tree. No special store/format. A repo
  with no `Templates/` folder → the picker simply shows only "Blank".
- **D-02 — Server list endpoint enumerates `Templates/*.md`.** A `GET /templates` (or
  `/api/v1/templates`) returns each template's display name + path (flat; non-recursive for v1.4).
  Reads are constrained to the `Templates/` subtree via `repo.Resolve` (path safety). Available to
  the same audience that can create (the picker only appears in the editor-only create flow); read
  of a template's content is non-secret.

### Substitution (D-03)
- **D-03 — Client-side substitution of the three placeholders (spec-literal).** `{{title}}`,
  `{{date}}`, `{{time}}` substituted with `String.replaceAll` + `Intl.DateTimeFormat` so dates/times
  are USER-LOCAL (matches Obsidian + the ED-06a-named APIs). Sensible default formats: date
  `YYYY-MM-DD`, time `HH:mm` (24h). NO templating engine; any other `{{…}}` text is left literal;
  placeholders cannot execute code (pure string replace).

### Robust merge + repair — SERVER via okf (D-04, D-05)
- **D-04 — The robust frontmatter merge + required-field repair happen SERVER-side via `okf`
  (the canonical yaml.Node engine), NOT client string-frontmatter.** This is the load-bearing
  ED-06a risk: a template's own `---` frontmatter must merge into ONE valid block with the new
  page's `title` (user title wins on conflict), with NO duplicate keys / malformed YAML, then
  `okf.Repair` adds required fields. Doing this in `okf` (Go) — which already round-trips
  frontmatter byte-stably and powers every save — is far safer than the client's string-based
  `frontmatter.ts`. An adversarial test (template with its own frontmatter, a conflicting `title:`,
  duplicate keys, no frontmatter, weird YAML) is mandatory.
- **D-05 — Extend the existing server create pipeline; client substitutes then posts.** The client
  fetches the chosen template's raw content, substitutes the three placeholders (D-03), and posts to
  the create flow (extended `POST /pages` or a sibling) with `{ title, folder, seededContent }`.
  The server parses the (single) submitted frontmatter via `okf`, sets the canonical `title` (path
  still derived from the user's title as today), `okf.Repair`s, and writes through the EXISTING
  single-writer queue + git commit + audit (`pages.Create` path). Blank/no-template default → the
  current title-only `pages.Create` path, unchanged. Reuse, don't fork, the create pipeline.

### Safety (D-06, D-07)
- **D-06 — Path safety via `repo.Resolve`, constrained to `Templates/`.** Both the list and the
  template-content read resolve through the fuzz-tested safe resolver; a template path that escapes
  `Templates/` (or the repo root) is rejected (`ErrUnsafePath`). No reading arbitrary files as
  "templates".
- **D-07 — No executable templates.** Static text + the three safe `{{…}}` placeholders only
  (literal replace; no eval, no arbitrary-key expansion, no Templater functions). Template bytes are
  untrusted page content and flow through the existing sanitized render path like any page.

### UX (D-08)
- **D-08 — Picker in the create-page modal, blank default.** `CreatePageModal.tsx` gains a template
  selector defaulting to "Blank" (current behavior). Minimal additive UI; choosing a template seeds
  the new page, then navigates to it (existing post-create navigation).

### Claude's Discretion
- The exact endpoint shape (`GET /templates` returning name+path vs name+content; whether the client
  does a second raw-content fetch on select); the date/time format strings within the
  `Intl.DateTimeFormat` brief; the picker's visual form (dropdown vs list); display-name derivation
  (filename without `.md`).
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements / roadmap
- `.planning/REQUIREMENTS.md` §ED-06a — locked acceptance criteria + security + out-of-scope.
- `.planning/ROADMAP.md` §"Phase 27" — goal + the four observable success criteria.

### Code to extend / reuse
- `internal/server/handlers_pages.go` — `handleCreatePage` (≈160) + `createPageRequest`/`Response`
  (≈22-31); the create entry point to extend with an optional template/seeded content. Add the
  templates list handler near it.
- `internal/pages/service.go` — `Create` (≈ the title→path→assemble→`enqueueWrite("create")` flow,
  ≈164); the single-writer write pipeline to reuse for a template-seeded create.
- `internal/okf/repair.go` — `Repair(d *Doc, now time.Time)` (line 22) + `SetField` (the byte-stable
  field set used in Phase 24); the robust merge + required-field repair engine (D-04).
- `internal/repo/path.go` — `Resolve(rel)` (line 83) + `ErrUnsafePath`; the safe resolver for
  template reads (D-06).
- `internal/server/router.go` — route registration (the editor group for create; where the
  templates list route is added).
- `web/src/api/client.ts` — `createPage` (≈327) + the `mutate` helper; add `listTemplates` +
  the template-seeded create.
- `web/src/components/CreatePageModal.tsx` — the new-page modal to add the picker to.
- `web/src/lib/frontmatter.ts` — client `setField`/`readField` (only for any client-side display;
  the AUTHORITATIVE merge is server `okf`, D-04).

### Prior-phase invariants (context)
- Phase 24 used `okf.SetField` for a byte-stable frontmatter write — the same engine the merge
  reuses. `.smtc/memories/qa-m1.4-phase24-verified.md`.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `pages.Create` + the single-writer write queue + git commit + audit — reuse for the seeded create.
- `okf.Repair` / `okf.SetField` / the `okf` yaml.Node engine — the robust merge + repair (D-04).
- `repo.Resolve` — path safety for template reads (D-06).
- `CreatePageModal.tsx` + `createPage` client — extend with the picker + seeded create.

### Established Patterns
- Files-as-truth (templates are just pages); git-versioned; the safe path resolver gates all reads.
- Server `okf` is the canonical frontmatter engine (byte-stable, repair on write).
- Editor-gated mutations + CSRF on the create POST (existing).

### Integration Points
- `router.go` — add the templates list route; the create route already exists (extend).
- `CreatePageModal.tsx` — the picker.
- `Templates/` folder in the repo data dir — the template source.

### Risk Notes
- The frontmatter merge (template-own-frontmatter + title → one valid block) is THE risk — keep it
  in server `okf`, adversarially tested. Avoid a client-side string merge.
- Don't let a template path escape `Templates/` (path-safety) and don't treat a non-`.md` or a
  nested path as a template in the flat-MVP model.
</code_context>

<specifics>
## Specific Ideas
- Obsidian-parity: drop `.md` files in `Templates/`, pick one when creating a page, with `{{title}}`,
  `{{date}}`, `{{time}}` filled in. A template can have its own frontmatter (tags, type, etc.) that
  carries into the new page.
</specifics>

<deferred>
## Deferred Ideas
- Insert-template-at-cursor into an existing page; per-folder default templates / Templater functions.
- Recursive/nested template folders; more placeholders; a template-management admin UI.
</deferred>

---

*Phase: 27-page-templates*
*Context gathered: 2026-06-30*
