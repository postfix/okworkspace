---
author: architect
---

# Phase 16: Security Hardening - Context

**Gathered:** 2026-06-27
**Status:** Ready for planning

<domain>
## Phase Boundary

Three defense-in-depth security improvements from v1.0 close notes. No new attack surface — these harden existing paths.

**In scope:** SEC-01 (admin sweep fan-out cap), SEC-02 (tag charset validation), SEC-03 (govulncheck resolution).

**Out of scope:** New security features, auth changes, threat model updates.
</domain>

<decisions>
- **D-01:** SEC-01: Config key `sweep.max_concurrent_jobs` defaults to 10. Enqueue batches up to cap.
- **D-02:** SEC-02: Allowed tag charset = `[a-zA-Z0-9_ -]`, validated server-side in SetTags.
- **D-03:** SEC-03: Try upgrading x/tools or Go version first. If still broken, document exception and add alternative scanner.
</decisions>

<canonical_refs>
- `internal/tagsweep/job.go` — sweep enqueue (SEC-01)
- `internal/okf/emit.go` — SetTags (SEC-02)
- `go.mod` — dependencies (SEC-03)
- `.planning/REQUIREMENTS.md` §SEC-01, SEC-02, SEC-03
</canonical_refs>
