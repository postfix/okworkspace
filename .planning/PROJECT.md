# OKF Workspace

## What This Is

OKF Workspace is a lightweight, self-hosted, OKF-native internal wiki built for the agent era. Human-readable Markdown files (with YAML frontmatter) are the source of truth, Git provides hidden version history, uploaded attachments stay downloadable as their original files, and a CloudWeGo Eino agent can read, search, summarize, and propose edits that a human approves before they are applied. An Obsidian-style knowledge graph (global + per-page) with backlinks lets the team *see* how pages connect, and approval-gated LLM tagging (per-page and bulk) keeps the workspace organized — all from a derived store that is never the source of truth. It targets a small internal team (~5 people) who want Notion-like simplicity without vendor lock-in, monthly cost, or a proprietary knowledge database.

## Core Value

A non-technical teammate can create, edit, and find knowledge — and get useful AI help on it — while every byte stays as plain Markdown + original files on disk, versioned in Git, with no proprietary store to escape. If everything else fails, *the data must remain open, portable, and the wiki usable without Git knowledge.*

## Current State

**Shipped: v1.6 Tag System Completion & Authoring Polish (2026-07-02).** Phases 31–37, two tracks —
bulk multi-tag ops, a NET-NEW on-disk tag-metadata store (`internal/tagmeta/`, `Tags/*.md`,
files-as-truth + rebuildable cache), display-only tag hierarchy, tag-op undo (dedicated op-journal),
usage analytics, plus authoring polish (`#tag` badge in react-markdown, copy-code button,
insert-at-cursor templates, description in search cards). Audit COMPLETE, security SHIP (0 high/med/low),
debt=0. Prior: v1.5 (tag regression fix + v1.3 closure), v1.4 (editor enhancements).

**Current milestone: v1.7 Rich-Block Editor (TipTap)** — a self-contained editor sub-project, Phases
38–42, one track. A TipTap (ProseMirror) WYSIWYG editor as an **opt-in** mode beside CM6, behind a
**two-tier Markdown-fidelity gate** (Tier 1: supported syntax semantic-lossless + idempotent; Tier 2:
unmodeled syntax — tables/math/raw-HTML — byte-for-byte preserved). CM6 stays THE default and
byte-exact surface. **RE-01 (Phase 38) is a GO/NO-GO spike gate** — if the round-trip corpus can't pass,
the milestone halts. Risk-first: the codec gate first, the ~949-line `PageEditor` host refactor isolated
as its own no-behavior-change phase (RE-05, Phase 40), TipTap features layered on last. The sharp
security surface is TipTap **paste** (MAJ-05). Requirements RE-01…05 + folded red-team findings in
`REQUIREMENTS.md`. One design fork deferred to Phase 42 discuss (shared-content Tier-1 normalization).

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
- ✓ **Knowledge graph** — v1.0 — derived link/tag store (rebuildable, never source of truth) + Obsidian-style global graph view & per-page local panel; configurable edge types (page links / backlinks / shared tags, shared-tag OFF by default); degree sizing, orphan distinction, hover-highlight, click-to-navigate
- ✓ **Backlinks** — v1.0 — reverse-link tracking + page-view "Referenced by" panel + `/graph/backlinks` endpoint
- ✓ **LLM auto-tagging** — v1.0 — per-page on-demand tag suggestion + admin bulk untagged sweep with a batch review queue; suggest→approve (no silent writes), byte-stable frontmatter apply, vocabulary-biased & capped, model output re-validated server-side

- ✓ **Graph filter sidebar** — v1.1 — live node search, visibility toggles (Tags/Attachments/Existing/Orphans), display sliders (text fade/node size/link thickness), arrows toggle, graph groups, reset, localStorage persistence
- ✓ **Documentation** — v1.1 — `docs/` contributor architecture docs (overview, graph subsystem, tag subsystem, API contracts) + root README update
- ✓ **UX improvement** — v1.1 — non-blocking bulk-approve progress indicator with polling status endpoint
- ✓ **Security hardening** — v1.1 — admin sweep fan-out cap, tag charset server-side validation, govulncheck triaged (0 reachable vulns)
- ✓ **Graph schema extension** — v1.2 — extended backend GraphNode with aliases, tags, exists, and attachment type; activated 3 inert Phase 17 filter toggles
- ✓ **Tag Management UX** — v1.3 (built `9de5077`; regression-fixed + verified + security-reviewed in v1.5) — admin-only workspace-wide tag rename / merge / delete with preview→approve; admin UI; derived-graph consistency
- ✓ **Editor enhancements & authoring polish** — v1.4 — inline `#tag` badges (frontmatter-canonical), LLM tag suggestions, LLM-generated description, code syntax highlighting, mermaid diagrams, page templates
- ✓ **Tag regression fix + v1.3 closure** — v1.5 — server-authoritative body-`#tag` rewrite (`okf.RewriteHashtags`, offset-parity-pinned to the Lezer grammar, CI-enforced) so rename/merge/delete are durable against inline-`#tag` materialization; reading-shell fold-in (Properties panel + Breadcrumb + DocStatusBar; inline nav search; per-page local graph removed); v1.3 Tag Management verified + security-reviewed retroactively
- ✓ **Tag system completion & authoring polish** — v1.6 — bulk multi-tag ops; a NET-NEW on-disk tag-metadata store (`internal/tagmeta/`, `Tags/*.md` frontmatter, files-as-truth + rebuildable `tag_meta` cache, migrations 0011–0013) for tag color/description; display-only cycle-safe tag hierarchy; tag-op undo (dedicated op-journal, inverse `ApplyTagsBatch`, 409-guarded); tag usage analytics; `#tag` badge in react-markdown (`remark-hashtag`, sanitized element); copy-code button; insert-at-cursor templates; description in search cards

