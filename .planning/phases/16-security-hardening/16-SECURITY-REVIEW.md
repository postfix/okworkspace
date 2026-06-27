---
author: security-engineer
phase: 16
plan: 01
status: reviewed
gate_result: pass
evidence_tier: high
---

# Security Review: v1.1 Hardening & Polish (Phases 13-17)

**Reviewer:** security-engineer (independent)
**Date:** 2026-06-27
**Milestone:** v1.1 Hardening & Polish

## Scope

| Phase | Description | Code Changes | Security-Relevant |
|-------|-------------|-------------|-------------------|
| 13 | Visual UAT | None (manual browser testing) | No |
| 14 | Documentation | None (docs only) | No |
| 15 | UX Improvement | Sweep progress endpoint + frontend polling | Minimal (new admin GET) |
| 16 | Security Hardening | Sweep fan-out cap, tag charset validation, govulncheck | Yes (primary focus) |
| 17 | Graph Filter Sidebar | Client-side filtering pipeline (Zustand, FilterControls, filter functions) | No (client-side only) |

## Verdict: PASSED

No blocking, critical, or high-severity security issues found. All Phase 16 hardening items (SEC-01, SEC-02, SEC-03) are correctly implemented. Existing trust boundaries, RBAC gates, and encoding practices remain intact.

---

## Detailed findings

### Phase 15: UX Improvement

**Status:** No open security findings

The Phase 15 changes are committed at `55a861a` and add:
- `GET /api/v1/admin/tags/sweep/status` — admin-only (behind `RequireRole(admin)`), GET with no request body, reads SQLite aggregated status
- Frontend polling via `TagReviewView` with 2s `refetchInterval`

**Security check:**
- Route is inside `admin.Group()` with `RequireRole(admin)` router gate
- GET endpoint has no request body to validate
- `SweepStatus()` uses parameterized SQL `WHERE kind='tag_suggest' GROUP BY status` — `'tag_suggest'` is a hardcoded constant, no injection vector
- `PendingApprovalCount()` uses parameterized SQL — no injection vector
- Error messages use generic `tagSweepUnavailable` with no internal vocabulary leaked (consistent with `searchUnavailable` / `graphUnavailable` pattern)
- The code review MAJOR finding (silently swallowed `PendingApprovalCount` error) was confirmed resolved — the handler now logs and returns 500

**Verdict:** No issues.

---

### Phase 16: Security Hardening (UNCOMMITTED)

The Phase 16 changes exist in the working tree but have not been committed. They modify:
- `internal/config/config.go` — `SweepConfig` struct, `DefaultSweepMaxConcurrent`
- `config.example.yaml` — `sweep.max_concurrent_jobs` documentation
- `internal/okf/repair.go` — `ValidateTagCharset`, `tagCharsetRE`, `SetTags` error return
- `internal/okf/charset_test.go` — new test file (untracked)
- `internal/okf/settags_test.go` — updated for `SetTags` new error return + XSS test
- `internal/pages/batch.go` — error propagation from `SetTags`
- `internal/server/handlers_agent.go` — charset validation in `handleApplyTags`
- `internal/server/handlers_tagsweep.go` — sweep cap + charset validation in `handleApproveTagSuggestions`

#### SEC-01: Sweep Fan-Out Cap

**Verification:** PASS

| Check | Evidence |
|-------|----------|
| Config struct exists | `internal/config/config.go:128-130`: `SweepConfig{MaxConcurrentJobs int}` |
| Default 10 applied | `internal/config/config.go:19`: `DefaultSweepMaxConcurrent = 10`; `line 180-182`: applied in `applyDefaults` |
| Handler reads config cap | `internal/server/handlers_tagsweep.go:72`: `maxConcurrent := h.config.Sweep.MaxConcurrentJobs` |
| Safety floor for < 1 | `internal/server/handlers_tagsweep.go:73-75`: `if maxConcurrent < 1 { maxConcurrent = 10 }` |
| Loop capped | `internal/server/handlers_tagsweep.go:78`: `if queued >= maxConcurrent { break }` |
| Response includes total | `internal/server/handlers_tagsweep.go:107`: `"total": len(targets)` in response |
| Config documented | `config.example.yaml:34-39`: `sweep.max_concurrent_jobs: 10` with comment |

