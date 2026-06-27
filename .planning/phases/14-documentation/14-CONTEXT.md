---
author: architect
---

# Phase 14: Documentation - Context

**Gathered:** 2026-06-27
**Status:** Ready for planning

<domain>
## Phase Boundary

Create `docs/` directory with contributor-facing architecture documentation for v1.0 graph and tag subsystems, plus update the root README to reflect current features.

**In scope:** `docs/README.md`, `docs/architecture/overview.md`, `docs/architecture/graph-subsystem.md`, `docs/architecture/tag-subsystem.md`, `docs/api/` endpoint contracts, root `README.md` update.

**Out of scope:** Code changes, new features, API changes.
</domain>

<decisions>
## Implementation Decisions

- **D-01:** Docs follow standard architecture documentation patterns — overview → subsystem deep-dives → API contracts.
- **D-02:** Cross-link to actual code files (e.g., `internal/graph/store.go`) so docs stay grounded.
- **D-03:** README update adds v1.0 features (graph, backlinks, LLM tagging) to existing content.
</decisions>

<canonical_refs>
## Canonical References

### Subsystems to document
- `internal/graph/` — graph store, rebuild, query layer
- `internal/tagsweep/` — tag suggestion, bulk sweep, apply path
- `internal/server/handlers_graph.go` — graph HTTP endpoints
- `internal/server/handlers_tagsweep.go` — tag sweep endpoints

### Requirements
- `.planning/REQUIREMENTS.md` §DOC-01, DOC-02
</canonical_refs>