### Active

<!-- v1.7 Rich-Block Editor (TipTap) — detailed RE-IDs in REQUIREMENTS.md -->

- **RE-01: Markdown codec + round-trip corpus (GO/NO-GO gate)** — headless TipTap↔Markdown codec (bounded slice) proven by a Tier-1 corpus written first; NO-GO halts the milestone.
- **RE-02: Verbatim preservation tier** — `PreservedBlock`/`PreservedInline` so tables/math/raw-HTML round-trip byte-for-byte (Tier 2); raw HTML kept inert (no `innerHTML`, MAJ-04).
- **RE-05: Editor-host abstraction** — extract `PageEditor`'s save/reconcile/lock/409 host seam; mount CM6 through it as default with ZERO behavior change; extend `editorMode` to 3-way.
- **RE-03: TipTap `#tag` + suggestions + slash + real save** — Tag node matching the corpus; `#`-suggest / `/`-slash; body `#tags` materialize through the REAL save; paste-XSS hardened (MAJ-05).
- **RE-04: Toggle + load guard + code-split** — persisted 3-way toggle; warn-AND-fall-back load guard; `assertLazyChunks` proves TipTap out of the entry chunk; shared-content fidelity design fork resolved.

### Deferred / Queued

<!-- Future items (detailed in REQUIREMENTS.md) -->

