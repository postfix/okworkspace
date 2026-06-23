# OKF Workspace

## What This Is

OKF Workspace is a lightweight, self-hosted, OKF-native internal wiki built for the agent era. Human-readable Markdown files (with YAML frontmatter) are the source of truth, Git provides hidden version history, uploaded attachments stay downloadable as their original files, and a CloudWeGo Eino agent can read, search, summarize, and propose edits that a human approves before they are applied. It targets a small internal team (~5 people) who want Notion-like simplicity without vendor lock-in, monthly cost, or a proprietary knowledge database.

## Core Value

A non-technical teammate can create, edit, and find knowledge — and get useful AI help on it — while every byte stays as plain Markdown + original files on disk, versioned in Git, with no proprietary store to escape. If everything else fails, *the data must remain open, portable, and the wiki usable without Git knowledge.*

## Current Milestone: v1.0 Knowledge Graph & LLM Auto-Tagging

**Goal:** Add Obsidian-style graph visualization and LLM-assisted tagging so the team can *see* how knowledge connects and keep it organized with minimal effort.

**Target features:**
- Global graph view — full-workspace graph (pages as nodes, links as edges), click-to-navigate, zoom/pan
- Local graph panel — per-page neighborhood (current page + direct connections) in a side panel
- Configurable edge types in the graph UI (Obsidian-style toggles): page links, backlinks, shared-tag edges
- Backlinks — reverse-link tracking feeding both the graph and a page-view backlinks panel
- LLM tag suggestion (per-page, on demand)
- LLM bulk tagging sweep over untagged (or all) pages
- Suggest→approve flow for tags (no silent frontmatter writes; consistent with the agent safety model)

## Requirements

### Validated

<!-- Shipped and confirmed valuable. -->

- ✓ **Skeleton & auth** — v0.9.9 — single Go binary serves the React frontend; Argon2id login with admin/editor/reader roles; admin bootstrap on first startup; SCS HttpOnly+SameSite sessions; data dir + Git repo initialized on startup
- ✓ **OKF pages** — v0.9.9 — create/rename/move/delete-to-trash/restore; edit title/tags/description/body; Markdown rendering; YAML frontmatter parse + required-field repair; page links; byte-stable round-trip
- ✓ **Navigation** — v0.9.9 — left file tree with expand/collapse, current-page highlight, recents, create-page-in-folder, create-folder (Phase 7 added right-click menus, drag-and-drop, folder rename/move/delete)
- ✓ **Attachments** — v0.9.9 — upload, byte-exact download, attachment cards, replace, orphan-delete; JSON sidecars; PDF/DOCX/TXT text extraction; SVG served as download
- ✓ **Search** — v0.9.9 — page title/body/tag, attachment filename, and extracted-text search returning page/heading/attachment results (Bleve, incremental index, ⌘K palette)
- ✓ **Eino agent** — v0.9.9 — ask/summarize/rewrite/draft/propose-patch over page/selection/attachment/workspace; propose→review-diff→approve→apply→commit; read-only 5-tool boundary, no direct writes
- ✓ **Git versioning (hidden)** — v0.9.9 — automatic identity-stamped commits on page/attachment/agent-approved changes; single-writer batched commits; history view; restore version; optional remote push
- ✓ **Collaboration (MVP)** — v0.9.9 — soft locks + presence indicator; optimistic concurrency with document revision; 409 conflict shows a diff with overwrite / manual-merge / save-as-copy
- ✓ **Security & audit** — v0.9.9 — fuzz-tested safe path resolver; upload size/MIME/extension limits; `Content-Disposition: attachment` for risky formats; Argon2id hashing; HttpOnly SameSite cookies; nosurf CSRF; SQLite+slog audit log
- ✓ **Live-preview editor (Obsidian-style)** — v0.9.9 (Phase 6) — CM6 inline-rendering Markdown editor with source toggle, byte-stable round-trip preserved

### Active

<!-- v1.0 scope. Detailed REQ-IDs in REQUIREMENTS.md. -->

- **Knowledge graph** — global graph view + per-page local graph panel; configurable edge types (page links / backlinks / shared tags); click-to-navigate
- **Backlinks** — reverse-link tracking + page-view backlinks panel
- **LLM auto-tagging** — per-page on-demand tag suggestion + bulk untagged sweep; suggest→approve (no silent writes)

### Out of Scope

<!-- Explicit boundaries (SPEC §4 non-goals) with reasoning to prevent re-adding. -->

- Notion-style databases, Kanban boards, database-object tables — not core to an open-Markdown wiki; high complexity
- Real-time Google-Docs-level collaboration (Yjs/CRDT) — deferred to a Phase-2 prototype; soft locks suffice for a 5-person team
- Comments, public sharing, web publishing, enterprise SSO — outside MVP value for an internal team
- Direct agent writes without user approval — violates the safety model; all agent writes require review
- In-browser editing of DOCX/PDF — originals are immutable; only extracted text is read
- TipTap / rich block editor — deferred to Phase 2, gated on Markdown round-trip tests passing
- Mobile native app, per-paragraph permissions, complex workflow automation — not MVP

