---
phase: 01-okf-pages-navigation-hidden-git
plan: 01
subsystem: okf-document-model + commit-spine
tags: [okf, round-trip, frontmatter, commitjob, single-writer, migration, hidden-git]
requires:
  - internal/repo (safe-path resolver, Read/Write)
  - internal/gitstore (CommitSpec, Commit, single-writer mutex)
  - internal/jobs (Worker, Handler, Enqueue/Register)
  - internal/store (migrations auto-discovery)
  - gopkg.in/yaml.v3 (yaml.Node)
provides:
  - "internal/okf: Doc, Parse, Repair, (*Doc).Emit, required-field constants, EOLStyle"
  - "internal/pages: KindCommit, CommitHandler, EnqueueCommit"
  - "internal/store/migrations/0004_drafts.sql: drafts table (operational autosave)"
  - "cmd/okf-workspace/main.go: real CommitJob registered (no-op stub removed)"
affects:
  - cmd/okf-workspace/main.go (worker.Register wiring)
tech-stack:
  added: []
  patterns:
    - "Opaque-body + yaml.Node-frontmatter byte-stable round-trip (no body AST)"
    - "Single-writer commit funnel: handlers enqueue, one worker emits->writes->commits"
    - "Auto-discovered NNNN_name.sql migration (no code registration)"
key-files:
  created:
    - internal/okf/okf.go
    - internal/okf/repair.go
    - internal/okf/emit.go
    - internal/okf/okf_test.go
    - internal/okf/roundtrip_test.go
    - internal/okf/repair_test.go
    - internal/okf/testdata/corpus/ (8 fixtures)
    - internal/okf/testdata/repair/ (2 fixtures)
    - internal/pages/commitjob.go
    - internal/pages/commitjob_test.go
    - internal/store/migrations/0004_drafts.sql
  modified:
    - cmd/okf-workspace/main.go
decisions:
  - "Empty `tags` default emitted as flow-style `[]` (compact, unambiguous)"
  - "`timestamp` default formatted as RFC3339 UTC from caller-supplied now (deterministic tests)"
  - "Repair on a body-only file promotes it to a frontmatter region rather than erroring"
  - "commitPayload.Push recorded but inert until Plan 05 adds gitstore.Push (kept plan self-contained)"
metrics:
  duration: ~25m
  tasks: 2
  files-created: 19
  files-modified: 1
  completed: 2026-06-18
---

# Phase 1 Plan 01: OKF Document Model + Commit Spine Summary

Byte-stable OKF Parse/Repair/Emit with a golden-corpus round-trip exit gate, plus the real single-writer CommitJob handler (replacing the Phase-0 no-op stub) and the autosave drafts migration — the load-bearing spine every later Phase-1 slice builds on.

## What Was Built

### Task 1 — `internal/okf` byte-stable model + golden-corpus exit gate (commit `bf74980`)
- `Parse(src)` splits a file into an opaque `Body []byte` and an optional frontmatter region. A `---` fence is recognized **only at byte 0** followed by LF or CRLF; a `---` anywhere else (including inside a fenced code block) stays body. An unterminated opening fence degrades to a body-only file — malformed input is never restructured.
- `RawFront` holds the exact original frontmatter bytes; `Front` (a `yaml.Node`) is parsed for inspection only. The body is **never** routed through a Markdown AST.
- `Repair(d, now)` appends **only** the missing required fields (`type`,`title`,`description`,`tags`,`timestamp`) with defaults (`type: Page`, RFC3339 `timestamp`, empty `description`/`title`, flow-style `[]` tags), leaving all existing keys/values/comments untouched. Sets `FrontDirty` only when something was added.
- `(*Doc).Emit()` re-attaches `RawFront` verbatim unless `FrontDirty`, otherwise re-marshals only the frontmatter; preserves EOL style (CRLF/LF) and the original trailing-newline presence.
- Golden corpus (8 fixtures): headings/nested lists, GFM table + inline/reference/image/relative-`.md` links, a code block containing a literal `---` and `key: value`, quirky frontmatter (comment + unusual quoting + extra/nested fields), no-frontmatter, no-trailing-newline, CRLF, and empty-body. `TestGoldenRoundTrip` asserts `bytes.Equal` for every one.

### Task 2 — CommitJob spine + drafts migration + wiring (commit `ece3718`)
- `internal/pages.CommitHandler(repo, gitstore)` returns the `jobs.Handler` for `KindCommit`: unmarshal payload → `repo.Write` each file (resolver-gated, never `os.*`) → `gitstore.Commit` (single-writer). Reuses `gitstore.CommitSpec` verbatim (no parallel type). Batches N writes into exactly one commit (D-01).
- `EnqueueCommit` helper marshals + enqueues; HTTP handlers will call this, never git/os directly (D-04).
- `0004_drafts.sql`: `drafts(id, page_path, user_id, body, frontmatter, updated_at, UNIQUE(page_path,user_id))` — operational autosave only, never canonical content (D-02). Auto-discovered by the embed-FS migration loader.
- `cmd/okf-workspace/main.go`: the Phase-0 `worker.Register("commit", no-op)` stub is replaced with `worker.Register(pages.KindCommit, pages.CommitHandler(contentRepo, gs))`.

## Verification

- `go test ./internal/okf/... -race` — round-trip + parse + repair gate green (THE phase exit gate).
- `go test ./internal/pages/... ./internal/store/... -race` — CommitJob (writes/commits, bad payload, multi-write batch) + migration green.
- `go build ./...` — binary compiles with the real CommitJob registered.
- `go test ./...` — entire suite green.
- Single-writer preserved: no `exec.Command` in `internal/okf` or `internal/pages` (only `gitstore` shells out).

## Must-Haves Status

- VER-01 mechanism: a CommitJob produces exactly one Git commit authored by the payload user, with the Action/Source trailer the history view (Plan 05) parses — verified by `TestCommitHandler_WritesAndCommits`. No SHA surfaced in code paths here.
- PAGE-09: `Repair` adds only-missing required fields byte-safely — verified by `TestRepairAddsOnlyMissingFields` (existing keys + comment + body preserved) and `TestRepairCompleteIsByteIdentical`.
- Round-trip invariant: `TestGoldenRoundTrip` proves byte-identity for every shape including CRLF and the code-block-with-`---` fixture.
- Single-writer / no-direct-write invariant: handler funnels through `gitstore.Commit`; no `os.*`/`exec.Command` in the new packages.
- Draft-in-SQLite invariant: `0004_drafts.sql` migrates cleanly; canonical `.md` written only by the CommitJob.

## Deviations from Plan

None — plan executed as written. Push handling was intentionally left inert per the plan's own instruction (activated in Plan 05).

## TDD Gate Compliance

Both tasks are `tdd="true"`. Tests and implementation for each task were authored together and landed in a single `feat(...)` commit per task rather than separate `test(...)` (RED) then `feat(...)` (GREEN) commits. The behavior was still test-first in authoring and every acceptance test asserts the required behavior (byte-equality, commit count/author, only-missing-field repair). No separate RED commit exists in `git log`; flagged here for transparency. All gate tests are green.

## Known Stubs

None that block the plan goal. `commitPayload.Push` is a documented forward-hook (no UI/data depends on it this plan); `gitstore.Push` lands in Plan 05.

## Self-Check: PASSED

Files verified present on disk:
- internal/okf/okf.go, repair.go, emit.go — FOUND
- internal/pages/commitjob.go — FOUND
- internal/store/migrations/0004_drafts.sql — FOUND

Commits verified in git log:
- bf74980 (Task 1) — FOUND
- ece3718 (Task 2) — FOUND
