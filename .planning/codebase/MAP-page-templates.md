---
author: codebase-archaeologist
responsible: codebase-archaeologist
phase: 27
milestone: v1.4
status: verified
confidence: direct_read
grounding: direct file read (SMTC daemon offline this session)
head: 1ba6d6b
---

# Codebase Map: Page Templates Create-Path

## Subsystem: Template-seeded page creation (ED-06a)

New pages can be seeded from flat `*.md` files in a top-level `Templates/` folder,
with `{{title}}` / `{{date}}` / `{{time}}` placeholders substituted client-side and the
frontmatter merged robustly server-side through the canonical `okf` engine. Added in
**Phase 27**; extends the pre-existing blank-create path rather than replacing it.

### Files

| File | Role |
|------|------|
| `internal/pages/templates.go` | `templatesDir = "Templates"`; `TemplateInfo{Name, Path}`; `ListTemplates(ctx)` (non-recursive flat `Templates/*.md` enumeration via `repo.Tree()`, every candidate re-validated through `repo.Resolve` — SEC-01); `CreateFromTemplate(ctx, folder, title, seedFrontmatter, seedBody, user)` + `ErrTemplateInvalid`. |
| `internal/okf` (`ValidateFrontmatter`) | Strict-decodes the assembled seed frontmatter, rejecting malformed YAML AND **duplicate top-level keys** with `ErrTemplateInvalid` (fail-closed: okf's faithful `yaml.Node` model would otherwise re-emit duplicates as a corrupt page). |
| `internal/server/handlers_pages.go` | `handleListTemplates` (GET `/templates`, any-authed); `handleCreatePage` (POST `/pages`, editor) — dispatches to `CreateFromTemplate` iff the body carries `frontmatter`/`body`, else the blank `Create`. Request/response structs: `createPageRequest{Folder,Title,Frontmatter?,Body?}`, `templateInfoResponse`, `listTemplatesResponse{Templates}`, `createPageResponse{Path}`. |
| `internal/server/router.go` | `authed.Get("/templates", …)` (line 177); `editor.Post("/pages", h.handleCreatePage)` (line 257). |
| `web/src/lib/templates.ts` | `substitutePlaceholders(text, {title?, now})` — pure `String.replaceAll` + `Intl.DateTimeFormat` (user-local). No templating engine; any other `{{…}}` is left literal. |
| `web/src/api/client.ts` | `listTemplates()` (GET, no CSRF), `createPage(...)` (CSRF-bearing POST). |
| `web/src/components/CreatePageModal.tsx` | The template picker + title input (consumes `listTemplates`, calls `createPage`). |

### Create flow

```
CreatePageModal
  → listTemplates() → GET /templates → flat Templates/*.md {name, path}   (or "Blank")
  → user picks a template → getPage(template.path) reads its content
  → substitutePlaceholders (client): {{date}}/{{time}} in BOTH; {{title}} in BODY only
  → createPage(folder, title, frontmatter, body) → POST /pages
      → handleCreatePage: frontmatter||body non-empty → CreateFromTemplate
          1. okf.Parse(AssembleSource(seedFrontmatter, seedBody))     (before the lock)
          2. okf.ValidateFrontmatter → ErrTemplateInvalid on malformed / duplicate keys (400)
          3. lockMutation → uniquePath(folder, title)
          4. okf.SetField(title) — USER title wins over any template title:
          5. okf.Repair — add remaining required fields
          6. enqueueWrite "create" + index/graph upsert (single-writer spine)
      → 201 {path}
```

### Key invariants & landmines (per-claim confidence)

1. **`{{title}}` is substituted into the BODY only, never the frontmatter.** (HIGH —
   `templates.ts:35-46`.) The canonical `title:` is owned by the server's `okf.SetField`
   (which YAML-quotes correctly). Injecting a raw title into a YAML value position would
   corrupt valid templates (e.g. a title `Meeting: Kickoff` breaks `title: {{title}}`).
   A future client change that substitutes title into frontmatter is a corrupt-YAML bug.

2. **The frontmatter merge is authoritative SERVER-side; a template PATH is never trusted.**
   (HIGH — `handlers_pages.go:199-224`, `templates.go:78-99`.) The client submits substituted
   CONTENT only; the server re-parses+validates+repairs into ONE valid frontmatter block. The
   user's title always wins over a template `title:`.

3. **`ValidateFrontmatter` fails closed on duplicate top-level keys.** (HIGH —
   `templates.go:12-18,108-114`.) okf's faithful `yaml.Node` model ACCEPTS duplicates and
   would emit a corrupt two-value page; the strict-decode gate is what prevents that. Do not
   relax it to "lenient" — the fail-closed behavior is the ED-06a/D-04 robust-merge requirement.

4. **`ListTemplates` is non-recursive and flat.** (HIGH — `templates.go:33-76`.) Nested
   `Templates/sub/x.md`, directories, and non-`.md` files are all skipped. A repo with no
   `Templates/` folder yields an empty list (not an error) → the picker just shows "Blank".

5. **Every listed path is `repo.Resolve`-gated (SEC-01 / D-06).** (HIGH — `templates.go:63-69`.)
   `Tree()` entries are within-root by construction, but routing through `Resolve` keeps every
   content path resolver-gated; a path that fails to resolve is silently dropped, never listed.

6. **`CreateFromTemplate` mirrors `Create` exactly except for the seed.** (HIGH —
   `templates.go:78-140`.) Same `uniquePath` slug, same single-writer `enqueueWrite "create"` +
   index/graph upsert. Parsing/validation happen BEFORE the mutation lock (pure, no shared
   state) so a bad template fails fast with a clean 400 and never holds the serialization lock.

### Blast radius (direct-read estimate)

- `CreateFromTemplate` is the only additional caller of `okf.ValidateFrontmatter`; changing that
  validator affects template creation robustness. `okf.SetField`/`Repair`/`Parse`/`Emit` are the
  shared byte-stable engine used across pages — do not special-case them for templates.
- `handleCreatePage` now branches on `frontmatter||body`; the blank-create path (`pages.Create`) is
  unchanged — a regression here would affect BOTH create modes.
- `substitutePlaceholders` is pure and client-only; a bug is cosmetic (wrong date), never a
  server-side integrity issue (the server owns the frontmatter merge).
