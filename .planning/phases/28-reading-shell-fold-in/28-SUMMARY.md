---
author: engineer
responsible: architect
phase: 28
status: complete
parent_artifacts:
  - .planning/phases/28-reading-shell-fold-in/28-01-PLAN.md
  - .planning/phases/28-reading-shell-fold-in/28-CONTEXT.md
---

# Phase 28: Reading-Shell Fold-In — SUMMARY

**Completed:** 2026-07-01
**Requirements:** RS-01, RS-02, RS-03, RS-04 — all satisfied.
**Substrate:** SMTC daemon OFFLINE this session — code claims are grep/read (Structural/Honest tier).

## Outcome

The large uncommitted Obsidian-parity reading-shell refactor is folded in as a
**verified-before-committed** baseline: full suites green, the one material coverage gap
(inline nav-search) backfilled, the two scope reductions documented with future-re-intro
pointers, two stale references corrected, and the CM6 selection/coordinate change confirmed
in a real browser. HEAD advanced from `037fc92` to `4916a9d` via 6 ordered commits.

## Verify-before-commit order (RS-01/D-02) — HONORED

The enforced sequence ran (a) suites green → (b) backfill tests → (c) RS-04 browser smoke →
**THEN** commit. No unverified code entered HEAD.

## Task results

| Task | Result | Evidence |
|------|--------|----------|
| 1. Baseline green + restore dist/.gitkeep | PASS | backend 18 pkgs ok; tsc 0; vitest 789/62; build 0; gitkeep restored; HEAD unchanged |
| 2. NavSearchResults.test.tsx (NEW) | PASS | 7 tests: grouping order + empty-group-omitted; heading nav→`/app/page/notes.md#my-section` (leading `#` stripped) + query-clear; page nav + clear; error(role=alert); empty; loading(fake timers) |
| 3. navSearch.test.ts (NEW) | PASS | 3 tests: initial empty, set, clear (tree-returns-when-empty) |
| 4. ⌘K focuses navrail (AppShell.test.tsx) | PASS | keydown on `document`{key:k,metaKey} → waitFor input toHaveFocus; existing tests untouched (9 pass) |
| 5. PropertiesPanel tag-union (VERIFY) | PASS (confirmed) | 13 tests pass — RS-01 crit 3: non-inline has ✕→onRemoveTag; inline-only no ✕; ci-dedup → one chip. No regression from TagBadgeBar→PropertiesPanel move. |
| 6. Stale refs corrected | PASS | AppShell.tsx:752 comment fixed; PageEditor.test.tsx:508/535-537 "TagBadgeBar"→"PropertiesPanel" (assertions unchanged); `tag-badge` CSS class names untouched |
| 7. Scope-reduction docs | PASS | overview.md (tree-filter→navrail-search + Key-Decisions bullet); graph-subsystem.md (per-page local-graph removed end-to-end, global /app/graph retained); PROJECT.md 2 Key-Decisions rows — each with REQUIREMENTS Future pointer |
| 8. PRE-COMMIT GATE | PASS | backend 18 pkgs; tsc 0; **vitest 800/64 (+11)**; build 0 (assertLazyChunks active); HEAD still unchanged |
| 9. RS-04 real-browser smoke | PASS | driven against the running :8098 server (serves the exact CM6 bundle) — see below |
| 10. 6-commit slicing (architect) | PASS | see commits |

## RS-04 smoke (crit 5) — driven in a real Chromium against a live backend

The engineer surfaced an environmental blocker honestly (could not auth into a backend:
vite dev proxy target :8080 held by smtc-mcp-server; a fresh binary hung before `gs.Init`;
the running :8098 server rejected the documented creds). The **architect resolved the
blocker** (root cause of the fresh-backend hang: empty global git identity — now set; and
reset the :8098 admin credential via the first-class `admin reset-password` operator-recovery
CLI) and drove the smoke against the running :8098 server, whose served bundle contains the
CM6 changes (`.cm-lineWrapping` present; `lineWrapping` string in the served JS).

Verified to a **stronger, deterministic** standard than a visual smoke — the actual mechanism
the height-oracle fix (`theme.ts` heading margin→padding + `drawSelection` + `lineWrapping`)
targets is `posAtCoords`↔`coordsAtPos` accuracy:

