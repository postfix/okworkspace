---
author: engineer
responsible: engineer
phase: "18"
status: complete
milestone: v1.2
---

# Phase 18: Graph Schema Extension — SUMMARY

**Plan file:** `.planning/phases/18-graph-schema-extension/18-01-PLAN.md`
**Commits:** (part of the ongoing working tree, not yet committed)

## Review disposition

| Finding | Severity | Disposition |
|---------|----------|-------------|
| Tag nodes incorrectly hidden by `existingFilesOnly` filter | BLOCKER | **FIXED** — Set `Exists: true` on tag nodes in `tagNodesAndEdges` (backend, `query.go:440`), AND scoped the frontend `filterNodes` check to `n.type === "page"` (frontend, `filter.ts:73`) |
| Double read+parse per page (pageMeta + titleFor) | MAJOR | **FIXED** — Replaced separate `pageMeta` + `titleFor` calls with unified `readPageMeta(pagePath) (title, aliases, tags)` in both `GraphData` and `Neighborhood` (`query.go:89-107`). One read+parse per page instead of two. |
| `pageAliases` has no unit test coverage | MAJOR | **FIXED** — Added `TestExtract_PageAliases` with 6 cases (flow sequence, block list, single scalar, no key, empty sequence, blank items skipped) in `extract_test.go`. |
| Unbounded frontmatter read per page | MAJOR | **ACKNOWLEDGED** — Pre-existing in `titleFor` (unchanged by this phase). Out of scope for this phase; would need a frontmatter-only reader shared by all callers. |
| `Exists` field lacks `omitempty` on tag nodes | MINOR | **FIXED** — Subsumed by the BLOCKER fix: tag nodes now carry `Exists: true`. |

## Conditional-gate skip-witness

```yaml
gate_result:
  record_author: engineer
  status: not_applicable
  gate: anvil-adversarial-tester
  phase: "18"
  basis: "Not a hard-bar phase: no safety/security invariant under test. The changes extend a data type (GraphNode) with new fields populated from existing frontmatter (aliases, tags, exists, type:attachment). No input validation, auth, concurrency, or untrusted-data path was added or altered."
```

## Files modified

| File | Change |
|------|--------|
| `internal/graph/extract.go` | Added `pageAliases(doc)` — reads `aliases` key from YAML frontmatter (mirrors `pageTags`). Handles SequenceNode and ScalarNode, skips blank items. |
| `internal/graph/extract_test.go` | Added `TestExtract_PageAliases` — 6 test cases covering flow sequence, block list, scalar, missing, empty, blank items. |
| `internal/graph/query.go` | Extended `GraphNode` struct with `Aliases []string`, `Tags []string`, `Exists bool`, Type extended to `"attachment"`. Added helpers: `readPageMeta(path) (title, aliases, tags)`, `inLiveSet(path) bool`, `attachmentFiles() map[string]struct{}`. Updated `GraphData` and `Neighborhood` to populate new fields and include attachment nodes. Tag nodes in `tagNodesAndEdges` carry `Exists: true`. |
| `web/src/api/client.ts` | Updated `GraphNode` interface: `type` includes `"attachment"`, added optional `aliases?`, `tags?`, `exists?`. |
| `web/src/lib/graph/filter.ts` | Removed all `as` type assertions. `matchesSearch` uses `node.aliases?.some()` and `node.tags?.some()`. `filterNodes` uses `n.type === "page" && !n.exists` for the existing-files-only check. |

## Verification

- `go build ./...` — clean
- `go test ./internal/graph/...` — 23/23 PASS
- `go vet ./internal/graph/...` — clean
- `npx tsc --noEmit -p web/tsconfig.json` — clean

## Design decisions

- **Single read+parse**: `readPageMeta` replaces separate `titleFor` + `pageMeta` calls, halving file I/O per graph request. Keeps the resilient-fallback pattern of `titleFor`.
- **Attachment enumeration from repo Tree**: Uses the existing `repo.Tree()` walk with an inverted `.md` filter, plus exclusion of `.json`/`.txt` sidecar files (matching the search index pattern in `internal/search/attachments.go`). Sidecar exclusion avoids creating 3 nodes per attachment.
- **`inLiveSet` uses `repo.Exists`**: Per-page existence check through the SEC-01 resolver, same belt-and-suspenders as the pre-existing page-set union logic. Safe for the small scale (5 users, <500 pages).
- **No schema changes to cache tables**: Aliases and tags are populated on-the-fly from frontmatter reads in the query path, keeping the rebuild path unchanged and avoiding migration complexity.

## Hoare-triple summary

- **Q (postcondition)**: GraphNode carries Aliases, Tags, Exists; Type supports "attachment"; matchesSearch and filterNodes use real fields.
- **S (code shape)**: Struct extension in query.go, extractor in extract.go, TS type in client.ts, filter logic in filter.ts.
- **P (precondition)**: The existing GraphNode struct, extract.go helpers, and GraphData/Neighborhood wiring exist. The filter toggles exist as inert UI.
- **S establishes Q**: The struct fields are populated at every page node construction site (GraphData and Neighborhood). pageAliases mirrors the verified pageTags pattern. The TS type mirrors the Go JSON tags. The filter assertions are removed and replaced with real optional chaining.
- **Invariant**: The lean-payload invariant (no body/frontmatter in graph payload) is preserved. The omitempty tags on Aliases/Tags ensure pages without aliases or tags emit clean JSON. The Exists field always serializes (no omitempty), but tag nodes carry `true` and the frontend filter is scoped to page-only.

```yaml
handoff:
  record_author: engineer
  accountable_owner: engineer
  id: H-18-execute
  from: engineer
  to: architect
  artifact: .planning/phases/18-graph-schema-extension/18-01-PLAN-SUMMARY.md
  intent: "Phase 18 execution complete — GraphNode extended, helpers wired, TS types updated, code-review findings fixed"
  status: ready
```

```yaml
evidence:
  record_author: engineer
  evidence_producer: engineer
  commissioned_by: engineer
  accountable_owner: engineer
  id: E-18-test-run
  subject_ref: "go test ./internal/graph/..."
  kind: test_result
  issuance:
    kind: producer_signed
  warrant:
    kind: external_cited
    scope:
      - "carry=23/23 PASS"
      - "carry=go build ./... clean"
      - "carry=tsc --noEmit clean"
    rationale: "exercises GraphData, Neighborhood, pageAliases, pageTags, rebuild, incremental-upsert, and store lifecycle against a real SQLite DB + git repo"
  freshness:
    status: fresh
    checked_against: HEAD
```
