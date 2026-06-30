---
author: qa
responsible: qa
phase: "27"
---

# Phase 27: Page Templates (ED-06a) — VERIFICATION

**Plan file:** `.planning/phases/27-page-templates/27-01-PLAN.md`
**Context file:** `.planning/phases/27-page-templates/27-CONTEXT.md`
**SUMMARY file:** `.planning/phases/27-page-templates/27-01-PLAN-SUMMARY.md` (claims checked against code reality, not trusted)
**Requirements:** ED-06a
**Commits verified (on `main`):** `4336980` (server) · `03adabb` (web) · `432486c` (web {{title}}-body-only fix) · `1e4a7c0` + `81a0ef8` (merged adversarial suite, test-only)
**Substrate note:** SMTC MCP disconnected for this run — evidence gathered via Bash/Read/grep + live execution (go build/vet/test, tsc/vitest/build). Adequate for a Go + TS phase; the load-bearing YAML-merge invariant is proven by an on-disk re-parse + strict-decode oracle in a passing adversarial table + fuzz target, not by my reasoning alone.

## Verdict

```yaml
gate_result:
  record_author: qa
  id: GR-27-verification
  phase: "27"
  verdict: pass
  basis: >
    All seven must-have truths hold against the real code (read, not trusted from the SUMMARY) and
    full re-run evidence (go build/vet/test exit 0 across every package incl. a fresh -count=1 run of
    server/pages/okf/repo; tsc 0; vitest 774/774 across 63 files; web build emitted assets, "built in
    537ms"). Templates are flat Templates/*.md enumerated by ListTemplates via repo.Tree() with a
    prefix+nesting+.md filter and a defense-in-depth repo.Resolve gate (templates.go:42-76); GET
    /templates is registered in the any-authed read group (router.go:177) and handleListTemplates
    returns {templates:[{name,path}]} (handlers_pages.go:182-197). The picker defaults to Blank, which
    routes to the UNCHANGED title-only pages.Create (handler else-branch handlers_pages.go:224;
    Create body service.go:135-170 not in the diff). Client substitution is a pure String.replaceAll +
    Intl.DateTimeFormat over exactly {{title}}/{{date}}/{{time}} (templates.ts:47-58), no eval, other
    {{…}} left literal. The code-review MAJOR fix is present and correct: {{title}} is substituted in
    the BODY only (CreatePageModal.tsx:50-51 passes title for body, omits it for frontmatter), the
    server's okf.SetField owns the canonical title: and yaml-quotes a colon-bearing title via Emit
    (repair.go:124-148) — so "Meeting: Kickoff" can never turn title: {{title}} into invalid YAML.
    THE load-bearing robust merge is server-side via okf: CreateFromTemplate parses the assembled seed,
    runs okf.ValidateFrontmatter (strict map[string]any decode rejecting duplicate keys + malformed
    YAML → ErrTemplateInvalid, fail-closed BEFORE the mutation lock), then SetField(title) (user wins)
    + Repair → Emit → the SAME single-writer pipeline as Create (lockMutation, uniquePath from the user
    title, enqueueWrite "create", index+graph upsert) — templates.go:99-140. The mandatory adversarial
    merge test + FuzzTemplateMerge exist and exercise own-frontmatter→single block, conflicting/
    duplicate title:/tags:/arbitrary keys (fail closed), malformed YAML, body-only, CRLF, literal
    {{…}}, body --- fences, huge values (200k scalar, 5000 keys), and a YAML alias bomb (≤15s, no hang)
    — with an on-disk oracle asserting exactly one fence, re-parse, strict-decode, user title wins, all
    five required fields. Path safety: ListTemplates + the template-read path resolve through the
    fuzz-tested repo.Resolve constrained to Templates/ (escape/NUL/traversal rejected); there is no
    template-PATH parameter to CreateFromTemplate — only substituted CONTENT — so a non-template path
    cannot be seeded and the write path derives solely from the user title (TitleControlsPathNotSeed).
    No executable surface (static okf.Parse/Emit over text; template bytes are normal sanitized page
    content). No regression: blank create unchanged, full pages/okf/repo/server suite green. Production
    changes are confined to the 4 planned Go files + 3 web files; the rest are tests.
  remediation_owner: null
  evidence:
    - "go build ./... → exit 0; go vet ./... → exit 0"
    - "go test ./... → exit 0 (all 18 packages ok); fresh go test -count=1 ./internal/server ./internal/pages ./internal/okf ./internal/repo → exit 0 (server 7.40s, pages 11.76s); go vet ./internal/server → exit 0 (test pkg compiles clean — NO DuplicateDecl)"
    - "web: npx tsc -b → exit 0; npx vitest run → 774 passed / 63 files, 0 fail (templates.test 8, CreatePageModal.test present); npm run build → assets emitted, 'built in 537ms', only pre-existing INEFFECTIVE_DYNAMIC_IMPORT warnings (Phase-25 markdown→html→css chain)"
    - "git diff --stat 4336980^..81a0ef8 → 13 files, +1501/-11; production (non-test) Go = okf.go, pages/templates.go, server/handlers_pages.go, server/router.go; web prod = client.ts, CreatePageModal.tsx, templates.ts"
    - "adversarial: TestCreateFromTemplate_AdversarialMergeTable (39 cases) + FuzzTemplateMerge + BillionLaughsNoHang + HugeFrontmatterManyKeys + ListTemplates_NeverEscapesTemplatesDir + Resolve_TemplateSurfaceTraversal + TitleControlsPathNotSeed + MaliciousFolderNeverEscapes — all green"
```

