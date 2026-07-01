---
author: architect
responsible: architect
phase: 30
phase_type: milestone_close
status: Ready for planning
hard_bar: true
security_relevant: true
design_fork: false
parent_artifacts:
  - .planning/ROADMAP.md
  - .planning/REQUIREMENTS.md
  - .planning/milestones/v1.3-REQUIREMENTS.md
---

# Phase 30: v1.3 Closure (verify + security + docs reconcile) - Context

**Gathered:** 2026-07-01
**Status:** Ready for planning

<domain>
## Phase Boundary

Backfill the closure v1.3 Tag Management never received. v1.3 shipped as working code
(commit `9de5077`, 2026-06-28) with no phase 19/20 dirs, **no independent qa verification, no
security review, no milestone audit**. This phase closes that gap for the WHOLE tag surface as it
now stands (v1.3 endpoints + admin UI + the Phase-29 body-rewrite fix). It is the milestone-close
verification/security spine; the MILESTONE-AUDIT + docs/codebase-map refresh are the close steps
that follow this phase.

**Requirements:** TC-01 (independent qa verification), TC-02 (independent security review), TC-03
(reconcile planning docs to reality).

**Non-goals:** new tag features; re-verifying Phase 28 (already verified in 28-VERIFICATION.md);
re-doing Phase 29's own qa verify (29-VERIFICATION.md covers TR-01…06) — TC-01 ADDS the v1.3
TM-01…05 verification never done + a holistic re-check of the whole tag surface as one system.

**This is the closure of the milestone's central purpose** (the milestone exists partly *because*
v1.3 shipped outside the workflow). The record must become honest.
</domain>

<decisions>
## Implementation Decisions

### TC-01 — independent qa verification (D-01)
- **D-01:** An independent qa pass verifies, goal-backward against CODE REALITY (re-read + re-run
  suites, not trust a SUMMARY): every v1.3 requirement — **TM-01 rename, TM-02 merge, TM-03 delete,
  TM-04 admin UI, TM-05 graph-store consistency** — AND that the Phase-29 fix (TR-01…06) holds as an
  integrated whole. Phase 29's 29-VERIFICATION.md already verified TR-01…06 in isolation; TC-01's
  NEW work is the v1.3 TM-01…05 verification (never done) + confirming the combined surface (v1.3
  admin ops + the body-rewrite fix) is coherent. Each item passes or a gap routes UP to the architect.
  Recorded in a `30-VERIFICATION.md` artifact.
- **D-01a:** TM-05 (graph-store consistency) specifically: after rename/merge/delete the derived graph
  reflects the new tag state (old node gone, target degree updated, orphan tag nodes removed) and a
  rebuild-from-disk reproduces identical adjacency. Verify against the graph tests + a reasoned trace,
  not just endpoint 200s.

### TC-02 — independent security review (the milestone-close gate) (D-02)
- **D-02:** An independent security review (ASVS L1, **block-on-high per project config**) covers the
  tag surface as a whole, following the v1.4-SECURITY-REVIEW.md format (threat-model table → findings
  most-severe-first with typed `finding`/`evidence` blocks → risk verdict). Scope (from TC-02):
  - **Admin-only RBAC + CSRF** on all tag-admin endpoints (rename/merge/delete/list) — role from
    session, never client input; nosurf CSRF on mutations.
  - **Server-side inline-tag re-validation** — `new_name`/`target` grammar-constrained by
    `ValidateInlineTag` so no space/metachar/all-digit token can be spliced into a body; confirm the
    `ValidateTagCharset`-vs-inline-grammar gap (the Phase-29 blocker) is genuinely closed.
  - **Byte-stable frontmatter + body writes** — no YAML injection, no Markdown injection, no prose
    corruption, no malformed document from a crafted tag/body.
  - **`RewriteHashtags` untrusted-input surface** — a crafted tag/body cannot break out of a code
    region or inject Markdown (the conservative region-aware scanner is the control).
  - **The NEW body-inclusive enumeration read path (TR-04)** — `PagesWithBodyTag`/`DetectStaleBodyTags`
    must go through the SEC-01 resolver with the SAME skip rules as the graph rebuild (no path
    traversal, no trashed/non-`.md` reads, no resolver bypass).
  - Verdict recorded (`SHIP` / findings by severity); **high/critical block** milestone completion.
