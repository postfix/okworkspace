---
author: architect
---

# Phase 24: LLM-Generated Description — Context

**Gathered:** 2026-06-30
**Status:** Ready for planning

<domain>
## Phase Boundary

Remove the manual Description field (ED-01a) and replace it with an on-demand agent generator
(ED-01b): an editor clicks "Generate description", reviews/edits/regenerates a ~1-sentence draft,
and approves it into byte-stable frontmatter via a dedicated server endpoint — never auto-runs, and
the apply is **autosave-safe** (the reconcile pattern Phase 23 established + proved). This is a
**backend + frontend** phase: there is currently **no** description-generation agent mode.

**In scope (ED-01a + ED-01b):**
- Backend: `agent.GenerateDescription(ctx, path) → (text, baseRev, err)` (single-shot, read-only);
  `POST /agent/generate-description` (any-authed read, never writes); `POST /agent/apply-description`
  (editor + CSRF, byte-stable `okf.SetField`, revision-checked, **re-validates the text**).
- Frontend: remove the Description `<input>` from `PageEditor`; a "Generate description" affordance
  (mirrors `TagSuggest`) → review (editable + regenerable) → approve → `applyDescription` → reconcile.
- Tests: input gone + description preserved; generate→review→approve writes byte-stably; never-auto;
  server re-validation; the autosave-safe apply (approve → keep typing body → no 409, no revert).

**Out of scope / unchanged:**
- `okf.Repair` (ED-01a relies on it being unchanged — it only ADDS a missing `description` key,
  never overwrites; `repair.go:43-49`). Do NOT modify repair.
- Surfacing the description in search cards / graph hover (deferred consumer wiring — Future).
- The other agent modes (summarize/rewrite/draft/tags) — untouched.
</domain>

<decisions>
- **D-01 — Apply via a DEDICATED server endpoint + reconcile (consistent with Phase 23; NOT the
  normal save).** ED-01b requires the output be "re-validated server-side before write" and the
  criterion-4 autosave reconcile. The normal page save does not re-validate a description's
  length/charset, so the apply MUST be a dedicated `POST /agent/apply-description` (editor + CSRF)
  that re-validates then writes the `description` key byte-stably via `okf.SetField` against
  `base_revision`. This mirrors `apply-tags` exactly and reuses the co-driver's chosen Phase-23
  pattern (server-apply + reconcile). No new design fork — decided by spec + Phase-23 precedent.

- **D-02 — GenerateDescription (backend), modeled on SuggestTags + SummarizePage.** New
  `agent.Service.GenerateDescription(ctx, path) (text string, baseRev string, err error)`: `Pages.Get`
  the page, **capture the revision at generate time** (echoed to apply → 409 on a moved page, like
  SuggestTags), `truncateForBudget(body)`, a new `buildDescribeMessages` prompt → `generateOnce`
  with a description-specific token cap + temperature. The prompt yields ONE plain-text meta sentence
  (no markdown, no quotes, no frontmatter), soft-capped ~160 chars. Validate/trim server-side before
  returning (retry on empty/over-cap/markup, like the other modes fail-closed).

