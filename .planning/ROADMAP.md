# Roadmap: OKF Workspace

Milestone **v1.5 — Tag Regression Fix, Reading-Shell Fold-In & v1.3 Closure**. Three phases
(28–30), an **assess-and-fix** milestone: fix a confirmed cross-milestone regression, fold in a
large uncommitted reading-shell refactor, and formally close the v1.3 Tag Management feature that
shipped as working code *outside the GSD workflow*.

> Phases continue numbering from v1.4's last (Phase 27). Phases 19–20 were the *planned* v1.3
> phase numbers; v1.3 shipped as code (`9de5077`) without those phase dirs, so v1.5 does not reuse
> 19/20 — it continues at 28 and closes v1.3 retroactively (Phase 30).

## Milestones

- ✅ **v0.9.9 MVP** — Phases 0–7 (shipped 2026-06-23)
- ✅ **v1.0 Knowledge Graph & LLM Auto-Tagging** — Phases 8–12 (shipped 2026-06-24)
- ✅ **v1.1 Hardening & Polish** — Phases 13–17 (shipped 2026-06-27)
- ✅ **v1.2 Graph Schema Extension** — Phase 18 (shipped 2026-06-28)
- ✅ **v1.3 Tag Management UX** — built `9de5077` (2026-06-28); **closed retroactively in v1.5**
- ✅ **v1.4 Editor Enhancements & Authoring Polish** — Phases 21–27 (shipped 2026-07-01)

**Coverage:** every v1.5 requirement (RS-01…04, TR-01…05, TC-01…03) maps to exactly one phase.
3 phases · all covered.

## Phases

| #  | Phase | Goal | Requirements | Depends on |
|----|-------|------|--------------|------------|
| 28 | Reading-Shell Fold-In | Commit the uncommitted Obsidian-parity refactor as a verified baseline; close the search-test gap; document scope cuts; smoke the CM6 coordinate change | RS-01, RS-02, RS-03, RS-04 | — |
| 29 | Tag-Regression Fix (server-authoritative) | `okf.RewriteHashtags` body-rewrite primitive (Lezer-parity-pinned); rename/merge/delete rewrite body `#tags`+frontmatter in one commit; body-inclusive enumeration; preview surfaces body scope | TR-01, TR-02, TR-03, TR-04, TR-05 | 28 |
| 30 | v1.3 Closure (verify + security + docs reconcile) | Independent qa verification of the shipped tag-admin surface + the fix; security review of the tag-admin + body-rewrite surface; reconcile planning docs to reality | TC-01, TC-02, TC-03 | 29 |

**Risk-first ordering rationale:** the milestone's central risk is the Phase-29 design fork
(a Go `#tag` scanner that must not drift from the TS Lezer grammar). That risk is retired *inside*
Phase 29 by landing the primitive + the shared parity test (TR-01 → TR-02) **before** wiring it into
the three admin operations. Phase 28 comes first only because a clean, committed baseline is a hard
prerequisite for isolating and verifying the surgical Phase-29 commits — Phase 28 is itself low-risk
(the code is written and green; the work is stabilize → test-backfill → commit → document).

