# Authoring API

The authoring API covers the v1.4 page-creation and AI-authoring endpoints: page
templates, page creation (blank or template-seeded), and the LLM-generated page
description (generate → review → apply). Tag suggestion/apply endpoints are documented
separately in [tagsweep.md](tagsweep.md).

**Base path:** `/api/v1`

Auth model: template list and description **generate** are read-only and available to any
authenticated user; page **create** and description **apply** are editor-gated and covered
by the global nosurf CSRF middleware on mutating methods. Neither AI mode is an Eino tool —
the read-only 5-tool allow-list is unchanged.

---

## `GET /api/v1/templates`

Lists the flat `*.md` files directly under the top-level `Templates/` folder, offered as
page-creation templates. Non-recursive: nested entries, directories, and non-`.md` files
are skipped. A workspace with no `Templates/` folder returns an empty list (the picker
shows only "Blank").

**Auth:** Any authenticated user (the create picker that consumes it is editor-only, but a
template's content is non-secret).

**Response `200 OK`:**

```json
{
  "templates": [
    {"name": "Meeting Notes", "path": "Templates/Meeting Notes.md"},
    {"name": "Decision Record", "path": "Templates/Decision Record.md"}
  ]
}
```

- `name` — the filename without its `.md` extension (the display label).
- `path` — the repo-relative path, passed back to `GET /pages/*` to read the chosen
  template's content.

Always returns `{"templates": [...]}` (empty array when there is no `Templates/` folder).

**Errors:**
- `500` — Generic unavailable.

**Implementation:** `pages.Service.ListTemplates` enumerates via the existing `repo.Tree()`
listing; every candidate path is re-validated through `repo.Resolve` so the SEC-01 invariant
(every content path is resolver-gated) holds for template reads too.

**Source:** [`internal/server/handlers_pages.go`](../../internal/server/handlers_pages.go) `handleListTemplates()`, [`internal/pages/templates.go`](../../internal/pages/templates.go) `ListTemplates()`

---

## `POST /api/v1/pages`

Creates a new page. With no template seed it is a title-only **blank** create; when the
body carries template `frontmatter`/`body` it is a **template-seeded** create in which the
server robustly merges the (already placeholder-substituted) content into one valid
frontmatter block.

**Auth:** Editor or admin.

**Request body:**

```json
{
  "folder": "docs",
  "title": "Q3 Planning",
  "frontmatter": "title: {{title}}\ntags:\n  - planning\n",
  "body": "# {{title}}\n\nCreated {{date}}.\n"
}
```

- `folder` — destination folder (empty = repo root).
- `title` — the page title (required); the slugged filename derives from it.
- `frontmatter`, `body` — **optional** template seed content. When either is non-empty the
  page is seeded from it; when both are empty/absent it is a blank create. The client has
  already substituted `{{title}}`/`{{date}}`/`{{time}}`. Only substituted **content** is
  submitted — a template PATH is never trusted.

**Response `201 Created`:**

```json
{"path": "docs/q3-planning.md"}
```

**Errors:**
- `400` — "Give your page a title to create it." (missing title), or "We couldn't create the
  page from that template — its formatting is invalid." (malformed YAML or duplicate top-level
  keys — the template fails closed rather than producing a corrupt page).
- `500` — Generic unavailable.

**Implementation:** The seed is parsed + strict-validated (`okf.ValidateFrontmatter`,
rejecting malformed YAML and duplicate keys) **before** the mutation lock, so a bad template
fails fast. The user's `title` always wins over any template `title:` (`okf.SetField`);
`okf.Repair` fills the remaining required fields; the write goes through the same
single-writer commit spine + index/graph upsert as a normal create.

**Source:** [`internal/server/handlers_pages.go`](../../internal/server/handlers_pages.go) `handleCreatePage()`, [`internal/pages/templates.go`](../../internal/pages/templates.go) `CreateFromTemplate()`

> **Placeholders** are substituted client-side by
> [`web/src/lib/templates.ts`](../../web/src/lib/templates.ts) `substitutePlaceholders`:
> `{{date}}`/`{{time}}` (user-local) are filled in both frontmatter and body; `{{title}}` is
> filled in the **body only** (the canonical frontmatter `title:` is owned by the server's
> `okf.SetField`, which quotes it correctly). Any other `{{…}}` is left literal — there is no
> templating engine.