## Per-truth verdicts

| # | Must-have truth | Verdict | Key evidence (file:line) |
|---|---|---|---|
| 1 | Templates = flat Templates/*.md; `ListTemplates` enumerates; `GET /templates` registered + reachable; picker offers them, Blank default; Blank uses unchanged title-only `pages.Create` | **PASS** | `ListTemplates` enumerates via `repo.Tree()`, prefix+nesting+`.md` filter + `repo.Resolve` gate (templates.go:42-76). Route `authed.Get("/templates", h.handleListTemplates)` (router.go:177); handler → `{templates:[{name,path}]}` 200 (handlers_pages.go:182-197). Modal `<select>` default Blank (CreatePageModal.tsx:26,34); Blank → `createPage(folder,t)` no seed (`:54`) → handler else → `h.pages.Create` UNCHANGED (handlers_pages.go:224; Create service.go:135-170 not in diff). Tests: templates_test.go, handlers_templates_test.go, CreatePageModal.test.tsx |
| 2 | Template seeds frontmatter + body; {{title}}/{{date}}/{{time}} substituted CLIENT-side via `replaceAll` + `Intl.DateTimeFormat` (user-local); only 3 placeholders; other {{…}} literal; no code execution. MAJOR fix: {{title}} in BODY only; colon title → valid quoted YAML, not 400 | **PASS** | `substitutePlaceholders` pure `replaceAll` over exactly the 3 placeholders, `Intl.DateTimeFormat` user-local (YYYY-MM-DD / HH:mm h23), title only when supplied (templates.ts:47-58). Modal: frontmatter `{ now }` (no title), body `{ title: t, now }` (CreatePageModal.tsx:50-51). Server `SetField` sets `!!str` scalar → Emit yaml-quotes colon titles (repair.go:124-148). Colon-title tests: templates.test.ts:89, CreatePageModal.test.tsx:82-101 (frontmatter has NO raw title; body verbatim) |
| 3 | **Robust merge SERVER via okf** — own `---` frontmatter → ONE valid block (no double `---`/dup keys/malformed YAML), user title wins, then `okf.Repair`; `ValidateFrontmatter` fails closed (400/ErrTemplateInvalid); adversarial test + FuzzTemplateMerge exist | **PASS** | `CreateFromTemplate`: `okf.Parse` → `ValidateFrontmatter` (BEFORE lock, fail-fast) → `SetField(title)` user-wins → `Repair` → `Emit` (templates.go:99-140). `ValidateFrontmatter` = strict `map[string]any` decode rejecting dup keys + malformed YAML (okf.go:107-113); `ErrTemplateInvalid` → 400 (handlers_pages.go:231-233). Adversarial table 39 cases + `FuzzTemplateMerge` (templates_adversarial_test.go:119-362) cover own-fm, dup title/tags/arbitrary→reject, malformed→reject, body-only, CRLF, literal {{…}}, body `---` fences, huge values, alias-bomb; on-disk oracle proves single fence + re-parse + strict-decode + title-wins + 5 required fields (`:75-113`). All green |
| 4 | Template reads resolve through `repo.Resolve` constrained to Templates/; escape/NUL/traversal rejected (ErrUnsafePath); non-template path can't be seeded | **PASS** | `ListTemplates` re-resolves every candidate via `repo.Resolve`, drops anything that fails (templates.go:63-69). `TestResolve_TemplateSurfaceTraversal` (`:428-457`) rejects 14 escape shapes + NUL with `ErrUnsafePath`. No template-PATH arg to `CreateFromTemplate` — only CONTENT — so `TestCreateFromTemplate_TitleControlsPathNotSeed` (`:463-479`) proves path derives from user title only; `MaliciousFolderNeverEscapes` (`:486-511`) |
| 5 | Template-seeded create reuses the EXISTING single-writer pipeline (uniquePath, enqueueWrite "create", index+graph upsert, audit); not a forked write path; path derives from user title | **PASS** | `CreateFromTemplate` mirrors `Create`: `lockMutation()` (templates.go:118), `uniquePath(folder,title)` (`:120`), `enqueueWrite(ctx,path,bytesOut,"create",user)` (`:134`), `enqueueIndexUpsert`/`enqueueGraphUpsert` (`:137-138`) — identical primitives to Create (service.go:145-168). Audit `ActionPageCreate` shared in handler (handlers_pages.go:238). Path from user title via uniquePath (proven on-disk: `runbooks/normal-title.md`, adversarial `:472`) |
| 6 | Backend + frontend; no executable/JS templates (static text + 3 placeholders); template bytes flow through the existing sanitized render path | **PASS** | Client substitution is pure string replace, no eval (templates.ts:47-58); server merge is static `okf.Parse`/`Emit` over text (templates.go:108-129). Only 3 placeholders; any other `{{…}}` persisted verbatim (`TestCreateFromTemplate_PlaceholdersStoredLiterally` `:195`). Templates are normal pages → read via the existing sanitized PageView render path. Adversarial suite confirms worst case is fail-closed, never a code-execution surface |
| 7 | No regression: blank/title-only create still works; existing pages/okf/repo tests stay green | **PASS** | Blank branch unchanged (handlers_pages.go:224 → `Create`; Create body untouched). Fresh `go test -count=1 ./internal/server ./internal/pages ./internal/okf ./internal/repo` → exit 0; full `go test ./...` exit 0; web vitest 774/774. Production diff confined to 4 Go + 3 web files |

## Build / test evidence (re-run by qa)

```
# backend (repo root)
$ go build ./...   → exit 0
$ go vet ./...     → exit 0
$ go test ./...    → exit 0 (all 18 packages: ok; cmd/agent/attachments/audit/auth/config/
                     gitstore/graph/jobs/locks/okf/pages/repo/search/server/store/tagsweep/users)
$ go test -count=1 ./internal/server ./internal/pages ./internal/okf ./internal/repo
                   → exit 0 (server 7.396s, pages 11.758s, okf 0.004s, repo 0.004s) — FRESH, not cached
$ go vet ./internal/server  → exit 0  (test package compiles clean)

# frontend (web/)
$ npx tsc -b       → exit 0 (clean)
$ npx vitest run   → 63 files passed, 774 tests passed, 0 fail
                     (templates.test 8, CreatePageModal.test present, plus the merged frontend suite)
$ npm run build    → assets emitted (index-CWVTa5nw.js 1,184.16 kB; mermaid renderers lazy),
                     "✓ built in 537ms"; only pre-existing [INEFFECTIVE_DYNAMIC_IMPORT] warnings
                     (the Phase-25 markdown→html→css/js dynamic-import chain) — warnings, not errors

$ git diff --stat 4336980^..81a0ef8  → 13 files, +1501 / -11
                     production: okf.go, pages/templates.go, server/handlers_pages.go,
                     server/router.go, web client.ts/CreatePageModal.tsx/templates.ts; rest tests
```

## Stale-diagnostics re-confirmation (explicit, at HEAD)

The harness reported stale diagnostics during the adversarial-test merge (DuplicateDecl for
`indexOf`/`contains`, `handleListTemplates` unused). **Independently re-confirmed RESOLVED at HEAD:**

- **Build:** `go build ./...` exit 0 — no DuplicateDecl, no unused symbol.
- **Test-package compile:** fresh `go test -count=1 ./internal/server ./internal/pages` exit 0 and
  `go vet ./internal/server` exit 0 — the `internal/server` and `internal/pages` *test* packages
  compile and run green; a DuplicateDecl would fail compilation. No such failure.
- **Route wired:** `handleListTemplates` is registered at `internal/server/router.go:177`
  (`authed.Get("/templates", h.handleListTemplates)`) and reachable — not unused.

The stale diagnostics were a transient mid-merge artifact; HEAD is clean.

## Robust-merge ruling (truth #3 — the load-bearing judgment)

**RULING: the server-side okf merge satisfies ED-06a's load-bearing requirement. The merge is
correct, fails closed, and is proven by a passing on-disk re-parse + strict-decode oracle over a
39-case adversarial table + fuzz target. PASS truth #3.**

- **The merge is genuinely server-side in the canonical okf engine** (D-04), not a client string
  merge. `CreateFromTemplate` (templates.go:108-129) parses the assembled seed, validates, sets the
  canonical title, repairs, and emits — all in Go's yaml.Node engine.
- **The duplicate-key hazard the engineer surfaced is real and correctly closed.** okf's yaml.Node
  model is *faithful* — it does NOT dedup duplicate frontmatter keys, and `SetField` only fixes the
  first occurrence, so a duplicate-key template would otherwise Emit a corrupt two-`title:` page. The
  engineer added `okf.ValidateFrontmatter` (okf.go:107-113), a strict `map[string]any` decode that
  rejects duplicate keys + malformed YAML, run **before the mutation lock** so it fails fast with
  `ErrTemplateInvalid` → 400. This sits inside the plan's own verify contract ("clean result OR clean
  error, never a corrupt two-block page") and keeps YAML confined to okf.
- **The fail-closed behaviour is empirically proven, not asserted.** The adversarial table's
  `wantReject` rows (duplicate title / tags / arbitrary key, malformed flow seq, sequence/scalar root,
  tab-indent, unresolved alias — templates_adversarial_test.go:160-169) all assert `ErrTemplateInvalid`
  and PASS in the green run. The success rows re-read the page off disk and prove via
  `assertSingleValidBlock` (`:75-113`): exactly one opening fence, re-parses, strict-decodes with no
  error (⇒ no dup keys / two-block / malformed), user title wins, all five required fields present.
- **Totality holds.** `FuzzTemplateMerge` (`:321-362`) drives arbitrary seeds with the totality
  invariant (every input either fails closed with `ErrTemplateInvalid` or Emits a valid single block);
  `BillionLaughsNoHang` bounds an alias bomb to ≤15s; `HugeFrontmatterManyKeys` (5000 keys) and a
  200k-char scalar pass. The worst any input makes the merge do is fail closed.

No hole. The load-bearing risk is discharged.

## Gaps routed UP

**None.** All seven truths PASS; the robust-merge ruling clears. No code change required for ED-06a.

## Milestone note (NON-gap — routed up, pre-existing, do NOT block Phase 27)

`web/src/api/client.ts` `getPage(path)` (client.ts:310-311) interpolates `path` RAW into
`/api/v1/pages/${path}` with **no `encodeURIComponent`**. A template filename containing `#`, `?`, or
`%` would mis-route when the picker reads it via `getPage(templatePath)` (CreatePageModal.tsx:43).

**Assessment — this does NOT break ED-06a's acceptance:**
- It is a **pre-existing, app-wide convention**, not introduced by Phase 27. Every `getPage` caller
  interpolates the raw path against the `/pages/*` wildcard — PageView.tsx:45 and PageEditor.tsx (8
  call sites: :80,:298,:331,:442,:555,:570,:582,:634). CreatePageModal merely follows the same pattern.
- **Phase 27's happy path works.** Normal template filenames (alphanumeric, spaces, hyphens — e.g.
  `Daily Note.md`, `Meeting.md`) route correctly (`fetch` percent-encodes spaces). Only exotic
  filenames with `#`/`?`/`%` mis-route — outside the ED-06a happy path.
- **Recommendation:** a focused, separately-scoped milestone-wide fix to the shared read path
  (per-segment encoding vs the server route's slash handling) — not smuggled into ED-06a. Forked up to
  the architect, consistent with the engineer's SUMMARY open-fork.

---

*Verified 2026-07-01 by qa. Goal-backward against commit range `4336980^..81a0ef8` + code reality; SMTC unavailable, evidence via Bash/Read + live go build/vet/test + tsc/vitest/build + git diff scope.*