**Shared-surface note (merge coordination):** the uncommitted refactor touches `router.go`,
`internal/graph/query.go`, `handlers_graph.go`, `web/src/api/client.ts`, and the reading-column
components — but does **NOT** touch `handlers_tags_admin.go`, `internal/pages/batch.go`, `internal/okf/*`,
or `web/src/lib/tag-reconcile.ts` (the core Phase-29 write surface). **One overlap to note:** the
refactor edits `internal/graph/query.go`, and Phase-29's enumeration (TR-04) reads tags — but TR-04
deliberately places its body-inclusive scan in the **pages service** (not `query.go`, whose "reads NO
bodies" invariant it must not break), so committing Phase 28 first fully separates the two. The fix's
core write surface is otherwise orthogonal to the refactor.

---

## Phase 28: Reading-Shell Fold-In

**Goal:** Commit the large uncommitted Obsidian-parity reading-shell refactor as a coherent, verified
baseline, close its one material test-coverage gap (the search redesign), document the two intentional
scope reductions it makes, and confirm the manual-QA-only editor coordinate change in a real browser.
After this phase, subsequent work builds on committed HEAD, not a working-tree blob.

**Requirements:** RS-01, RS-02, RS-03, RS-04

### Context

A ~64-file refactor sits uncommitted on top of the v1.4-complete commit (`95ca67d`). It is a
*consolidation* (net −2176/+420): the reading column becomes a document (folder Breadcrumb + inline
title + Obsidian-style `PropertiesPanel` + word/char/backlink `DocStatusBar`), the ⌘K modal search
becomes an inline navrail full-text surface (`NavSearchResults` + `stores/navSearch.ts`), and the
per-page local-graph feature is removed end-to-end (Go `Neighborhood`/`linkNeighbors` + `/graph/local`
route + handler + TS client + `LocalGraphPanel`/`DepthControl` + tests — grep-clean, zero dangling
refs). It is coherent and green today (`go test ./...`, `tsc -b`, `vitest 789/62`, `npm run build`).
Two gaps must close before it is a trustworthy baseline: the search redesign dropped 196 lines of
`SearchPalette` tests with no replacement (`NavSearchResults`/`navSearch` are untested), and a bundled
CM6 selection/coordinate change (`drawSelection` + `lineWrapping` + heading `margin`→`padding`) is not
unit-testable and needs a real-browser smoke.

### Success Criteria

1. **Verify BEFORE committing (enforced order):** the sequence is (a) confirm the suites are green,
   (b) backfill the `NavSearchResults`/`navSearch` tests (criterion 3), (c) run the RS-04 browser smoke
   (criterion 5) — THEN commit. No unverified code enters the load-bearing baseline (the exact
   "committed-then-breaks" pattern this milestone exists to fix — v1.3 did precisely that).
2. Full suites green at commit: `go build/vet/test ./...`, `tsc -b`, `vitest`, and `npm run build`
   (with `assertLazyChunks` active) all pass; the refactor lands as feature-scoped commits, with the
   manual-QA-only CM6 selection/coordinate change (`theme.ts` margin→padding + `drawSelection`/
   `lineWrapping`) landed as its **own independently-revertable commit**, not buried in the blob.
3. The canonical frontmatter-tag model is confirmed intact after the `TagBadgeBar`→`PropertiesPanel`
   move: tags = `readTags(frontmatter) ∪ inlineTagsFromBody(body)`, inline-origin tags carry no ✕, only
   non-inline tags are removable (byte-stable `setTags`).
4. `NavSearchResults` + `stores/navSearch.ts` have tests covering grouping, heading navigate-and-scroll
   with query-clear, loading/error/empty states, and ⌘K focusing the navrail input.
5. The two scope reductions (in-place tree-filter removed; per-page local-graph removed) are documented
   in `docs/` + PROJECT.md Key Decisions; stale references (`AppShell` "top bar" comment,
   `PageEditor.test.tsx` "TagBadgeBar" descriptions) corrected.
6. A manual browser smoke confirms drag-select + copy across rendered constructs and correct caret
   placement around headings / on wrapped lines (no selection drift); result recorded in the SUMMARY.

### Key Design Decisions

- **Fold, don't re-design.** The Obsidian-parity direction, local-graph removal, and search redesign
  are accepted as-is; this phase stabilizes and documents them, it does not re-litigate them.
- **Commit before the fix.** A clean committed baseline is what lets Phase 29's surgical commits be
  isolated and independently verifiable — mixing them with a 64-file blob is a merge/verification hazard.
- **Close only the material gap.** The single highest-value coverage gap is the untested search redesign
  (the largest new interactive surface); backfill that. Other new components (PropertiesPanel, Breadcrumb,
  DocStatusBar) already have tests.

### Key Files

- Commit surface: all working-tree changes (see `git status`).
- Test-backfill: `web/src/components/search/NavSearchResults.{tsx,test.tsx}`, `web/src/stores/navSearch.ts` (+test).
- Docs: `docs/` (architecture/nav + graph subsystem notes), `.planning/PROJECT.md` Key Decisions.
- Stale refs: `web/src/routes/AppShell.tsx`, `web/src/routes/PageEditor.test.tsx`.
- Smoke target: `web/src/components/LivePreviewEditor.tsx`, `web/src/lib/cm/theme.ts`.

### Dependencies

- v1.4 close (Phase 27 / `95ca67d`) — the base commit the refactor sits on.

---

## Phase 29: Tag-Regression Fix (server-authoritative)

**Goal:** Make workspace tag rename / merge / delete durable against v1.4's inline-`#tag`
materialization by rewriting affected pages' body `#tags` server-side, in the same byte-stable batched
commit as the frontmatter change — via a new code-region-aware `okf.RewriteHashtags` primitive that is
provably in lock-step with the TS Lezer grammar. This closes the confirmed regression at the source for
**all future operations** — including pages never reopened after the operation. Historical corruption
already left by pre-fix ops is handled separately (TR-06), not silently claimed away.

**Requirements:** TR-01, TR-02, TR-03, TR-04, TR-05, TR-06

### Context

Confirmed regression: `handleTagRename`/`handleTagMerge`/`handleTagDelete` rewrite the frontmatter
`tags:` only — `ApplyTagsBatch` hands `doc.Body` back verbatim (`batch.go:133-137`) — while the client
`reconcileFrontmatterTags` re-unions `inlineTagsFromBody(currentBody)` into frontmatter on every save
(`tag-reconcile.ts:78`). So a renamed-away tag that still appears as a literal `#tag` in a page body
**reappears on the next client save**; delete fully reverts. A client-only guard is insufficient (it
can't fix pages edited by a stale/other client or never reopened). The fix is server-authoritative: a
new `okf.RewriteHashtags` byte-scanner modeled on the existing `okf.RewriteLinks`/`FindLinks`
(`links.go:28-83,349`, which already skips fenced/inline-code/escaped regions for links). The central
risk is the Phase-21-warned "second `#tag` parser" drift; the mitigation is a shared corpus parity test
against `web/src/lib/cm/hashtag.ts` (`hashtag.test.ts` positive/negative sets). A second landmine:
`PagesByTag`/`page_tags` is frontmatter-only, so a body-only `#tag` is invisible to the affected-set
query — enumeration must be widened.

### Build order (risk-first, within the phase)

0. **`ValidateInlineTag` validator FIRST** — a strict validator (inline-`#tag` grammar subset: no space,
   ≥1 non-digit, `[A-Za-z0-9_/-]`, slash policy) gates rename `new_name` AND merge `target` at the
   handler (400 on fail). This is a BLOCKER-fix: today `ValidateTagCharset` (`^[a-zA-Z0-9_ -]+$`) permits
   a space / all-digit and forbids slash — incompatible with body injection — and merge/delete call no
   validator at all. Nothing may splice an invalid token into a body.
1. **TR-01 + TR-02** — land `okf.RewriteHashtags` AND its offset-level, whole-document Lezer-parity test
   together (with the FULL exclusion set: fenced/inline/escape PLUS indented-code + HTML-block + ATX
   heading). This retires the central drift risk before any handler depends on the primitive.
2. **TR-04** — body-inclusive enumeration **in the pages service** (reads bodies through the SEC-01
   resolver), NOT in `graph/query.go` (whose "reads NO bodies" invariant must hold).
3. **TR-03** — wire the primitive + enumeration into rename/merge/delete. **Load-bearing coupling
   (BLOCKER-fix):** the batch inclusion predicate must widen to `(frontmatterChanged OR bodyWillChange)`
   computed from the batch's freshly-read body, AND `ApplyTagsBatch`/`TagApplyItem` must gain a
   body-rewrite channel — otherwise body-only pages are enumerated then silently dropped at
   `changed=false` (the current `buildTagApplyItems` behavior).
4. **TR-05** — surface the body-rewrite scope in the preview/admin UI.
5. **TR-06** — the stale-body remediation (detect+report at minimum) for pre-fix historical corruption.

### Success Criteria

1. `okf.RewriteHashtags(body, old, new|"")` rewrites body `#tag` occurrences (rename/merge/delete),
   skips **every** region the Lezer parser excludes (fenced ``` / ~~~, inline-code, `\#`-escape,
   indented code, HTML block, ATX heading), is **conservative (skip-when-uncertain)**, and is
   byte-identical on no-match.
2. An **offset-level, whole-document parity test** (expected results generated by the real Lezer parser
   over block-context fixtures) proves the Go scanner and TS grammar agree on spliced output, not just
   tag-set membership; single-sourced corpus; a Lezer-corpus addition the Go scanner misses fails CI.
3. Rename `new_name` + merge `target` pass `ValidateInlineTag` (400 on fail) before any write; no
   space/metachar/all-digit token can reach a body.
4. Rename/merge/delete rewrite affected pages' body `#tags` AND frontmatter `tags:` in one byte-stable
   `ApplyTagsBatch` commit (N pages → 1 commit); the inclusion predicate is `(frontmatterChanged OR
   bodyWillChange)` so **body-only pages still get written**; only tags-region frontmatter lines and
   matched body `#tag` bytes change.
5. **Regression closed (golden tests):** rename `#ops`→`#devops` on a page with body `#ops`, then run the
   client `reconcileFrontmatterTags` for the next save (unchanged body) → byte-stable, `ops` does NOT
   reappear. Equivalent goldens for merge, delete, AND a body-only page. Plus a **stale-open-client 409**
   integration test (a save carrying the pre-rewrite body at the pre-rewrite `base_revision` is rejected,
   never clobbering the rewrite).
6. Affected-set enumeration (in the pages service, via the SEC-01 resolver) finds body-only pages;
   preview reflects the true set and distinguishes body-affected pages; per-page 409 + audit logging
   preserved. TR-06 remediation detects+reports (or fixes) pre-fix stale-body corruption.

### Key Design Decisions

- **Validate before injecting.** The inline-tag grammar validator is the first gate — the charset the
  admin API accepts must be a subset of what the body grammar can represent, or a rename corrupts prose.
- **Server-authoritative, one commit.** Body + frontmatter change together through the existing
  single-writer byte-stable batched path — immediately consistent with the graph rebuild.
- **Offset-parity, conservative Go scanner.** `RewriteHashtags` is NOT a general Markdown re-impl; it
  targets the exact conservative `#tag` subset the Lezer parser recognizes, skips every structural region
  Lezer excludes, and skips-when-uncertain (a miss leaves a stale tag, self-healing — never corruption).
  The offset-level whole-document parity test is the anti-drift guarantee.
- **Enumeration in the pages service.** The body scan reads through the SEC-01 resolver in the pages
  service, not the lean `graph/query.go` layer whose no-body-reads invariant must hold.
- **Keep `reconcileFrontmatterTags`.** The frontmatter-canonical sync-at-save model (ED-02b) stays; the
  server fix makes it naturally no-op on renamed pages (no stale body `#tag` left to re-inject).

### Key Files

- New: `internal/okf/` — `RewriteHashtags` primitive + `ValidateInlineTag` (patterns: `internal/okf/links.go`,
  `repair.go`) + tests.
- Parity: a shared `#tag` corpus fixture consumed by both the Go test and `web/src/lib/cm/hashtag.test.ts`,
  with expected results generated by the real Lezer parser.
- Wire-in: `internal/server/handlers_tags_admin.go` (rename/merge/delete + validator), `internal/pages/batch.go`
  (`ApplyTagsBatch`/`TagApplyItem` gain a body-rewrite channel; widened change predicate), the enumeration
  scan in the **pages service** (not `graph/query.go`).
- Preview UI: `web/src/components/admin/TagManagement.tsx`, `web/src/api/client.ts`.

### Dependencies

- Phase 28 (committed baseline) — for isolated, verifiable commits.
- Existing v1.3/v1.0 infrastructure: `SetTagsFrontmatter`, `ApplyTagsBatch`, `okf.RewriteLinks` pattern,
  the Lezer `hashtag.ts` grammar + corpus.

---

## Phase 30: v1.3 Closure (verify + security + docs reconcile)

**Goal:** Backfill the closure v1.3 Tag Management never received — an independent goal-backward qa
verification of the shipped tag-admin surface plus the v1.5 fix, an independent security review of the
tag-admin + new body-rewrite surface (the milestone-close security gate), and a reconciliation of the
planning docs that falsely described v1.3 as "deferred / not built."

**Requirements:** TC-01, TC-02, TC-03

### Context

v1.3 shipped as working code (`9de5077`, 2026-06-28) with no phase 19/20 dirs, no independent
verification, no security review, and no milestone audit. This phase closes that gap for the whole tag
surface as it now stands (v1.3 endpoints + admin UI + the Phase-29 body-rewrite fix). It is the
milestone-close verification/security spine; the milestone-audit + docs/codebase-map refresh are the
close steps that follow phase completion.

### Success Criteria

1. Independent qa verification: every v1.3 requirement (TM-01 rename, TM-02 merge, TM-03 delete, TM-04
   admin UI, TM-05 graph consistency) and every v1.5 fix requirement (TR-01…05) is verified goal-backward
   against code reality with the full suites re-run; each passes or a gap is routed up. Recorded in a
   `VERIFICATION` artifact.
2. Independent security review (ASVS L1, block-on-high per project config) covers the tag-admin endpoints
   (RBAC + CSRF + server-side tag re-validation), the byte-stable body+frontmatter writes (no YAML /
   Markdown injection, no prose corruption, no path escape), and the untrusted-input surface of
   `RewriteHashtags`. Verdict recorded; high/critical block.
3. PROJECT.md, STATE.md, and MILESTONES.md reflect that v1.3 shipped and was closed in v1.5 (no longer
   "deferred"); the v1.3 archive is annotated with a closure note so the historical record is coherent.

### Key Design Decisions

- **Verify against code REALITY, not summaries.** The verification re-reads the tag surface as it is on
  disk and re-runs the suites; a passing SUMMARY is not evidence.
- **Security is a required close gate here**, not optional — v1.3 skipped it, and the Phase-29 body
  rewrite adds a new untrusted-input surface (a crafted tag/body must not break out of a code region,
  inject Markdown, or produce a malformed document).
- **The record must become honest.** Correcting the "deferred" claim is part of closing the milestone,
  not cosmetic — the docs are load-bearing for the next milestone's start.

### Key Files

- `.planning/phases/30-*/` — VERIFICATION artifact.
- `.planning/v1.5-SECURITY-REVIEW.md` (milestone security review).
- `.planning/PROJECT.md`, `.planning/STATE.md`, `.planning/MILESTONES.md`, `.planning/milestones/v1.3-*` (annotation).

### Dependencies

- Phase 29 (the fix must exist to be verified + security-reviewed).
- Phase 28 (the committed baseline the verification runs against).

---

## Progress

| Phase | Milestone | Plans | Status | Completed |
| ----- | --------- | ----- | ------ | --------- |
| 18. Graph Schema Extension | v1.2 | 1/1 | Complete | 2026-06-28 |
| 21–27 (Editor Enhancements) | v1.4 | 7/7 | Complete | 2026-07-01 |
| **28. Reading-Shell Fold-In** | **v1.5** | **0** | **Planned** | — |
| **29. Tag-Regression Fix** | **v1.5** | **0** | **Planned** | — |
| **30. v1.3 Closure** | **v1.5** | **0** | **Planned** | — |

---

## Execution Order

- Phase 28 (baseline) → Phase 29 (fix) → Phase 30 (close). Strictly sequential: 29 needs a clean
  committed base; 30 verifies + security-reviews the fix from 29.
- Within Phase 29, build order is risk-first: `ValidateInlineTag` → TR-01 + TR-02 (primitive +
  offset-level Lezer-parity test) → TR-04 (pages-service enumeration) → TR-03 (wire into
  rename/merge/delete, widened change predicate + body channel) → TR-05 (preview scope) → TR-06
  (stale-body remediation).
