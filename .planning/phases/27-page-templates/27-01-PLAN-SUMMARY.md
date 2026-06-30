---
author: engineer
responsible: engineer
phase: 27-page-templates
plan: "01"
type: implement
status: complete
requirements: [ED-06a]
---

# Phase 27 — Page Templates (ED-06a) — Execution Summary

**Span:** plan-accept → committed code + this SUMMARY. Author of record: engineer.
**Verdict:** EXECUTE COMPLETE. All tasks done TDD, both gates run + dispositioned, full suite green.

## Plan handshake

PLAN ACCEPTED after verifying the grounding facts against the real code (pages.Create
pipeline, AssembleSource, okf.Parse/SetField/Repair/Emit, repo.Tree, repo.Resolve,
handleCreatePage, client createPage/getPage). One load-bearing nuance surfaced and was
handled in-scope (not a refusal): okf's yaml.Node model is *faithful* and does NOT dedup
duplicate frontmatter keys — `SetField` only fixes the first occurrence, so a duplicate-key
template would otherwise Emit a corrupt page. I added a strict-decode validation gate
(`okf.ValidateFrontmatter`, a `map[string]any` decode that rejects duplicate keys + malformed
YAML — verified empirically) that fails closed with a clean `ErrTemplateInvalid` → 400. This
sits inside the plan's own verify contract ("clean result OR clean error, never a corrupt
two-block page") and keeps YAML confined to the okf engine (D-04).

## Per-task

### T1 — server: list templates + GET /templates
- **`internal/okf/okf.go`** — added `ValidateFrontmatter(d *Doc) error`: strict map-decode of
  `d.RawFront`, rejecting duplicate top-level keys (and nested) + malformed YAML. Empty/no
  frontmatter is valid.
- **`internal/pages/templates.go`** — `ListTemplates(ctx) ([]TemplateInfo, error)`: enumerates flat
  `Templates/*.md` via `repo.Tree()`, filters out dirs / nested / non-`.md`, re-resolves each via
  `repo.Resolve` (SEC-01/D-06), empty when no folder. `TemplateInfo{Name, Path}`.
- **`internal/server/handlers_pages.go`** — `handleListTemplates` → `{templates:[{name,path}]}` (200).
- **`internal/server/router.go`** — `authed.Get("/templates", …)` in the any-authed read group.
- Tests: `internal/pages/templates_test.go` (empty-no-folder, flat-md-only skips noise, all-paths-resolve);
  `internal/server/handlers_templates_test.go` (200 + list, reader-authed, nested skipped).

### T2 — server: CreateFromTemplate + extend POST /pages + adversarial merge
- **`internal/pages/templates.go`** — `CreateFromTemplate(ctx, folder, title, seedFrontmatter, seedBody, user)`:
  mirrors `pages.Create` exactly (uniquePath from user title → `enqueueWrite "create"` → index+graph
  upsert, `ErrTitleRequired` on empty), seeding from `okf.Parse(AssembleSource(...))`. Parse+`ValidateFrontmatter`
  run BEFORE the mutation lock (pure, fail-fast 400); then `SetField(title)` (user wins) → `Repair` →
  `Emit`. A client-sent template PATH is never trusted — only substituted CONTENT.
- **`internal/server/handlers_pages.go`** — `createPageRequest` gains optional `Frontmatter`/`Body` (omitempty);
  `handleCreatePage` branches to `CreateFromTemplate` when either is non-empty, else unchanged blank `Create`.
  `ErrTemplateInvalid` → 400.
- **Adversarial merge test (mandatory, present):** `templates_test.go` covers own-frontmatter→single block,
  conflicting `title:`→user wins, duplicate keys→`ErrTemplateInvalid`, malformed YAML→clean error, body-only→
  repaired, re-Parse round-trip. `handlers_templates_test.go` covers seeded create (merged single block +
  body, user title wins), blank-unchanged, reader 403, missing-CSRF rejected, duplicate-key → 400.

### T3 — client: substitution lib + listTemplates + picker
- **`web/src/lib/templates.ts`** — `substitutePlaceholders(text, {title?, now})`: `{{date}}`/`{{time}}` via
  `Intl.DateTimeFormat` (YYYY-MM-DD / HH:mm h23, user-local), `{{title}}` only when supplied; other `{{…}}`
  literal; pure, no eval.
- **`web/src/api/client.ts`** — `listTemplates()` (GET /templates) + `createPage(folder, title, seed?)`.
- **`web/src/components/CreatePageModal.tsx`** — Blank-default `<select>` fed by `listTemplates`; on a chosen
  template: `getPage` → substitute → `createPage` with seed; Blank → unchanged. No-templates → no select.
