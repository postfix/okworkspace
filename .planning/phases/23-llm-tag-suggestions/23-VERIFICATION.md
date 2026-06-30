---
author: qa
responsible: qa
phase: "23"
---

# Phase 23: LLM Tag Suggestions (ED-03a) — VERIFICATION

**Plan file:** `.planning/phases/23-llm-tag-suggestions/23-01-PLAN.md`
**SUMMARY file:** `.planning/phases/23-llm-tag-suggestions/23-01-PLAN-SUMMARY.md`
**Requirements:** ED-03a
**Commits verified (on `main`):** `58e6707`, `08cefd4`, `5a26b3a`

## Verdict

```yaml
gate_result:
  record_author: qa
  id: GR-23-verification
  phase: "23"
  verdict: pass
  basis: >
    All five ED-03a criteria hold against the real code (read, not trusted from the
    SUMMARY) and re-run evidence. (1) <TagSuggest> is rendered after <TagBadgeBar> in
    PageEditor.tsx:749 and editor-gated (`if (!canEdit) return null`, TagSuggest.tsx:164);
    nothing writes until Apply — suggest only opens the surface, onApply alone fires
    applyMutation (pinned: Esc → applyTags ZERO calls). (2) Approve → server apply-tags
    path; the server handler (handlers_agent.go:715-794, ZERO Go diff in the 3 commits)
    re-validates+normalizes (ValidateTags 422), charset-checks (ValidateTagCharset 400),
    and writes ONLY the tags region via setTagsFrontmatter with pg.Body passed through
    UNCHANGED (line 775) against base_revision on the 409 floor — body is never touched,
    no #tag enters prose; the applied tag renders in TagBadgeBar as a removable non-inline
    badge. (3) Union de-dup is real + falsifiable: triggerSuggest captures
    onPage = lower-cased ∪(inlineTagsFromBody(body), readTags(frontmatter)) into a ref at
    suggest-CLICK time and onSuccess filters `!onPage.has(tag.trim().toLowerCase())` —
    covering UNSAVED inline #tags the server cannot see; tests filter Inline/fmtag/a-b
    (ci, nested) and keep `fresh`, with queryByText null assertions (genuinely falsifiable).
    (4) Charset-invalid is rejected server-side (no backend change): zero Go diff confirmed,
    the shipped re-validation read in the handler, and a client pin asserts a 422 surfaces
    the apply-error line and does NOT call onApplied (client never trusts its own list).
    (5) THE HARD BAR holds: reconcileAppliedTags merges the approved tags into LOCAL
    frontmatter via setTags+unionTags BEFORE the post-apply getPage (PageEditor.tsx:390-393
    precede 402), advances baseRevision to the fresh revision, and converges via one
    autosave only on divergence — so apply→type→autosave saves at the post-apply revision,
    does NOT 409, does NOT revert the tag, and an unsaved title/description is preserved.
    The data-loss fix 5a26b3a is the merge-before-getPage reordering and is verified by a
    GENUINE failing-before test: in an isolated worktree with pre-fix PageEditor.tsx
    (08cefd4) + the HEAD adversarial suite, exactly the two data-loss tests FAIL
    (2 failed | 7 passed), and they PASS at HEAD (9/9) — a fix-before test the only way it
    can be proven. Full web suite 592/592 (54 files), npm run build clean (tsc -b && vite
    build, exit 0), tsc -p tsconfig.test.json --noEmit exit 0, web/-only (no .go diff).
  tiers:
    evidence: smtc_honest
    reasoning: manual_review
  provenance:
    - "Read the implementation, not the SUMMARY: TagSuggest.tsx (full), PageEditor.tsx (reconcileAppliedTags 381-417, runSaver 210-330, conflict resolution 510-565, render 729-754), tag-frontmatter.ts (readTags/unionTags/setTags), and the server handler internal/server/handlers_agent.go:715-794 + client api/client.ts:963-983"
    - "DATA-LOSS FIX verified the only way a fix-before test can be: git worktree at pre-fix code (`git checkout 08cefd4 -- web/src/routes/PageEditor.tsx`) + the HEAD adversarial suite → `npx vitest run PageEditor.reconcile.adversarial` = 2 failed | 7 passed; the two failures are EXACTLY 'a failed post-apply refresh keeps the approved tag merged locally' + 'after a stale 409, Overwrite PRESERVES the applied tag'. Same file at HEAD = 9/9 pass. Worktree removed; main tree untouched."
    - "Scope: `git diff HEAD~3..HEAD -- '*.go' internal/ cmd/` → EMPTY (web/-only, criterion-4 'no backend change' by construction)"
    - "Test runs (from web/): `npx vitest run` → 592/592 (54 files); `npm run build` → clean exit 0 (>500kB main-chunk warning is pre-existing, not introduced); `npx tsc -p tsconfig.test.json --noEmit` → exit 0 (editor LSP toBeInTheDocument/.at()/'./*.css' diagnostics confirmed spurious). internal/web/dist/.gitkeep restored after vite emptyOutDir."
    - "Per-criterion oracles read directly (not just the green count): TagSuggest.test.tsx union/onApplied/no-new/422-pin (265-370); PageEditor.test.tsx lands-in-frontmatter (505), autosave-at-post-apply-rev-no-409 (542), unsaved-title-preserved (602); adversarial suite (9 tests) incl. format-divergence termination + in-flight interleave"
```