---

## `POST /api/v1/agent/generate-description`

Generates one plain-text sentence describing a page (the LLM-generated `description:`
frontmatter value). Read-only — it returns a proposal for the review surface and never
writes. Returns the page's base revision (the optimistic-concurrency token the later apply
must echo back).

**Auth:** Any authenticated user.

**Request body:**

```json
{"page_path": "docs/getting-started.md"}
```

**Response `200 OK`:**

```json
{
  "page_path": "docs/getting-started.md",
  "description": "A step-by-step guide to setting up and running the workspace.",
  "base_revision": "abc123def"
}
```

- `description` — the validated, cleaned, single-line proposal (≤ 200 runes; no
  control/NUL/newline characters; internal whitespace collapsed).
- `base_revision` — Git revision captured at generate time (echoed to `/apply-description`
  so a page moved in between returns 409 instead of clobbering).

**Errors:**
- `400` — "Choose a page to describe." (missing/invalid path).
- `404` — "This page no longer exists. It may have been moved or deleted."
- `422` — "The assistant couldn't produce a clean description. Try again." (validation failed
  across all 3 attempts).
- `502` — "The assistant is unavailable right now. Try again in a moment." (provider error).
- `503` — "The assistant is turned off. Ask an administrator to enable it." (agent disabled).

**Implementation:** `agent.Service.GenerateDescription` — a single-shot `ChatModel.Generate`
**mode** (not a tool). It reads the body via the role-scoped pages reader, truncates it to
the input budget, runs a validate-and-retry loop (1 + 2 retries = 3 attempts), and returns
the cleaned text. `agent.ValidateDescription` is the shared output gate.

**Source:** [`internal/server/handlers_agent.go`](../../internal/server/handlers_agent.go) `handleGenerateDescription()`, [`internal/agent/describe.go`](../../internal/agent/describe.go) `GenerateDescription()`, `ValidateDescription()`

---

## `POST /api/v1/agent/apply-description`

Applies an approved (and possibly user-edited) description to a page byte-stably. The
client value is re-validated + cleaned server-side (never trusted verbatim). The base
revision is checked for optimistic concurrency (409 on stale).

**Auth:** Editor or admin (CSRF-protected).

**Request body:**

```json
{
  "page_path": "docs/getting-started.md",
  "description": "A step-by-step guide to setting up and running the workspace.",
  "base_revision": "abc123def"
}
```

**Response `204 No Content`:** Empty body on success — the `description:` scalar was written
and committed.

**Errors:**
- `400` — "Invalid request." (missing/invalid path).
- `404` — "This page no longer exists. It may have been moved or deleted."
- `409` — "This page changed since the assistant described it. Generate a fresh description
  against the latest version." (base revision moved).
- `422` — "That description can't be applied. Generate a fresh one and try again."
  (re-validation failed).
- `500` — Generic unavailable.

**Implementation:** Re-validates + cleans the client description via
`agent.ValidateDescription` (trim; reject NUL/control/newline → single line; length cap),
writes **only** the `description:` scalar byte-stably via `okf.SetField`, and reuses
`pages.Service.Save` with the generate-time `base_revision` (single-writer commit; 409s on a
moved revision and writes nothing). The approval is audited with the non-secret role only —
never the description text or page content.

**Source:** [`internal/server/handlers_agent.go`](../../internal/server/handlers_agent.go) `handleApplyDescription()`, `setDescriptionFrontmatter()`

> **Autosave-safe apply.** Both apply-tags and apply-description are a *second* writer against
> the editor's autosave. The client uses a shared **merge-first reconcile** in
> [`web/src/routes/PageEditor.tsx`](../../web/src/routes/PageEditor.tsx) (`reconcileApplied`):
> merge the approved value into the local frontmatter region first (so a transient refresh
> failure can't strand it), then advance the base revision past the apply write, then schedule
> one autosave if the local bytes diverge — no false 409, no revert.

---

## Related endpoints

- **`POST /api/v1/agent/apply-tags`** — the editor+CSRF write half of the LLM tag-suggestion
  flow, documented in [tagsweep.md](tagsweep.md). It shares the same optimistic-concurrency
  floor and the same client-side merge-first reconcile as apply-description.
