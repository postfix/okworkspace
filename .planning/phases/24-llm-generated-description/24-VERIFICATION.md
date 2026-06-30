---
author: qa
responsible: qa
phase: "24"
---

# Phase 24: LLM-Generated Description (ED-01a / ED-01b) — VERIFICATION

**Plan file:** `.planning/phases/24-llm-generated-description/24-01-PLAN.md`
**Context file:** `.planning/phases/24-llm-generated-description/24-CONTEXT.md`
**SUMMARY file:** none authored for this phase — verified goal-backward against the
**commit range + code reality** (the task's explicit instruction), not against a SUMMARY's claims.
**Requirements:** ED-01a, ED-01b
**Commits verified (on `main`):** `14ed93e` `e58f071` `b4338a4` `7773904` `b634050`

## Verdict

```yaml
gate_result:
  record_author: qa
  id: GR-24-verification
  phase: "24"
  verdict: pass
  basis: >
    All ten must-have truths hold against the real code (read, not trusted from a SUMMARY —
    none exists) and full re-run evidence. (1) The manual Description <input id="fm-description">
    block is DELETED from PageEditor.tsx (replaced by an ED-01a comment ~791) and the `description`
    frontmatter key is preserved — okf.Repair (repair.go) and okf/charset.go have ZERO diff in the
    range, and the body-autosave test asserts an existing `description: Existing blurb` survives.
    (2) agent.GenerateDescription (describe.go:102-146) is single-shot (generateOnce per attempt,
    NOT a tool/ReAct loop), read-only (no Save), returns (text, baseRev, err) capturing baseRev at
    generate time; SMTC confirms its ONLY non-test caller is handleGenerateDescription
    (handlers_agent.go:873) — it never auto-runs. (3) The prompt asks for ONE plain-text sentence
    ~160 chars (prompts.go:303-322); ValidateDescription enforces a 200-rune hard cap (slack over
    the 160 soft target); the review surface is an EDITABLE textarea with Regenerate
    (DescriptionGenerate.tsx:292-298, 93-97). (4) /agent/generate-description is registered in the
    any-authed group (router.go:218); /agent/apply-description in the editor subgroup
    (router.go:249) under RequireEditor + the global nosurf CSRF — proven by TestApplyDescriptionRBAC
    (reader 403) + TestApplyDescriptionMissingCSRF. (5) Approve writes ONLY the `description` scalar
    byte-stably via setDescriptionFrontmatter → pages.SetFieldFrontmatter → okf.SetField
    (batch.go:251, repair.go:124 unchanged), through pages.Save with the echoed base_revision →
    ErrStaleRevision → 409 (handlers_agent.go:955-964; TestApplyDescriptionStaleRevision). (6) The
    handler RE-validates the client text with agent.ValidateDescription and writes the CLEANED value,
    never the client value verbatim → 422 on invalid (handlers_agent.go:926-930;
    TestApplyDescriptionRevalidatesServerSide). (7+8) reconcileAppliedDescription routes through the
    shared reconcileApplied (PageEditor.tsx:417-455, 472-478) which MERGES FIRST
    (setField(cur,"description",text) → frontmatterRef) BEFORE the getPage refresh, advances
    baseRevision to the fresh revision, and converges via one autosave only on divergence — so apply
    → keep typing → next autosave saves at the post-apply revision, does NOT 409 and does NOT revert
    (pinned by the post-apply-revision test asserting base_revision rev-2 + description present + no
    conflict dialog); a transient getPage failure keeps the merged value locally (recoverable copy).
    (9) Nothing writes without explicit approval: generate fires only on the button click
    (DescriptionGenerate.tsx:112), apply only on Apply (mutate(draft), :90), reconcile only via
    onApplied on a 2xx — pinned at both component and PageEditor level by "mounting issues no
    generate/apply call". (10) D-08b CLOSED — the targeted apply-vs-in-flight-autosave interleave
    Phase 23 lacked now exists: it holds a body save in flight, lands the apply (advancing the
    revision), 409s the released body save ON THE OWN APPLY, asserts the data-safe Overwrite
    preserves the description AND the D-08a accurate copy ("Your change overlapped with a save in
    progress", NOT "changed somewhere else") via the self-expiring fromOwnApply window
    (OWN_APPLY_409_WINDOW_MS, PageEditor.tsx:57/145/291/744-752). Evidence: go build/vet exit 0; go
    test ./... all packages ok (incl. fresh agent+server with the new TestValidateDescription /
    TestGenerateDescription / TestApplyDescription* / TestGenerateDescriptionEndpoint); web
    `npx vitest run` 604/604 across 55 files (DescriptionGenerate.test 8, PageEditor.test 19,
    adversarial 9); `npm run build` (tsc -b + vite) clean exit 0.
  remediation_owner: null
  tiers:
    evidence: smtc_honest
    reasoning: manual_review
  provenance:
    - "Read the implementation, not a SUMMARY (none exists): describe.go (full), prompts.go:291-322, handlers_agent.go:840-990, internal/pages/batch.go:242-266 (SetFieldFrontmatter), router.go:198-251, PageEditor.tsx (reconcileApplied 412-478, runSaver 409 attribution 283-310, conflict resolution 560-615, DiffReviewDialog copy 736-757, input-removal region 791-805, wiring 831-839), DescriptionGenerate.tsx (full)"
    - "SMTC: GenerateDescription has exactly ONE non-test call site — handlers_agent.go:873 (find_name_occurrences, calltarget) → proves 'never auto-runs' server-side; setDescriptionFrontmatter (find_declarations) delegates to pages.SetFieldFrontmatter → okf.SetField (byte-stable scalar write)"
    - "Scope check: `git diff --stat 14ed93e~1..b634050 -- internal/okf/repair.go internal/okf/charset.go` EMPTY → okf.Repair unchanged (ED-01a relies on it); the validator landed in agent/describe.go (ValidateDescription) + pages/batch.go (SetFieldFrontmatter), a clean relocation from the plan's tentative okf/charset.go, not a gap"
    - "Backend tests fresh (-count=1): `go test ./internal/agent/ ./internal/server/` both ok; TestValidateDescription, TestGenerateDescription (over-cap/empty rejected+retried 3 calls fail-closed, disabled→ErrAgentDisabled), TestApplyDescriptionStaleRevision (409), TestApplyDescriptionRevalidatesServerSide, TestApplyDescriptionRBAC (reader 403), TestApplyDescriptionMissingCSRF, TestGenerateDescriptionEndpoint all PASS"
    - "Web (from web/): `npx vitest run` 604 passed (604) / 55 files; DescriptionGenerate.test.tsx 8/8; PageEditor.test.tsx 19/19 (incl. the 4 new ED-01a/b + D-08a/b tests); PageEditor.reconcile.adversarial.test.tsx 9/9. `npm run build` clean exit 0 (>500kB chunk warning pre-existing). internal/web/dist/.gitkeep restored after vite emptyOutDir."
```

## Verification method

Goal-backward verification from the ten `must_haves.truths` and the `<success_criteria>` to the
real generate → review → approve → reconcile path, reading the shipped code (no SUMMARY exists for
this phase — verified directly against the commit range, as instructed) and re-running the full
build/test suites as independent evidence. ED-01a/ED-01b is a mixed Go + TS/React phase; SMTC's
LSP Decision tier is not load-bearing for these correctness truths, so the strongest available
grounding is SMTC structural reads + caller proof + direct code reading + test/build execution +
git `--stat` scope. Evidence tier is therefore `smtc_honest` (sound, re-run, caller-proven), not a
Decision-grade cert.

---

## 1. Verification acceptance handshake

| Check | Status |
|-------|--------|
| Inputs present (CONTEXT, PLAN, REQUIREMENTS §ED-01a/§ED-01b) | PASS |
| SUMMARY present | N/A — none authored; task directs verification against the commit range + code reality (acceptable: code is the source of truth and the commits resolve) |
| PLAN ↔ code consistency (claimed symbols exist + do what's described) | PASS — GenerateDescription, ValidateDescription, handleGenerate/ApplyDescription, SetFieldFrontmatter, reconcileAppliedDescription, DescriptionGenerate all exist and match the described shape |
| Goal verifiable (each truth is a concrete, testable behavior) | PASS |
| Commits exist + scope is sane | PASS — `14ed93e e58f071 b4338a4 7773904 b634050` resolve; okf.Repair/charset.go zero-diff confirmed |

No refusal: inputs are complete, consistent, and verifiable. The absence of a SUMMARY is noted as a
process observation, not a blocker — the task explicitly scoped verification to the committed code.

---

## 2. Per-truth verdicts (truth → evidence → pass/fail)

### Truth 1 — Description `<input>` removed, `description` key preserved, `okf.Repair` unchanged — **PASS**
- **Evidence:** The `<div className="field"> … <input id="fm-description"> …` block is DELETED from
  `PageEditor.tsx` (diff replaces it with an ED-01a comment around line 791 noting "okf.Repair only
  ADDS a missing key, never blanks an existing one"). `git diff --stat 14ed93e~1..b634050 --
  internal/okf/repair.go internal/okf/charset.go` is **EMPTY** → `okf.Repair` is unchanged. The
  `description` key is never written by the editor on a plain body save (it rides in `frontmatterRef`).
  Test: "removes the Description input and preserves an existing description across a body autosave"
  asserts `queryByLabelText("Description")` is null, the Generate button renders, and after a body
  autosave the saved frontmatter still contains `description: Existing blurb`. `tsc -b` clean → the
  removal introduced no dangling-import/type error.

### Truth 2 — `GenerateDescription` single-shot, read-only, returns `(text, baseRev, err)`, never auto-runs — **PASS**
- **Evidence:** `describe.go:102-146` — `func (s *Service) GenerateDescription(ctx, path) (text, baseRev string, err error)`:
  `Pages.Get` → `Pages.Revision` captured AT generate time → `truncateForBudget` → `generateOnce`
  per attempt (single ChatModel.Generate, **not** a tool/ReAct loop) → validate-and-retry fail-closed.
  No `Save`/write anywhere. SMTC `find_name_occurrences` shows the **only** non-test call site is
  `handlers_agent.go:873` (the explicit-request handler) → it never auto-runs server-side. Tests:
  `TestGenerateDescription` (normal → trimmed single-line + non-empty baseRev; over-cap/empty →
  rejected+retried (3 calls) fails closed; disabled → `ErrAgentDisabled`).

### Truth 3 — ~1 plain-text sentence, soft-cap ~160, editable + regenerable before apply — **PASS**
- **Evidence:** `prompts.go:303-322` — `describeSystemPrompt` asks for ONE plain-text sentence
  "(roughly 160 characters or fewer)", no Markdown/quotes/line-breaks, page body delimited as
  untrusted DATA. `ValidateDescription` (`describe.go:67-87`) trims, rejects NUL/control chars (→
  enforces single line), collapses internal whitespace, and caps at `maxDescriptionLen = 200` runes
  — a deliberate hard ceiling above the 160 soft target (documented `describe.go:21-26`). The review
  surface is an EDITABLE textarea (`DescriptionGenerate.tsx:292-298`) with a Regenerate button
  (:93-97, :318-325). Tests: "clicking Generate opens the editable draft in a textarea",
  "editing the textarea then Apply sends the EDITED text", "Regenerate re-fetches generateDescription".

### Truth 4 — generate = any-authed read (never writes); apply = editor + CSRF — **PASS**
- **Evidence:** `router.go:218` registers `/agent/generate-description` in the **any-authed** group
  (`authed.Post`, alongside suggest-tags) and `router.go:249` registers `/agent/apply-description`
  in the **editor subgroup** (`editor.Post`, RequireEditor) — both POSTs inherit the global nosurf
  CSRF (router comment ~187/228). `handleGenerateDescription` (`handlers_agent.go:855-887`) calls
  only `GenerateDescription` + `writeJSON` — no `pages.Save`. Tests: `TestGenerateDescriptionEndpoint`
  (200 + base_revision), `TestApplyDescriptionRBAC` (reader → 403), `TestApplyDescriptionMissingCSRF`
  (rejected).

### Truth 5 — approve writes byte-stably via `okf.SetField` through a revision-checked save (409 on moved rev) — **PASS**
- **Evidence:** `handleApplyDescription` (`handlers_agent.go:947`) → `setDescriptionFrontmatter`
  (:988-990) → `pages.SetFieldFrontmatter(frontmatter, body, "description", cleaned)`
  (`batch.go:251`) which calls `okf.SetField` (`repair.go:124`, unchanged) — mirrors
  `SetTagsFrontmatter` exactly, body passed through unchanged. `pages.Save(... req.BaseRevision ...)`
  (:955) reuses the shared single-writer commit + 409 floor; `ErrStaleRevision` → 409 (:961-964).
  Test: `TestApplyDescriptionStaleRevision` (stale base_revision → 409, no write).

### Truth 6 — approved text re-validated server-side before write; client value never trusted — **PASS**
- **Evidence:** `handleApplyDescription` (`handlers_agent.go:926-930`) calls
  `agent.ValidateDescription(req.Description)` BEFORE any fetch/write and writes the returned
  **`cleaned`** value (server-authoritative), never `req.Description` verbatim → 422 on invalid. A
  tampered multi-line / over-cap / control-bearing value cannot reach the page. Test:
  `TestApplyDescriptionRevalidatesServerSide`. *(Verified by reading the handler, not only the test —
  the load-bearing fact is that the variable handed to `setDescriptionFrontmatter` is `cleaned`.)*

### Truth 7 — apply autosave-safe: approve → keep typing body → next autosave does NOT 409 / does NOT revert — **PASS**
- **Evidence:** `reconcileApplied` (`PageEditor.tsx:417-455`) advances `baseRevision.current =
  fresh.revision` after the apply so the next autosave saves at the post-apply revision (no
  self-409). Test: "autosave after applying a description uses the post-apply revision and does NOT
  409 / does NOT revert it" — asserts `base_revision === "rev-2"`, the saved frontmatter contains
  `description: Draft desc`, and **no** conflict dialog ("changed somewhere else" / "overlapped")
  opened.

### Truth 8 — reconcile merges into local frontmatter BEFORE getPage (merge-first; transient getPage failure cannot strand/revert) — **PASS**
- **Evidence:** `reconcileApplied` (`PageEditor.tsx:417-455`): (1) stamp `lastApplyAt`; (2) **MERGE
  FIRST** — `const merged = merge(cur); setFrontmatter(merged); frontmatterRef.current = merged;`;
  (3) THEN `try { fresh = await getPage(path) } catch { setSaveError(refreshErrorMsg); return; }` —
  on a refresh failure the merged value is KEPT locally and a later conflict-Overwrite (which saves
  `frontmatterRef`) preserves rather than reverts it. `reconcileAppliedDescription` (:472-478) =
  `reconcileApplied((cur) => setField(cur, "description", text), …)`. This is the Phase-23
  merge-first pattern **generalized into a shared helper** (`merge` fn + refresh-error copy) — the
  exact data-loss bug the Phase-23 gate caught is structurally not reintroduced, and the interleave
  test's Overwrite-preserves assertion exercises the description path.

### Truth 9 — nothing written without explicit approval; no generate on mount/save/create — **PASS**
- **Evidence:** `DescriptionGenerate.tsx` calls `generateMutation.mutate()` only on the button
  `onClick` (:112), `applyMutation.mutate(draft)` only on the Apply `onClick` (:88-91); there is NO
  `useEffect` that auto-generates on mount. `onApplied` fires only in `applyMutation.onSuccess`
  (2xx). Tests: "mounting issues NO generate or apply call (never auto-runs)" (component) and
  "mounting issues no generate/apply call (never-auto, ED-01b)" (PageEditor) assert
  `generateDescription`/`applyDescription` not called on mount; "Esc cancels and calls
  applyDescription ZERO times" pins no-write-until-approve.

### Truth 10 — targeted apply-vs-in-flight-autosave interleave test exists; own-apply 409 copy not misattributed — **PASS**
- **Evidence:** The test "an apply landing while a body save is in flight 409s as OWN-apply, stays
  data-safe, and is attributed (D-08a/b)" (`PageEditor.test.tsx`) drives the **exact** ordering
  Phase 23 lacked: a body autosave is held in flight (asserted `savePage` called once), then
  generate+apply land (reconcile advances `baseRevision` to rev-2 and merges into `frontmatterRef`),
  then the held save is released and **409s on the own apply**. It asserts (a) the accurate copy
  "your change overlapped with a save in progress" is shown and "changed somewhere else" is absent,
  and (b) resolving via Overwrite saves frontmatter still containing `description: Draft desc`
  (data-safe). The copy is driven by `fromOwnApply` (`PageEditor.tsx:99`), computed at 409-detection
  time against the self-expiring `OWN_APPLY_409_WINDOW_MS` window (:57/:291-292/:568-569), and the
  `DiffReviewDialog` title/summary branch on it (:744-752). The window closes
  (`lastApplyAt = NEGATIVE_INFINITY`) on every successful save (:344/:587) and conflict resolution
  (:614), so a genuine later external 409 still reads accurately.

---

## 3. D-08 closure check (the two carried Phase-23 caveats)

| Caveat | Status | Evidence |
|--------|--------|----------|
| **D-08a** — the own-apply 409 surfaced a misattributed "changed somewhere else" dialog | **CLOSED** | `fromOwnApply` attribution + accurate copy ("Your change overlapped with a save in progress") gated by the self-expiring `OWN_APPLY_409_WINDOW_MS` (PageEditor.tsx:57, 99, 291-302, 568-574, 744-752); window reset on save/resolve so external 409s are never misattributed |
| **D-08b** — the targeted apply-vs-in-flight-autosave interleaving test Phase 23 lacked | **CLOSED** | "an apply landing while a body save is in flight 409s as OWN-apply…" — drives the precise held-save → apply → release-409-on-own-apply ordering, asserts both the accurate copy AND Overwrite-preserves-description data safety. Falsifiable: a merge-after-getPage regression would drop the description from the Overwrite save; a broken window would fail the copy assertion |

Both caveats the Phase-23 gate carried forward are explicitly closed by this phase.

---

## 4. Build / test evidence (commands + actual numbers)

| Command | Result |
|---------|--------|
| `go build ./...` | **exit 0** |
| `go vet ./internal/agent/... ./internal/server/... ./internal/pages/...` | **exit 0** |
| `go test ./...` | **all packages ok** (exit 0, ~12.6s wall); pages 10.19s, search 2.30s, graph 2.22s — no failures |
| `go test -count=1 ./internal/agent/ ./internal/server/` (fresh, not cached) | **both ok** — agent 8.69s, server 8.66s |
| new agent tests | `TestValidateDescription`, `TestGenerateDescription` (over-cap/empty rejected+retried fail-closed; disabled→ErrAgentDisabled) **PASS** |
| new server tests | `TestApplyDescriptionStaleRevision` (409), `TestApplyDescriptionRevalidatesServerSide`, `TestApplyDescriptionRBAC` (reader 403), `TestApplyDescriptionBadRequest`, `TestApplyDescriptionMissingCSRF`, `TestGenerateDescriptionEndpoint` **PASS** |
| `npx vitest run` (from `web/`) | **604 passed (604)** across **55 passed (55)** files (Phase 23 was 592/54; +1 file +12 tests) |
| `DescriptionGenerate.test.tsx` | **8 passed (8)** |
| `PageEditor.test.tsx` | **19 passed (19)** (incl. the 4 new ED-01a/b + D-08a/b tests) |
| `PageEditor.reconcile.adversarial.test.tsx` | **9 passed (9)** |
| `npm run build` (`tsc -b && vite build`) | **clean, exit 0**; `index` chunk 1,462.39 kB (>500 kB warning is **pre-existing**, not introduced) |
| `git diff --stat 14ed93e~1..b634050 -- internal/okf/repair.go internal/okf/charset.go` | **EMPTY** (okf.Repair unchanged) |

`internal/web/dist/.gitkeep` was removed by vite `emptyOutDir` during the build and **restored**
(`git checkout -- internal/web/dist/.gitkeep`) — not a Phase-24 defect; the main working tree carries
no implementation edits from this verification.

---

## 5. Gaps routed UP to the architect

**None.** All ten must-have truths PASS against the real code with full re-run evidence; D-08a and
D-08b (the two carried Phase-23 caveats) are both closed. No code change is required and nothing
routes up.

**Non-blocking observations (FYI, not gaps):**
- The validator + byte-stable write landed in `internal/agent/describe.go` (`ValidateDescription`)
  and `internal/pages/batch.go` (`SetFieldFrontmatter`) rather than the plan's tentatively-named
  `internal/okf/charset.go`. This is a clean placement (the validator sits beside the mode that
  produces the text; the region-write sits beside `SetTagsFrontmatter`), keeps `okf.Repair`
  untouched as ED-01a requires, and is fully covered by tests. Worth noting for the codebase map.
- No SUMMARY was authored for Phase 24. Verification proceeded against the commit range per the
  task's instruction; a SUMMARY would make future audits cheaper but is not required for this gate.

---

## Conclusion

**VERIFICATION: passed.** ED-01a and ED-01b are delivered as specified across all ten must-have
truths. The manual Description input is gone and existing descriptions are preserved (Repair
untouched); generate is single-shot, read-only, never auto-runs; the review surface is editable +
regenerable; apply is editor + CSRF, server-re-validated, byte-stable via `okf.SetField`, and
revision-checked (409). The autosave-safe merge-first reconcile is reused as a shared helper, and
both Phase-23 carried caveats (D-08a misattributed copy, D-08b missing interleave test) are closed —
the latter pinned by the precise held-save→apply→own-409 test Phase 23 was missing. Backend
`go build/vet/test ./...` green; web `npx vitest run` 604/604 and `npm run build` clean.
