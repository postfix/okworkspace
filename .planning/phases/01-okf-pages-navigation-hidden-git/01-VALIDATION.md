---
phase: 01
slug: okf-pages-navigation-hidden-git
status: draft
nyquist_compliant: true
wave_0_complete: true
created: 2026-06-18
---

# Phase 01 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (backend) · vitest (frontend) |
| **Config file** | none for Go; `web/vitest.config.ts` (Wave 0 installs if absent) |
| **Quick run command** | `go test ./internal/okf/...` |
| **Full suite command** | `go test ./... -race && (cd web && npm test -- --run)` |
| **Estimated runtime** | ~90 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/okf/...` (the round-trip exit gate) plus the task's own `<automated>` command.
- **After every plan wave:** Run `go test ./... -race && (cd web && npm test -- --run)`.
- **Before `/gsd-verify-work`:** Full suite must be green.
- **Max feedback latency:** 90 seconds.

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 01-01-01 | 01-01 | 1 | PAGE-09 | T-01-03 | Round-trip byte-stability; body never re-serialized through an AST (no Markdown corruption) | unit | `go test ./internal/okf/... -run 'TestGoldenRoundTrip\|TestParse\|TestRepair' -v` | ✅ W0 (corpus + roundtrip_test created here) | ⬜ pending |
| 01-01-02 | 01-01 | 1 | VER-01 | T-01-01 / T-01-02 | All page writes funnel through the single-writer CommitJob (no os.* / handler-side git); paths resolver-gated | integration | `go test ./internal/pages/... ./internal/store/... -run 'TestCommitHandler\|TestMigrat' && go build ./...` | ✅ | ⬜ pending |
| 01-02-01 | 01-02 | 2 | PAGE-01, PAGE-02, PAGE-03, NAV-03 | T-02-01 | Slug/path rejects ../absolute/NUL; 409 floor before write; mutations enqueue via CommitJob only | integration | `go test ./internal/pages/... -race -run 'TestCreate\|TestSave\|TestTree\|TestPushFlag' -v && go test ./internal/okf/...` | ✅ | ⬜ pending |
| 01-02-02 | 01-02 | 2 | PAGE-01, NAV-01 | T-02-02 / T-02-05 | Editor RBAC from session (RequireRole), not request body; stale base_revision → 409 before write; mutation audited | integration | `go test ./internal/server/... -race -run 'TestGetPage\|TestCreatePageRBAC\|TestSavePageConflict\|TestTreeHandler\|TestWildcard' -v && go build ./...` | ✅ | ⬜ pending |
| 01-02-03 | 01-02 | 2 | NAV-01, NAV-02, NAV-04, NAV-05 | T-02-06 | CSRF-echoing mutate client; reader-gated create affordances; no Git vocabulary in create-modal copy | unit (vitest) | `npm --prefix web install && npm --prefix web run build && npm --prefix web test -- --run LeftTree recent` | ✅ | ⬜ pending |
| 01-02-04 | 01-02 | 2 | PAGE-02, PAGE-03 | T-02-03 | Stored-XSS guard: rehype-sanitize ON, rehype-raw OFF, no dangerouslySetInnerHTML; 409 conflict banner; no Git vocabulary | unit (vitest) | `npm --prefix web run build && npm --prefix web test -- --run PageView PageEditor` | ✅ | ⬜ pending |
| 01-03-01 | 01-03 | 3 | PAGE-04, PAGE-05, PAGE-08 | T-03-02 | Structural (not substring) link rewrite through round-trip-safe okf; code spans never matched; one atomic commit | integration | `go test ./internal/okf/... ./internal/pages/... -race -run 'TestRewriteLinks\|TestRename\|TestMove' -v` | ✅ | ⬜ pending |
| 01-03-02 | 01-03 | 3 | PAGE-04, PAGE-05 | T-03-03 / T-03-01 | /rename dispatch new_title→Rename / new_parent→Move (exactly-one-of); editor RBAC; paths re-resolved; distinct audit | integration | `go test ./internal/server/... -race -run 'TestRename\|TestMove' && npm --prefix web run build && npm --prefix web test -- --run PageActionMenu` | ✅ | ⬜ pending |
| 01-04-01 | 01-04 | 4 | PAGE-06, PAGE-07 | T-04-04 / T-04-01 | Delete is a recoverable git mv into .okf-workspace/trash (never git rm); restore collision-suffixed, never clobbers; provenance recorded | integration | `go test ./internal/pages/... ./internal/store/... -race -run 'TestTrash\|TestRestore\|TestListTrash\|TestDelete' -v` | ✅ | ⬜ pending |
| 01-04-02 | 01-04 | 4 | PAGE-06, PAGE-07 | T-04-02 / T-04-03 | Editor RBAC on DELETE/restore; destructive confirm (backdrop does not confirm); deletes/restores audited; no Git vocabulary | integration | `go test ./internal/server/... -race -run 'TestDelete\|TestRestore' && npm --prefix web run build && npm --prefix web test -- --run TrashView` | ✅ | ⬜ pending |
| 01-05-01 | 01-05 | 5 | VER-02, VER-03, VER-04 | T-05-02 / T-05-05 | No SHA leaves backend (opaque token only); restore is a forward commit (no reset/rewrite); push ff-only, alert-not-merge; Push set from config across every payload | integration | `go test ./internal/gitstore/... ./internal/pages/... -race -run 'TestHistory\|TestRestore\|TestPush\|TestViewVersion\|TestCommitJobPush' -v` | ✅ | ⬜ pending |
| 01-05-02 | 01-05 | 5 | VER-02, VER-03 | T-05-03 / T-05-01 | Editor RBAC on restore route; version token server-issued (never user-typed SHA); restore audited; no SHA/Git vocabulary in history UI | integration | `go test ./internal/server/... -race -run 'TestHistory\|TestRestoreVersion' && npm --prefix web install && npm --prefix web run build && npm --prefix web test -- --run HistoryPanel` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

> **Exit-gate (from RESEARCH.md §Validation Architecture):** `internal/okf` must have a
> golden-corpus byte-stable round-trip test — parse → (no-op or surgical frontmatter edit) →
> re-serialize must be byte-identical for every fixture (including a CRLF fixture). This test
> blocks Markdown round-trip rot and is the Phase 1 exit gate. It is created and made green by
> **Task 01-01-01** (`internal/okf/roundtrip_test.go` + `internal/okf/testdata/corpus/`) — the
> first outputs of the first task in Wave 1 — and is re-run after every later task to guard
> against regression (every plan's verification re-runs `go test ./internal/okf/...`).

---

## Wave 0 Requirements

Wave 0 is satisfied inside **Plan 01-01 Task 1**, whose first outputs are the round-trip exit-gate
scaffold (the golden corpus + the byte-stable round-trip test) before any editor or mutation path
ships. No separate pre-wave is required.

- [x] `internal/okf/roundtrip_test.go` — golden-corpus byte-stable round-trip fixtures (PAGE-09 / round-trip exit gate) — created by Task 01-01-01.
- [x] `internal/okf/testdata/corpus/` — corpus fixtures (LF, CRLF, no-frontmatter, code block containing `---`, unknown/quoted frontmatter) — created by Task 01-01-01.
- [x] `internal/okf/testdata/repair/` — repair fixtures (missing-required-frontmatter) — created by Task 01-01-01.
- [ ] `web/vitest.config.ts` — frontend test framework install (only if Phase-0 did not already configure vitest; first frontend task 01-02-03 installs/confirms it).

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| End-to-end create → edit → save → read → navigate loop feels correct (no Git vocabulary visible, saved indicator + version history appear) | PAGE-01..03, NAV-01..05, VER-01..02 | Subjective UX / hidden-Git polish is checked at the phase `/gsd-verify-work` human-verify step, not per-task | Run the binary, sign in as an editor, create a page, edit + Save, confirm "Saved" + a version-history entry appear and no SHA/commit/branch text is visible anywhere; sign in as a reader and confirm mutate affordances are absent. |

*All automatable phase behaviors have automated verification (see Per-Task Verification Map); only subjective hidden-Git UX polish is manual.*

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify (every task has an `<automated>` command)
- [x] Wave 0 covers all MISSING references (the round-trip exit gate is created by Task 01-01-01 before any consumer)
- [x] No watch-mode flags (all frontend runs use `-- --run`; Go runs are one-shot)
- [x] Feedback latency < 90s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-06-18
</content>
