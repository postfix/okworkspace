---
author: engineer
responsible: engineer
phase: "23"
status: complete
milestone: v1.4
---

# Phase 23: LLM Tag Suggestions — SUMMARY

**Plan file:** `.planning/phases/23-llm-tag-suggestions/23-01-PLAN.md`
**Requirements:** ED-03a
**Commits (on `main`):**
- `58e6707` feat(web): TagSuggest union de-dup + onApplied handoff (ED-03a)
- `08cefd4` feat(web): autosave-safe apply→reconcile in PageEditor (ED-03a)
- `5a26b3a` fix(web): merge applied tags before post-apply refresh (no data loss) *(gate disposition — both gates' converged defect)*

## What was built

Re-homed the shipped LLM tag-suggestion flow into the live editor (the per-page suggest
surface was lost when Phase 22 deleted `InlineTagEditor`). A "Suggest tags" affordance
now sits next to `TagBadgeBar` in `PageEditor`; it suggests against the LIVE
body+frontmatter (union de-dup), applies via the shipped **server apply-tags** path, and
PageEditor reconciles the apply into autosave safely. **Phase 23 is `web/`-only — zero Go
change** (suggest mode, apply-tags write, server re-validation, audit, and the 409 floor
were all already shipped; confirmed by `git diff HEAD~3 --stat` = web/ only and a HEAD
diff of `internal/` = empty).

- **`TagSuggest.tsx`** — the default export gained `body`/`frontmatter` props + an
  `onApplied(appliedTags)` callback:
  - **Union de-dup (criterion 3, D-03):** `triggerSuggest` captures
    `onPage = lower-cased ∪(inlineTagsFromBody(body), readTags(frontmatter))` into a ref
    **at suggest-click time** (deterministically the live page, immune to react-query
    closure timing); `suggestMutation.onSuccess` filters `res.suggestions` by it. Covers
    **unsaved** inline `#tags` the server can't see. The server `existing` flag (vocab
    membership → "new" badge) is kept as a separate axis. An all-on-page result opens a
    quiet "No new tags to suggest" state and never writes.
  - **onApplied (D-02):** replaced the bare `invalidateQueries(["page"])` (which cannot
    re-seed the editor past its `seeded` guard and races autosave) with
    `onApplied?.(appliedTags)` where `appliedTags` is the mutation variable (the CHECKED
    set). PageEditor owns the reconciliation — it alone holds the refs. The 409→stale /
    non-409→applyError split is intact.
- **`tag-frontmatter.ts`** — new pure `unionTags(existing, added)`: trim+lowercase,
  de-dupe case-insensitively, existing order first then appended. Normalization mirrors
  `reconcileFrontmatterTags`'s `norm`, so an apply and the next body-save never churn
  each other's bytes.
- **`PageEditor.tsx` `reconcileAppliedTags`** (the load-bearing ED-01b pattern Phase 24
  reuses) — on `applyTags` success: **merge the applied tags into LOCAL frontmatter FIRST**
  (`setTags(frontmatterRef.current, unionTags(readTags(frontmatterRef.current), applied))`
  — touches only the tags region → an unsaved title/description survives; the apply is
  authoritative so the merge does NOT depend on the refresh), then `getPage` to advance
  `baseRevision.current = fresh.revision` + set `lastSavedFrontmatter.current =
  fresh.frontmatter`, and `scheduleAutosave()` only if `merged !== fresh.frontmatter`.
  Body refs (`bodyRef`/`lastSavedBody`) untouched (apply did not change the body). The
  `runSaver` single-flight/409/lock logic is untouched; `<TagSuggest>` is rendered after
  `TagBadgeBar` (editor-gated by TagSuggest itself).

## Gate outcomes

Both gates ran. **Hard-bar phase:** the autosave-safe apply→reconcile is a load-bearing
data-integrity invariant (no 409-thrash, no tag revert, no loss of an unsaved title).