## Context

- **Domain:** internal knowledge base / wiki, "agent-era" — files-as-truth so both humans and agents read the same Markdown.
- **Current state (post-v0.9.9, 2026-06-23):** MVP shipped — all 8 phases (36 plans, 82 tasks) complete and verified. A single CGO-free Go binary serves the embedded React SPA, backed by `internal/{config,server,auth,users,repo,okf,attachments,search,gitstore,agent,jobs,locks,audit,web}` + `cmd/okf-workspace`. All phase verifications closed via live browser UAT on 2026-06-24 (auth/RBAC/seed, attachments byte-exact + extraction, ⌘K search, agent summarize, two-session presence/lock/409 conflict floor). Tech stack as locked: chi, Goldmark, Bleve, Eino + DeepSeek (provider-agnostic), modernc SQLite, React 19 + Vite + CM6.
- **Repository origin:** began greenfield from `SPEC.md` (the product+technical spec, source of truth for the build).
- **Storage model:** OKF-compatible Markdown + first-class attachments on the filesystem inside a Git repo; SQLite holds *operational data only* (users, sessions, jobs, indexing cache, attachment references, UI prefs, audit mirror) and must never become the source of truth for content.
- **Repo layout (workspace data):** `index.md`, topic folders (`runbooks/`, `architecture/`, `decisions/`), `assets/{originals,extracted,metadata}/`, and app-private `.okf-workspace/{manifest.json,trash/,locks/}`.
- **Backend service shape (SPEC §16):** `internal/{config,server,auth,users,repo,okf,attachments,search,gitstore,agent,jobs,audit,web}` plus `cmd/okf-workspace/main.go`.
- **Tooling note:** SMTC semantic-analysis MCP server is connected for this repo (Go + TypeScript first-class) with architecture/security/protocol capabilities active — useful for later impact/security review.

## Constraints

- **Tech stack — Backend:** Go, single process, **chi** router, Goldmark + YAML for Markdown/frontmatter, **shell out to git CLI first** for versioning, slog/zerolog logging — must compile to one binary.
- **Tech stack — Search:** **Bleve** (pure-Go full-text index) — chosen over SQLite FTS5 for richer relevance/faceting.
- **Tech stack — Frontend:** **React** + Vite + TypeScript, bundled into static assets served/embedded by the Go backend; MVP editor is a Markdown editor with preview (not a rich block editor).
- **Tech stack — Agent:** CloudWeGo **Eino**, used only for orchestration/agentic flows (Q&A, summarize, patch proposal, attachment Q&A) — not for plain CRUD. LLM access is **provider-agnostic / configurable** via `config.yaml` (OpenAI-compatible endpoint; local Ollama or remote both supported).
- **Tech stack — DB:** SQLite for operational metadata only.
- **Deployment:** must self-host as one Go binary + a data directory on a small VPS/homelab/VM — no PostgreSQL, Redis, Elasticsearch, or Kubernetes required; portable by copying the repo + config; Docker + systemd packaging supported.
- **Team/scale:** small internal team (~5 users) — informs the "soft locks beat realtime CRDT" tradeoff.
- **Security:** untrusted input surface (uploads, paths, agent tools, auth) must be hardened per SPEC §21 — path-safety resolver, upload validation, agent sandbox, CSRF/session safety, audit logging.
- **Data openness:** all content must remain plain files copyable off the server; Git history must stay readable/useful.

## Key Decisions

<!-- Decisions that constrain future work. -->

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Target = full MVP (SPEC Phases 0–5), SPEC.md is source of truth | User confirmed the whole MVP, not just the §24 first-prototype slice | — Pending |
| Frontend framework: React | Largest ecosystem; Markdown renderer, editor (TipTap later), diff, and upload components target React first | — Pending |
| Search engine: Bleve | Richer relevance/faceting than SQLite FTS5; pure-Go, no extra service | — Pending |
| HTTP router: chi | Lightweight, idiomatic, composable middleware over net/http | — Pending |
| Agent LLM: provider-agnostic via config | Keep deployment flexible (local Ollama or remote) for the "lightweight self-host" goal | — Pending |
| Files-as-truth; SQLite = operational data only | Avoid proprietary lock-in; data stays portable/agent-readable | — Pending |
| Git hidden behind backend; commits automatic | Non-technical users must not need Git knowledge | — Pending |
| Agent writes require explicit user approval | Safety: agent proposes diffs, human approves before apply/commit | — Pending |
| MVP editor = Markdown-with-preview; TipTap deferred to Phase 2 | Protect Markdown round-trip; avoid corruption risk of a rich editor in MVP | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-06-24 — milestone v1.0 (Knowledge Graph & LLM Auto-Tagging) started*
