# Milestone v1.5 Requirements — Tag Regression Fix, Reading-Shell Fold-In & v1.3 Closure

## Overview

An **assess-and-fix** milestone. It does three things:

1. **Fixes a confirmed cross-milestone regression.** v1.4's inline-`#tag` materialization
   (shipped 2026-07-01) silently broke the v1.3 workspace tag rename/merge/delete
   operations (committed 2026-06-28): those operations rewrite the frontmatter `tags:`
   only, but the client re-materializes body `#tags` into frontmatter on every save — so
   a renamed/merged/deleted tag that still appears literally as `#tag` in a page body
   **reappears on the next client save**, silently undoing the admin operation.
2. **Folds in a large uncommitted reading-shell refactor** already present in the working
   tree (Obsidian-parity Properties panel + Breadcrumb + DocStatusBar; inline nav search
   replacing the ⌘K modal; local-graph feature removed). It is coherent and green
   (`go test`, `tsc -b`, `vitest` all pass) but must be committed as a clean baseline,
   have its test-coverage gaps closed, and its intentional scope reductions documented.
3. **Formally closes v1.3 Tag Management**, which shipped as working code *outside the GSD
   workflow* — no phase 19/20 dirs, **no independent qa verification, no security review,
   no milestone audit**. This milestone backfills that closure so the shipped tag-admin
   surface is verified, security-reviewed, and the planning docs reflect reality.

**Locked design decisions (from kickoff analysis + user ratification):**

- **The regression fix is SERVER-AUTHORITATIVE.** A new `okf.RewriteHashtags` primitive,
  modeled on the existing code-region-aware `okf.RewriteLinks` byte-scanner, rewrites body
  `#tag` text (rename → retag; merge → retag to target; delete → remove) in the **same
  byte-stable commit** as the frontmatter change. A client-only guard is insufficient
  (it cannot fix never-reopened pages or pages edited by a stale/other client).
- **The Go tag-boundary scanner MUST NOT drift from the TS Lezer grammar.** This partially
  reopens v1.4's "no second server-side `#tag` parser" decision; the mitigation is a
  **shared corpus parity test** pinning the Go scanner's tag-boundary behavior against the
  canonical `web/src/lib/cm/hashtag.ts` grammar corpus (`hashtag.test.ts` positive/negative
  sets). The Go scanner targets the same conservative subset the Lezer parser recognizes.
- **Affected-set enumeration must not rely on the frontmatter-only `page_tags` cache alone.**
  A page carrying `#tag` in its body but not (yet) in frontmatter is invisible to
  `PagesByTag`; enumeration for a body rewrite must also cover body occurrences so no
  affected page is missed.
- **All existing tag-admin safety invariants are preserved:** admin-only RBAC + CSRF,
  server-side tag re-validation, byte-stable single-writer batched commit (N pages → 1
  commit), per-page stale-revision (409) handling, audit logging.
- **The refactor is folded, not re-designed.** v1.5 commits and hardens the working-tree
  refactor as-is; it does not re-litigate the Obsidian-parity direction, the local-graph
  removal, or the search redesign. Gaps (missing tests, undocumented scope cuts) are closed.

Phases continue from v1.4's last (Phase 27) → **v1.5 spans Phases 28–30** (3 phases).

---

## RS — Reading-Shell Refactor Fold-In *(Phase 28)*

### RS-01: Commit the reading-shell refactor as a clean, verified baseline
**Requirement:** The uncommitted working-tree refactor is committed as a coherent baseline
after its coherence is re-verified, so subsequent phases build on committed HEAD.
**Acceptance criteria:**
- [ ] Full suites green pre-commit: backend `go build ./... && go vet ./... && go test ./...`;
      frontend (from `web/`) `tsc -b` clean, `vitest` all pass, `npm run build` clean with the
      `assertLazyChunks` guard active.
- [ ] The refactor is committed (feature-scoped message) — Properties panel + Breadcrumb +
      DocStatusBar reading shell, inline nav search, and the end-to-end local-graph removal.