- **D-03 — Server re-validation on apply (ED-01b "re-validated server-side before write").**
  `handleApplyDescription` re-validates the client-approved text independently: trim; reject control
  chars / NUL / newlines; enforce the length cap; collapse to a single line. A text that fails →
  422/400 (never written). The client list is never trusted (mirrors apply-tags' `ValidateTags`).
  Add a small `agent.ValidateDescription`/`okf` charset check as the shared validator.

- **D-04 — Byte-stable write via okf.SetField (mirror setTagsFrontmatter).** Add a server helper
  (e.g. `setDescriptionFrontmatter(frontmatter, body, text)` or `pages.SetFieldFrontmatter`) that
  produces the new frontmatter region with ONLY the `description` key changed, via `okf.SetField`
  (repair.go:124) — body and every other key unchanged — then `pages.Save(... base_revision ...)`
  (the same single-writer commit + 409 floor apply-tags uses). Returns 204 (like apply-tags).

- **D-05 — Frontend apply→reconcile (reuse Phase-23's PROVEN merge-first pattern).** A new
  `DescriptionGenerate` component (mirror `TagSuggest`) drives generate→review→approve. On
  `applyDescription` 204 success, `PageEditor.reconcileAppliedDescription(text)`:
  **merge first** — `setField(frontmatterRef.current, "description", text)` into local frontmatter
  (set state + `frontmatterRef`) — THEN `getPage` to advance `baseRevision = fresh.revision` and
  `lastSavedFrontmatter = fresh.frontmatter`; if local diverges, let autosave converge. Merge-first
  is load-bearing: apply is authoritative, so a transient `getPage` failure must not strand/revert
  the approved description (the EXACT data-loss bug the Phase-23 gate caught — do not reintroduce it).
  Generalize Phase-23's reconcile helper if clean, else parallel it.

- **D-06 — Remove the Description input (ED-01a).** Delete the `<input id="fm-description">` block in
  `PageEditor.tsx` (632-643) + its label. The `description` frontmatter key is untouched (no input
  writing to it → an existing human description is preserved; `okf.Repair` still guarantees the key
  exists). The generate affordance is the only editor surface for it now.

- **D-07 — Never auto-runs (ED-01b).** Generate fires ONLY on explicit click; apply ONLY on explicit
  approve. No generate on save/create/open. No silent write. Pin with a test that mounting the editor
  issues no generate/apply call.

- **D-08 — Carry Phase-23's two flagged caveats (qa ruling).** (a) The apply-vs-in-flight-autosave
  409 surfaces a misattributed "changed somewhere else" dialog — fix the COPY for the
  own-apply case if cheap (polish), and (b) ADD the targeted in-flight-autosave interleaving test
  that Phase 23 lacked (data-safety holds by construction, but pin it here since this phase re-treads
  the path). Both are explicitly in-scope for this phase per the Phase-23 handoff.

- **D-09 — Shared surface coordination.** `PageEditor.tsx` was edited by Phase 22 (TagBadgeBar at
  ~644) and 23 (TagSuggest + reconcile). Phase 24 removes the description input (632-643, disjoint)
  and adds the generate affordance near the title/tags area. Additive; no conflict with the tag work.
</decisions>

<canonical_refs>
- `internal/agent/agent.go` — `SummarizePage` (369; single-shot template), `generateOnce` (346),
  `buildSummarizeMessages`/prompts pattern; mirror for `GenerateDescription`.
- `internal/agent/suggesttags.go` — `SuggestTags` (266; the capture-baseRev-then-generate shape).
- `internal/server/handlers_agent.go` — `handleSuggestTags` (650; read-mode handler template),
  `handleApplyTags` (715; editor+CSRF, re-validate, `setTagsFrontmatter`, 204, 409 floor; the
  apply template), `auditAgentMode`.
- `internal/okf/repair.go` — `SetField` (124; byte-stable field set), `SetTags` (180); `okf.Repair`
  (only adds missing keys — DO NOT change).
- `internal/server/router.go` — agent route registration (≈191-198); add the two new routes.
- `web/src/components/TagSuggest.tsx` — the generate→review→apply component template.
- `web/src/routes/PageEditor.tsx` — the Description `<input>` to delete (632-643); `reconcileAppliedTags`
  (Phase 23) — the reconcile to parallel; refs 121-135; `setField`/`readField` import (16).
- `web/src/lib/frontmatter.ts` — client `setField`/`readField` (local merge for the reconcile).
- `web/src/api/client.ts` — `suggestTags`/`applyTags` shapes to mirror as `generateDescription`/
  `applyDescription`; `getPage`.
- `.planning/REQUIREMENTS.md` §ED-01a §ED-01b; `.planning/phases/23-llm-tag-suggestions/23-VERIFICATION.md`
  (the carried caveats + the merge-first data-loss lesson).
</canonical_refs>