### `anvil-code-reviewer` (`/tmp/anvil-spawn-out/23-REVIEW.md`)
Verdict: **SHIP — 0 blockers, 1 MAJOR (needs-disposition), 2 MINOR.** Independently
traced the four load-bearing invariants and confirmed: reconcile preserves unsaved
frontmatter and cannot self-409 on the happy path; the convergence save provably
**terminates** (`lastSavedFrontmatter := sentFrontmatter` ⇒ `frontmatterRef ===
lastSavedFrontmatter` after one save, stable even under server 4-space re-marshal); the
union filter is total and case-symmetric; `saving` single-flight is never touched by
reconcile (no stuck guard); zero backend leakage.

### `anvil-adversarial-tester` (branch, not merged — file cherry-picked + post-fix-corrected)
Verdict: **1 defect reproduced (failing test) + 1 characterized consequence; 7 attacks
confirm the design holds.** The reproduced defect is the SAME one the code-reviewer
flagged MINOR — convergence under independent attack. Delivered a 9-test durable suite
(`PageEditor.reconcile.adversarial.test.tsx`); I cherry-picked the file onto `main` and
corrected the two pre-fix-pinning tests to the post-fix data-safe contract.

## Review disposition

| Finding | Source | Severity | Disposition |
|---------|--------|----------|-------------|
| **Post-apply refresh failure loses the applied tag.** `reconcileAppliedTags` merged the tags + advanced `baseRevision` only AFTER its `getPage`. Since `applyTags` already committed server-side (onSuccess fires only on a 2xx), a transient `getPage` failure left the approved tag unmerged locally (vanished from the editor), stranded `baseRevision` one revision behind, and a later conflict-**Overwrite** (which saves local frontmatter) **permanently reverted** the server's just-applied tag. Breaks the criterion-5 hard bar. | both gates (code-reviewer MINOR, adversarial DEFECT) | **DATA LOSS** | **FIXED (`5a26b3a`).** The merge does not depend on `fresh`, so do it FIRST (apply is authoritative). On refresh failure: keep the merged tags locally + surface the recoverable banner, leave `baseRevision` as-is; the next save then routes through the revision-checked path exactly like `runSaver`'s own read-back-failure handling — recoverable, and now data-safe (the tag is in `frontmatterRef`, so a conflict-Overwrite preserves it). Pinned by 2 reworked adversarial tests (tag stays merged; Overwrite preserves it). Happy path byte-for-byte unchanged. |
| **Apply can self-409 an in-flight autosave into a *spurious* "changed somewhere else" dialog.** `applyTags` is a second writer from the SAME session; when an apply wins the server race against this editor's own in-flight body PUT, that PUT 409s into the conflict dialog (false attribution; if the in-flight save was frontmatter-only the diff renders empty body). | code-reviewer | MAJOR (needs-disposition) | **ACCEPTED (documented).** Bounded by the optimistic-concurrency floor — **no data loss / corruption / stuck-state in any ordering** (both gates confirm; `bodyRef`/`frontmatterRef` intact, every resolution converges). It is inherent to **D-02's deliberately chosen second-writer model** ("applying is a SECOND writer"), and consistent with `runSaver`'s already-accepted recoverable-409 on its own read-back failure. A client-side fix is partial at best (an already-in-flight `savePage` can't be clawed back), so adding bespoke state for a rare, lossless, recoverable UX wart is not warranted. Flagged for Phase 24's awareness (it reuses this pattern). |
| Apply sends original-case tag to the server; `unionTags` lowercases → one redundant no-content version on a non-lowercase suggestion. | code-reviewer | MINOR | **INTENDED — no change.** Cosmetic (one extra hidden Git version), only for a non-lowercase suggestion, and consistent with the app's lowercase-everywhere norm (`reconcileFrontmatterTags`). The server re-validates/normalizes regardless. |

## Conditional-gate skip-witness

None — this is a hard-bar phase; both `[GATE]` conditions held, so both gates fired (no
`not_applicable` skip-witness is emitted).

## Files modified

