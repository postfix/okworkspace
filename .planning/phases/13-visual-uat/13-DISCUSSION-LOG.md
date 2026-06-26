# Phase 13: Visual UAT - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md - this log preserves the alternatives considered.

**Date:** 2026-06-27
**Phase:** 13-visual-uat
**Areas discussed:** Test Environment, Browser Coverage, Test Data, Bug Workflow

---

## Test Environment

| Option                 | Description                                                          | Selected |
| ---------------------- | -------------------------------------------------------------------- | -------- |
| Local dev script       | Run `./scripts/dev.sh` locally, open localhost:8098 in browser       | ✓        |
| Staged deployment      | Deploy to staging server, test against deployed build                |          |
| Both (local + staged)  | Local for initial pass, staged for final verification                |          |

**User's choice:** Local dev script — matches existing Phase 10 UAT pattern. Fast iteration, full control.

---

## Browser Coverage

| Option                      | Description                                                    | Selected |
| --------------------------- | -------------------------------------------------------------- | -------- |
| Chromium only (desktop)     | One modern Chromium browser (Chrome/Edge/Brave)                | ✓        |
| All major desktop browsers  | Chrome + Firefox + Safari (desktop)                            |          |
| Desktop + simulated mobile  | Chrome desktop + DevTools mobile viewport simulation           |          |
| Desktop + real mobile       | Chrome desktop + physical mobile device                        |          |

**User's choice:** Chromium desktop only. Canvas rendering is consistent across Chromium variants; covers the primary desktop use case.

---

## Test Data

| Option                            | Description                                                  | Selected |
| --------------------------------- | ------------------------------------------------------------ | -------- |
| Use existing workspace            | Test against whatever pages/tags are already there           | ✓        |
| Dedicated test workspace          | Create controlled scenarios (orphans, high-degree nodes)     |          |
| Existing + manual creation        | Start with existing, create missing scenarios manually       |          |

**User's choice:** Use existing workspace data. Fastest — no setup. Note missing scenarios as data limitations.

---

## Bug Workflow

| Option                          | Description                                                    | Selected |
| ------------------------------- | -------------------------------------------------------------- | -------- |
| File immediately (as todos)     | File each bug in `.planning/todos/pending/` with severity      | ✓        |
| Collect and review at end       | Collect during session, review at end, decide which to file    |          |
| Informal — note in VERIFICATION | Report findings in VERIFICATION.md summary only                |          |

**User's choice:** File immediately as `.planning/todos/pending/` UAT bugs with severity (blocker/major/minor). Blockers and majors block phase close; minors noted but non-blocking.

---

## Claude's Discretion

- Specific page selection during testing (explore naturally; prioritize pages with diverse link/tag counts)
- Exact visual threshold for "smooth" zoom/pan (subjective but should feel responsive, no frame-drops)

## Deferred Ideas

- Mobile viewport testing
- Cross-browser testing (Firefox, Safari)
- Accessibility audit of graph canvas
- GraphCanvas bundle lazy-load (already deferred from v1.0 close)
