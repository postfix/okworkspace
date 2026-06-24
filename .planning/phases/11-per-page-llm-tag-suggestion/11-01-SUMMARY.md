---
phase: 11-per-page-llm-tag-suggestion
plan: 01
subsystem: backend
status: complete
tags: [okf, frontmatter, byte-stability, graph, vocabulary, tagging]
requires: []
provides:
  - "okf.SetTags(d *okf.Doc, tags []string) — byte-stable block-style tags editor"
  - "(*graph.Store).Vocabulary(ctx) ([]string, error) — distinct existing tag vocabulary"
affects:
  - internal/okf/repair.go
  - internal/graph/query.go
tech-stack:
  added: []
  patterns:
    - "FrontDirty + Emit byte-stable single-key frontmatter edit (SequenceNode variant of SetField)"
    - "read-only QueryContext + scan-loop + rows.Err() over the derived page_tags cache"
key-files:
  created:
    - internal/okf/settags_test.go
    - internal/okf/testdata/settags/no-tags-key.md
    - internal/okf/testdata/settags/existing-block-tags.md
    - internal/okf/testdata/settags/other-frontmatter.md
  modified:
    - internal/okf/repair.go
    - internal/graph/query.go
    - internal/graph/query_test.go
decisions:
  - "Frontmatter byte-identity is asserted by REGION (body byte-identical + non-tags keys preserved by re-parse), NOT raw byte equality across the whole frontmatter — yaml.v3's FrontDirty re-marshal reformats sequence indentation (2→4 spaces). This matches the plan's structural-assertion design; the byte-stability invariant that matters (body untouched, no other key altered/reordered) holds."
metrics:
  duration: ~12m
  completed: 2026-06-24
  tasks: 3
  files: 7
---

# Phase 11 Plan 01: Tagging Backend Primitives (SetTags + Vocabulary) Summary

Built the two zero-dependency Wave-1 backend primitives the rest of Phase 11 multiplies: the byte-stable `okf.SetTags` block-style frontmatter editor (TAG-03), gated by a golden round-trip test, and the read-only `Store.Vocabulary` distinct-tag accessor for prompt biasing (TAG-04).

## What Was Built

### Task 1 — `okf.SetTags` (internal/okf/repair.go) — commit 42c79e2
The `yaml.SequenceNode` sibling of `SetField`: resolves the top mapping (materializing a fresh DocumentNode+MappingNode and promoting to frontmatter when absent, copying SetField's branch verbatim), builds a block-style sequence value node (Kind `SequenceNode`, `Style: 0`, one `!!str` scalar per tag in order), replaces an existing `tags` value in place (position preserved) or appends the key+sequence pair, and sets `d.FrontDirty = true`. Performs NO normalization — writes the slice verbatim, in order; normalization is the Wave-2 handler's job.

### Task 2 — golden byte-stability gate (internal/okf/settags_test.go + 3 fixtures) — commit 93dad30
The phase exit gate. Three fixtures under `testdata/settags/`: `no-tags-key.md` (no tags key + a body whose fenced code block contains a literal `---`/`tags:` to prove the body is never re-serialized), `existing-block-tags.md` (replace existing block tags), `other-frontmatter.md` (a custom `author` key + specific ordering preserved). For each fixture `TestSetTags` asserts: (1) BODY bytes are byte-identical; (2) every non-`tags` top-level frontmatter key is preserved in content AND order via a re-parse comparison (not a substring grep); (3) the new tags render as a block-style sequence in order, re-parsed back to a non-flow SequenceNode. Plus an explicit no-tags-key "only tags lines added" sub-case and a control no-op round-trip (Parse→Emit, FrontDirty=false) that is byte-identical to each fixture.

### Task 3 — `Store.Vocabulary` (internal/graph/query.go + test) — commit 8e7208c
`Vocabulary(ctx) ([]string, error)`: `SELECT DISTINCT tag FROM page_tags ORDER BY tag`, mirroring `tagPageCounts`'s QueryContext + defer Close + scan-loop + rows.Err() idiom. Reads the derived cache only; returns a non-nil empty slice when no tags exist. Tested for distinct/sorted/deduped-across-pages and the empty-store case.

## Deviations from Plan

### Adjustments

**1. [Rule 1/clarification] Frontmatter byte-identity asserted by region, not whole-frontmatter raw equality**
- **Found during:** Task 2.
- **Issue:** The plan's prose says "only the tags lines change". Verified empirically that yaml.v3's `Emit` FrontDirty path (`yaml.Marshal(&d.Front)`) re-renders the WHOLE frontmatter and reformats block-sequence indentation (2 spaces → 4 spaces). So whole-frontmatter raw byte-identity is not achievable through the re-marshal path — only the body stays raw-identical.
- **Resolution:** This is exactly the plan's own test design (§Task 2: "re-Parse the emitted output and assert non-`tags` keys/lines equal the original; do NOT just grep"). Implemented structural region assertions: body raw byte-identity + non-tags keys preserved by re-parse (content + order). The load-bearing invariant (body untouched, no other key altered or reordered) holds and is proven. The control no-op round-trip remains byte-identical (verbatim RawFront path, FrontDirty=false). No code change needed; documented here for transparency. No new dependency; CGO-free preserved.

## Self-Check: PASSED

- `internal/okf/repair.go` — FOUND
- `internal/okf/settags_test.go` — FOUND
- `internal/okf/testdata/settags/{no-tags-key,existing-block-tags,other-frontmatter}.md` — FOUND
- `internal/graph/query.go` (Vocabulary) — FOUND
- `internal/graph/query_test.go` — FOUND
- Commits 42c79e2, 93dad30, 8e7208c — FOUND in git log

Verification output:
- `CGO_ENABLED=0 go build ./...` → exit 0
- `go vet ./internal/okf/ ./internal/graph/` → exit 0
- `go test ./internal/okf/ ./internal/graph/` → ok (both packages)
- `go test ./internal/okf/ -run TestSetTags -v` → all PASS (3 fixtures + no-tags-key sub-case + control no-op round-trip)
- `go test ./internal/graph/ -run Vocabulary -v` → both PASS