| File | Change |
|------|--------|
| `web/src/components/TagSuggest.tsx` | Default export: `body`/`frontmatter` props + `onApplied` callback; union de-dup captured at suggest-click time; `onApplied(checked)` replaces the query invalidate; `emptyLabel` prop on `TagSuggestList` (distinguishes "no suggestions" from "no NEW tags"). `useQueryClient` removed. |
| `web/src/components/TagSuggest.test.tsx` | `renderSuggest` takes body/frontmatter/onApplied; +4 tests: union de-dup (case + nested), onApplied with exactly the approved tags, quiet no-new state (no write), criterion-4 server-rejection (422/400) pin (onApplied not called). 15 tests. |
| `web/src/lib/tag-frontmatter.ts` | Added pure `unionTags(existing, added)` (trim+lowercase, ci-dedupe, order-preserving). |
| `web/src/lib/tag-frontmatter.test.ts` | +4 `unionTags` tests (append-new/order, ci-dedupe, normalize, empties). 16 tests. |
| `web/src/routes/PageEditor.tsx` | `reconcileAppliedTags` (merge-first, autosave-safe); render `<TagSuggest>` after `TagBadgeBar`; import `unionTags`. `runSaver`/409/lock/single-flight untouched. |
| `web/src/routes/PageEditor.test.tsx` | +3 integration tests: apply → tag in frontmatter + TagBadgeBar ✕; apply → keep typing → autosave at the post-apply revision (no 409, no revert); unsaved title before apply NOT lost. 15 tests. |
| `web/src/routes/PageEditor.reconcile.adversarial.test.tsx` (new) | Adversarial suite (gate-authored, cherry-picked + post-fix-corrected): refresh-failure data-safety (×2), unsaved description, unsaved title+description, format-divergence convergence+termination, applyTags-409 runs no reconcile, in-flight body-save interleave, union de-dup of unsaved inline + frontmatter (ci), nested `a/b` round-trip. 9 tests. |

## Verification

- `npx vitest run` (full web suite, **from `web/`**) — **592/592 PASS**, 54 files (no
  regression; the existing 409/lock/single-flight/materialization PageEditor tests still
  green). Pre-Phase-23 baseline was 583; +9 adversarial.
- Targeted: `TagSuggest` 15, `tag-frontmatter` 16, `PageEditor` 15,
  `PageEditor.reconcile.adversarial` 9.
- `npm run build` (`tsc -b && vite build`) — **clean** (tsc passed; the >500 kB
  main-chunk warning is pre-existing, not introduced here). `internal/web/dist/.gitkeep`
  restored after the vite `emptyOutDir` delete (never committed).
- **web/-only confirmed:** `git diff HEAD~3 --stat` shows only `web/`; `git diff HEAD
  --stat -- '*.go' internal/ cmd/` is empty; `grep apply-tags|SuggestTags internal/` is
  unchanged from HEAD. Phase 23 is purely frontend (D-07 held — no backend gap; no
  `EXECUTE BLOCKED`).

## Hoare-triple summary

- **Q (postcondition):** after apply→reconcile, `frontmatter.tags ⊇ approvedTags`
  (case-folded), any unsaved local frontmatter edit (title/description) is preserved, the
  body is untouched, `baseRevision` equals the post-apply server revision, and the next
  autosave saves cleanly (no 409, no revert). A suggestion already on the live page is
  never offered; a charset-invalid approved tag is rejected server-side.
- **S (code shape):** `reconcileAppliedTags` merges `appliedTags` into `frontmatterRef`
  via `setTags`+`unionTags` (independent of the refresh), then advances `baseRevision`/
  `lastSavedFrontmatter` from `getPage` and converges via one autosave on divergence.
- **P (precondition):** `applyTags` committed server-side (204) so the tags are
  authoritative; `setTags` rewrites only the tags region; `getPage` returns the fresh
  revision (or fails → handled).
