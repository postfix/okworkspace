---
author: qa
responsible: qa
phase: "18"
---

# Phase 18: Graph Schema Extension — VERIFICATION

**Plan file:** `.planning/phases/18-graph-schema-extension/18-01-PLAN.md`
**SUMMARY file:** `.planning/phases/18-graph-schema-extension/18-01-PLAN-SUMMARY.md`

## Verdict

```yaml
gate_result:
  record_author: qa
  id: GR-18-verification
  phase: "18"
  verdict: pass
  basis: "All 3 inert Phase 17 filter toggles are now functional. GraphNode carries Aliases, Tags, Exists fields and Type 'attachment'. pageAliases extractor mirrors pageTags. GraphData/Neighborhood populate all new fields. Attachment nodes created for non-.md files. TypeScript GraphNode matches backend without type assertions. Go build, go test (23/23), go vet, tsc --noEmit, vitest (388/388) all clean. GF-02 acceptance criteria met."
  tiers:
    evidence: smtc_honest
    reasoning: manual_review
  provenance:
    - "Semantic analysis: SMTC get_declaration_body, find_references, search_in_files on internal/graph/query.go, extract.go, rebuild.go, web/src/api/client.ts, web/src/lib/graph/filter.ts, web/src/stores/graphFilters.ts"
    - "Test runs: go test ./internal/graph/..., npx tsc --noEmit -p web/tsconfig.json, npx vitest run, go vet ./..."
```

## Verification method

