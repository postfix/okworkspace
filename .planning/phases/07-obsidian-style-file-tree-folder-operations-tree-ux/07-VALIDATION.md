---
phase: 7
slug: obsidian-style-file-tree-folder-operations-tree-ux
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-06-21
---

# Phase 7 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Backend: Go `testing` (table-driven, real repo+git+worker fixture — `newServiceFixture`). Frontend: vitest 3.2.4 + @testing-library/react 16.3.0 + user-event 14.6.1 |
| **Config file** | Backend: none (`go test`). Frontend: `web/vitest.config.ts` |
| **Quick run command** | Backend: `go test ./internal/pages/ -run TestFolder`. Frontend: `cd web && npx vitest run src/components/LeftTree.test.tsx` |
| **Full suite command** | `go test ./...` AND `cd web && npm test` |
| **Estimated runtime** | ~10s (Go pkg), ~20–40s (frontend) |

---

## Sampling Rate

- **After every task commit:** the affected quick command (Go package run or the single vitest file)
- **After every plan wave:** `go test ./...` + `cd web && npm test`
- **Before `/gsd-verify-work`:** full suite green AND `go test ./internal/okf/ -run TestGoldenRoundTrip` green
- **Max feedback latency:** ~40 seconds

---

## Per-Task Verification Map

| Req | Behavior | Test Type | Automated Command | File Exists | Status |
|-----|----------|-----------|-------------------|-------------|--------|
| TREE-01 | Folder/page/root menu item sets + a11y (focus trap, Esc, arrow nav, viewport clamp) | unit (vitest) | `cd web && npx vitest run src/components/TreeContextMenu.test.tsx` | ✅ extend | ⬜ |
| TREE-02 | Folder rename/move = ONE commit; all inbound links rewritten; round-trip holds | unit (Go) | `go test ./internal/pages/ -run TestRelocateFolder` | ❌ W0 | ⬜ |
| TREE-02 | No corruption (code-block link-like text untouched) | unit (Go) | `go test ./internal/pages/ -run TestRelocateFolder_NoCorruption` + okf round-trip | ⚠️ okf ✅ / folder ❌ W0 | ⬜ |
| TREE-03 | Optimistic apply + rollback on error over `["tree"]` cache | unit (vitest) | `cd web && npx vitest run src/components/LeftTree.test.tsx` | ✅ extend | ⬜ |
| TREE-04 | Folder delete → N trash rows w/ shared group id; pages restorable | unit (Go) | `go test ./internal/pages/ -run TestDeleteFolder` | ❌ W0 | ⬜ |
| TREE-05 | Grouped restore recreates structure (index.md first); per-page restore unchanged | unit (Go) | `go test ./internal/pages/ -run TestRestoreGroup` | ❌ W0 | ⬜ |
| TREE-06 | Folder collision → 409 (no merge) | unit (Go) | `go test ./internal/pages/ -run TestRelocateFolder_Collision` | ❌ W0 | ⬜ |
| TREE-06 | Invalid drag (self/descendant) prevented (path-prefix guard) | unit (vitest) | `cd web && npx vitest run src/components/LeftTree.test.tsx` | ✅ extend | ⬜ |
| ALL (regression) | Clean-Rebuild Behavior Inventory preserved (pin BEFORE rebuild) | unit (vitest) | `cd web && npm test` | ✅ pin first | ⬜ |
| SEC | Editor-gated folder ops (RBAC from session role); path-safety; SQL bind for group id | unit (Go) | `go test ./internal/server/ ./internal/pages/` | ⚠️ extend | ⬜ |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/pages/rename_test.go` — add `TestRelocateFolder`, `TestMoveFolder`, `TestRelocateFolder_Collision`, `TestRelocateFolder_NoCorruption` (cross-linked descendants — Pitfall 1). Reuse `newServiceFixture` + `waitForFile`/`waitForGone`/`commitCount`.
- [ ] `internal/pages/trash_test.go` — add `TestDeleteFolder` (group id on rows), `TestRestoreGroup` (structure recreated, collision-suffix batched), `TestDeleteFolder_PartialProgress` (Pitfall 3 / A2).
- [ ] `internal/store/migrations/0008_trash_group.sql` — add nullable `delete_group_id`; assert the column exists via `st.Migrate` in the fixture.
- [ ] `web/src/components/LeftTree.test.tsx` — extend `vi.mock("../api/client")` with `moveFolder/renameFolder/deleteFolder/restoreFolderGroup`; add folder-DnD self/descendant guard + optimistic-apply/rollback tests.
- [ ] `web/src/components/__regression__` (or equivalent) — pin the Clean-Rebuild Behavior Inventory (page menu, page DnD, folder-scoped create, dialog-footer, commit-wait) BEFORE rebuilding the components.
- [ ] Framework install: none — all present.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Drag-and-drop feel (drag a folder onto another; drop-target ring; not-allowed on self/descendant) | TREE-03/06 | Native DnD pointer interaction + visual ring not assertable in jsdom | In the running app, drag a folder onto another folder and onto root; confirm it moves, the target highlights on drag-over, and dragging onto itself/a child shows not-allowed and no-ops |
| Optimistic update snappiness (moved node appears instantly, reconciles) | TREE-03 | Perceptual timing | Move a folder; confirm the tree updates immediately (before the network settles) and stays correct after |
| Context menu placement near the viewport edge (4px clamp) | TREE-01 | Layout/viewport geometry | Right-click a row near the bottom/right edge; confirm the menu stays fully on-screen |

*Backend folder relocate / grouped restore / collision are fully automated; only DnD pointer feel and menu geometry are manual (and browser-validatable via Playwright).*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Regression net pinned BEFORE the clean rebuild
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 40s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