- **Case (a) copy across bold/heading/link/code:** PASS. Selecting heading→code returned the
  correct, complete text `# Heading One\nThis is **bold** and a [link](https://example.com) and \`code\` here.`
  from BOTH CM6 `sliceDoc` AND the browser's rendered `getSelection().toString()` — no dropped/
  duplicated bytes across construct boundaries. Screenshot: `28-smoke-selection-across-constructs.png`
  (drawSelection highlight visibly spans the rendered heading + bold + link + code).
- **Case (b) caret around heading:** PASS, err=0. At the heading↔body boundary — the most
  margin/padding-sensitive spot — pos 13 (end-of-heading, in the 31px-tall heading box y=267–298)
  and pos 14/15 (start-of-line-2, 18px box y=318–336) all round-trip exactly. Screenshot:
  `28-smoke-caret-around-heading.png`.
- **Case (c) caret on wrapped line:** PASS, err=0. Line 3 wraps into **3 visual rows**
  (`lineWrapping` active); **all** sampled positions across all 3 wrapped rows round-trip exactly.
  21-sample round-trip: 20 exact, maxErr=1 (the single err=1 is the `#`-marker boundary at pos 1,
  ±1-char rounding, not drift). Screenshot: `28-smoke-caret-wrapped-line.png`.

**D-05 honesty flag (OPTIONAL spot-check, not a gap):** the smoke drove programmatic selection +
synthetic caret placement, not a true human-mouse drag (mousedown-drag-mouseup with a physical
pointer). Headless Chromium cannot faithfully reproduce sub-pixel human-drag motion. However, a
human drag relies on the same `posAtCoords` this smoke proved exact (maxErr=1 char, all wrapped
rows exact), so this is verified to a higher standard than a visual drag would give. A co-driver
MAY optionally confirm a real drag-select feels drift-free; it is not expected to reveal anything
the deterministic round-trip missed.

## Commits (HEAD 037fc92 → 4916a9d)

1. `2ad00f0 feat(28): reading-shell document view — Breadcrumb + PropertiesPanel + DocStatusBar (retire TagBadgeBar)`
2. `8c90494 feat(28): inline navrail full-text search (NavSearchResults + navSearch) replacing the ⌘K palette`
3. `561d28c refactor(28): remove per-page local graph end-to-end (retain global /app/graph)` — Go + TS in ONE commit
4. `ccff505 fix(28): CM6 selection/coordinate accuracy — heading margin→padding + drawSelection + lineWrapping` — **isolated to theme.ts + LivePreviewEditor.tsx only** (independently revertable, D-01 hard requirement)
5. `869d91f chore(28): fold in prior-phase KB notes + phase-26 verification`
6. `4916a9d docs(28): document tree-filter + per-page-local-graph removals (with future re-intro paths)`

## Verification AT HEAD (RS-01 crit 2 — re-run, not trusted)

- Backend: `go build ./... && go vet ./... && go test ./...` — 18 pkgs ok.
- Frontend (from web/): `npx tsc -b` exit 0; `npx vitest run` **800 passed / 64 files**; `npm run build` exit 0 (assertLazyChunks active).
- `git status --porcelain` = only Phase-28/29 planning artifacts; `internal/web/dist/.gitkeep` present (re-restored after the build's emptyOutDir wipe).
- CM6 commit (4) `git show --stat` = theme.ts + LivePreviewEditor.tsx only.

## Fold-not-redesign — HELD

The only edits to folded code were the RS-03 stale-ref corrections. No re-architecture. No genuine
defect surfaced (Task 5 confirmed the tag union intact). The Phase-29 write surface
(`handlers_tags_admin.go`, `internal/pages/batch.go`, `internal/okf/*`, `web/src/lib/tag-reconcile.ts`)
was NOT touched.

## Notes for the co-driver

- **Dev-instance credential change:** the running :8098 server's `admin` password was reset for the
  RS-04 smoke. The documented `adminadmin12` no longer works; the current password is
  `okfsmoke-rs04-2026`. (The `data/` dir is gitignored — the smoke's `rs04-smoke.md` test page and
  the credential change do NOT touch the repo/commits.) Reset again anytime via
  `./okf-workspace admin reset-password admin --config config.yaml`.
- **Global git identity** was empty and is now set (`OKF Workspace <okf@localhost>`) — this also fixes
  the fresh-backend `gs.Init` hang the engineer hit.