- [ ] The canonical frontmatter-tag model is confirmed preserved: `PropertiesPanel` rebuilds
      the exact `readTags(frontmatter) ∪ inlineTagsFromBody(body)` union with inline-origin
      tags carrying no ✕ (body-managed) and only non-inline tags removable via byte-stable
      `setTags` — no tag-model regression from the `TagBadgeBar` deletion.

### RS-02: Close the search-redesign test-coverage gap
**Requirement:** The inline nav-search surface introduced by the refactor is tested to at
least the standard the deleted ⌘K `SearchPalette` was (196 lines of tests were dropped).
**Acceptance criteria:**
- [ ] `NavSearchResults` has a test file covering: Pages/Headings/Attachments grouping,
      heading-result navigate-and-scroll (rehype-slug id) with query-clear, and the
      loading / error / empty states.
- [ ] `stores/navSearch.ts` has coverage for its query state transitions (set / clear;
      tree-returns-when-empty).
- [ ] ⌘K focuses the navrail search input (opens nav first) — asserted.

### RS-03: Document the intentional scope reductions
**Requirement:** The two behavior removals the refactor makes are recorded so they read as
decisions, not accidental loss.
**Acceptance criteria:**
- [ ] The **in-place file-tree filter** removal (search now replaces the tree with full-text
      results rather than pruning it) is documented in `docs/` and PROJECT.md Key Decisions.
- [ ] The **per-page local-graph** removal (global `/app/graph` retained; `/graph/local`
      route + `Neighborhood` store method + `LocalGraphPanel`/`DepthControl` deleted) is
      documented in `docs/` and PROJECT.md Key Decisions. Any deferred re-introduction is
      captured as a Future item.
- [ ] Stale references corrected: the `AppShell.tsx` "Search now lives in the top bar"
      comment and the `PageEditor.test.tsx` test descriptions still naming "TagBadgeBar".

### RS-04: Smoke the manual-QA-only editor coordinate change
**Requirement:** The CM6 selection/coordinate change bundled in the refactor
(`drawSelection()` + `EditorView.lineWrapping` + heading `margin`→`padding` in `theme.ts`,
made so CM6's height oracle keeps `posAtCoords`/`coordsAtPos` accurate) is confirmed in a
real browser, since it is not unit-testable.
**Acceptance criteria:**
- [ ] A manual smoke confirms: drag-select + copy across rendered constructs (bold, heading,
      link, code) yields correct text, and caret placement around headings and on wrapped
      lines lands where clicked (no selection drift). Result recorded in the phase SUMMARY /
      VERIFICATION.

---

## TR — Tag-Regression Fix (server-authoritative body rewrite) *(Phase 29)*

### TR-01: `okf.RewriteHashtags` body-rewrite primitive
**Requirement:** A new `okf` primitive rewrites `#tag` occurrences in a Markdown body —
rename (`#old`→`#new`), merge (`#source`→`#target`), and delete (`#tag`→removed) — that is
code-region-aware and byte-stable, modeled on the existing `okf.RewriteLinks`/`FindLinks`
scanner but with the FULL set of Lezer structural exclusions.
**Acceptance criteria:**
- [ ] The scanner skips **every region the Lezer parser excludes**, not just the link-scanner's
      set: fenced code (``` / ~~~), inline-code backtick runs, `\#`-escapes, **indented (4-space/
      tab) code blocks**, **HTML blocks**, and ATX-heading leading `#`. (RewriteLinks/`FindLinks`
      skips only fenced/inline/escape — indented-code + HTML-block skips must be ADDED, since the
      corpus already asserts indented-code exclusion, `hashtag.test.ts` `\t#deploy → []`.)
- [ ] Tag-boundary recognition reuses the exact `scanBody` predicates the grammar documents
      (`hashtag.ts:49-68`): body alphabet `[A-Za-z0-9_/-]`, ≥1 non-digit, nested `#a/b`,
      trailing-`/` trim, start-boundary rule; case-insensitive match, original-case elsewhere
      untouched.
- [ ] **Conservative (skip-when-uncertain):** if the scanner cannot confidently classify a `#`
      run's region, it leaves it UNCHANGED — a miss leaves a stale tag (self-healing on the next
      correct op), never a corrupted code region or malformed document.
