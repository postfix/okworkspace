# Phase 35: Tag-Operation Undo - Discussion Log

> **Audit trail only.**

**Date:** 2026-07-02
**Phase:** 35-tag-operation-undo
**Areas discussed:** Op-journal shape (design fork)

## Op-journal shape (DESIGN FORK — co-driver decided)

| Option | Description | Selected |
| ------ | ----------- | -------- |
| New SQLite op-journal table | A new tag_op_journal SQLite table holds invertible op records. Operational data (not content). Keeps Phase 35 dependent on Phase 31 only. Mirrors tag_suggestions (0010). | ✓ |
| Phase-32 metadata store | Store op-journal in the Tags/ metadata store. Adds depends_on: 32. Semantic mismatch: Tags/ is for user-authored content metadata, while the op-journal is operational/derived data. | |

**Co-driver's choice:** New SQLite op-journal table — the cleaner option. Operational data belongs in SQLite; the Tags/ store is for user-authored content. No Phase 32 dependency.

## Claude's Discretion

- Whether the undo journals itself (undo-of-undo = redo for free).
- The exact journal row shape (JSON columns vs typed).
- Whether a GET /admin/tags/undo/status endpoint is needed.
- Commit slicing.

## Deferred Ideas

- Multi-level undo (full undo stack) — deferred beyond v1.6.
- Undo of non-tag operations — out of scope.