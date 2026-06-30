---
author: engineer
responsible: engineer
phase: "24"
status: complete
milestone: v1.4
reconstructed_at_close: true
---

# Phase 24: LLM-Generated Description — SUMMARY

**Plan file:** `.planning/phases/24-llm-generated-description/24-01-PLAN.md`
**Requirements:** ED-01a, ED-01b
**Commits (on `main`):**
- `14ed93e` feat(agent): add GenerateDescription single-shot mode (ED-01b)
- `e58f071` feat(server): add generate-description + apply-description endpoints (ED-01b)
- `b4338a4` feat(web): add generateDescription + applyDescription API client (ED-01b)
- `7773904` feat(web): add DescriptionGenerate review surface (ED-01b)
- `b634050` feat(web): wire description generator into PageEditor + merge-first reconcile (ED-01a/ED-01b)

> **Provenance note.** No SUMMARY was authored during execution; this record was
> reconstructed at the verification gate from the five committed implementation commits
> (range `14ed93e^..b634050`) and checked against the qa `24-VERIFICATION.md` (verdict
> PASS, all 10 truths). The execution-time adversarial gates (`anvil-code-reviewer` /
> `anvil-adversarial-tester`) have **no recorded provenance** for this phase — the
> independent qa pass is the gate of record here (see Gaps).

## What was built

Replaced the manual Description field with an on-demand, never-auto agent generator:
**generate (read-only) → review/edit/regenerate → approve (server re-validates, writes
byte-stably, revision-checked) → reconcile (autosave-safe, merge-first)**. Backend +
frontend. Reuses — and generalizes — the Phase-23 apply→reconcile pattern, and closes its
two carried caveats (D-08a/D-08b).

### Backend (Go)
- **`internal/agent/describe.go` (new)** — `GenerateDescription(ctx, path) (text, baseRev, err)`:
  a single-shot ChatModel MODE (NOT a tool — the read-only 5-tool allow-list is unchanged),
  mirroring `SuggestTags`. `Pages.Get` → capture `baseRev = Pages.Revision(path)` at generate
  time (echoed to apply → a moved page 409s, never clobbers) → `truncateForBudget(body)` →
  `buildDescribeMessages` → `generateOnce` (`describeMaxTokens=256`, `describeTemperature=0.2`)
  → **validate-and-retry** (1 + 2 retries; fail-closed with `ErrDescriptionInvalid`). Never
  writes.
  - **`ValidateDescription(text) (string, error)` (exported, shared)** — the ONE validator
    used both as the generate loop's output gate AND as the apply handler's server-side
    re-validation: trim; reject empty; reject NUL + any control char (newline/CR/tab → enforces
    a single plain-text line); collapse internal whitespace; cap at `maxDescriptionLen=200`
    runes (prompt asks ~160 soft). Returns the CLEANED text.
- **`internal/agent/prompts.go`** — `buildDescribeMessages(body, attempt)`: instructs ONE
  plain-text meta sentence, no markdown / no surrounding quotes / no newlines, ≤ ~160 chars;
  the page body is supplied inline as untrusted DATA.
- **`internal/pages/batch.go`** — `SetFieldFrontmatter(frontmatter, body, field, value)`:
  the scalar-field analog of `SetTagsFrontmatter`, the ONE place the byte-stable scalar-region
  write is owned (assemble → `okf.Parse` → `okf.SetField` → emit → re-parse → return
  `RawFront`). Body + every other key untouched; cannot drift from the tags-region write.
- **`internal/server/handlers_agent.go`** —
  - `handleGenerateDescription`: POST `{page_path}`, **any-authed**, read-only; audits the mode;
    returns `{page_path, description, base_revision}`; 404 on missing page; fail-closed on
    agent-off/error (mirrors `handleSuggestTags`).
  - `handleApplyDescription`: POST `{page_path, description, base_revision}`, **editor + CSRF**;
    **re-validates via `agent.ValidateDescription` and writes the CLEANED value, never
    `req.Description`** (422 on invalid — the client value is never trusted); `pages.Get` →
    `setDescriptionFrontmatter` (delegates to `SetFieldFrontmatter`) → `pages.Save(...,
    base_revision, ...)` (the same single-writer commit + 409 floor as apply-tags;
    `ErrStaleRevision` → 409, writes nothing); audits the approval with **role only, never the
    text or content**; returns 204.
