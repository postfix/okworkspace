---
phase: 01
slug: okf-pages-navigation-hidden-git
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-06-18
---

# Phase 01 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (backend) · vitest (frontend) |
| **Config file** | none — Wave 0 installs (vitest config under web/) |
| **Quick run command** | `go test ./internal/okf/...` |
| **Full suite command** | `go test ./... && (cd web && npm test)` |
| **Estimated runtime** | ~{N} seconds |

---

## Sampling Rate

- **After every task commit:** Run `{quick run command}`
- **After every plan wave:** Run `{full suite command}`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** {N} seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| {N}-01-01 | 01 | 1 | REQ-{XX} | T-{N}-01 / — | {expected secure behavior or "N/A"} | unit | `{command}` | ✅ / ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

> **Exit-gate (from RESEARCH.md §Validation Architecture):** `internal/okf` must have a
> golden-corpus byte-stable round-trip test — parse → (no-op or surgical frontmatter edit) →
> re-serialize must be byte-identical for every fixture (including a CRLF fixture). This test
> blocks Markdown round-trip rot and is the Phase 1 exit gate. The planner wires the concrete
> task rows below from PLAN.md during execution.

---

## Wave 0 Requirements

- [ ] `internal/okf/roundtrip_test.go` — golden-corpus byte-stable round-trip fixtures (PAGE-06 / round-trip exit gate)
- [ ] `internal/okf/testdata/` — corpus fixtures (LF, CRLF, missing-required-frontmatter, unknown-fields)
- [ ] `web/vitest.config.ts` — frontend test framework install (if no framework detected)

*If none: "Existing infrastructure covers all phase requirements."*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| {behavior} | REQ-{XX} | {reason} | {steps} |

*If none: "All phase behaviors have automated verification."*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < {N}s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** {pending / approved YYYY-MM-DD}