- **TipTap rich editing of tables/math** — v1.7 preserves them verbatim (Tier 2); structured WYSIWYG editing is a follow-on.
- **Hierarchy-aware tag behavior** — auto-apply parent tags, descendant-expanding search, hierarchy graph grouping (v1.6's TF-03 is display-only).
- **Multi-level tag-op undo** — v1.6's TF-05 is last-op undo; a full undo stack is deferred.
- **Per-page local graph** / **in-place file-tree filter** — v1.5 removals; re-introduce if missed.

### Out of Scope

<!-- Explicit boundaries (SPEC §4 non-goals) with reasoning to prevent re-adding. -->

- Notion-style databases, Kanban boards, database-object tables — not core to an open-Markdown wiki; high complexity
- Real-time Google-Docs-level collaboration (Yjs/CRDT) — deferred to a Phase-2 prototype; soft locks suffice for a 5-person team
- Comments, public sharing, web publishing, enterprise SSO — outside MVP value for an internal team
- Direct agent writes without user approval — violates the safety model; all agent writes require review
- In-browser editing of DOCX/PDF — originals are immutable; only extracted text is read
- **Replacing CM6 with TipTap** (TipTap as default / CM6 retirement) — a v2.0 question; v1.7 ships TipTap opt-in only, CM6 stays the byte-exact default. Degrading the fidelity gate to force a ship is also out of scope (NO-GO is a valid outcome).
- Mobile native app, per-paragraph permissions, complex workflow automation — not MVP

## Context

- **Domain:** internal knowledge base / wiki, "agent-era" — files-as-truth so both humans and agents read the same Markdown.
- **Current state (post-v1.0, 2026-06-24):** v1.0 Knowledge Graph & LLM Auto-Tagging shipped — 5 phases (14 plans, 34 tasks) on top of the v0.9.9 MVP (8 phases, 36 plans). A single CGO-free Go binary serves the embedded React SPA, backed by `internal/{config,server,auth,users,repo,okf,attachments,search,gitstore,agent,jobs,locks,audit,graph,tagsweep,web}` + `cmd/okf-workspace`. v1.0 added the `internal/graph` derived link/tag store + query layer, `internal/tagsweep` async bulk-suggest jobs, `internal/agent` SuggestTags mode, and the React graph/backlinks/tag-review UI (one new frontend dep: `react-force-graph-2d`, Canvas-only, three.js absent). ~85 commits over 2026-06-23→24. Tech stack as locked: chi, Goldmark, Bleve, Eino + DeepSeek (provider-agnostic), modernc SQLite, React 19 + Vite + CM6. Known carry-forward: P10 canvas-pixel visual UAT and GraphCanvas bundle lazy-load (see STATE.md Deferred Items); `docs/` not yet authored (standing-team docs refresh deferred at v1.0 close).
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
| Target = full MVP (SPEC Phases 0–5), SPEC.md is source of truth | User confirmed the whole MVP, not just the §24 first-prototype slice | ✓ Good — MVP shipped v0.9.9 |
| Frontend framework: React | Largest ecosystem; Markdown renderer, editor (TipTap later), diff, and upload components target React first | ✓ Good — React 19 + Vite + CM6 through v1.0 |
| Search engine: Bleve | Richer relevance/faceting than SQLite FTS5; pure-Go, no extra service | ✓ Good — shipped v0.9.9 |
| HTTP router: chi | Lightweight, idiomatic, composable middleware over net/http | ✓ Good — RBAC subgroups reused cleanly in v1.0 |
| Agent LLM: provider-agnostic via config | Keep deployment flexible (local Ollama or remote) for the "lightweight self-host" goal | ✓ Good — DeepSeek used live; no code change for provider |
| Files-as-truth; SQLite = operational data only | Avoid proprietary lock-in; data stays portable/agent-readable | ✓ Good — v1.0 graph/tag store is a derived cache, rebuildable from files |
| Git hidden behind backend; commits automatic | Non-technical users must not need Git knowledge | ✓ Good — tag-apply rides the same single-writer commit path |
| Agent writes require explicit user approval | Safety: agent proposes diffs, human approves before apply/commit | ✓ Good — extended to tagging: suggest→approve, no silent writes |
| MVP editor = Markdown-with-preview; TipTap deferred to Phase 2 | Protect Markdown round-trip; avoid corruption risk of a rich editor in MVP | ✓ Good — CM6 live-preview shipped Phase 6; TipTap still deferred |
| v1.0: derived graph/tag store is a SQLite cache, never source of truth | Preserve files-as-truth; deleting + rebuilding from `.md` reproduces byte-identical adjacency | ✓ Good — `-race`-clean rebuild parity proven; admin reindex backstop |
| v1.0: byte-stable `okf.SetTags` via `yaml.Marshal`, model output re-validated server-side | Only the `tags` region changes; never trust LLM output or client tag list at the write | ✓ Good — security review confirmed no YAML/frontmatter injection vector |
| v1.0: `react-force-graph-2d` (Canvas-only), lockfile guard that `three` is absent | One new frontend dep; keep the initial editor bundle lean (graph code-splits) | ✓ Good — three.js provably absent; graph in its own lazy chunk |
| v1.5: in-place tree-filter removed; navrail search replaces the tree | One find surface; full-text > title-prefix for a growing wiki | Removed in the reading-shell fold-in; Future re-intro tracked (REQUIREMENTS.md "In-place file-tree filter") |
| v1.5: per-page local graph removed; global `/app/graph` retained | Local neighborhood underused vs its route+handler+panel weight | `/graph/local` + `Neighborhood` + `LocalGraphPanel`/`DepthControl` removed end-to-end; Future re-intro tracked (REQUIREMENTS.md "Re-introduce a per-page local graph") |
| v1.6: tag metadata/hierarchy live in a NEW committed on-disk store (files-as-truth), SQLite only a rebuildable cache | Tag color/description/parent are durable data; a cache-only store would violate files-as-truth (data must copy off the server) | Store shape (`.okf-workspace/tags.json` vs `Tags/*.md`) decided in Phase 32 discuss-phase; metadata is adjunct — tags fully usable with the store absent |
| v1.6: TipTap (rich editor) split to v1.7 | Independent red-team: combined 11-phase v1.6 was the largest milestone ever; TipTap is a self-contained editor sub-project + its shared-content fidelity decision must not be rushed | Co-driver accepted; v1.6 = tag completion + authoring polish (7 phases); TipTap requirements + red-team findings preserved in REQUIREMENTS.md §R |
| v1.7: TipTap coexists opt-in beside CM6 under a TWO-TIER Markdown-fidelity gate; CM6 stays the byte-exact default | CM6 holds byte-stability by construction (doc IS the Markdown); TipTap serializes a tree so it can't — Tier-1 semantic-lossless+idempotent for supported syntax, Tier-2 byte-for-byte verbatim for unmodeled (tables/math/HTML) | Locked decision for v1.7; the round-trip corpus is a GO/NO-GO spike gate — the gate is never degraded to force a ship |

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
*Last updated: 2026-07-02 — v1.6 (Tag System Completion & Authoring Polish) shipped; v1.7 milestone (Rich-Block Editor / TipTap) started: RE-01…05 requirements + risk-first roadmap (Phases 38–42) authored, v1.6 §R red-team findings folded, independent red-team gate run.*
