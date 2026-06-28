---
slug: onboarding
created_at: 2026-06-17T14:29:41Z
modified_at: 2026-06-17T15:44:13Z
---
# SMTC Onboarding — OKF Workspace

**Project root:** `/home/john/go/src/github.com/postfix/okworkspace`
**Onboarded:** 2026-06-17 (re-onboarded 15:44Z)

## What this is
"OKF Workspace" — a lightweight self-hosted internal wiki for the agent era.
Currently **planning/spec-only**: the repo contains `SPEC.md`, `CLAUDE.md`
(full locked tech-stack contract), `.planning/` (GSD phase 0 artifacts),
`LICENSE`, `.gitignore`.
**No source code exists yet** (`onboard_project` reported `file_count: 0`,
`analysis_depth: structural`, repo map `workspace_index_unavailable`, and zero
recommended starting points). Re-run `onboard_project` once code lands to build
the semantic index.

## Planned stack & fidelity tier
- **Backend: Go** — first-class (full semantic via gopls when available).
- **Frontend: TypeScript** (Vite + React/Vue) — first-class.
- Agent layer: **CloudWeGo Eino** (Go). Storage: OKF Markdown files + SQLite
  (operational metadata only) + Git for versioning.
- Both primary languages are first-class tier → cross-file results are
  trustworthy once an index exists.

## Capabilities activated
`architecture`, `security`, `protocol` (session bootstrap: `switch_mode(admin)`
then `activate_capabilities`). Security catalogs loaded for Go (271 sinks / 82
sources) and TypeScript (198 / 83).

### Why each capability matters here (from SPEC)
- **security** — heavy untrusted-input surface: file uploads (§6.3, §11, §21.2),
  path-traversal/symlink-escape defense via a safe path resolver (§21.1),
  agent tool sandbox / no-secrets boundary (§15.3, §21.3), auth + CSRF + session
  cookies (§21.4). Watch for path traversal, SSRF (agent URL fetch), unsafe
  deserialization, command injection (git CLI shell-out, §8.1).
- **protocol** — auth guards, soft-lock acquire/release (§13.1), git commit
  queue + job worker lifecycle (§16), agent read→propose→approve→apply→commit
  state machine (§15.4). Good fit for guard/validator/acquire/release checks.
- **architecture** — layered Go services: `internal/{config,server,auth,users,
  repo,okf,attachments,search,gitstore,agent,jobs,audit,web}` (§16). Use
  layer_check / boundary_crossings / cycle_explain once implemented.

## Layer rules
**`.smtc-rules.yaml` authored (2026-06-17, 5 layers / 11 rules).** Layers
top→down: `entrypoint` (cmd) > `transport` (server, web) > `services` (auth,
users, okf, attachments, search, agent, jobs) > `storage` (repo, gitstore) >
`foundation` (config, audit). Rules use SMTC deny-semantics: each `{from,to}`
entry is a FORBIDDEN crossing. Enforces (1) no upward calls and (2) transport
must not skip services to reach storage. `layer_check`/`boundary_crossings`
validate clean (0 violations — no code yet). Re-run once packages exist to
confirm `patterns` path-prefix matching; keep okf→repo direction (storage must
not call up into services).

## Top hotspots / PageRank files
None yet — no indexable source. (Empty until implementation begins.)

## Routing
Use the `smtc` skill for tool routing; delegate heavy analysis to
`smtc-explorer` / `smtc-security-auditor` / `smtc-architecture-reviewer` /
`smtc-impact-analyzer`.

> _Session note (15:44Z): this file was refreshed via direct markdown edit
> because the SMTC `write_memory`/`edit_memory` tools were transiently offline.
> Format preserved so `read_memory`/`list_memories` still parse it._