Goal-backward verification from the Phase 17 UI toggles to the backend data sources, reading code (not trusting the SUMMARY's claims alone). Confirmed each claim by examining the actual code with SMTC semantic tools and running the build/test suite.

---

## 1. Verification acceptance handshake

| Check | Status |
|-------|--------|
| Inputs complete (PLAN, SUMMARY, REQUIREMENTS) | PASS |
| PLAN-REQ linkage clear (GF-02) | PASS |
| Verifiability: "GraphNode has fields", "filterNodes uses real fields" are concrete, falsifiable claims | PASS |

---

## 2. PLAN-SUMMARY consistency check

Checked every truth claim from the PLAN `must_haves` against the code:

| PLAN claim | Code location | Status |
|---|---|---|
| `GraphNode` has `Aliases, Tags, Exists` | `query.go:38-45` | CONFIRMED |
| `GraphNode.Type` includes `"attachment"` | `query.go:41` | CONFIRMED |
| `pageAliases` extractor reads from frontmatter | `extract.go:103-132` | CONFIRMED |
| Rebuild populates Aliases and Tags | `query.go:170-174` (via `readPageMeta` in `GraphData`) | CONFIRMED |
| Rebuild populates `Exists` | `query.go:174` (`onDisk`) in `GraphData`; `query.go:245` (`s.inLiveSet(p)`) in `Neighborhood` | CONFIRMED |
| Attachment nodes for non-.md files | `query.go:194-198` (via `attachmentFiles()`) | CONFIRMED |
| TS GraphNode type updated — no type assertions | `client.ts:258-265` | CONFIRMED |
| `matchesSearch` uses real aliases/tags | `filter.ts:55-56` | CONFIRMED |
| `filterNodes` uses real type and exists | `filter.ts:67-69` | CONFIRMED |
| All 3 inert toggles become functional | See section 3 below | CONFIRMED |

---

## 3. The 3 inert Phase 17 filter toggles: verified functional

### 3a. Attachments toggle

**Backend**: `GraphData` (query.go:194-198) enumerates every non-.md binary file from the repo Tree, skipping sidecar .json/.txt files, and produces a `GraphNode{Type: "attachment", Exists: true}` for each.

**Frontend**: `filterNodes` (filter.ts:68) drops nodes where `!filters.showAttachments && n.type === "attachment"`. Toggle defaults to OFF (`showAttachments: false`, `graphFilters.ts:46`).

**Verification**: Backend emits attachment nodes; frontend correctly filters them based on toggle state. Functional.

### 3b. Existing files only toggle

**Backend**: `Exists` field is populated from `liveSet[p]` (GraphData) or `s.inLiveSet(p)` (Neighborhood). Tag nodes get `Exists: true` (query.go:451) to prevent them from being hidden by this toggle.

**Frontend**: `filterNodes` (filter.ts:69) uses `n.type === "page" && !n.exists`, scoped to pages only — fixing the Phase 17 tag-node-hiding bug.

**Verification**: Backend reports real existence. Frontend filter is scoped to pages. Tag nodes survive the filter. Functional.

### 3c. Search by aliases/tags

**Backend**: `readPageMeta` (query.go:77-95) reads the page file once (single read+parse), extracts title + aliases (`pageAliases`) + tags (`pageTags`). `pageAliases` (extract.go:103-132) handles SequenceNode (multi-alias) and ScalarNode (single alias) forms, mirroring `pageTags`.

**Frontend**: `matchesSearch` (filter.ts:55-56) checks `node.aliases?.some(...)` and `node.tags?.some(...)` against the query string.

**Verification**: Backend populates aliases/tags on every page node. Frontend searches them without any workaround or type assertion. Functional.

---

## 4. Detailed code verification

### 4a. GraphNode struct (query.go:38-45)

| Field | JSON tag | Present |
|---|---|---|
| `ID string` | `json:"id"` | YES |
| `Label string` | `json:"label"` | YES |
| `Type string` | `json:"type"` | YES (+ `"attachment"` in comment) |
| `Aliases []string` | `json:"aliases,omitempty"` | YES |
| `Tags []string` | `json:"tags,omitempty"` | YES |
| `Exists bool` | `json:"exists"` | YES |

### 4b. pageAliases extractor (extract.go:103-132)

- Reads `aliases` key from top-level frontmatter mapping via `topMapping`.
- SequenceNode: collects every non-empty trimmed ScalarNode item.
- ScalarNode: wraps trimmed value in single-element slice.
- Returns `nil` when key absent or value is not a recognized node kind.
- Tests: `TestExtract_PageAliases` with 6 sub-cases (flow sequence, block list, single scalar, missing key, empty sequence, blank items skipped). All PASS.

### 4c. readPageMeta unified helper (query.go:77-95)

Returns `(title string, aliases, tags []string)` in ONE read+parse pass. Called at 2 construction sites:

- `GraphData` (line 170)
- `Neighborhood` (line 241)

Fallback to basename + nil slices on nil repo, read error, or parse error (same resilient pattern as `titleFor`).

### 4d. GraphData page node construction (query.go:168-176)

```go
for _, p := range sortedKeys(pageSet) {
    _, onDisk := liveSet[p]
    title, aliases, tags := s.readPageMeta(p)
    g.Nodes = append(g.Nodes, GraphNode{
        ID: p, Label: title, Type: "page",
        Aliases: aliases, Tags: tags,
        Exists: onDisk,
    })
}
```

`onDisk` derives from `liveSet` (pre-computed via `s.livePages()`), avoiding a per-page `s.repo.Exists` call.

### 4e. Attachment node creation (query.go:194-198)

```go
for _, p := range sortedKeys(s.attachmentFiles()) {
    g.Nodes = append(g.Nodes, GraphNode{
        ID: p, Label: path.Base(p), Type: "attachment", Exists: true,
    })
}
```

`attachmentFiles()` (query.go:112-133) enumerates repo Tree, excluding dirs, trashed paths, .md files, and sidecar .json/.txt files.

### 4f. Neighborhood page node construction (query.go:240-247)

Same `readPageMeta` pattern, with `Exists` from `s.inLiveSet(p)`.

### 4g. Tag node construction (query.go:450-452)

```go
nodes = append(nodes, GraphNode{ID: tagNodeID(tag), Label: tag, Type: "tag", Exists: true})
```

Tag nodes carry `Exists: true` so the existing-files-only toggle does not hide them.

### 4h. TypeScript GraphNode interface (client.ts:258-265)

```typescript
export interface GraphNode {
    id: string;
    label: string;
    type: "page" | "tag" | "attachment";
    aliases?: string[];
    tags?: string[];
    exists?: boolean;
}
```

Mirrors Go JSON tags exactly. `aliases` and `tags` are optional (match `omitempty`). `exists` is optional (frontend may receive older payloads during rollout, or undefined for pre-Phase-18 cached data).

### 4i. filter.ts — no type assertions (filter.ts:62-74)

Search for `as {` in filter.ts returns zero matches. Confirmed no remaining type assertions.

- `matchesSearch` (line 55-56): `node.aliases?.some(...)` and `node.tags?.some(...)` — uses real optional fields.
- `filterNodes` (line 67-69): `n.type === "attachment"` (no cast), `n.type === "page" && !n.exists` (no cast).

### 4j. Toggle defaults (graphFilters.ts:43-53)

```text
showAttachments: false  (OFF by default per GF-01)
existingFilesOnly: false
```

---

## 5. GF-02 acceptance criteria

| # | Criterion | Status |
|---|---|---|
| 1 | GraphNode extended with Aliases, Tags, Exists | PASS (`query.go:38-45`) |
| 2 | GraphNode.Type includes "attachment" | PASS (`query.go:41`) |
| 3 | Graph store populates Aliases and Tags from frontmatter | PASS (`query.go:170-174` via `readPageMeta`) |
| 4 | Exists = true for pages on disk, false for unresolved links | PASS (`query.go:174`, `query.go:245`) |
| 5 | Attachment nodes for uploaded files | PASS (`query.go:194-198` via `attachmentFiles()`) |
| 6 | TS GraphNode includes aliases, tags, exists, type | PASS (`client.ts:258-265`) |
| 7 | matchesSearch uses real node.aliases and node.tags | PASS (`filter.ts:55-56`) |
| 8 | filterNodes uses real node.type and node.exists | PASS (`filter.ts:67-69`) |
| 9 | Attachments toggle filters attachment nodes | PASS (`filter.ts:68`) |
| 10 | Existing files only toggle hides exists===false | PASS (`filter.ts:69`, scoped to page type) |
| 11 | Search matches aliases and tags | PASS (`filter.ts:55-56`) |
| 12 | go test + tsc clean | PASS |

---

## 6. Build/test verification

| Command | Result |
|---|---|
| `go build ./internal/graph/...` | PASS (no errors) |
| `go build ./...` | PASS (no errors) |
| `go test ./internal/graph/...` | PASS (23/23, fresh run 2.278s) |
| `go vet ./internal/graph/...` | PASS (no warnings) |
| `npx tsc --noEmit -p web/tsconfig.json` | PASS (no errors) |
| `npx vitest run` | PASS (47 files, 388 tests) |

---

## 7. Summary findings addressed

| Finding (from SUMMARY) | Fix verified | Status |
|---|---|---|
| Tag nodes hidden by `existingFilesOnly` | Tag nodes now carry `Exists: true` (query.go:451); filter scoped to `n.type === "page"` (filter.ts:69) | VERIFIED |
| Double read+parse per page | `readPageMeta` replaces separate `titleFor` + `pageMeta` (query.go:77-95) | VERIFIED |
| `pageAliases` had no unit test coverage | `TestExtract_PageAliases` with 6 cases in `extract_test.go` | VERIFIED |
| `Exists` lacked `omitempty` on tag nodes | Subsumed by BLOCKER fix: all tag nodes carry `Exists: true` | VERIFIED |

---

## Conclusion

**Verdict: PASS.** The Phase 18 implementation is complete, correct, and meets all GF-02 acceptance criteria. The 3 inert Phase 17 filter toggles (attachments, existing files only, search by aliases/tags) are now backed by real backend data flowing through the correct frontend fields. Go build, Go tests, Go vet, TypeScript type-check, and frontend tests all pass cleanly.

No gaps found. No refusals needed.