- **D-02a:** Written to `.planning/v1.5-SECURITY-REVIEW.md` (milestone security review, matching the
  v1.4 file's location + format). Range: the v1.5 commit span on `main`.

### TC-03 — reconcile planning docs to reality (D-03)
- **D-03:** Correct the docs that falsely describe v1.3 as "deferred / not built":
  - `.planning/PROJECT.md`, `.planning/STATE.md`, `.planning/MILESTONES.md` reflect that v1.3 Tag
    Management **shipped** (`9de5077`), was regression-fixed + verified + security-reviewed in v1.5, and
    is no longer "deferred."
  - The v1.3 archive (`.planning/milestones/v1.3-REQUIREMENTS.md`, `v1.3-ROADMAP.md`) is annotated with
    a closure note so the historical record is coherent — v1.3 built outside the workflow, closed
    retroactively in v1.5.
- **D-03a:** TC-03 is the architect's docs authorship (this is a planning-doc reconcile, distinct from
  the archaeologist's `docs/` + codebase-map refresh at milestone close). Keep the two separate.

### Span ownership (D-04)
- **D-04:** TC-01 is the **qa** span (independent verify), TC-02 is the **security-engineer** span
  (independent review), TC-03 is the **architect** span (planning-doc reconcile). Gaps/findings from
  TC-01/TC-02 route UP to the architect, who decides remediation (a gap-closure plan back to the
  engineer, a re-scope, or a tracked-debt note). High/critical security findings BLOCK.

### The agent's Discretion
- The exact structure of the 30-VERIFICATION.md (per-requirement table + evidence).
- Whether TM-05 graph consistency needs a fresh test run or a reasoned trace over existing green tests.
- The security review's depth allocation (the body-rewrite + enumeration are the NEW surface — deepest focus there; the v1.3 RBAC/CSRF is established but must still be confirmed on the record since it was never reviewed).
</decisions>

<disciplines>
## Lenses surfaced for this phase

> **Lenses, not rules.** Adopt, refine, or reason past them.

~~~yaml
disciplines:
  - section: "TC-01"
    name: "verify-against-reality-not-summary"
    scope: ["the whole tag surface"]
    guidance: "Re-read the code on disk and RE-RUN the suites. A passing SUMMARY (incl. Phase 28's
      and Phase 29's) is not evidence for TC-01 — it is a claim to independently confirm. v1.3
      shipped with NO verification precisely because someone trusted that it worked; do not repeat
      that."
    rationale: "The milestone exists because unverified code shipped. TC-01's whole value is being
      the independent check that never happened."
  - section: "TC-02"
    name: "untrusted-tag-into-prose (block-on-high)"
    scope: ["ValidateInlineTag", "RewriteHashtags", "the enumeration read path", "tag-admin endpoints"]
    guidance: "A rename/merge target is untrusted input spliced into PROSE and enumerated via a body
      read. Enumerate sources→sinks→sanitizers: source = the admin's tag string + page bodies; sinks =
      on-disk .md bytes (YAML region + body) and the enumeration file reads; sanitizers = ValidateInlineTag
      + the conservative region-aware scanner + the SEC-01 resolver. A high/critical finding BLOCKS
      milestone completion — severity follows evidence."
    rationale: "Body injection is a strictly larger surface than v1.3's frontmatter-only writes, and
      v1.3's surface was never security-reviewed at all. This is the milestone-close gate."
  - section: "TC-03"
    name: "the-record-must-become-honest"
    scope: [".planning/PROJECT.md", ".planning/STATE.md", ".planning/MILESTONES.md", ".planning/milestones/v1.3-*"]
    guidance: "Correcting the 'deferred' claim is part of CLOSING the milestone, not cosmetic — the docs
      are load-bearing for the next milestone's start. State plainly: v1.3 shipped 9de5077, closed
      retroactively in v1.5."
    rationale: "A dishonest record propagates: the next milestone starts from these docs."
~~~

</disciplines>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before verifying/reviewing.**

### The bar
- `.planning/ROADMAP.md` §"Phase 30: v1.3 Closure" — goal, success criteria.
- `.planning/REQUIREMENTS.md` §TC-01…TC-03 — acceptance criteria.
- `.planning/milestones/v1.3-REQUIREMENTS.md` §TM-01…TM-05 — the v1.3 bar TC-01 verifies (rename/merge/delete/admin-UI/graph-consistency, each with its own acceptance + security criteria).

### Prior verification/review to build on (do NOT re-do)
- `.planning/phases/28-reading-shell-fold-in/28-VERIFICATION.md` — Phase 28 already verified (RS-01…04 passed).
- `.planning/phases/29-tag-regression-fix/29-VERIFICATION.md` — Phase 29 already verified (TR-01…06); TC-01 confirms the integrated whole, not a re-run of this.
- `.planning/v1.4-SECURITY-REVIEW.md` — the FORMAT + rigor precedent for the v1.5 security review (threat-model table, typed finding/evidence blocks, risk verdict, block-on-high).

### The surface under review
- v1.3 tag-admin: `internal/server/handlers_tags_admin.go` (rename/merge/delete/list + RBAC + CSRF), `internal/server/handlers_tags_admin_test.go` (RBAC/preview/apply/validation tests), `web/src/components/admin/TagManagement.tsx`.
- Phase-29 fix: `internal/okf/hashtag.go` (RewriteHashtags), `internal/okf/validate.go` (ValidateInlineTag), `internal/pages/batch.go` (body channel), `internal/pages/tagscan.go` (PagesWithBodyTag + DetectStaleBodyTags).
- Graph consistency (TM-05): `internal/graph/query.go`, `internal/graph/query_test.go`, the enqueueGraphUpsert path in `internal/pages/batch.go`.
- Path safety: `internal/repo/path.go` (the SEC-01 resolver the enumeration must use).

### Docs to reconcile (TC-03)
- `.planning/PROJECT.md`, `.planning/STATE.md`, `.planning/MILESTONES.md`, `.planning/milestones/v1.3-REQUIREMENTS.md` + `v1.3-ROADMAP.md` (annotate).
</canonical_refs>

<code_context>
## Existing Code Insights

### Established (verify on the record; do not assume)
- v1.3 tag-admin RBAC: the endpoints sit behind the admin subgroup (`RequireRole(admin)` + nosurf CSRF)
  per handlers_tags_admin.go's header comment; handlers_tags_admin_test.go asserts editor/reader → 403.
  Confirm this on the record (it was never security-reviewed).
- `SetTagsFrontmatter` (batch.go:214) is the byte-stable frontmatter write; Phase 29 added the body
  channel alongside it. Byte-stability is golden-tested (batch_bodyrewrite_test.go).
- The SEC-01 resolver (`repo.Resolve`, internal/repo/path.go) is the path-safety control — layered
  lexical `..`/NUL/absolute reject + percent-decode + os.Root + EvalSymlinks (per the v1.4 review).

### Landmines
- TC-01 must NOT rubber-stamp Phase 28/29's SUMMARYs — those are claims. The value is the independent
  re-run + re-read. If `anvil-cc measure refusal-rate` reads a long clean streak here, that is a signal
  to check the verify spine is actually exercising the work.
- The security review must confirm the `ValidateTagCharset`-vs-`ValidateInlineTag` distinction is real
  (a crafted `new_name` like `a b` or `1984` or `../etc` must be rejected 400 before any body splice).
- SMTC daemon OFFLINE — cert tiers capped at Structural/Honest; a grep-only claim about what code does
  is MEDIUM, never HIGH. State the substrate honestly in both artifacts.

### The regression, closed (context for TC-01)
- The v1.4 inline-#tag materialization silently reverted v1.3 tag ops; Phase 29 fixed it
  server-authoritatively. TC-01 confirms the v1.3 ops now durably work end-to-end (rename/merge/delete
  → body + frontmatter → no client re-materialization of the removed tag).
</code_context>

<specifics>
## Specific Ideas
- The security review's deepest focus is the NEW surface (RewriteHashtags untrusted-input + the
  body-inclusive enumeration read path); the v1.3 RBAC/CSRF is established but must still be confirmed
  on the record (v1.3 got no review at all).
- TC-01's TM-05 check can lean on the existing green graph tests + a reasoned trace of the
  enqueueGraphUpsert path after a batched tag commit, rather than a fresh harness.

## Deferred Ideas
- The v1.3-original out-of-scope tag features (bulk multi-tag, metadata, hierarchy, dedicated undo,
  analytics) remain deferred (REQUIREMENTS Future) — TC-01 does NOT verify them.
- The v1.4-carried follow-ups (F-02 JSON body cap, F-01 pin mermaid deps) are not v1.5 scope; the
  security review MAY note if the body-rewrite path re-touches them, else leave them tracked.

*Phase: 30-v13-closure*
*Context gathered: 2026-07-01*
