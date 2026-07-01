---
author: qa
responsible: architect
phase: 30-v13-closure
task: TC-01
status: complete
verdict: passed
parent_artifacts:
  - .planning/phases/30-v13-closure/30-01-PLAN.md
  - .planning/phases/30-v13-closure/30-CONTEXT.md
  - .planning/milestones/v1.3-REQUIREMENTS.md
  - .planning/phases/28-reading-shell-fold-in/28-VERIFICATION.md
  - .planning/phases/29-tag-regression-fix/29-VERIFICATION.md
---

# Phase 30 / TC-01: v1.3 Tag Management (TM-01…05) + Integrated v1.5 Fix — VERIFICATION

**Verified:** 2026-07-01 by qa (independent, goal-backward, against committed HEAD `c343a30` docs /
`1435ea6` code — the tag surface).
**Substrate:** SMTC daemon **OFFLINE**. Every claim about what code *does* by reading/grep is
**Structural/Honest tier → MEDIUM** confidence, never Decision-tier/HIGH. Every claim from a suite I
**re-ran** is **Empirical**. This is the independent verification v1.3 Tag Management **never received**
(shipped `9de5077`, no phase-19/20 verify) — it is NOT a rubber-stamp of the Phase-28/29 SUMMARYs; I
re-read the source on disk and re-ran the gates myself.

## Verdict: **PASSED**

All five v1.3 requirements (TM-01 rename, TM-02 merge, TM-03 delete, TM-04 admin UI, TM-05 graph-store
consistency) hold against code reality, AND the integrated tag surface (v1.3 admin ops + the Phase-29
body-rewrite fix) is coherent end-to-end. **One LOW finding** routes up to the architect (test-granularity
only, non-blocking; not correctness/security). Tree left CLEAN (`internal/web/dist/.gitkeep` restored
after `npm run build`).

**Scope note:** Phase 28's RS-* and Phase 29's TR-* are NOT re-verified here (they have their own
VERIFICATION artifacts). TC-01 is the never-done v1.3 TM-* verification + a holistic coherence check.
I cross-reference 29-VERIFICATION.md's TR-03 goldens for the no-re-materialization proof rather than
re-deriving it.

## Suites re-run at HEAD (Empirical)

| Gate | Command | Result |
|------|---------|--------|
| Backend build | `go build ./...` | exit 0 |
| Backend vet | `go vet ./...` | exit 0 |
| Backend test | `go test ./...` | **18 pkgs ok**, 2 no-test — okf, pages, server, graph all green |
| Graph race | `go test ./internal/graph/... -race` | **ok** (clean, ~23s) — TM-05 acceptance gate |
| Frontend types | `npx tsc -b` | exit 0 |
| Frontend unit | `npx vitest run` | **804 passed / 65 files** (includes TagManagement.test.tsx) |
| Frontend build | `npm run build` | exit 0 (only benign INEFFECTIVE_DYNAMIC_IMPORT + chunk-size notices) |

