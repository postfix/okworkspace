---
author: architect
---

# Phase 23: LLM Tag Suggestions — Context

**Gathered:** 2026-06-30
**Status:** Ready for planning

<domain>
## Phase Boundary

Make the agent's tag suggestions feed the **frontmatter-canonical** tag set Phase 22 established:
suggest → review → approve, landing in **frontmatter** (never in prose), de-duped against the
**union** of body inline `#tags` + frontmatter tags. The backend `SuggestTags` mode + `apply-tags`
write path are **already shipped**; the work is **frontend integration** — the per-page suggest UI
was lost when Phase 22 deleted `InlineTagEditor`, so it must be re-homed, the de-dup widened to the
union, and the server-apply wired to the live editor's autosave safely.

**In scope (ED-03a):**
- Re-home a per-page "Suggest tags" affordance into `PageEditor` (drive the existing, tested
  `TagSuggest` review surface via the shipped `suggestTags`/`applyTags` endpoints).
- De-dup suggestions against `union(inlineTagsFromBody(body), readTags(frontmatter))`, case-insensitive.
- Apply via the server `apply-tags` path **+ an autosave reconciliation** (the co-driver's chosen
  approach) so applying a tag does not 409-thrash or revert the in-flight editor.
- Tests: suggest→approve writes frontmatter + renders in the badge bar; on-page tag not re-offered;
  charset-invalid rejected; apply→keep-typing-body autosave stays clean.

**Out of scope / unchanged:**
- The backend `SuggestTags` agent mode, the `apply-tags` handler, `ValidateTags`/`ValidateTagCharset`
  — all shipped and **re-validate server-side**; Phase 23 needs NO backend change (engineer confirms).
- Bulk sweep review (`TagReviewView`) — untouched.
- The agent never writes prose; `apply-tags` only rewrites the frontmatter `tags` region (server
  passes the body through unchanged — `handlers_agent.go:773`).
</domain>

<decisions>
- **D-01 — Re-home per-page suggest into `PageEditor`.** The deleted `InlineTagEditor` was the live
  per-page suggest surface (it called `suggestTags` directly + applied locally). The standalone
  `TagSuggest({ pagePath })` component still exists (suggest→review→apply via the shipped endpoints,
  fully tested) but is currently rendered nowhere live. Render a compact "Suggest tags" affordance
  near `TagBadgeBar` in `PageEditor` that drives the `TagSuggest` review surface. Minimal: reuse the
  shipped component; do not rebuild the review UI.

- **D-02 — Apply path = server `apply-tags` + autosave reconciliation (CO-DRIVER DECISION, 2026-06-30).**
  `applyTags` POSTs to the server (re-validates, writes the tags region byte-stably via `okf.SetTags`
  against `base_revision`, advances the page revision) and returns **204 (no body)**. Because this is
  a SECOND writer against the live editor's autosave, applying naively leaves `baseRevision` stale →
  the next autosave 409s or reverts the tag. The reconciliation (mirrors the ED-01b pattern this
  phase ESTABLISHES for Phase 24 to reuse): on `applyTags` success, `PageEditor`
  1. `getPage(path)` → fresh `{ frontmatter, revision }` (204 gave us nothing).
  2. Merge the applied tags into the LOCAL frontmatter (preserve any unsaved local frontmatter edits):
     `merged = setTags(frontmatterRef.current, union(readTags(frontmatterRef.current), appliedTags))`;
     set `frontmatter` state + `frontmatterRef.current = merged`.
  3. Advance `baseRevision.current = fresh.revision`; set `lastSavedFrontmatter.current =
     fresh.frontmatter` (what the server now holds) and leave `lastSavedBody` untouched (apply did not
     change the body).
  Net: the tag is on the server AND locally merged; `baseRevision` is fresh, so the next body autosave
  saves cleanly (no 409, no revert). If the user had unsaved frontmatter/body edits, `merged ≠
  fresh.frontmatter` and the normal autosave persists them against the fresh revision. `TagSuggest`
  exposes an `onApplied(appliedTags: string[])` callback; `PageEditor` owns the reconciliation (it
  alone holds the refs). Do NOT blindly overwrite local frontmatter with the server's (that would lose
  unsaved edits — the seeded-guard/stale-frontmatter contention the red-team flagged).

- **D-03 — De-dup against the UNION (criterion 3).** Filter out any suggestion whose tag
  (case-insensitive) is already in `union(inlineTagsFromBody(body), readTags(frontmatter))` — covering
  **unsaved** inline `#tags` the server (which reads the saved page) cannot see. Pass the editor's live
  `body` + `frontmatter` into `TagSuggest` (currently it takes only `pagePath`). The server's per-tag
  `existing` flag is a DIFFERENT axis (vocabulary membership → the "new" badge); keep it for the badge,
  but the on-page union filter is the criterion-3 control. A suggestion list that filters to empty
  shows a quiet "no new tags" state (no write).

- **D-04 — Frontmatter only, never prose.** `apply-tags` rewrites only the `tags` region (server,
  body unchanged); the badge bar reflects frontmatter. Nothing inserts `#tag` into the body. Locked.

- **D-05 — Server re-validation is already there.** `handleApplyTags` runs `agent.ValidateTags` +
  `okf.ValidateTagCharset` (cap, normalize, dedupe, charset) before the write (422/400 on failure).
  Criterion 4 holds with no backend change — pin it with a test that a charset-invalid approved tag is
  rejected (the client never trusts its own list either).

- **D-06 — Renders in the badge bar.** After reconcile, the applied tag is in local frontmatter and is
  NOT an inline body tag → `TagBadgeBar` shows it as a non-inline tag with an ✕ (removable). No extra
  wiring beyond D-02's local merge + the existing badge-bar read.

- **D-07 — No backend change expected.** Suggest, apply, validation, audit, the stale-revision 409
  floor are all shipped. Engineer verifies (grep/build) that Phase 23 is purely `web/` — if a real
  backend gap appears (e.g. the suggest prompt doesn't exclude on-page tags and the client filter is
  insufficient), that is an `EXECUTE BLOCKED` back to planning, not a silent backend edit.
</decisions>

<canonical_refs>
- `web/src/components/TagSuggest.tsx` — the shipped per-page suggest→review→apply surface
  (`suggestTags`, `applyTags`, base_revision token, stale/409 handling, `TagSuggestList`); add
  `body`/`frontmatter` props for the union filter + an `onApplied` callback.
- `web/src/routes/PageEditor.tsx` — render the suggest affordance near `TagBadgeBar` (the 644 region,
  now `TagBadgeBar`); the refs `frontmatter`/`frontmatterRef`/`baseRevision`/`lastSavedFrontmatter`
  (121–135) + `getPage` usage in `runSaver` (276–288) are the reconciliation template.
- `web/src/lib/cm/hashtag.ts` — `inlineTagsFromBody` (union de-dup); `web/src/lib/tag-frontmatter.ts`
  — `readTags`/`setTags` (local merge, byte-stable).
- `web/src/api/client.ts` — `suggestTags` (≈963), `applyTags` (≈977, returns void/204), `getPage`.
- `internal/server/handlers_agent.go` — `handleSuggestTags` (650), `handleApplyTags` (715; 204; body
  unchanged 773; re-validation 746–758) — SHIPPED, confirm no change.
- `internal/agent/suggesttags.go` — `SuggestTags` (266; `existing[]` = vocab membership, not on-page).
- `.planning/REQUIREMENTS.md` §ED-03a; `.planning/ROADMAP.md` §Phase 23.
</canonical_refs>