- [ ] Byte-stable on no-match: a body with no matching `#tag` is returned byte-identical.
- [ ] **Delete whitespace rule (chosen, not deferred):** delete removes `#`+token and collapses
      exactly one adjacent space **only when the token was space-bounded on both sides**; it never
      touches whitespace otherwise and never merges across a newline. Corpus-tested against
      `see #ops here`, `foo #ops.`, a `#ops`-only line.
- [ ] **Merge adjacent-duplicate rule (chosen):** a merge yielding `#b #b` leaves both tokens
      as-is (byte-safer than collapsing; the frontmatter reconcile dedupes the set anyway).
      Corpus-tested.

### TR-02: Lezer↔Go grammar parity test (offset-level, whole-document)
**Requirement:** A shared corpus test proves the Go `RewriteHashtags` scanner recognizes the
same `#tag` boundaries — at the byte-offset level — as the canonical TS Lezer grammar over WHOLE
documents, preventing the second-parser drift the Phase-21 retirement warns against.
**Acceptance criteria:**
- [ ] Expected results are generated by running the **real Lezer parser** over whole-document
      fixtures (not a hand-transcribed name list), so structural block contexts are covered:
      indented code, HTML blocks, tables, lazy blockquote continuation, `>#tag` block-marker abut,
      plus the full `hashtag.test.ts` positive/negative corpus (`#a/b`, `#_`/`#-`, `#abc`/`#fff`;
      rejects `#1984`, `foo#bar`, `a/#frag`, `\#esc`, bare `#`, `# heading`, `#12/34`, fenced/inline).
- [ ] The test asserts **byte offsets / spliced output**, not merely tag-set equality — proving
      the Go scanner splices the correct span (trailing-`/` trim, nested `#a/b`, adjacent punctuation),
      since `inlineTagsFromBody` returns names without offsets.
- [ ] The corpus is single-sourced (a shared checked-in fixture consumed by both sides); a
      Lezer-corpus addition the Go scanner would miss fails CI.

### TR-03: Rename/merge/delete rewrite body `#tags` in the same commit
**Requirement:** The three admin tag operations rewrite affected pages' body `#tags` via
`RewriteHashtags` AND the frontmatter `tags:` in the one byte-stable batched commit — including
pages whose only occurrence is in the body.
**Acceptance criteria:**
- [ ] **Strict target validation (BLOCKER fix):** `new_name` (rename) AND `target` (merge) MUST
      pass a `ValidateInlineTag` validator — the inline-`#tag` grammar subset (no space, ≥1
      non-digit, `[A-Za-z0-9_/-]`, explicit slash policy) — BEFORE any write, failing 400 at the
      handler. The current `okf.ValidateTagCharset` (`^[a-zA-Z0-9_ -]+$`, permits space + all-digit,
      forbids slash) is INCOMPATIBLE with body injection and must NOT be the sole gate; merge/delete
      (which today call no validator, handlers_tags_admin.go) must validate too.
- [ ] **Body-inclusive change predicate (BLOCKER fix):** the per-page inclusion test in
      `buildTagApplyItems` becomes `(frontmatterChanged OR bodyWillChange)`, where `bodyWillChange`
      is computed by running `RewriteHashtags` on the batch's **freshly-read** body (not the
      handler's stale read). A body-only page (frontmatter unchanged) MUST still produce a write —
      it is currently dropped at `changed=false`.
- [ ] **Body channel through the batch:** `pages.TagApplyItem` / `ApplyTagsBatch` gain a body-rewrite
      step keyed to the op, so `ApplyTagsBatch` no longer writes `doc.Body` verbatim for affected
      pages. Per affected page: one write updates body (via `RewriteHashtags`) AND frontmatter (via
      `SetTagsFrontmatter`), committed through the single-writer path (N pages → 1 commit).
- [ ] Byte-stability preserved: only the tags-region frontmatter lines and the matched body `#tag`
      bytes change; all other bytes identical.
- [ ] **Regression closed (golden test):** rename `#ops`→`#devops` on a page with body `#ops`, then
      run the client `reconcileFrontmatterTags` for the next save (unchanged body) → byte-stable and
      `ops` does **not** reappear (frontmatter `[devops]`, body `#devops`). Equivalent golden tests
      for merge and delete, AND for a body-only page (tag only in body, not frontmatter).
