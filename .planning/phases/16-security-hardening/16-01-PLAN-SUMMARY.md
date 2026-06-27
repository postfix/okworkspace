---
author: engineer
responsible: engineer
phase: 16
plan: 01
status: executed
commits: todo-after-review
requirements: [SEC-01, SEC-02, SEC-03]
---

# Phase 16 Summary: Security Hardening

## Overview

Three defense-in-depth security hardening items executed:

| Task | Requirement | Status | Key Files |
|------|------------|--------|-----------|
| SEC-01 | Configurable sweep fan-out cap (default 10) | DONE | `internal/config/config.go`, `internal/server/handlers_tagsweep.go`, `config.example.yaml` |
| SEC-02 | Tag charset validation server-side | DONE | `internal/okf/repair.go`, `internal/server/handlers_agent.go`, `internal/server/handlers_tagsweep.go`, `internal/okf/charset_test.go`, `internal/okf/settags_test.go` |
| SEC-03 | govulncheck triage | DONE | (scan result documented below) |

---

## SEC-01: Sweep Fan-Out Cap

### Changes

1. **`internal/config/config.go`**: Added `SweepConfig` struct with `MaxConcurrentJobs int` (yaml tag `max_concurrent_jobs`), field `Sweep SweepConfig` on `Config`, and default `DefaultSweepMaxConcurrent = 10` applied in `applyDefaults()`.

2. **`config.example.yaml`**: Added `sweep:` section with `max_concurrent_jobs: 10`.

3. **`internal/server/handlers_tagsweep.go`** (`handleStartTagSweep`):
   - Reads `h.config.Sweep.MaxConcurrentJobs` before the enqueue loop
   - Breaks from the loop when `queued >= maxConcurrent` (safety floor at 10 if config reads < 1)
   - Response now includes `"total"` alongside `"queued"`: `{"ok": true, "queued": N, "total": M}`
   - Audit Detail extended to include `total=%d`

### Design decisions
- **Remaining targets are not lost** — the admin can re-run the sweep (re-staging supersedes per existing behavior). The worker drain naturally processes already-enqueued jobs.
- **Response shape** changed from `{ok, queued}` to `{ok, queued, total}` so the UI can show progress (`N/M pages enqueued`).

---

## SEC-02: Tag Charset Validation

### Changes

1. **`internal/okf/repair.go`**:
   - Added `tagCharsetRE = regexp.MustCompile("^[a-zA-Z0-9_ -]+$")` — the allowed charset: letters, numbers, underscores, dashes, spaces.
   - Added `ValidateTagCharset(tag string) error` — rejects tags with characters outside the allowed set.
   - Modified `SetTags(d *Doc, tags []string) error` — now validates charset at the entry point, returning an error on the first invalid tag. This is defense-in-depth: valid callers should have already validated.

2. **`internal/pages/batch.go`** (`SetTagsFrontmatter`): Updated to handle the error from `okf.SetTags`.

3. **`internal/server/handlers_agent.go`** (`handleApplyTags`): Added charset validation loop on the normalized tags (after `agent.ValidateTags`) — returns 400 with the specific error message for invalid tags.

4. **`internal/server/handlers_tagsweep.go`** (`handleApproveTagSuggestions`): Same charset validation added after the `agent.ValidateTags` normalization step.

### Tag flow with defense-in-depth
```
Client tags → agent.ValidateTags (normalize/cap/dedupe)
           → okf.ValidateTagCharset (charset gate, new)
           → pages.SetTagsFrontmatter
           → okf.SetTags (charset gate, new)
           → YAML structural write
```

### Tests
- `internal/okf/charset_test.go`: Valid tags pass, invalid tags (XSS vectors, Unicode, HTML metacharacters, dots, slashes, quotes, empty) are rejected.
- `internal/okf/settags_test.go`: `TestSetTags_RejectsXSSCharset` verifies that `</script>` is rejected by `SetTags`.

### Regex design note
The charset regex `^[a-zA-Z0-9_ -]+$` places the space and dash AT THE END of the character class. In most regex dialects, a dash in the middle of a character class defines a range (e.g., `a-z`). Here, the dash is placed at the end of the class, right before `]`, so it is treated as a literal dash, not a range operator. The space precedes it. This is correct and has been verified with test cases.

---

## SEC-03: govulncheck Triage

### Result
```
govulncheck@latest ./... — No vulnerabilities found.
Your code is affected by 0 vulnerabilities.
This scan also found 1 vulnerability in packages you import and 18
vulnerabilities in modules you require, but your code doesn't appear to call
these vulnerabilities.
```

### Analysis
- **Reachable vulnerabilities: 0** — no callable path from our code to any known vulnerability.
- **Transitive vulnerabilities: 19 total** (1 in imported packages, 18 in required modules) — none are reachable from our code paths. These are in indirect dependencies (Bleve, sonic, etc.) with no call chain from our application.
- **`go vet ./...`**: Clean — zero warnings.

### Resolution
No code changes needed. The govulncheck result is clean for reachable vulnerabilities. The transitive vulnerabilities are documented here for tracking; they would be addressed by upstream dependency updates when available.

---

## Files Modified

| File | Change |
|------|--------|
| `config.example.yaml` | Added `sweep.max_concurrent_jobs: 10` |
| `internal/config/config.go` | Added `SweepConfig`, `Sweep` field, default |
| `internal/server/handlers_tagsweep.go` | Sweep loop cap, charset validation in approve handler |
| `internal/okf/repair.go` | `ValidateTagCharset`, charset regex, `SetTags` charset gate |
| `internal/pages/batch.go` | `SetTags` error handling |
| `internal/server/handlers_agent.go` | Charset validation in `handleApplyTags` |

## Files Created

| File | Content |
|------|--------|
| `internal/okf/charset_test.go` | `ValidateTagCharset` test suite + `SetTags` charset rejection test |

## Code Review

Pre-handoff `anvil-code-reviewer` identified 2 findings:

1. **[BLOCKER] `SetTagsFrontmatter` silently discarded `okf.SetTags` error** — `internal/pages/batch.go:228` had `okf.SetTags(doc, tags)` without capturing the error return. Fixed to `if err := okf.SetTags(...); err != nil { return "", err }`.
2. **[MINOR] `TestSetTags_NoTagsKey_OnlyTagsLinesAdded` missed error check** — `internal/okf/settags_test.go:253` called `okf.SetTags` without checking error. Fixed to `t.Fatalf` on error.

Both fixed and verified (build + test all packages pass).

## Tests Modified

| File | Change |
|------|--------|
| `internal/okf/settags_test.go` | Updated to handle `SetTags` error return; added `TestSetTags_RejectsXSSCharset` |