- **S establishes Q:** advancing `baseRevision := fresh.revision` blocks the next
  self-409; merging into LOCAL frontmatter (not the server bytes) preserves unsaved
  edits; the `merged !== fresh.frontmatter` guard makes the no-unsaved-edits case a clean
  no-op. **The fix repaired a broken Q on the refresh-failure branch** — the merge sat
  after the `getPage`, so a transient failure failed to establish "approved tags in local
  frontmatter"; merging first restores it (and renders the later Overwrite data-safe).
- **Invariant (object-state):** across the editor's lifetime, an apply's tags are merged
  into local frontmatter exactly once (ci-deduped), and the convergence autosave is
  idempotent → terminates (`frontmatterRef === lastSavedFrontmatter` after one save), so
  no 409-thrash. The second-writer race (apply vs in-flight autosave) is bounded by the
  409 floor and lossless.

```yaml
handoff:
  record_author: engineer
  accountable_owner: engineer
  id: H-23-execute
  from: engineer
  to: architect
  artifact: .planning/phases/23-llm-tag-suggestions/23-01-PLAN-SUMMARY.md
  intent: "Phase 23 execution complete — re-homed the per-page LLM tag-suggest surface into PageEditor (union de-dup over the LIVE body+frontmatter, captured at suggest time), applied via the shipped server apply-tags path with an autosave-safe reconcile (merge-first, advance baseRevision, converge on divergence). web/-only, zero Go change (D-07 held). Both gates run + dispositioned; a data-loss defect on the post-apply refresh-failure branch (converged by both gates) was fixed merge-first, and a same-session apply-vs-autosave 409 (MAJOR) was consciously ACCEPTED as bounded/lossless/inherent to D-02."
  status: ready
```

```yaml
evidence:
  record_author: engineer
  evidence_producer: engineer
  commissioned_by: engineer
  accountable_owner: engineer
  id: E-23-test-run
  subject_ref: "npx vitest run (web/) + npm run build"
  kind: test_result
  issuance:
    kind: producer_signed
  warrant:
    kind: external_cited
    scope:
      - "carry=full web suite 592/592 PASS (54 files); pre-phase baseline 583"
      - "carry=criterion-5 hard bar PASS (apply→type→no 409/no revert; unsaved title preserved)"
      - "carry=refresh-failure data-loss FIX regression PASS (tag merged + Overwrite preserves)"
      - "carry=union de-dup (ci + nested + unsaved inline) PASS; server-rejection 422 pin PASS"
      - "carry=npm run build (tsc -b && vite build) clean; web/-only (no Go diff)"
    rationale: "exercises reconcileAppliedTags (merge-first, happy + refresh-failure + format-divergence + in-flight interleave), the union de-dup, and the server-authority rejection path against a headless RTL/jsdom PageEditor; full suite confirms no 409/lock/single-flight/materialization regression"
  freshness:
    status: fresh
    checked_against: HEAD
```

```yaml
gate_result:
  record_author: engineer
  status: applicable
  gate: anvil-code-reviewer
  phase: "23"
  basis: "Pre-handoff diff review (every SUMMARY, not scope-gated). Verdict SHIP — 0 blockers, 1 MAJOR (same-session apply-vs-autosave 409: ACCEPTED, bounded/lossless/inherent to D-02), 2 MINOR (1 = the refresh-failure data-loss, FIXED 5a26b3a; 1 = case-churn, INTENDED). All dispositioned above. Artifact: /tmp/anvil-spawn-out/23-REVIEW.md"
```

```yaml
gate_result:
  record_author: engineer
  status: applicable
  gate: anvil-adversarial-tester
  phase: "23"
  basis: "Hard-bar phase (autosave-safe apply→reconcile data integrity). 1 defect reproduced (post-apply refresh failure reverts the applied tag + strands the revision → later Overwrite data loss) — FIXED merge-first (5a26b3a); 7 attacks confirmed the design holds. 9-test suite cherry-picked onto main; the 2 pre-fix-pinning tests corrected to assert the post-fix data-safe contract. Branch NOT merged."
```
