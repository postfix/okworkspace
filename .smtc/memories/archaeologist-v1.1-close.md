---
slug: archaeologist-v1.1-close
created_at: "2026-06-27T22:00:00Z"
modified_at: "2026-06-27T22:00:00Z"
tags: [milestone-close, v1.1, archaeology]
---

# v1.1 Milestone Close — Archaeology Notes

## Summary
v1.1 (Hardening & Polish) shipped 2026-06-27. Five phases (13-17), 9 plans total.

## Key Findings

### Graph Filter Sidebar (Phase 17)
- Entirely client-side filtering (Zustand store + pure functions + FilterControls component).
- Three known gaps: Attachments toggle and Existing files only toggle are non-functional (backend doesn't emit attachment-type nodes or exists field); search aliases/tags matching is dead code.
- All core pipeline works correctly: search by name/path, 4 visibility toggles, 3 display sliders, arrows toggle, groups, reset, localStorage persistence.

### Sweep Progress (Phase 15)
- SweepProgress struct + SweepStatus() method added to tagsweep/store.go.
- GET /admin/tags/sweep/status endpoint.
- Frontend polls at 2s interval, auto-stops.
- Key invariant: status constants duplicated in tagsweep package to avoid import cycle with internal/jobs.

### Security Hardening (Phase 16)
- ValidateTagCharset at internal/okf/repair.go: regex ^[a-zA-Z0-9_ -]+$ -- dash at end of class is literal.
- SweepConfig.MaxConcurrentJobs (default 10) -- cap in handleStartTagSweep, safety floor 10.
- govulncheck: 0 reachable vulnerabilities, 19 transitive (none callable).

### Doc Drift Fixed During This Close
- suggest-tags response is nested objects (suggestTagsResponse with suggestions array of {tag, existing} objects plus page_path), NOT flat parallel arrays as originally documented.
- apply-tags returns 204 No Content, NOT 200 OK with {"ok": true}.
- Route line numbers updated to correct values.