**Security assessment:** The cap correctly bounds the enqueue loop. It reads from config with a reasonable safety floor. The cap is defense-in-depth over an already admin-only path. No injection vector: `maxConcurrent` is an integer from config, and `targets` is server-enumerated (not client-supplied).

**Finding [INFO]: Sweep cap is approximate, not precise.**
The PLAN explicitly chose "cap the loop and expect admin to re-run" over "internally re-enqueue." Remaining targets beyond the cap are discarded, not queued. The `total` field in the response lets the UI report `N/M` progress. This is a documented design choice, not a defect. No security impact.

#### SEC-02: Tag Charset Validation

**Verification:** PASS

| Check | Evidence |
|-------|----------|
| Regex defined | `internal/okf/repair.go:153`: `^[a-zA-Z0-9_ -]+$` — dash at end of class (literal), space literal |
| `ValidateTagCharset` function | `internal/okf/repair.go:159-164`: valid signature |
| Gate in `SetTags` (low-level) | `internal/okf/repair.go:181-185`: validates each tag, returns error on first invalid |
| Gate in `handleApplyTags` (per-page) | `internal/server/handlers_agent.go:753-758`: validates after `agent.ValidateTags` normalization, returns 400 |
| Gate in `handleApproveTagSuggestions` (bulk) | `internal/server/handlers_tagsweep.go:199-204`: validates after normalization, returns 400 |
| `SetTags` error propagated in `SetTagsFrontmatter` | `internal/pages/batch.go:228-229`: error from `SetTags` returned as `""` + error |
| Charset tests | `internal/okf/charset_test.go`: 11 valid cases, 17 invalid cases (XSS script, img, onclick, javascript:, slashes, dots, quotes, emoji, accented, CJK, etc.) |
| `SetTags` XSS test | `internal/okf/settags_test.go:279-292`: `TestSetTags_RejectsXSSCharset` tests `</script>` and `<img>` |

**Security assessment:** The charset validation is correctly implemented with three independent gating points (defense-in-depth). The regex accurately reflects the allowed character set. Validation order is correct — `agent.ValidateTags` (normalize/lowercase/dedupe) runs first, then charset validation catches any remaining problematic characters.