- [ ] **Stale-open-client 409 (the path a client-only fix can't cover):** an integration test asserts
      a save carrying the PRE-rewrite body at the pre-rewrite `base_revision` is rejected 409
      (`ErrStaleRevision`, service.go:235) — never clobbering the server rewrite — pinning the
      invariant the server-authoritative choice depends on.
- [ ] Per-page stale-revision (409) doesn't sink the batch; audit-log entries recorded, reflecting
      body+frontmatter scope.

### TR-04: Body-inclusive affected-set enumeration (in the pages service, not the graph layer)
**Requirement:** The affected-page set includes pages carrying the tag in their body, enumerated
via a body read that respects the SEC-01 path resolver — placed in the pages service, NOT bolted
into the lean graph query layer.
**Acceptance criteria:**
- [ ] Enumeration finds a page whose `#tag` appears in the body even if the tag is not in that
      page's frontmatter (the `PagesByTag`/`page_tags` frontmatter-only blind spot is closed).
- [ ] The body scan lives in the **pages service** (which already reads bodies through the SEC-01
      resolver) — NOT in `internal/graph/query.go`, whose documented invariant is "reads NO bodies —
      only path enumeration." It reuses the same resolver + skip rules as the graph rebuild (live
      `.md` pages only; no trashed/non-`.md` reads; no path escape).
- [ ] No affected page is missed and no unaffected page is rewritten (preview reflects the true set);
      the preview→approve contract holds. A merge whose source is in page A's body and target in page
      B's frontmatter resolves both correctly.
- [ ] Bounded/efficient at ~5-user, modest-page-count scale: a one-shot per-operation scan, never a
      per-keystroke scan.

### TR-05: Preview shows body + frontmatter changes
**Requirement:** The preview step of each operation reflects that bodies will change, so the
admin approves the true diff.
**Acceptance criteria:**
- [ ] The rename/merge/delete preview response distinguishes frontmatter-only changes from pages
      whose body `#tags` will also be rewritten (affected-page list conveys both).
- [ ] The Tag Management admin UI (`TagManagement.tsx`) surfaces the body-rewrite scope in the
      preview so the admin isn't surprised that prose changes.

### TR-06: Remediate stale body `#tags` left by pre-fix operations
**Requirement:** Renames/deletes run between v1.3 (2026-06-28) and this fix already left stale body
`#tags` that the client re-unioned into frontmatter (half-reverted state). The milestone must not
claim "closed for all pages" while that corruption persists.
**Acceptance criteria:**
- [ ] A bounded one-shot remediation scans the workspace for pages whose body `#tags` disagree with
      their frontmatter in a way indicative of a pre-fix rename/delete (e.g. a body `#tag` whose
      frontmatter counterpart is absent because a prior op removed it), and reports them; OR the admin
      is given a way to re-run the affected prior operations through the now-correct path.
