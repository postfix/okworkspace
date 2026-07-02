---
author: architect
responsible: architect
phase: 32
phase_type: implementation
status: Ready for planning
hard_bar: false
security_relevant: true
design_fork: true
parent_artifacts:
  - .planning/ROADMAP.md
  - .planning/REQUIREMENTS.md
---

# Phase 32: Tag Metadata Store (color/description) - Context

**Gathered:** 2026-07-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Introduce a **net-new**, committed, human-readable, on-disk tag-metadata store
(files-as-truth) with a rebuildable SQLite cache + its migration; let an admin set
a tag's color + description, surfaced in the admin list and on rendered badges. This
is the tag-track's load-bearing unknown — tag metadata has NO on-disk source today,
so this phase creates the on-disk format, a cache table + migration, the
rebuild-from-disk harness, the admin endpoint + UI, and the badge color plumbing.

**Requirements:** TF-02

**Non-goals:** tag hierarchy (Phase 33, but the store shape chosen here MUST extend
trivially to the parent field); tag-op undo (Phase 35); rewriting page bodies (this
phase is metadata-only — zero page bytes change); the `MarkdownProse` react-markdown
badge (Phase 36 — this phase only colors the CM6 `HashtagWidget`).

**This phase is substantial** (per the red-team scope-acknowledgement): it is NOT a
thin cache layer. It invents the disk format, a cache table + migration, and the
rebuild harness from scratch.
</domain>

<decisions>
## Implementation Decisions

### On-disk store shape (D-01) — `Tags/` folder of `.md` files [CO-DRIVER CHOSEN]
- **D-01:** Tag metadata persists as one Markdown file per tag under a top-level
  `Tags/` folder — `Tags/{tagname}.md` — with YAML frontmatter carrying the metadata
  fields. The body is free-form documentation (optional, ignored by the cache). This
  is OKF-native: the same Markdown + YAML frontmatter format as wiki pages, editable
  with the same editor, Git diffs per-tag, and it extends trivially to Phase 33's
  `parent` field (one more frontmatter key). It mirrors the `Templates/` pattern
  (`pages/templates.go:22` — `const templatesDir = "Templates"`, flat `*.md` files,
  `repo.Tree()` + prefix filter + `repo.Resolve` SEC-01 enumeration).
- **D-01a (frontmatter schema):**
  ```yaml
  ---
  tag: deploy        # the tag name (canonical, normalized); matches the filename stem
  color: "#4caf50"   # optional; a hex color OR a palette token; absent = default
  description: "Deployment & release ops"  # optional; single-line, length-capped, HTML-safe
  ---
  # deploy
  Optional free-form documentation body (ignored by the cache; user-editable).
  ```
  The `tag` key is the canonical name (the filename stem is the display path; the
  frontmatter `tag` is the authoritative key, validated to match the normalized
  filename). Phase 33 adds `parent: ops` here.
- **D-01b (file naming):** filename = `{tagname}.md`, where `tagname` is the normalized
  tag (lowercase, spaces → the tag charset allows spaces but filenames with spaces
  need care — use the raw tag name as the filename, same as Templates does with page
  titles). Reuse the `repo.Write` / `repo.Resolve` SEC-01 path (no traversal).
- **D-01c (absent store = graceful):** a workspace with no `Tags/` folder yields empty
  metadata — every tag renders as today. The feature is additive. This mirrors
  `ListTemplates` (empty list when no folder, not an error, `templates.go:36`).