- Tests: `templates.test.ts` (3 placeholders, replaceAll, other-`{{}}` literal, passthrough, no-eval, title-omitted
  literal, colon-title in body); `CreatePageModal.test.tsx` (Blank→no seed, template→getPage+substituted seed,
  no-templates→Blank only, title-required intact). LeftTree mock extended for the new exports.

## Gate outcomes + dispositions

### anvil-code-reviewer — `/tmp/anvil-spawn-out/27-REVIEW.md` (FIX FIRST: 1 MAJOR, 1 minor)
- **MAJOR — ACCEPTED + FIXED** (commit 432486c). The client substituted the *raw* user title into the
  template's frontmatter `{{title}}` before the server's `SetField`, so a common title ("Meeting: Kickoff")
  turned `title: {{title}}` into invalid YAML (→400) and a `#`-bearing title could silently truncate a
  non-title field. Fix: `{{title}}` is substituted in the **body only** (opaque Markdown — colon titles safe);
  the server's okf `SetField` owns the frontmatter title and yaml-quotes it. Verified the server emits
  `title: 'Meeting: Kickoff'`. Regression tests added at lib + modal level.
- **MINOR — ACKNOWLEDGED, deferred (route up).** `getPage(path)` does not `encodeURIComponent` the path, so a
  template filename containing `#`/`?`/`%` would mis-route. This is a **pre-existing app-wide convention**
  (every `getPage` caller — PageView/PageEditor — interpolates raw against the `/pages/*` wildcard); the diff
  merely exposes it to arbitrary template filenames. Fixing it touches the shared read path for ALL pages and
  belongs in a focused, separately-scoped change (per-segment encoding vs the server route's slash handling),
  not smuggled into this phase. Practically low-risk (template files are normally simple names). **Forked up to
  the architect as a note, not silently dropped.**

### anvil-adversarial-tester — `/tmp/anvil-spawn-out/27-ADVERSARIAL-TEST.md` (HOLDS — 0 defects)
- 12 adversarial tests incl. a 39-case boundary table + `FuzzTemplateMerge` (~279k execs, 0 crashes) +
  path-safety, all pass. The worst any input makes the merge do is fail closed with `ErrTemplateInvalid` (400);
  could not produce a panic/hang/two-block/duplicate-key/corrupt page, escape `Templates/`, or make the user
  title lose. Confirmed no executable-template / code-execution surface (static `okf.Parse`/`Emit` over text).
  Two initial "failures" were test-premise errors (bare `{{x}}` in unquoted FM is YAML flow-mapping → correctly
  rejected; absolute `folder` is trimmed within-root) — both pin correct behavior.
- **Durable suite MERGED** into `internal/pages/templates_adversarial_test.go` (test-only, no production change),
  passes on main.

## Verification (final, on merged HEAD)
- Backend: `go build ./...` ✓, `go vet ./...` ✓, `go test ./...` ✓ (all packages, incl. merged adversarial suite + fuzz target).
- Frontend (`web/`): `npx tsc -b` ✓, `npx vitest run` ✓ **774 passed / 63 files**, `npm run build` ✓ clean.
- No regression in the existing create flow / pages / okf / repo. `internal/web/dist/.gitkeep` restored (vite emptyOutDir); no dist artifacts committed.

## Commits (main)
- `4336980` feat(server): page templates — list + template-seeded create (ED-06a)
- `03adabb` feat(web): template picker + client-side placeholder substitution (ED-06a)
- `432486c` fix(web): substitute {{title}} in template body only, not frontmatter (ED-06a)
- `1e4a7c0` + `81a0ef8` (merge): adversarial test suite (test-only)

## Handoff (to qa via the architect)
```yaml
handoff:
  record_author: engineer
  accountable_owner: engineer
  from: engineer
  to: qa
  phase: "27"
  requirements: [ED-06a]
  intent: "verify page templates: list, blank-default picker, client {{title}}/{{date}}/{{time}} substitution (title→body only), server okf robust merge (single valid block, user title wins, fail-closed on dup-key/malformed), repo.Resolve-gated Templates/ subtree, reuse of the single-writer create pipeline, no regression in blank create"
  artifacts:
    - internal/pages/templates.go
    - internal/pages/templates_test.go
    - internal/pages/templates_adversarial_test.go
    - internal/okf/okf.go (ValidateFrontmatter)
    - internal/server/handlers_pages.go
    - internal/server/handlers_templates_test.go
    - internal/server/router.go
    - web/src/lib/templates.ts
    - web/src/components/CreatePageModal.tsx
    - web/src/api/client.ts
  open_forks:
    - "MINOR (pre-existing, app-wide): getPage does not encodeURIComponent the path; template filenames with #/?/% mis-route. Out of phase scope; recommend a focused getPage-encoding change milestone-wide."
  status: sent
```