**Finding [LOW]: charset validation runs AFTER `agent.ValidateTags` normalization, which is the correct order — the charset gate catches any characters that normalization might introduce (though in practice it doesn't).** No action needed.

**One edge-noted:** The regex `^[a-zA-Z0-9_ -]+$` does NOT allow periods (`.`). This means tag names containing dots (like `v1.0` or `doc.md`) would be rejected. The SPEC's allowed charset intentionally excludes dots. This is by design — not a security issue.

#### SEC-03: govulncheck Resolution

**Verification:** PASS

- `go vet ./...` exits 0 — clean
- `govulncheck@latest ./...` reports 0 reachable vulnerabilities
- The v1.0 issue (Go 1.26/x-tools toolchain panic) is resolved

**Verdict:** Clean.

---

### Phase 17: Graph Filter Sidebar

**Status:** No security findings

Changes are client-side only (committed in 5 commits from `8bbaae4` to `d8ba734`):
- `web/src/stores/graphFilters.ts` — Zustand store with `persist({ name: "okf.graph.filters" })`
- `web/src/lib/graph/filter.ts` — `filterNodes`, `matchesSearch`, `applyOrphanFilter`
- `web/src/components/graph/FilterControls.tsx` — search input, 4 visibility toggles, display sliders
- `web/src/components/graph/GraphView.tsx` — filtering pipeline integration

**Security check:**
- All filtering is client-side `useMemo` over the existing graph payload — no new network calls
- Search input is a plain `<input type="search">` controlled by Zustand — no HTML rendering of the query; node labels are drawn as canvas text (existing `T-10-03` XSS guard)
- localStorage key `okf.graph.filters` is properly namespaced — persisted state contains only UI settings (booleans, floats, strings), never secrets or tokens
- No `dangerouslySetInnerHTML` — all rendering is React components or canvas `fillText`
- The QA-identified gaps (P17-G01: attachments toggle dead code, P17-G02: existing-files toggle dead code, P17-G03: aliases/tags search dead code) are correctness/UX issues, not security issues. The dead-code branches use type assertions (`as { exists?: boolean }`) which silently return `undefined` — no injection vector.

**Verdict:** No security issues.

---

## Cross-cutting analysis

### Attack surface delta

| New/Modified Attack Surface | Type | Gate | Risk |
|----------------------------|------|------|------|
| `GET /api/v1/admin/tags/sweep/status` | Info-leak (admin) | `RequireRole(admin)` only | Very low — returns aggregate counts only, no per-page details |
| `POST /api/v1/admin/tags/sweep` (sweep cap) | Resource consumption | Admin + CSRF | Reduced — cap bounds enqueue |
| `POST /api/v1/admin/tags/approve` (charset gate) | Input validation | Admin + CSRF + charset gate | Reduced — new validation layer |
| `POST /agent/apply-tags` (charset gate) | Input validation | Editor + CSRF + charset gate | Reduced — new validation layer |
| Graph filter controls | Client-side only | None (no new trust boundary) | None |

### Trust boundaries

No new trust boundaries introduced. All endpoints sit behind existing RBAC (admin-only or editor-only) and the global CSRF middleware.

### Taint analysis (SMTC)

`find_taint_sinks` returned 0 matching call sites across the codebase (no Go SQL/command-injection/SSRF patterns matched by the catalog).

`find_validation_gaps` returned 29 results, all in established pre-existing code paths (search query builder, page CRUD handlers, attachment delete). These are pre-existing gaps unrelated to this milestone's changes. The Phase 16 charset validation gates are not recognized by SMTC's `input.validated` token model, but manual inspection confirms they correctly guard the tag-write paths.

`find_missing_guards` returned 0 violations for the new/modified code paths.

### Secrets handling

The `AgentConfig.String()` / `GoString()` redaction of the LLM API key was established in earlier phases and is unchanged. No new secrets handling introduced.

### localStorage

The `okf.graph.filters` persist key follows the project convention (`okf.graph.edges` precedent). Persisted state contains only:
- `searchQuery` (string) — user's graph search text
- `showTags`, `showAttachments`, `showOrphans`, `showArrows`, `existingFilesOnly` (booleans)
- `textFadeThreshold`, `nodeSize`, `linkThickness` (floats, 0-1)
- `groups` (array of `{id, name, query, color, enabled}`)

No secrets, tokens, or sensitive data. No security concern.

## Summary of findings

| ID | Severity | Phase | Description | Status |
|----|----------|-------|-------------|--------|
| — | NONE | 13 | Visual UAT — no code changes | Not security-relevant |
| — | NONE | 14 | Documentation — no code changes | Not security-relevant |
| — | NONE | 15 | Sweep progress endpoint — admin GET, no injection vector | Clean |
| SEC-01 | INFO | 16 | Sweep cap is approximate stop-gap (doesn't re-enqueue remaining) | Documented design choice |
| SEC-02 | LOW | 16 | Charset validation runs after normalization (correct order) | Noted, no action |
| SEC-03 | NONE | 16 | govulncheck clean (0 reachable) | Clean |
| — | NONE | 17 | Client-side graph filtering — no new backend surface | Clean |

**No blocking issues. No critical or high findings.**

---

## Recommendations

1. **Commit the Phase 16 working tree changes** — The security hardening items (SEC-01, SEC-02, SEC-03) are correctly implemented. The uncommitted state is the only risk: losing these changes before a build would revert to the pre-hardening state. All three are ready to commit.

2. **(GF-01 gap mitigation, not security-related)** — The Phase 17 QA-identified gaps (Attachments toggle, Existing files only toggle, aliases/tags search) are correctness/UX issues. The architect may choose to:
   - Remove the non-functional toggles now and add them when the backend graph model supports them
   - Keep them as inert forward-looking UI (no security risk either way)

---

## Evidence sources

- Code review of all changed files (Phases 13-17)
- SMTC: `find_taint_sinks`, `find_validation_gaps`, `find_missing_guards`, `find_defense_gaps`
- `git diff HEAD` for uncommitted Phase 16 changes
- QA verification docs (Phase 15 `15-VERIFICATION.md`, Phase 16 `16-VERIFICATION.md`, Phase 17 `17-VERIFICATION.md`)
- Phase 15 code review (`15-REVIEW.md`) — major finding confirmed resolved
- `go vet ./...` — clean
- `govulncheck@latest ./...` — 0 reachable