### SQLite cache + migration (D-02) — rebuildable derived cache
- **D-02:** A new SQLite cache table `tag_meta` holds the parsed metadata, fully
  rebuildable from the `Tags/` files. A new migration `0011_tag_meta.sql` creates it.
  The discipline mirrors `page_tags` (migration `0009_graph.sql`: "operational/derived
  data ONLY — deleting these tables and running RebuildTagMeta reproduces identical
  rows; SQLite is NEVER the source of truth").
  ```sql
  CREATE TABLE IF NOT EXISTS tag_meta (
      tag         TEXT PRIMARY KEY,
      color       TEXT NOT NULL DEFAULT '',   -- hex or palette token; '' = no color
      description TEXT NOT NULL DEFAULT ''     -- single-line, HTML-safe; '' = none
  );
  ```
  No index needed beyond the PK — the table is tiny (~tens of rows for a 5-user wiki).
- **D-02a (rebuild harness):** `RebuildTagMeta(ctx)` walks `Tags/*.md` via the SEC-01
  resolver (`repo.Tree()` + prefix filter + `repo.Read` + `okf.Parse`), parses each
  file's frontmatter, and populates `tag_meta`. Delete-cache → rebuild = identical
  metadata. This is the parity test (like `TestDerivedOnly_RebuildReproducesAdjacency`
  in `graph/rebuild_test.go:23`). A `graph_meta`-style key in `graph_meta` or a new
  `tag_meta_meta` row records the last-rebuilt HEAD (optional — the planner decides if
  a last-head check is needed or a simple full-rebuild is sufficient at this scale).

### Store package (D-03) — `internal/tagmeta/`
- **D-03:** A new `internal/tagmeta/` package owns the on-disk store + cache + rebuild.
  Interface: `Store` with `GetMeta(ctx, tag) (Meta, bool)`, `AllMeta(ctx) ([]Meta, error)`,
  `SetMeta(ctx, tag, color, description) error` (writes the `Tags/{tag}.md` file via
  `repo.Write` + commits via the single-writer path), `RebuildTagMeta(ctx) error`, and
  `ClearMeta(ctx, tag) error` (removes color/description — writes the file with empty
  values, or deletes the file if no metadata + no body).
  - `Meta{Tag, Color, Description string}` — the parsed metadata record.
  - `SetMeta` writes the `.md` file (frontmatter + optional body) via `repo.Write`,
    committed through the single-writer `EnqueueCommit` path (same as page writes).
    It does NOT touch any page's frontmatter or body — only `Tags/` files.
  - The package takes a `*repo.Repo` (SEC-01 resolver) + `*store.Store` (SQLite cache)
    + the single-writer commit enqueue (same as `pages.Service`).

### Admin endpoint (D-04) — preview→approve not needed; simple set/clear
- **D-04:** `POST /admin/tags/meta` — admin-only, CSRF-protected (inherited from the
  admin route subgroup). Body: `{tag, color, description}`. Server validates:
  - `tag` via `ValidateTagCharset` (the frontmatter charset gate — the tag name must
    be a valid frontmatter tag).
  - `color` = a hex color (`#RRGGBB` or `#RGB`) OR a named palette token from a fixed
    safe set (e.g. `red`, `blue`, `green`, `amber` — the planner picks the palette);
    reject anything else with a 400. The color is applied to the badge DOM via a
    `style.color` or `style.backgroundColor` attribute — NEVER `innerHTML`.
  - `description` = single-line, ≤200 chars, HTML-metachar-safe (the server escapes /
    rejects `<`, `>`, `&`, `"`, `'` — it is rendered as `textContent` in the admin UI,
    never as HTML). Reject newlines (single-line enforced).
  - Returns the updated `Meta`. Audit: `audit.ActionTagMeta = "tag_meta"` (new constant,
    `audit.go:65-67` pattern).
- **D-04a:** `GET /admin/tags/meta` — admin-only read returning all `[]Meta` (for the
  admin UI to populate the metadata editor). Or extend `handleTagList` to include
  metadata inline — the planner decides (extending `handleTagList` is simpler; a
  separate endpoint is cleaner). Recommend extending `handleTagList` to include
  `color`/`description` per tag (the admin UI already fetches the tag list).

### Badge color plumbing (D-05) — CM6 `HashtagWidget` only
- **D-05:** The CM6 `HashtagWidget` (`widgets.ts:335`) gains an optional `color`
  parameter. When set, `toDOM` applies it via `span.style.color = this.color` (or
  `span.style.backgroundColor` — the planner picks which CSS property carries the
  color; `color` is simpler and the theme already uses `var(--color-accent)` for text).
  The DOM stays `createElement` + `textContent` — NO `innerHTML`. The XSS invariant
  holds (the color is a validated hex/palette token, never user-HTML). The
  `livePreview.ts` call site that constructs `HashtagWidget` reads the tag's color
  from the metadata cache (passed via the editor's props or a client-side fetch).
- **D-05a:** The `MarkdownProse` react-markdown badge is Phase 36 (EP-01), NOT this
  phase. This phase colors only the CM6 editor badge.

### Frontend (D-06) — metadata editor in TagManagement
- **D-06:** `TagManagement.tsx` gains a metadata editor — either a new column in the
  tag table (color swatch + description tooltip) + an edit dialog, or an inline edit
  on each tag row. The edit dialog sets color (a color picker constrained to the safe
  palette + hex) and description (single-line text input). It calls the new
  `setTagMeta` / `getTagMeta` client functions. On save, invalidates `TAGS_KEY`.
- **D-06a:** The admin tag list (`listTags`) response is extended to include
  `color` + `description` per tag (from the metadata cache), so the table renders
  color swatches without a second fetch.

### The agent's Discretion
- The exact safe-palette set for colors (a fixed named-token set + hex — the planner
  picks the named tokens).
- Whether `color` sets text color or background color on the badge (CSS property
  choice — both are safe; pick the one that looks right with the existing theme).
- Whether `handleTagList` is extended inline or a separate `GET /admin/tags/meta`
  endpoint is added (recommend extending `handleTagList` — simpler).
- Whether a last-rebuilt HEAD check is needed for the cache or a full-rebuild is
  sufficient at ~5-user scale (recommend full-rebuild — the table is tiny).
- The `SetMeta` file-write shape (frontmatter-only file vs frontmatter + body
  template — recommend frontmatter + a `# {tag}` heading body for human-readability).
- Commit slicing.
</decisions>

<disciplines>
## Lenses surfaced for this phase

~~~yaml
disciplines:
  - section: "TF-02 store"
    name: "files-as-truth"
    scope: ["internal/tagmeta/", "the Tags/ folder", "the 0011_tag_meta.sql migration"]
    guidance: "Tag metadata is durable content, not operational data. The on-disk
      Tags/*.md files are the source of truth; the SQLite tag_meta table is a
      rebuildable derived cache. Deleting the cache and running RebuildTagMeta
      reproduces identical rows. A cache-only store would violate the project's
      files-as-truth invariant (data must copy off the server as plain files)."
    rationale: "The whole project's data-openness contract: all content stays plain
      Markdown on disk. Tag metadata is content (user-authored color/description),
      not operational data like locks or trash rows. Putting it in SQLite-only
      would make it non-portable."
    override_signals:
      - "None — this is the project's foundational invariant, not a lens. A
        cache-only store is a constraint violation, not an override."
  - section: "TF-02 badge DOM"
    name: "no-innerhtml-badge"
    scope: ["web/src/lib/cm/widgets.ts HashtagWidget.toDOM"]
    guidance: "The badge color MUST be applied via style.color / style.backgroundColor
      (a CSS property on the createElement'd span), NEVER via innerHTML. The
      no-innerHTML XSS invariant (MAP-cm-editor-subsystem #8) is shared with Phase
      36's copy-code button. A color value is a validated hex/palette token, never
      user-HTML — but the DOM API used to apply it must be the safe one regardless."
    rationale: "The badge renders untrusted tag names via textContent. Introducing
      innerHTML to set color would reopen the stored-XSS surface the widget was
      designed to close. The color is safe data, but the DOM API discipline must hold."
    override_signals:
      - "None — innerHTML on a widget rendering untrusted content is always wrong.
        Use style properties."
  - section: "TF-02 color validation"
    name: "safe-color-subset"
    scope: ["the POST /admin/tags/meta handler color validation"]
    guidance: "The color is constrained to a hex subset (#RRGGBB / #RGB) OR a fixed
      named-palette token set. No arbitrary CSS, no url(), no var(), no expressions.
      The server validates and rejects anything outside the subset with a 400. This
      is a trust-boundary input (admin-set, but still untrusted at the server gate)."
    rationale: "The color flows into a style attribute on the badge DOM. An
      unrestricted CSS value could inject `url(javascript:...)` or expression
      payloads in browsers that still process those in style attributes. The subset
      eliminates the surface entirely."
    override_signals:
      - "A proven-safe richer CSS color function (e.g. color-mix) the team explicitly
        wants to support — extend the subset deliberately, don't open it."
~~~

</disciplines>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase spec & requirements
- `.planning/ROADMAP.md` §"Phase 32: Tag Metadata Store" — goal, success criteria, key files, scope acknowledgement.
- `.planning/REQUIREMENTS.md` §TF-02 (lines 126-141) — acceptance criteria (the authoritative bar).

### The pattern to mirror (on-disk store + rebuildable cache)
- `internal/pages/templates.go:22-77` (`templatesDir = "Templates"`, `ListTemplates` —
  the flat `*.md`-files-in-a-folder + `repo.Tree()` + prefix filter + `repo.Resolve`
  SEC-01 enumeration pattern; the `Tags/` store mirrors this exactly).
- `internal/store/migrations/0009_graph.sql` — the "operational/derived data ONLY,
  rebuildable cache, never truth" migration tone (the `0011_tag_meta.sql` migration
  mirrors this language).
- `internal/store/migrations.go:13-65` — the embed + `Migrate` harness (add
  `0011_tag_meta.sql` to the embedded `migrations/` dir; `Migrate` auto-applies it).
- `internal/graph/rebuild.go:33` + `internal/graph/rebuild_test.go:23` —
  `RebuildGraph` + the `TestDerivedOnly_RebuildReproducesAdjacency` parity test pattern
  (the `RebuildTagMeta` + parity test mirrors this).
- `internal/repo/files.go:27` (`Repo.Read`) + `internal/repo/files.go` (`Repo.Write`,
  `Repo.Resolve`, `Repo.Tree`) — the SEC-01 resolver path every file read/write routes
  through.

### The write surface to extend
- `internal/server/handlers_tags_admin.go:507-529` (`handleTagList` — the admin tag
  list; extend to include `color`/`description` per tag from the cache, OR add a
  separate `GET /admin/tags/meta`), `:37-72` (request/response types — add
  `tagMetaRequest`/`tagMetaResponse`), `:265-505` (the admin handler preview→apply
  pattern — though metadata set/clear is simpler, no preview needed).
- `internal/server/router.go:321-343` (admin tag route subgroup — register
  `POST /admin/tags/meta` here; inherits `RequireRole(admin)` + nosurf CSRF).
- `internal/audit/audit.go:63-67` (`ActionTagRename/Merge/Delete/Bulk` — add
  `ActionTagMeta = "tag_meta"` in this block).

### Badge color plumbing
- `web/src/lib/cm/widgets.ts:327-359` (`HashtagWidget` — add optional `color`
  parameter; apply via `span.style.color` in `toDOM`; DOM stays
  `createElement`+`textContent`).
- `web/src/lib/cm/theme.ts:122-128` (`.cm-hashtag` theme — the default color is
  `var(--color-accent)`; a set color overrides it via inline style).
- `web/src/lib/cm/livePreview.ts` (the `HashtagWidget` construction site — pass the
  color from the metadata cache; the planner verifies the exact call site).

### Frontend
- `web/src/components/admin/TagManagement.tsx` (the admin tag surface; add metadata
  editor — color swatch column + edit dialog or inline edit).
- `web/src/api/client.ts:1348-1417` (the tag API functions; add `setTagMeta` /
  `getTagMeta`; extend `TagEntry` with `color`/`description` if `handleTagList` is
  extended inline).

### No external specs
- No third-party specs/ADRs — requirements fully captured above.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `pages.ListTemplates` (`templates.go:42`) — the flat-`*.md`-in-a-folder + SEC-01
  enumeration pattern. The `Tags/` store mirrors this: `repo.Tree()` + prefix filter
  (`Tags/`) + `repo.Resolve` per file.
- `store.Migrate` (`migrations.go:25`) — the embedded-migration harness; add
  `0011_tag_meta.sql` to `internal/store/migrations/` and it auto-applies.
- `graph.RebuildGraph` (`rebuild.go:33`) + the parity test pattern
  (`rebuild_test.go:23`) — the rebuild-from-disk + reproduce-identical-rows pattern.
- `okf.Parse` + frontmatter extraction — the same YAML frontmatter parse used for
  pages; tag metadata files use the same format.
- `repo.Write` / `repo.Read` / `repo.Resolve` / `repo.Tree` — the SEC-01 resolver path.
- `HashtagWidget` (`widgets.ts:335`) — the badge DOM; add `color` parameter, apply
  via `style.color`.

### Established Patterns
- Files-as-truth + rebuildable SQLite cache (page_tags, graph, search — all derived
  from .md files; SQLite is never truth).
- Single-writer commit path (`EnqueueCommit`) for file writes — `SetMeta` writes
  `Tags/{tag}.md` via `repo.Write` + commits through the same path as page saves.
- Admin-only RBAC + nosurf CSRF inherited from the route subgroup.
- `createElement` + `textContent` DOM invariant (the no-innerHTML XSS guard).

### Integration Points
- Router: `admin.Post("/admin/tags/meta", h.handleTagMeta)` in the admin tag subgroup
  (`router.go:343`); optionally `admin.Get("/admin/tags/meta", h.handleTagMetaList)`.
- Migration: `internal/store/migrations/0011_tag_meta.sql` (auto-applied by `Migrate`).
- New package: `internal/tagmeta/` (store + cache + rebuild).
- Frontend: extend `TagManagement.tsx` with a metadata editor; extend `client.ts`
  with `setTagMeta`; extend `TagEntry` with `color`/`description` (or add `TagMeta` type).
- `HashtagWidget` color: `livePreview.ts` passes the color when constructing the widget.

### Landmines
- **Do NOT put tag metadata in `.okf-workspace/`** — that dir holds operational data
  (locks, trash, manifest), not content. Tag metadata is user-authored content; it
  belongs in `Tags/` as committed Markdown files, portable with the repo.
- **Do NOT use `innerHTML` to set the badge color** — use `style.color` /
  `style.backgroundColor`. The no-innerHTML invariant is shared with Phase 36.
- **The color must be a validated subset** — hex or palette token only; no arbitrary
  CSS (url(), var(), expressions). Reject at the server gate with a 400.
- **The `Tags/` folder must be skipped by the page tree** — `tree.go:123`
  (`isSkippedDir`) currently skips `.git` and `.okf-workspace`. Add `Tags/` to the
  skip list so tag-metadata files don't leak into the page navigation tree. This is
  a load-bearing edit — verify it in planning.
- **`SetMeta` writes via the single-writer path** — do NOT fork a parallel commit
  path. `repo.Write` + `EnqueueCommit` (the same path page saves use).
- **Absent metadata = graceful** — a tag with no `Tags/{tag}.md` file renders as today.
  The cache returns empty/default metadata; the badge uses the default theme color.
</code_context>

<specifics>
## Specific Ideas

- The `Tags/{tag}.md` file body can hold human-readable documentation about the tag
  (ignored by the cache, but useful for the team). The frontmatter carries the
  machine-readable metadata (color, description, later parent).
- The rebuild parity test: seed `Tags/deploy.md` with color `#4caf50` + description
  `"Deployment ops"`, run `RebuildTagMeta`, verify the cache row; delete the cache
  table, rebuild, verify identical rows — mirroring
  `TestDerivedOnly_RebuildReproducesAdjacency`.
- The color swatch in the admin tag table: a small colored dot or pill beside each
  tag name, with the description as a tooltip or a second column.
</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope. The hierarchy (Phase 33) extends this
store with a `parent` field; that is Phase 33's scope, not this one's. The
`MarkdownProse` badge color (Phase 36 / EP-01) is also deferred — this phase only
colors the CM6 editor badge.
</deferred>

---

*Phase: 32-tag-metadata-store*
*Context gathered: 2026-07-02*