- [ ] If full auto-remediation is judged too broad, this requirement is explicitly scoped to
      **detect + report** (not auto-fix), and the ROADMAP over-claim ("for all pages including
      never-reopened ones") is softened to "for all future operations" — the historical corruption is
      surfaced, not silently left. Whichever path is chosen is stated in the phase plan.

---

## TC — v1.3 Tag Management Closure *(Phase 30)*

### TC-01: Independent qa verification of the shipped tag-management surface
**Requirement:** The already-shipped v1.3 tag-admin surface (TM-01…TM-04) plus the v1.5
regression fix are verified goal-backward by an independent pass against code reality, since
v1.3 never received one.
**Acceptance criteria:**
- [ ] Every v1.3 requirement (TM-01 rename, TM-02 merge, TM-03 delete, TM-04 admin UI, TM-05
      graph-store consistency) is verified against the current code + tests, with the full
      suites re-run; each is marked passed or a gap is routed up.
- [ ] The regression-fix requirements (TR-01…TR-06) are verified.
- [ ] A `VERIFICATION` artifact records the verdict and evidence.

### TC-02: Security review of the tag-admin + body-rewrite surface
**Requirement:** The tag-management surface — never security-reviewed in v1.3 — plus the new
`RewriteHashtags` body-rewrite path get an independent security review (the milestone-close
security gate; ASVS L1, block-on-high per project config).
**Acceptance criteria:**
- [ ] The review covers: admin-only RBAC + CSRF on all tag-admin endpoints; server-side
      **inline-tag** re-validation (`new_name`/`target` grammar-constrained — no space/metachar
      token can be spliced into a body; the `ValidateTagCharset` vs inline-grammar gap is closed);
      byte-stable frontmatter + body writes (no YAML/Markdown injection, no prose corruption, no
      malformed document); the untrusted-input surface of `RewriteHashtags` (a crafted tag/body
      cannot break out of a code region or inject Markdown); AND the **new body-inclusive
      enumeration read path** (TR-04) — it must go through the SEC-01 resolver with the same skip
      rules as the graph rebuild (no path traversal, no trashed/non-`.md` reads, no resolver bypass).
- [ ] Verdict recorded (`SHIP` / findings by severity); block-on-high enforced.

### TC-03: Reconcile planning docs to reality
**Requirement:** The planning docs that falsely describe v1.3 as "deferred / not built" are
corrected, and v1.3's closure is recorded.
**Acceptance criteria:**
- [ ] PROJECT.md, STATE.md, and MILESTONES.md reflect that v1.3 Tag Management **shipped**
      (commit `9de5077`), was regression-fixed + verified + security-reviewed in v1.5, and is
      no longer "deferred."
- [ ] The v1.3 requirements/roadmap archive is annotated (or a v1.3 closure note added) so the
      historical record is coherent — v1.3 built outside the workflow, closed retroactively in
      v1.5.

---

## Future Requirements (deferred)

- **Re-introduce a per-page local graph** if it proves missed (removed in the v1.5 refactor).
- **In-place file-tree filter** (narrow-the-tree-by-title) if the full-text-replaces-tree
  model proves worse for quick navigation.
- The v1.3-original "Out of Scope" tag features remain deferred: bulk multi-tag operations,
  tag color/description metadata, tag hierarchy, dedicated undo, tag usage analytics.
- v1.4-carried follow-ups not in this milestone: F-02 (global JSON body `MaxBytesReader` cap),
  F-01 (pin mermaid deps + `npm audit` in CI), the 2 Phase-26 manual UAT items, `govulncheck`
  re-run when the Go 1.26 toolchain panic is fixed.

## Out of Scope (explicit)

- **Re-designing the reading-shell refactor.** v1.5 folds it in as-is; the Obsidian-parity
  direction, local-graph removal, and search redesign are accepted, not re-opened.
- **A client-only regression fix.** Rejected as incomplete — cannot fix never-reopened or
  stale-client pages.
- **A full second Markdown parser in Go.** `RewriteHashtags` is a conservative, corpus-pinned
  `#tag`-boundary scanner (like `RewriteLinks`), NOT a general Markdown/Lezer re-implementation.
- **Removing the client-side `reconcileFrontmatterTags`** — the frontmatter-canonical
  sync-at-save model (ED-02b) stays; the server fix makes it naturally no-op on renamed pages.
- **New tag-management features** (bulk multi-tag, metadata, hierarchy) — deferred as above.

## Traceability

| REQ-ID | Phase | Status |
|--------|-------|--------|
| RS-01, RS-02, RS-03, RS-04 | 28 | planned |
| TR-01, TR-02, TR-03, TR-04, TR-05, TR-06 | 29 | planned |
| TC-01, TC-02, TC-03 | 30 | planned |

**v1.3 requirements re-validated in this milestone (built in `9de5077`, closed here):**

| REQ-ID | Origin | v1.5 disposition |
|--------|--------|------------------|
| TM-01 (rename) | v1.3 | regression-fixed (TR-03) + verified (TC-01) |
| TM-02 (merge) | v1.3 | regression-fixed (TR-03) + verified (TC-01) |
| TM-03 (delete) | v1.3 | regression-fixed (TR-03) + verified (TC-01) |
| TM-04 (admin UI) | v1.3 | body-scope surfaced (TR-05) + verified (TC-01) |
| TM-05 (graph consistency) | v1.3 | verified (TC-01) |
