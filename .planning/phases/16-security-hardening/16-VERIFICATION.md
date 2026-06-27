---
author: qa
responsible: qa
phase: 16
plan: 01
status: verified
gate_result: pass
evidence_tier: high
---

# Verification: Phase 16 — Security Hardening

## Verdict: PASSED

All three requirements (SEC-01, SEC-02, SEC-03) have been verified against the real codebase and meet their must-have truths. Build, vet, test, and govulncheck are all clean.

---

## Summary of evidence

| Requirement | Verdict | Evidence |
|-------------|---------|----------|
| SEC-01: Sweep fan-out cap | PASS | Config struct, example YAML, handler loop cap confirmed. `config.example.yaml` line 38, `config.go` lines 19/128-130/180-182, `handlers_tagsweep.go` lines 72-79/107. |
| SEC-02: Tag charset validation | PASS | `ValidateTagCharset` in `repair.go` lines 153-164, `SetTags` calls it lines 181-185, charset test suite in `charset_test.go`, XSS test in `settags_test.go` lines 290-292. Handlers call charset validation at `handlers_agent.go` lines 753-758 and `handlers_tagsweep.go` lines 199-204. |
| SEC-03: govulncheck triage | PASS | `go vet ./...` clean; `govulncheck ./...` reports 0 reachable vulnerabilities. Clean run documented in SUMMARY. |

---

## Acceptance criteria cross-check

### SEC-01 — Sweep fan-out cap

| Criterion | Status | Evidence |
|-----------|--------|----------|
| Config setting for max concurrent jobs (default 10) | PASS | `DefaultSweepMaxConcurrent = 10` at `config.go:19`; `SweepConfig.MaxConcurrentJobs` at `config.go:129`; `applyDefaults` applies it at line 180-182. |
| `sweep.max_concurrent_jobs` in config.example.yaml | PASS | `config.example.yaml` line 38: `max_concurrent_jobs: 10` under `sweep:` section. |
| Sweep caps enqueue at configured max | PASS | `handlers_tagsweep.go` line 78: `if queued >= maxConcurrent { break }` — loop stops at cap. Safety floor at line 73-75 if config reads < 1. |
| Response includes total count | PASS | Line 107: `writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "queued": queued, "total": len(targets)})`. |
| go test + go build clean | PASS | All tests pass (`go test ./...`), build clean (`go build ./...`). |

**Note on REQUIREMENTS.md vs PLAN:** REQUIREMENTS.md says "remaining queued until slots free" but the PLAN explicitly chose a simpler approach — cap the loop and expect the admin to re-run for remaining targets. The implementation matches the PLAN. The API does report `total` so the UI can show `N/M` progress.

### SEC-02 — Tag charset validation

| Criterion | Status | Evidence |
|-----------|--------|----------|
| `ValidateTagCharset` in `repair.go` | PASS | `repair.go` lines 158-163. |
| `tagCharsetRE` pattern `^[a-zA-Z0-9_ -]+$` | PASS | `repair.go` line 153. Dash at end of class — correctly literal. |
| `SetTags` validates charset | PASS | `repair.go` lines 181-185: `ValidateTagCharset` called per tag, returns error on first invalid. |
| XSS `<script>` rejected via `SetTags` | PASS | `settags_test.go` lines 290-292: `TestSetTags_RejectsXSSCharset` tests `SetTags([]string{"</script>"})`. |
| `handleApplyTags` validates charset | PASS | `handlers_agent.go` lines 753-758: charset validation after `agent.ValidateTags`. Returns 400 with error message. |
| `handleApproveTagSuggestions` validates charset | PASS | `handlers_tagsweep.go` lines 199-204: charset validation after normalization. Returns 400. |
| `SetTagsFrontmatter` propagates error | PASS | `batch.go` lines 228-229: `if err := okf.SetTags(doc, tags); err != nil { return "", err }`. Code review finding fixed. |
| XSS test cases exist | PASS | `charset_test.go`: 17 invalid cases covering `<script>`, `<img>`, `onclick`, `javascript:`, Unicode, CJK, dots, slashes, quotes, etc. All PASS. |

### SEC-03 — govulncheck

| Criterion | Status | Evidence |
|-----------|--------|----------|
| `go vet ./...` exits 0 | PASS | Clean output. |
| govulncheck run | PASS | `govulncheck@latest ./...` reports 0 reachable vulnerabilities. |
| Root cause documented | PASS | SUMMARY section documents 0 reachable, 19 transitive (not callable). |

---

## SMTC verification notes

- **Get declaration body** confirmed `ValidateTagCharset` is the public function at `repair.go:158` with signature `func ValidateTagCharset(tag string) error`.
- **Regex analysis**: `^[a-zA-Z0-9_ -]+$` — the dash is at the end of the character class (before `]`), treated as literal. Space also literal. This is correct Go regexp syntax.
- **Blast radius** for `ValidateTagCharset`: called from `SetTags`, `handleApplyTags`, `handleApproveTagSuggestions` — all three caller sites verified.
- **Build & test**: `go build ./...` and `go test ./...` both clean with no cached failures.

---

## Artifacts

- `config.example.yaml` line 38: `max_concurrent_jobs: 10` with documentation comment
- `internal/config/config.go`: `SweepConfig`, `DefaultSweepMaxConcurrent`, `applyDefaults` for sweep
- `internal/okf/repair.go`: `ValidateTagCharset`, `tagCharsetRE`, `SetTags` charset gate
- `internal/okf/charset_test.go`: charset validation test suite (valid, invalid, empty, SetTags rejection)
- `internal/server/handlers_agent.go` lines 753-758: charset validation in `handleApplyTags`
- `internal/server/handlers_tagsweep.go` lines 73-79: enqueue cap; lines 199-204: charset validation in `handleApproveTagSuggestions`
- `internal/pages/batch.go` lines 228-229: error propagation fixed