- **`internal/server/router.go`** — `generate-description` registered in the authed group;
  `apply-description` in the editor + CSRF subgroup (alongside `apply-tags`).
- **`internal/okf/repair.go` + `internal/okf/charset.go` — ZERO diff** (ED-01a relies on
  `okf.Repair` being unchanged: it only ADDS a missing `description` key, never overwrites, so
  an existing human description is preserved). The validator landed in `agent/describe.go` and
  the byte-stable write in `pages/batch.go` — a cleaner placement than the plan's tentative
  `okf/charset.go`, and it keeps Repair untouched.

### Frontend (React/TS)
- **`web/src/api/client.ts`** — `generateDescription(pagePath)` (POST
  `/agent/generate-description`) and `applyDescription({page_path, description, base_revision})`
  (POST `/agent/apply-description`, 204; CSRF via `mutate`), mirroring `suggestTags`/`applyTags`.
- **`web/src/components/DescriptionGenerate.tsx` (new, +css, +test)** — editor-gated (a reader
  renders nothing). A "Generate description" button → `generateDescription` → opens a review
  surface with the draft in an **editable `<textarea>`** + **Regenerate** (re-fetches, new
  `base_revision`) / **Apply** / **Cancel**. Apply sends the EDITED text + `base_revision`; on
  204 → `onApplied(editedText)` + close; on 409 → stale "changed since… regenerate" state
  (does not call `onApplied`); other errors → recoverable line. **Never calls generate on
  mount.**
- **`web/src/routes/PageEditor.tsx`** —
  - **ED-01a:** deleted the `<input id="fm-description">` block + label.
  - **D-05 generalize:** factored the Phase-23 reconcile into a shared, merge-first
    `reconcileApplied(merge, refreshErrorMsg)`. It **merges the applied region into LOCAL
    frontmatter FIRST** (apply is authoritative — a transient `getPage` failure can't strand
    or revert the value; a later conflict-Overwrite then preserves it), THEN advances
    `baseRevision`/`lastSavedFrontmatter` from `getPage`, and schedules ONE convergence
    autosave only on divergence (no 409, no revert). `reconcileAppliedTags` (`setTags`+`union`)
    and `reconcileAppliedDescription` (`setField(cur, "description", text)`) both delegate to it.
  - Renders `<DescriptionGenerate pagePath={path} onApplied={reconcileAppliedDescription} />`
    near the title/tags area.
  - **D-08a (own-apply 409 copy):** `reconcileApplied` stamps `lastApplyAt.current = Date.now()`;
    a save 409 within the own-apply window is attributed accurately ("Your change overlapped
    with a save in progress") instead of the misleading "changed somewhere else".
  - **D-08b:** the body refs (`bodyRef`/`lastSavedBody`) and the `runSaver`
    single-flight/lock/409 logic are untouched by reconcile.

## Verification (independent qa — `24-VERIFICATION.md`, verdict PASS)

- All 10 must-have truths PASS against the real code (goal-backward, read not trusted).
  D-08a and D-08b both **closed**.
- `go build ./...`, `go vet`, `go test ./...` — all exit 0 (~12.6s); fresh `-count=1` agent +
  server `ok` with the new `TestValidateDescription`, `TestGenerateDescription`,
  `TestApplyDescription*` (RBAC / MissingCSRF / StaleRevision-409 / RevalidatesServerSide-422),
  `TestGenerateDescriptionEndpoint`.
- From `web/`: `npx vitest run` → **604/604 across 55 files** (DescriptionGenerate 8,
  PageEditor 19, `PageEditor.reconcile.adversarial` 9); `npm run build` (`tsc -b && vite`)
  clean. `internal/web/dist/.gitkeep` restored after vite's `emptyOutDir` delete (never
  committed).
- `git diff --stat …repair.go charset.go` → EMPTY (Repair unchanged — ED-01a).

## Hoare-triple summary