## Verification method

Goal-backward verification from the ED-03a acceptance criteria to the real apply→reconcile
path, reading the code (not trusting the SUMMARY) and re-running the build/tests as
independent evidence. ED-03a is a TS/React phase; SMTC's LSP semantic (Decision) tier is
Go/Rust/Java, so the strongest available grounding here is structural reads + test
execution + git `--stat` + the **pre-fix source run in an isolated worktree**. Evidence
tier is therefore `smtc_honest` (sound, re-run, pre-fix-source-confirmed), not a
Decision-grade cert.

---

## 1. Verification acceptance handshake

| Check | Status |
|-------|--------|
| Inputs complete (CONTEXT, PLAN, SUMMARY, REQUIREMENTS §ED-03a) | PASS |
| PLAN ↔ SUMMARY consistency (claimed symbols exist + do what's claimed) | PASS — `reconcileAppliedTags`, `unionTags`, `onApplied`, `triggerSuggest` all exist and match the SUMMARY's described shape |
| Goal verifiable (each criterion is a concrete, testable behavior) | PASS |
| Commits exist + are web/-only | PASS — `58e6707 08cefd4 5a26b3a` resolve; `git diff HEAD~3..HEAD -- '*.go' internal/ cmd/` empty |

No refusal: inputs are complete, consistent, and verifiable.

---

## 2. Per-criterion verdicts (truth → evidence → pass/fail)

### Criterion 1 — per-page "Suggest tags" affordance, editor-gated, nothing writes until approve — **PASS**
- **Truth:** A "Suggest tags" affordance is rendered in PageEditor next to `TagBadgeBar`, shown only to editor/admin, driving the shipped suggest→review→apply surface; nothing is written until the explicit Apply.
- **Evidence:** `PageEditor.tsx:749` renders `<TagSuggest pagePath body frontmatter onApplied={reconcileAppliedTags}/>` immediately after `<TagBadgeBar>`. `TagSuggest.tsx:164` `if (!canEdit) return null` (canEdit = editor|admin from the `["me"]` query). `triggerSuggest`/`suggestMutation` only fetch + `setOpen(true)`; the only write is `onApply → applyMutation.mutate`. Tests: "does not render the trigger for a reader" (editor-gated) and "Esc cancels and calls applyTags ZERO times (nothing written until Apply)". The approval surface keeps the locked trust-gate (Cancel is DOM-first focus, Apply never auto-focused, new tags default UNCHECKED).

### Criterion 2 — apply lands in frontmatter byte-stable + server-revalidated, renders in TagBadgeBar, body untouched — **PASS**
- **Truth:** Approving applies via the server `apply-tags` path; the tag lands in frontmatter `tags:` (byte-stable, server-revalidated), renders in `TagBadgeBar`, and the body is never touched (no `#tag` in prose).
- **Evidence:** `applyTags` (client.ts:977) POSTs to `/api/v1/agent/apply-tags`. The server handler (`handlers_agent.go:715-794`, **zero Go diff**) re-validates+normalizes (`ValidateTags`), charset-checks, then writes the new frontmatter via `setTagsFrontmatter(pg.Frontmatter, pg.Body, normalized)` with **`pg.Body` passed through unchanged** (comment + arg at line 773-775) against `base_revision` on the shared `pages.Save` 409 floor. Locally `reconcileAppliedTags` merges into frontmatter only (`setTags` rewrites only the tags region). Test "approving suggested tags lands the tag in frontmatter and renders it in TagBadgeBar (non-inline → removable)" asserts the `Remove tag release` badge appears. The applied tag is non-inline (never in the body), so `TagBadgeBar` shows it with an ✕.

### Criterion 3 — union de-dup over the LIVE body+frontmatter, case-insensitive, real + falsifiable — **PASS**
- **Truth:** A suggestion equal (case-insensitive) to an inline body `#tag` OR a frontmatter tag is filtered out, computed over the live editor body+frontmatter (covering unsaved inline tags).
- **Evidence:** `triggerSuggest` (TagSuggest.tsx:132-139) captures `onPageRef = new Set([...inlineTagsFromBody(body), ...readTags(frontmatter)].map(t => t.trim().toLowerCase()))` at suggest-click time; `suggestMutation.onSuccess` filters `res.suggestions` by `!onPage.has(s.tag.trim().toLowerCase())`. The filter is **real and falsifiable**: TagSuggest.test.tsx:265 feeds `Inline/fmtag/a/b/fresh` against `body="see #inline and #a/b here"` + `frontmatter` tag `fmtag` and asserts ONLY `fresh` survives (`queryByText("Inline")`/`("fmtag")`/`("a/b")` all null) — a no-op filter would fail those null assertions. The adversarial suite re-pins the unsaved-inline + ci case. An all-on-page result opens the quiet "No new tags to suggest" state and never writes (pinned).

### Criterion 4 — charset-invalid approved tag rejected server-side, zero Go diff, re-validation is the shipped path — **PASS**
- **Truth:** A charset-invalid approved tag is rejected server-side; Phase 23 made no backend change and relies on the already-shipped re-validation.
- **Evidence:** `git diff HEAD~3..HEAD -- '*.go' internal/ cmd/` is **empty**. The shipped path read directly: `handlers_agent.go:746` `agent.ValidateTags(req.Tags, nil)` → 422 on empty-after-normalize; `:753-758` `okf.ValidateTagCharset(tag)` → 400 on invalid charset — both BEFORE the write. Client pin (TagSuggest.test.tsx:347) asserts a 422 from `applyTags` surfaces "Couldn't apply the tags. Try again.", keeps the surface open, and does NOT call `onApplied` — the client never trusts its own list and surfaces (does not swallow) the server's authority. *(Note: the client pin mocks the 422 at the boundary; the server-side enforcement itself is pre-shipped code, verified by reading the handler, not re-exercised by a new Go test — appropriate given the zero-Go-change scope.)*

### Criterion 5 — THE HARD BAR: autosave-safe + no data loss + merge-before-getPage data-loss fix with a failing-before test — **PASS**
- **Truth:** apply → keep typing the body → the next autosave does NOT 409 and does NOT revert the applied tag; an unsaved title is NOT lost. The fix `5a26b3a` merges applied tags BEFORE the post-apply `getPage` (apply is authoritative) so a transient `getPage` failure cannot strand/revert the approved tag — and a test reproduces the PRE-fix data loss.
- **Evidence — happy path:** `reconcileAppliedTags` (PageEditor.tsx:381-417): merge into LOCAL frontmatter (`setTags(cur, unionTags(readTags(cur), appliedTags))`, lines 390-393) → `getPage` → advance `baseRevision.current = fresh.revision` (409 floor moved past the apply write) → `lastSavedFrontmatter.current = fresh.frontmatter` → `scheduleAutosave()` only if `merged !== fresh.frontmatter` (terminating convergence). Tests: "autosave after applying tags uses the post-apply revision and does NOT 409" (asserts `base_revision === "rev-2"`, `release` in saved frontmatter, NO conflict dialog) and "an unsaved title typed BEFORE applying tags is NOT lost" (asserts `title: New Title` + `release` both in the saved frontmatter at `rev-2`). Adversarial suite adds unsaved-description, title+description, format-divergence termination (exactly one save, no churn), and the in-flight interleave (no lost write, terminal "Saved").
- **Evidence — data-loss FIX is a genuine failing-before test (the key check):** The merge was reordered to precede `getPage` (`git show 5a26b3a` diff confirms the merge block moved above the `await getPage`). Because the adversarial file landed in the SAME commit as the fix, "passes now" is insufficient — so I ran the HEAD adversarial suite against **pre-fix** code in an isolated git worktree (`git checkout 08cefd4 -- web/src/routes/PageEditor.tsx`; pre-fix ordering confirmed: getPage precedes the merge). Result against pre-fix: **2 failed | 7 passed**, and the two failures are EXACTLY:
  - `a failed post-apply refresh keeps the approved tag merged locally (no revert)`
  - `after a stale 409, Overwrite PRESERVES the applied tag (no server-side revert)`

  The same file at HEAD passes **9/9**. This is the gold-standard fix-before proof: the two tests reproduce the pre-fix data loss (tag dropped on a transient refresh failure; later conflict-Overwrite permanently reverts the server's applied tag) and pass only with the merge-first fix.

---

## 3. The data-loss-fix reproduction finding (explicit)

`5a26b3a` is **not** a fix-without-a-failing-test. Verified empirically, not by reasoning
alone:

| | Pre-fix (`08cefd4` PageEditor.tsx, merge AFTER getPage) | Post-fix (HEAD, merge BEFORE getPage) |
|---|---|---|
| `…refresh keeps the approved tag merged locally` | **FAIL** (tag never merged on getPage failure → `Remove tag release` absent) | PASS |
| `…Overwrite PRESERVES the applied tag` | **FAIL** (frontmatterRef lacks the tag → Overwrite reverts it server-side) | PASS |
| other 7 adversarial tests | PASS | PASS |
| suite total | 2 failed \| 7 passed | **9 passed** |

The defect class (transient post-apply `getPage` failure → approved tag stranded/reverted)
is closed because the merge no longer depends on the refresh; the apply's 2xx makes the
approved set authoritative, and merging it into `frontmatterRef` first means a later
conflict-Overwrite (which saves `frontmatterRef`) preserves rather than reverts it.

---

## 4. Ruling on the accepted MAJOR limitation (apply vs in-flight autosave 409)

**The limitation:** applying tags while this session's own autosave PUT is already
in-flight can, under one ordering (the server apply-tags write wins the race), 409 that
in-flight body PUT into a conflict dialog that says "changed somewhere else" — a
misattribution, since the change was this same session's apply (and if the in-flight save
was frontmatter-only the diff body renders empty).

**My ruling: genuinely LOSSLESS and RECOVERABLE — ACCEPT, do NOT block.** Verified against
the code, not the disposition prose:

1. **No silent data loss.** On a body-PUT 409, `runSaver`'s catch (PageEditor.tsx:264-285)
   does NOT advance `baseRevision`, `lastSavedBody`, or `lastSavedFrontmatter`, and never
   mutates `bodyRef`/`frontmatterRef`. The user's body and the merged tag remain in the
   editor refs intact.
2. **The applied tag survives any resolution.** The merge-first fix puts the approved tag
   in `frontmatterRef` before any conflict can arise; every conflict-dialog exit re-routes
   through the revision-checked save carrying `frontmatterRef` — Overwrite (520-524) and
   manual-merge (537-543, advances `baseRevision` to the server revision) both keep it;
   Save-as-Copy (550+) is a zero-loss escape hatch. The "Overwrite PRESERVES the applied
   tag" adversarial test proves the 409→conflict→Overwrite path keeps the tag.
3. **No stuck state.** The conflict dialog is the established, complete recovery surface;
   `scheduleAutosave` is gated while it is open (no thrash), and resolving resumes a clean
   save at the advanced revision.
4. **Inherent to D-02's chosen second-writer model**, and consistent with `runSaver`'s
   already-accepted recoverable-409 on its own read-back failure. A client-side fix is
   partial at best (an already-in-flight `savePage` cannot be clawed back), so bespoke
   state for a rare, lossless, recoverable wart is not warranted.

**Caveats I flag up (for the architect / Phase 24 — NOT blockers):**
- The misattribution copy ("changed somewhere else" when it was your own apply) + the
  possibly-empty conflict diff is a real, if minor, usability wart. Worth a follow-up
  polish ticket; it does not gate the milestone.
- The *exact* trigger "in-flight body PUT 409s **because** apply won the race" is
  reasoned-safe and its data-safety conclusion is the same code path the Overwrite-preserves
  test proves — but no single test drives the conflict via that precise interleaving (the
  in-flight-interleave test resolves the held save rather than 409ing it). This is a
  coverage observation, not a gap; the data-safety holds by construction. Phase 24 reuses
  this pattern, so a targeted test of that ordering would be cheap insurance.

---

## 5. Build / test evidence (commands + actual numbers)

All run from `web/` (repo-root vitest loads the wrong config — avoided):

| Command | Result |
|---------|--------|
| `npx vitest run` | **592 passed (592)** across **54 passed (54)** files — matches the SUMMARY's claim |
| `npm run build` (`tsc -b && vite build`) | **clean, exit 0**; main `index` chunk 1,457.58 kB (>500 kB warning is **pre-existing**, not introduced by Phase 23) |
| `npx tsc -p tsconfig.test.json --noEmit` | **exit 0** (editor LSP `toBeInTheDocument`/`.at()`/`'./*.css'` diagnostics confirmed spurious) |
| `git diff HEAD~3..HEAD -- '*.go' internal/ cmd/` | **EMPTY** (web/-only) |
| pre-fix worktree: `npx vitest run PageEditor.reconcile.adversarial` | **2 failed \| 7 passed** (the two data-loss tests) — proves the fix-before property |
| HEAD: same file | **9 passed** |

`internal/web/dist/.gitkeep` was deleted by vite `emptyOutDir` during build and restored
(`git checkout -- internal/web/dist/.gitkeep`) — not a Phase-23 defect. The isolated
worktree was removed; the main working tree carries no implementation edits.

---

## Conclusion

**VERIFICATION: passed.** ED-03a is delivered as specified across all five criteria. The
criterion-5 hard bar holds, and its data-loss fix is backed by a genuine failing-before
test (empirically confirmed against pre-fix source). The one accepted MAJOR limitation
(same-session apply vs in-flight autosave 409) is genuinely lossless and recoverable — an
acceptable, bounded UX wart, not a blocking gap — with two minor follow-up notes flagged
for the architect and Phase 24.
