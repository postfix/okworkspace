# Phase 32: Tag Metadata Store (color/description) - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md - this log preserves the alternatives considered.

**Date:** 2026-07-02
**Phase:** 32-tag-metadata-store
**Areas discussed:** On-disk store shape (design fork)

---

## On-disk store shape (DESIGN FORK — co-driver decided)

| Option | Description | Selected |
| ------ | ----------- | -------- |
| Tags/ folder of .md files | One Markdown file per tag (Tags/deploy.md) with YAML frontmatter (color, description, parent). OKF-native — editable with the same editor, Git diffs per-tag, extends trivially to Phase 33's parent field. Mirrors the Templates/ pattern. | ✓ |
| .okf-workspace/tags.json | A single JSON file in the operational dir. Simpler — one read, one write, atomic temp+rename. But NOT OKF-native: JSON, not Markdown; not editable with the wiki editor. Tag metadata is content, not operational data, so .okf-workspace is a semantic mismatch. | |

**Co-driver's choice:** Tags/ folder of .md files — OKF-native, editable with the same editor, Git diffs per-tag, extends trivially to Phase 33's parent field. Mirrors the Templates/ pattern exactly.

**Notes:** The design fork was genuine (>1 defensible answer). The co-driver chose the OKF-native option, consistent with the project's files-as-truth + Markdown-everywhere invariant. The .okf-workspace/tags.json option was rejected because tag metadata is user-authored content, not operational data.

## Claude's Discretion

- The exact safe-palette set for colors (hex + named tokens — planner picks the named tokens).
- Whether `color` sets text color or background on the badge (CSS property choice).
- Whether `handleTagList` is extended inline or a separate `GET /admin/tags/meta` endpoint.
- Whether a last-rebuilt HEAD check is needed or full-rebuild is sufficient.
- The `SetMeta` file-write shape (frontmatter + body template).
- Commit slicing.

## Deferred Ideas

None — discussion stayed within phase scope.