- **Q (postcondition):** after generate→approve→reconcile, the page's `description` scalar
  equals the server-validated CLEANED text (single line, ≤200 runes), written byte-stably (body
  + every other frontmatter key unchanged); the value is merged into LOCAL frontmatter,
  `baseRevision` equals the post-apply server revision, and the next body autosave saves cleanly
  (no 409, no revert). No description is ever written without an explicit approve; a moved page
  409s; a server-invalid text 422s. The manual input no longer exists; an existing description
  is preserved.
- **S (code shape):** `GenerateDescription` validate-and-retries a single-shot reply
  (`ValidateDescription`); `handleApplyDescription` re-validates the client value, writes the
  cleaned text via `SetFieldFrontmatter`/`okf.SetField` through revision-checked `pages.Save`;
  `reconcileApplied` merges into `frontmatterRef` first, then advances the revision and converges
  on divergence.
- **P (precondition):** apply committed server-side (204) so the description is authoritative;
  `setField` rewrites only the `description` region; `getPage` returns the fresh revision (or
  fails → handled, value kept locally).
- **S establishes Q:** server re-validation makes the written value trusted regardless of the
  client; merge-first makes the approved value survive a refresh failure (and a later Overwrite);
  advancing `baseRevision := fresh.revision` blocks the self-409; the `merged !== fresh` guard
  makes the no-unsaved-edits case a clean no-op (terminates — no thrash).

## Gaps / follow-ups

- **Execution-time adversarial-gate provenance is unknown** (no SUMMARY was written during
  execution). The independent qa verification (PASS, all truths, full suites green) is the gate
  of record. If the project requires the code-reviewer/adversarial-tester gate trail per phase,
  spawn them retroactively against this diff; otherwise the qa pass stands.
- Non-blocking (qa FYI): the validator + byte-stable write landed in `agent/describe.go` +
  `pages/batch.go` rather than the plan's tentative `okf/charset.go` — a clean placement worth a
  codebase-map note.
- Out of scope (deferred Future, per CONTEXT): surfacing the description in search cards / graph
  hover.

```yaml
handoff:
  record_author: engineer
  accountable_owner: engineer
  id: H-24-execute
  from: engineer
  to: architect
  artifact: .planning/phases/24-llm-generated-description/24-01-PLAN-SUMMARY.md
  intent: "Phase 24 execution complete — removed the manual Description input (ED-01a) and added an on-demand, never-auto agent generator (ED-01b): GenerateDescription single-shot read-only mode + generate-description (any-authed) / apply-description (editor+CSRF, server re-validates via the shared ValidateDescription, byte-stable okf.SetField, revision-checked 409) endpoints, a DescriptionGenerate review surface (editable + regenerable), and an autosave-safe merge-first reconcile generalized from Phase 23 (reconcileApplied shared by tags + description). Phase-23 caveats closed: D-08a own-apply-409 copy attributed accurately, D-08b in-flight-autosave interleave test added. okf.Repair untouched. SUMMARY reconstructed at close; qa VERIFICATION.md = PASS (all 10 truths)."
  status: ready
```

```yaml
evidence:
  record_author: engineer
  evidence_producer: qa
  accountable_owner: engineer
  id: E-24-verification
  subject_ref: "24-VERIFICATION.md (go build/vet/test ./... + web vitest/build)"
  kind: test_result
  issuance:
    kind: producer_signed
  warrant:
    kind: external_cited
    scope:
      - "carry=all 10 must-have truths PASS (goal-backward, code-read not trusted)"
      - "carry=go build/vet/test ./... exit 0 (~12.6s); new agent+server describe tests ok"
      - "carry=web 604/604 across 55 files (DescriptionGenerate 8, PageEditor 19, adversarial 9); npm run build clean"
      - "carry=D-08a own-apply-409 copy + D-08b interleave test both closed"
      - "carry=okf/repair.go + okf/charset.go zero diff (ED-01a)"
    rationale: "independent qa goal-backward verification against the committed implementation; full backend + frontend suites re-run, not trusted from a SUMMARY"
  freshness:
    status: fresh
    checked_against: HEAD
```