`npm run build` wiped `internal/web/dist/.gitkeep` (expected; `dist/` is gitignored except the keep file)
— **restored via `git checkout`**. Final `git status --porcelain`: only `zz_sec_probe_test.go`
(Phase30Security's file, not mine) is untracked; no qa source change. Targeted re-runs:
`go test ./internal/pages/ -run 'BodyRewrite|ApplyTagsBatchStale' -v` → all 6 PASS.

---

## Per-requirement verdicts

### TM-01 — Tag Rename — **PASSED** (MEDIUM structural + Empirical tests)

Verified against `internal/server/handlers_tags_admin.go` (`handleTagRename` :266) +
`handlers_tags_admin_test.go`, route wired in `router.go:336` (admin subgroup).

- **Endpoint + body.** `POST /api/v1/admin/tags/rename` decodes `{old_name, new_name, preview}`
  (`tagRenameRequest` :37). Wired under `authed.Group{ admin.Use(RequireRole(RoleAdmin)) }`
  (router.go:297-338) — admin-only + global nosurf CSRF on the mutating POST.
- **Preview → affected pages.** `req.Preview` returns `tagPreviewResponse{AffectedPages, TotalAffected}`
  with per-page `{page_path, current_tags, new_tags, body_affected}` (:315-321).
  `TestTagRename_PreviewThenApply` asserts `TotalAffected==2` for 2 seeded pages.
- **Approve → SetTags + single-writer batched commit (N→1).** Apply path
  `h.pages.ApplyTagsBatch(items, actor)` (:331). In `batch.go:200-213` the whole batch is ONE
  `commitPayload{Writes}` → ONE `EnqueueCommit` (`batchCommitSubject(len(writes))`), N pages → 1 commit.
  Frontmatter written byte-stably via `SetTagsFrontmatter` (okf.SetTags) — the shared byte-stable builder
  (batch.go:162, :253-279).
- **Normalization: trim/lowercase/reject-empty.** `normalizeTag = ToLower(TrimSpace(...))` (:76);
  empty→400 (:274-276). `TestTagRename_EmptyName` covers `""`, whitespace-only.
- **Case-only no-op.** `oldName==newName` after normalization → 200 with empty results, no write
  (:284-289). `TestTagRename_CaseOnlyNoOp` (`"Ops"→"ops"`) asserts the tag survives on 1 page.
- **Conflict (new_name already on some pages → merge/dedupe).** `computeRenameTags` (:126) guards each
  append with `containsNormalized(out, newName)` so a page carrying BOTH `old` and `new` collapses to
  `new` only (no duplicate). Correct by trace. *(See LOW finding — no dedicated seeded handler test for
  this exact case; the analogous merge-dedup IS tested.)*
- **Per-page 409 doesn't sink the batch.** `ApplyTagsBatch` compares each item's staged `BaseRevision`
  vs current `Revision`; a moved page → `TagApplyStale`, excluded from the commit, others still apply
  (batch.go:127-134). `TestApplyTagsBatchStaleDoesNotSink` PASS (re-run).
- **Audit entry.** `audit.Record(Event{Action: ActionTagRename, Actor, Target: old→new, Detail:"N pages"})`
  (:336-341); `ActionTagRename="tag_rename"` (audit.go:65).
- **Server-side validation (never trust client).** `okf.ValidateInlineTag(newName)` (:278) is the SOLE
  `Validate*` in the handler — the stricter inline grammar (validate.go:32). `TestTagRename_BadCharset`
  (`<script>`) + `TestTagRename_InlineGrammarGate` (`"a b"`, `"1984"` — accepted by legacy charset,
  rejected by inline grammar) prove the new gate is the active HTTP validator.
- **Body rewrite integrated (Phase-29 fix preserved alongside v1.3 frontmatter behavior).** Rename passes
  `bodyOld=oldName, bodyNew=newName` (:309); the batch rewrites the page's OWN freshly-read body via
  `okf.RewriteHashtags` (TOCTOU-safe, batch.go:149-153) in the SAME commit. `TestBodyRewriteRenameGolden`
  (re-run PASS) proves body `#ops`→`#devops` AND frontmatter `[devops]` in one commit.

### TM-02 — Tag Merge — **PASSED** (MEDIUM structural + Empirical tests)

`handleTagMerge` (:346), route `router.go:337`.

- **Endpoint + body.** `POST /admin/tags/merge` decodes `{source, target, preview}` (:43). Admin-gated.
- **Preview → affected pages.** Same preview shape; `TestTagMerge_PreviewThenApply` seeds `a{ops}`,
  `b{devops}`, `c{ops,devops}` and applies.
- **Approve → SetTags + single-writer commit.** Same `ApplyTagsBatch` path.
- **Target already present → source removed, target untouched (no duplicate).** `computeMergeTags` (:145)
  drops `source`, sets `hasTarget` if already present, and only appends `target` when absent — dedup via
  `containsNormalized`. **This is the conflict/dedup case, and it IS exercised**: `c.md` carries both
  `{ops,devops}`; after merge the test asserts `devops` on all 3 pages and `ops` gone (0 pages).
- **source==target → 400.** `:358-361`; `TestTagMerge_SameSourceTarget` PASS.
- **Source ceases to exist (no orphan tag nodes).** After the commit, `page_tags` rows for `source`
  are gone (rebuilt from the rewritten page); tag nodes are derived from `page_tags` (see TM-05), so the
  source node vanishes. Test asserts `PagesByTag("ops")==0`.
- **Per-page 409 same as TM-01** (shared `ApplyTagsBatch`).
- **Audit entry.** `ActionTagMerge` (:413-418).
- **Validation.** `ValidateInlineTag(target)` (:362); `TestTagMerge_InlineGrammarGate` (`"a b"`,`"1984"`)
  proves the inline gate is active on merge's target too.
- **Body rewrite integrated.** `bodyOld=source, bodyNew=target` (:386); merge enumerates body-`#source`
  ONLY (not body-`#target`, :374-381 comment + `PagesWithBodyTag(source)`) — correct: adding `#target`
  to a page that merely mentions it would be wrong. `TestBodyRewriteMergeGolden` PASS.

### TM-03 — Tag Delete — **PASSED** (MEDIUM structural + Empirical tests)

`handleTagDelete` (:423), route `router.go:338`.

- **Endpoint + body.** `POST /admin/tags/delete` decodes `{tag, preview}` (:49). Admin-gated.
- **Preview → affected pages + current tags.** `TestTagDelete_PreviewThenApply` asserts `TotalAffected==2`.
- **Approve removes tag from all pages via SetTags + single-writer commit.** `computeDeleteTags` (:169)
  drops the tag; `ApplyTagsBatch` commits. Test: `PagesByTag("ops")==0`, unrelated `docs` untouched (==1).
- **Tag node removed (no orphan).** Derived-from-`page_tags` → node auto-disappears (TM-05).
- **Per-page 409 same as TM-01** (shared path).
- **Audit entry.** `ActionTagDelete` (:481-486).
- **No-op on nonexistent tag.** `TestTagDelete_NonexistentTag` → 200 (empty affected set), no write.
- **RBAC.** `TestTagAdmin_EditorForbidden` asserts editor → 403 on rename/merge/delete.
- **Confirmation dialog with affected-page count** (v1.3 delete-specific security note) — see TM-04.
- **Body rewrite integrated.** `bodyOld=tag, bodyNew=""` (:454); delete needs no new-token validator
  (correct — it only removes). `TestBodyRewriteDeleteGolden` PASS (incl. D-03a whitespace collapse
  `text #ops here`→`text here`).

### TM-04 — Tag Admin UI — **PASSED** (MEDIUM structural + Empirical tests)

`web/src/components/admin/TagManagement.tsx` + `TagManagement.test.tsx` (11 tests, all green in the 804).

- **Tag list w/ page counts.** `useQuery(listTags)` → `TagTable` renders one row per tag (name + count +
  3 action buttons). `GET /api/v1/admin/tags` → `handleTagList` (:492) from `graph.TagPageCounts`.
  Test: "renders one row per tag with name + count + 3 action buttons" (header + 3 rows; 3× each action;
  counts 5/3/1 render).
- **Rename dialog → preview→approve.** `RenameDialog` (:101): input → `renameTag({preview:true})`, then
  confirm flips to "Apply rename" → `renameTag(...)`. Test "rename: preview then apply calls renameTag
  twice and refreshes" asserts both calls + list refresh (`listTags` called twice).
- **Merge dialog → two-tag picker + preview.** `MergeDialog` (:170): `<select>` of other tags →
  preview→apply. Test "merge: preview then apply" asserts both `mergeTag` calls.
- **Delete dialog → confirmation w/ affected count.** `DeleteDialog` (:243) auto-previews on open,
  shows "Remove **tag** from all **N** page(s). This cannot be undone." Test "delete: auto-previews on
  open, then apply calls deleteTag" asserts preview-on-open + apply.
- **Error states.** Every dialog has `error` state (network/preview/apply failures) rendered with an
  `AlertTriangle`. List-level: loading ("Loading tags"), empty ("no tags yet"), error ("Couldn't load
  tags") — all three asserted by tests.
- **Filter/pagination.** Case-insensitive substring filter (tests: narrows to 1 match; "No tags match"
  empty row); preview list is paginated (`slice(0,20)` + "…and N more").
- **Body-scope marker (integrated fix surfaced in UI).** All three dialogs render "N page(s) also have
  body #tags that will be rewritten" when any previewed page has `body_affected`; rename/merge add a
  per-page "prose" badge. Delete dialog's note is at TagManagement.tsx:288-292. Test "delete: preview
  with a body_affected page renders the body #tags marker (TR-05)" asserts it.
- **CSRF.** Client mutations go through `mutate(...)` (client.ts) which carries the CSRF token; the
  server admin subgroup enforces nosurf on the POSTs.

> **Note (Phase-29 LOW findings closed at HEAD):** 29-VERIFICATION.md routed up two LOW items —
> (1a) delete dialog omitted the body marker, (1b) no TagManagement render test. Both are **closed** by
> commit `1435ea6`: the delete-dialog note (TagManagement.tsx:288-292) and `TagManagement.test.tsx`
> (11 tests incl. the body-marker assertion) now exist and pass. Verified on the record here.

### TM-05 — Graph Store Consistency — **PASSED** (MEDIUM structural + Empirical `-race`)

Verified against `internal/graph/graphjob.go`, `query.go`, `rebuild_test.go`, the `enqueueGraphUpsert`
path in `internal/pages/batch.go` + `service.go`. `go test ./internal/graph/... -race` **clean**.

- **Incremental update after batched commit.** After the ONE batch commit lands, `ApplyTagsBatch` fires
  `s.enqueueGraphUpsert(ctx, path)` per committed page (batch.go:221-224). `enqueueGraphUpsert`
  (service.go:354) enqueues `graph.KindGraph` upsert **fire-and-forget** (`worker.Enqueue`, never
  `EnqueueAndWait`) — a save is never blocked by graph maintenance; correctness is guaranteed by the
  idempotent `RebuildGraph` + startup drift backstop (a dropped enqueue reconciles).
- **Reasoned trace of `upsertPage` (graphjob.go:89).** For each committed path: read the (now-rewritten)
  file → `pageTags(doc)` from frontmatter → in ONE tx `DELETE FROM page_tags WHERE page_path=?` then
  re-`INSERT` the current tags (:134-150). So a renamed/merged/deleted page's stale tag rows are removed
  and replaced with the new set atomically per page.
- **Tag rename: old node gone / new node present.** Tag nodes are **derived**, not stored: `PagesByTag`
  (query.go:428) and `TagPageCounts`/`tagNodesAndEdges` (query.go:312-363) read `page_tags` rows. After
  the rewrite, no page carries `old` → its rows are gone → the old tag node has no membership rows → it
  is absent from `emittedTags` (query.go:353-361). `new` appears because its rows now exist.
  `TestTagRename_PreviewThenApply` asserts `PagesByTag("ops")==0`, `PagesByTag("devops")==2` after apply.
- **Tag merge: source node removed, target degree updated.** Same derivation — `source` rows gone,
  `target` membership grows (edge count = degree). Test asserts source 0 / target 3.
- **Tag delete: tag node removed, page tag edges removed.** `computeDeleteTags` drops the tag →
  `page_tags` rows removed on upsert → node + its membership edges vanish. Test asserts delete 0,
  unrelated tag intact.
- **Orphan tag nodes removed.** Structurally guaranteed: a tag with zero `page_tags` rows produces zero
  edges and never enters `emittedTags`, so no orphan tag node can survive (query.go:337-361). No separate
  "prune" step is needed — the derived model makes orphan-removal automatic.
- **Rebuild-from-disk reproduces identical adjacency (the invariant).**
  `TestDerivedOnly_RebuildReproducesAdjacency` (rebuild_test.go:23): rebuild → snapshot links+tags →
  DELETE both tables → rebuild again → `reflect.DeepEqual` byte-identical rows. PASS.
  `TestIncrementalEqualsRebuild` (:68): per-page `upsertPage` (what mutations enqueue) yields the SAME
  tables as a from-scratch `RebuildGraph`. PASS. `TestUpsertReplacesOnlyPageRows`,
  `TestDeletePageRemovesInboundAndOutbound`, `TestUpsertMissingFileIsDelete`, `TestGraphHandler_Dispatch`
  all PASS under `-race`.
- **Substrate honesty on the enqueue path.** The HTTP-flow incremental update is fire-and-forget async;
  the handler *tests* call `RebuildGraph` before asserting graph state (they verify the rebuild path
  reflects the mutation). The **incremental** path is proven separately at the graph unit level
  (`TestIncrementalEqualsRebuild` + `TestGraphHandler_Dispatch` drive `upsertPage`/`UpsertPagePayload`
  directly) and traced above from `enqueueGraphUpsert`. Both the incremental optimization and the
  rebuild backstop are covered; the invariant (rebuild = truth) is the load-bearing one and holds.

---

## Holistic coherence — integrated v1.3 ops + Phase-29 body-rewrite fix — **PASSED**

The regression the milestone fixed: v1.4's inline-`#tag` client materialization silently reverted v1.3
tag ops (a client re-added the removed tag from prose on the next reconcile). Confirmed the combined
surface now works end-to-end, server-authoritatively:

- **A rename durably rewrites BOTH body + frontmatter in one commit.** `handleTagRename` enumerates
  frontmatter pages (`graph.PagesByTag`) UNION body pages (`pages.PagesWithBodyTag`) via `uniqueStrings`
  (:294-304), then `buildTagApplyItems` widens inclusion to `changed || bodyWillChange` (:240) —
  body-only pages are no longer dropped (`SkipFrontmatter` keeps their tags region byte-identical while
  the body is rewritten). Merge (:374-381) and delete (:438-448) do the same.
- **No client re-materialization.** `TestBodyRewriteRenameGolden` (re-run PASS) feeds the committed page
  back through the SAME recognizer (`bodyHasTag`, batch_bodyrewrite_test.go:105) and proves `ops` is gone
  from the body — so a client reconcile (`fmTags ∪ inlineTags`) on the next unchanged-body save cannot
  re-add it. This is the exact regression, closed at the source.
- **Stale-client path a client-only fix cannot cover.** `TestBodyRewriteStaleClient409` (PASS): after the
  server rewrite lands, a stale client saving the PRE-rewrite body at the PRE-rewrite revision is rejected
  `ErrStaleRevision` — the rewrite is NOT clobbered.
- **Single recognizer, no drift.** `okf.FindHashtags`/`RewriteHashtags` is the one body-`#tag` scanner,
  offset-parity-pinned to the Lezer corpus (29-VERIFICATION.md TR-02 confirmed the drift-fails-CI guard
  by perturbation — cross-referenced, not re-run). No third parser.
- **Byte-stability + single-writer invariants intact.** Only tags-region frontmatter + matched body
  `#tag` bytes change (`TestBodyRewriteBodyOnlyGolden`: frontmatter region byte-identical for a body-only
  rewrite); N pages → 1 commit (batch.go:200-213); per-page 409 + audit preserved.

Cross-referenced 29-VERIFICATION.md's TR-03 goldens for the no-re-materialization proof (TR-03/TR-04 not
re-derived — they hold in isolation per that artifact; TC-01 confirms they compose with the v1.3 ops as
one coherent system).

---

## Findings routed UP to the architect

| # | Severity | Finding | Evidence | Recommendation |
|---|----------|---------|----------|----------------|
| 1 | LOW | TM-01 **rename conflict/dedupe** (a page carrying BOTH `old_name` and `new_name` → collapse to `new_name` only) has no dedicated seeded handler/unit test. | `TestTagRename_PreviewThenApply` seeds `a{ops}`, `b{ops,docs}` — neither page carries the target `devops`, so the `containsNormalized(out,newName)` dedup branch (handlers_tags_admin.go:132) is not exercised for rename. | Non-blocking, test-granularity only. **Mitigated:** the logic is correct by trace, and the *identical* dedup shape IS covered for merge (`c.md{ops,devops}` in `TestTagMerge_PreviewThenApply` asserts no duplicate + correct collapse). Optional Phase-30+ follow-up: add a rename test seeding a page with both tags and asserting a single deduped `new_name`. |

No HIGH/blocking correctness findings. (Security is TC-02's lane — `Phase30Security` owns the block-on-high
review of RBAC/CSRF, inline re-validation, byte-stable writes, RewriteHashtags input surface, and the
enumeration read path.)

## Tree state

CLEAN. `internal/web/dist/.gitkeep` restored after `npm run build` (`git checkout`). Final
`git status --porcelain` shows only `internal/server/zz_sec_probe_test.go` untracked — that is
Phase30Security's probe file, NOT a qa change. HEAD unchanged (`c343a30` docs / `1435ea6` code). No qa
source edit made.
