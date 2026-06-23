# Feature Research

**Domain:** Self-hosted, OKF-native internal team wiki / knowledge base (lightweight Notion / Outline / Wiki.js / BookStack alternative) for a ~5-person team, with files-as-truth, hidden Git versioning, and an approval-gated Eino AI agent.
**Researched:** 2026-06-17
**Confidence:** HIGH (scope is fixed by SPEC.md as source of truth; competitor research used only to validate table-stakes vs differentiator categorization)

> Scope note: this file is mapped to the SPEC's MVP (Phases 0–5), not to generic wiki feature lists. Every "Launch With" item traces to a SPEC §6 / §13 / §14 / §15 requirement. Competitor features outside the SPEC scope are recorded as anti-features or deferred items with the SPEC §4 non-goal that excludes them.

## Feature Landscape

### Table Stakes (Users Expect These)

Missing any of these and the team falls back to "a shared folder plus random documents" — i.e. the product fails its own success criterion (SPEC §25). Validated against Outline, Wiki.js, BookStack, LeafWiki — all ship these.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Local login + logout, session cookies (SPEC §6.5, Phase 0) | Internal tool gating; no anonymous wiki | MEDIUM | Argon2id/bcrypt, HTTPOnly + SameSite, CSRF on mutations (SPEC §21.4). Admin bootstrapped on first startup. |
| Roles: admin / editor / reader (SPEC §6.5) | Even a 5-person team needs read-only vs edit | LOW | Coarse RBAC only; per-paragraph/per-page ACLs are an anti-feature (SPEC §4). |
| Left-side page tree, expand/collapse, current-page highlight (SPEC §6.2, Phase 1) | Primary navigation; every competitor has a tree/sidebar | MEDIUM | Built from filesystem walk + `.okf-workspace/manifest.json`. Folder = directory, page = `.md`. |
| Create / rename / move / delete-to-trash / restore page (SPEC §6.1, Phase 1) | Basic CRUD; "delete" must be recoverable to feel safe | MEDIUM | Trash lives in `.okf-workspace/trash/`. Move/rename must update links (see dependencies). |
| Create folder, create page inside folder (SPEC §6.2) | Users organize as they write | LOW | Folder creation = mkdir + optional `index.md`. |
| Markdown editor with live preview + render (SPEC §6.1, §8.2, Phase 1) | The core read/write loop | MEDIUM | MVP is Markdown-with-preview, NOT a rich block editor. Goldmark renders; round-trip safety is paramount (SPEC §5.5, §22.3). |
| Page metadata: title, tags, description (SPEC §6.1) | Findability + frontmatter contract | MEDIUM | Stored in YAML frontmatter; required-field repair on save (SPEC §10). |
| Link to another page (SPEC §6.1) | A wiki without internal links is just notes | MEDIUM | Relative Markdown links; resolver must survive moves/renames. |
| Full-text + title + tag search (SPEC §6.3 nav, §12, Phase 3) | "If search feels unreliable, a wiki becomes a graveyard" (competitor consensus) | MEDIUM | Bleve index over pages; result types page/attachment/heading. |
| Recent pages (SPEC §6.2) | Fast return to in-progress work | LOW | UI pref / SQLite operational data. |
| Upload + download original attachments (SPEC §6.3, Phase 2) | Knowledge bases carry PDFs/diagrams; download must be byte-identical | MEDIUM | Original never modified (SPEC §5.2, §11). `Content-Disposition: attachment` for risky types (SPEC §21.2). |
| Attachment cards (file name, size, uploader, date) (SPEC §19) | Visible, actionable attachment UX | LOW | Image attachments get inline preview; documents get Download/Summarize/Ask/Remove. |
| Page version history + restore previous version (SPEC §6.1, §6.6, Phase 1) | "Track changes, revert" is universally expected | MEDIUM | Sourced from Git log; presented as plain version list (no Git jargon). |
| Audit log of key actions (SPEC §21.5) | Internal accountability | LOW–MEDIUM | login, page CRUD, attachment up/down/delete, agent prompt, patch approval, config change. SQLite audit mirror. |

### Differentiators (Competitive Advantage)

These are the product's reason to exist over Outline/Wiki.js/BookStack. They align directly with PROJECT.md Core Value: "data stays open, portable, and the wiki usable without Git knowledge." Competitor research confirms none of the three mainstream rivals combine all three.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Files-as-truth: plain Markdown + YAML frontmatter on disk** (SPEC §5.1, §10) | Zero lock-in; copy the folder and you have everything. Outline/Wiki.js trap content in Postgres/MySQL; BookStack in MySQL. | MEDIUM | SQLite is operational-only and must never become source of truth. This is a hard architectural invariant, not just a feature. |
| **Attachments as first-class immutable originals + metadata sidecar + extracted-text sidecar** (SPEC §5.2, §11) | A PDF stays a PDF forever; humans download originals, agents read extracted text. Notion/Outline re-encode or hide originals. | MEDIUM | Three-part model: `originals/`, `metadata/*.json`, `extracted/*.txt`. SHA-256 + collision-resistant stored names. |
| **Hidden Git versioning for non-technical users** (SPEC §5.3, §14) | Full Git history/backup/audit with zero Git knowledge required — no commits, branches, or paths exposed. | HIGH | Automatic commits on save/upload/approved-patch; batched (not per-keystroke); identity in commit metadata; optional remote push to Gitea/GitLab. This is where most engineering risk concentrates. |
| **Approval-gated Eino AI agent (propose → review diff → approve → apply → commit)** (SPEC §5.4, §15) | "AI helps but never silently writes." Matches the 2026 human-in-the-loop pattern (Dify Human Input, Karpathy LLM Wiki) but as a first-class, self-hosted, file-native flow. | HIGH | Agent uses tools only; read tools free, write tools (`apply_page_patch`, `create_page`, `attach_file_to_page`) require explicit user approval. Hard sandbox (SPEC §15.3, §21.3). |
| **Attachment Q&A / summarize from extracted text** (SPEC §6.3, §6.4) | "Summarize this contract.pdf" / "Ask AI about this DOCX" — answers grounded in uploaded docs, not just pages. | MEDIUM–HIGH | Depends on the text-extraction pipeline (see dependencies). Scanned/image PDFs degrade — extraction status surfaced in UI. |
| **Workspace-scoped agent context modes** (current page / selection / attachment / whole workspace) (SPEC §6.4, §15.2) | One prompt bar, scoped intent — feels like Notion AI but local and provider-agnostic. | MEDIUM | Modes: ask / summarize / rewrite / patch / create. Maps to Eino tool orchestration, not plain CRUD. |
| **Provider-agnostic LLM (local Ollama or remote OpenAI-compatible)** (PROJECT constraints, config §20.3) | Self-host with no cloud dependency or recurring cost; swap models via `config.yaml`. | LOW–MEDIUM | OpenAI-compatible endpoint abstraction; `api_key_env` for remote. |
| **Single Go binary + data dir deployment** (SPEC §3.3, §20) | Runs on a homelab/VPS with no Postgres/Redis/Elasticsearch/K8s. Wiki.js/Outline need a DB server + often Redis. | MEDIUM | Frontend embedded/served by Go; SQLite + filesystem only. Docker + systemd packaging. |
| **Soft locks + presence + conflict-diff resolution** (SPEC §6 collaboration, §13.1, Phase 5) | Right-sized collaboration for 5 people without CRDT complexity. | MEDIUM | Optimistic concurrency via document revision; conflict offers overwrite / manual-merge / save-as-copy. |

### Anti-Features (Deliberately NOT Built — SPEC §4 Non-Goals)

Each row records the surface appeal, the real cost, and the SPEC §4 reasoning so they are not re-added during roadmap/requirements work.

| Feature | Why Requested | Why Problematic / SPEC reasoning | Alternative |
|---------|---------------|----------------------------------|-------------|
| Notion-style databases / Kanban / DB-object tables | "Notion does it" | Not core to an open-Markdown wiki; very high complexity; breaks files-as-truth (a DB table is not a Markdown file). SPEC §4. | Plain Markdown tables; external tools for project tracking. |
| Real-time Google-Docs / Yjs / CRDT co-editing | Feels modern; avoids conflicts | Heavy infra (WS rooms, CRDT, debounced export, commit queue); overkill for 5 users. SPEC §4, §13.2. | Soft locks + optimistic concurrency + conflict diff (Phase 5). Yjs only as a Phase-2 *research prototype*. |
| TipTap / rich block editor | Nicer WYSIWYG | Can silently corrupt Markdown round-trip — directly violates SPEC §5.5. PROJECT defers it. | Markdown editor with preview; TipTap only after round-trip tests pass (SPEC §8.2, §22.3). |
| Comments / inline discussion | Collaboration nicety | Out of MVP value for a 5-person team; adds storage + notification surface. SPEC §4. | Edit the page directly; Git history shows who/what. |
| Public sharing / web publishing | Share docs externally | Expands auth/exposure surface; internal-only product. SPEC §4. | Export the Markdown file; it is portable by design. |
| Enterprise SSO (SAML/OIDC) | "IT wants it" | Heavy integration for 5 internal users; outside MVP value. SPEC §4. | Local username/password with roles (SPEC §6.5). |
| Direct agent writes without approval | "Let the AI just fix it" | Violates the core safety model — the single most load-bearing non-goal. SPEC §4, §5.4, §15.3. | propose → review diff → approve → apply → commit (SPEC §15.4). |
| In-browser editing of DOCX/PDF | Edit attachments in place | Originals must stay immutable; editing breaks byte-identical download + provenance. SPEC §4, §11. | Download, edit externally, re-upload as a replacement version (SPEC §6.3 replace). |
| Per-paragraph / per-page permission model | Granular control | High complexity, low value at 5 users; roles suffice. SPEC §4. | Coarse admin/editor/reader roles. |
| Complex workflow automation | "Automate processes" | Scope explosion; not a knowledge-base concern. SPEC §4. | Out of scope; keep the tool focused on read/write/find/ask. |
| Mobile native app | On-the-go access | Build/maintain cost; responsive web is enough for internal use. SPEC §4. | Responsive web UI. |
| Right-side document outline panel | TOC convenience | Explicitly excluded from MVP layout to keep the three-pane UI simple. SPEC §2. | Optional toggleable floating panel *later*, if needed. |
| Agent downloading remote URLs / running shell by default | "Let it fetch/run things" | Expands attack surface; violates agent sandbox. SPEC §15.3, §21.3. | Tools only, no remote fetch/shell unless a future admin-only, explicit tool. |

## Feature Dependencies

```
Phase 0: Auth + session + data-dir/Git init
    └──required by──> EVERYTHING (no feature ships without the skeleton)

Phase 1: OKF pages
    Frontmatter parse/repair ──required by──> Page metadata (tags/desc/title)
    Page CRUD ──requires──> Safe path resolver (SPEC §21.1)
    Page rename/move ──should-update──> internal page links (link integrity)
    Page history/restore ──requires──> Git commit pipeline (hidden Git)
    Markdown render ──requires──> Markdown-round-trip safety (gates TipTap forever)

Phase 2: Attachments
    Upload/store original ──requires──> Safe path resolver + upload validation (§21.2)
    Text-extraction pipeline ──requires──> Attachment store + job worker
    Extracted-text sidecars ──required by──> Attachment search (Phase 3)
    Extracted-text sidecars ──required by──> Agent attachment Q&A / summarize (Phase 4)

Phase 3: Search
    Bleve index ──requires──> Page store (P1) + extracted text (P2)
    Search ──enhances──> Navigation and Agent (agent uses search_pages/search_attachments tools)

Phase 4: Eino agent
    Read tools (read_page/search/read_attachment_text) ──require──> P1+P2+P3
    propose_page_patch ──requires──> diff generation + DiffReviewDialog UI
    apply_page_patch (write) ──requires──> approval gate ──then──> Git commit (P1 pipeline)
    Agent sandbox ──requires──> safe path resolver + secret isolation (§21.3)

Phase 5: Collaboration
    Soft locks + presence ──require──> document revision / optimistic concurrency
    Conflict UI (overwrite/merge/save-as-copy) ──requires──> diff rendering (shared with P4)
```

### Dependency Notes

- **Text extraction (P2) is the gate for both attachment search (P3) and agent attachment Q&A (P4).** If extraction is weak (e.g. scanned/image-only PDFs needing OCR — a known hard problem per LlamaIndex/Tika research), those downstream features silently degrade. Surface extraction status in the attachment card and treat OCR as out-of-MVP; extract embedded text layers only.
- **Hidden Git pipeline (P1) is reused by history/restore (P1), attachment commits (P2), and agent-approved patches (P4).** Build the commit/batching/identity layer once, correctly, in Phase 1 — it is the spine of versioning. Restore = a new commit reverting content, never a destructive history rewrite (keeps history readable per SPEC §25).
- **Diff rendering is shared between agent patch review (P4) and conflict resolution (P5).** Build a reusable diff component (DiffReviewDialog) so the same UI serves both.
- **Safe path resolver (§21.1) is a cross-cutting dependency** of page CRUD, attachment storage, and the agent sandbox. It must land in Phase 0/1 and be unit-tested first (SPEC §22.1 lists it first for a reason).
- **Markdown round-trip safety conflicts with any future rich editor.** TipTap (P2-future) must not ship until round-trip tests (headings, nested lists, code blocks, tables, links, images, attachment links, frontmatter) pass. This is a hard gate, not a preference.
- **Rename/move (P1) conflicts with naive link storage.** Internal links and attachment `linked_pages` references must be updated (or resolved indirectly) on move, or links silently break — a classic wiki rot pitfall.

## MVP Definition

### Launch With (v1) — SPEC Phases 0–5

Minimum viable product: the full SPEC MVP. PROJECT.md confirms target = full MVP, not the §24 first-prototype slice.

- [ ] **Phase 0 — Skeleton & auth** — single Go binary serves embedded React UI; local login/logout, sessions, admin bootstrap, RBAC; data dir + Git repo init. *Essential: nothing runs without it.*
- [ ] **Phase 1 — OKF pages + navigation + hidden Git** — tree, page CRUD + trash/restore, frontmatter parse/repair, Markdown render+edit, internal links, history/restore, automatic batched commits. *Essential: this is the wiki.*
- [ ] **Phase 2 — Attachments** — upload/download originals, cards, replace, delete (+ repo cleanup when unreferenced), metadata sidecars, text extraction for PDF/DOCX/TXT. *Essential differentiator: first-class files.*
- [ ] **Phase 3 — Search (Bleve)** — title, body, tag, attachment filename, extracted-text search; page/attachment/heading result types. *Essential: search is table stakes.*
- [ ] **Phase 4 — Eino agent** — bottom prompt; ask/summarize/rewrite/draft/propose-patch over page/selection/attachment/workspace; propose→review→approve→apply→commit; read/write tool boundary + sandbox. *Essential differentiator: the agent edge.*
- [ ] **Phase 5 — Collaboration (MVP)** — soft locks, presence, optimistic concurrency, conflict diff with overwrite/merge/save-as-copy. *Essential for safe multi-user editing.*
- [ ] **Cross-cutting — Security & audit** — safe path resolver, upload validation, agent sandbox, CSRF/session safety, audit log. *Essential: untrusted input surface (SPEC §21).*

### Add After Validation (v1.x)

- [ ] **XLSX / ZIP text extraction** — upload/download already in MVP; add extraction once core extraction proven (trigger: users want spreadsheet/archive content searchable/askable).
- [ ] **Toggleable document outline panel** — add if users ask for in-page TOC (SPEC §2 explicitly defers it).
- [ ] **Richer agent tools** (cross-link generation, index/nav page updates) — SPEC §3.2 agent goals beyond MVP tool list; add once base agent loop is trusted.
- [ ] **Remote push hardening / pull-on-startup conflict handling** — basic optional push is in MVP; robust two-way sync after single-node use is validated.

### Future Consideration (v2+)

- [ ] **TipTap rich editor** — gated on Markdown round-trip tests passing (SPEC §8.2). Defer until the editor model is provably lossless.
- [ ] **Realtime Yjs/CRDT collaboration** — only if soft locks prove insufficient at scale (SPEC §13.2). Phase-2 prototype first.
- [ ] **OCR for scanned/image PDFs** — defer; embedded-text extraction only in MVP. Heavy dependency, uncertain quality.
- [ ] **Notion-style databases, comments, public sharing, SSO, mobile app** — remain anti-features unless product direction changes (SPEC §4).

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Auth + sessions + RBAC (P0) | HIGH | MEDIUM | P1 |
| Page tree + CRUD + frontmatter (P1) | HIGH | MEDIUM | P1 |
| Markdown editor + preview + render (P1) | HIGH | MEDIUM | P1 |
| Hidden Git: commits + history + restore (P1) | HIGH | HIGH | P1 |
| Attachments: upload/download originals + cards (P2) | HIGH | MEDIUM | P1 |
| Text extraction pipeline (P2) | HIGH | HIGH | P1 |
| Search incl. extracted text, Bleve (P3) | HIGH | MEDIUM | P1 |
| Eino agent: ask/summarize (read-only) (P4) | HIGH | HIGH | P1 |
| Agent propose-patch → approve → commit (P4) | HIGH | HIGH | P1 |
| Soft locks + presence + conflict diff (P5) | MEDIUM | MEDIUM | P1 (MVP) |
| Audit log (cross-cutting) | MEDIUM | LOW–MEDIUM | P1 |
| Provider-agnostic LLM config | HIGH | LOW–MEDIUM | P1 |
| Single-binary + Docker/systemd packaging | HIGH | MEDIUM | P1 |
| XLSX/ZIP extraction | MEDIUM | MEDIUM | P2 |
| Document outline panel | LOW | LOW | P3 |
| Agent cross-link / nav-update tools | MEDIUM | MEDIUM | P2 |
| TipTap rich editor | MEDIUM | HIGH | P3 (gated) |
| Realtime Yjs/CRDT | LOW (at 5 users) | HIGH | P3 |
| OCR for scanned PDFs | MEDIUM | HIGH | P3 |

**Priority key:** P1 = must have for launch (full SPEC MVP) · P2 = add after validation · P3 = future/gated.

## Competitor Feature Analysis

| Feature | Outline | Wiki.js | BookStack | LeafWiki | Our Approach (OKF Workspace) |
|---------|---------|---------|-----------|----------|------------------------------|
| Storage of truth | Postgres | Postgres/MySQL/SQLite + assets | MySQL | Markdown on disk + SQLite | **Markdown files on disk = truth; SQLite operational-only** |
| Versioning | DB revisions | DB page history | DB page revisions | Git-backed | **Hidden Git, automatic batched commits, restore via revert** |
| Editor | Rich (slash cmds, realtime) | Markdown + visual | WYSIWYG + Markdown | Markdown | **Markdown + preview (rich editor deferred, round-trip-gated)** |
| Search | Built-in | Pluggable (Elastic/DB) | DB full-text | SQLite FTS5 | **Bleve (pages + extracted attachment text)** |
| Attachments | Stored/managed | Asset store | Image/file attach | Files on disk | **First-class immutable originals + metadata + extracted-text sidecars** |
| AI / agent | Notion-style AI (cloud) | None core | None core | None | **Self-hosted Eino agent, approval-gated, provider-agnostic** |
| Collaboration | Realtime CRDT | Basic | Page locking | Basic | **Soft locks + optimistic concurrency + conflict diff** |
| Deployment | Node + Postgres (+Redis) | Node + DB | PHP + MySQL | Single Go binary + SQLite | **Single Go binary + data dir; no external services** |
| Auth | SSO-heavy | Modular incl. SSO | Local + SSO | Local | **Local users + RBAC (SSO is an anti-feature for MVP)** |

Takeaway: the closest neighbor architecturally is **LeafWiki** (Go + SQLite + Markdown on disk). OKF Workspace's differentiation over it is the **immutable first-class attachment model**, **hidden Git** (LeafWiki added Git/FTS5 but does not hide it as a non-technical UX), and the **approval-gated AI agent** — none of the four mainstream rivals combine all three.

## Sources

- [XWiki vs BookStack vs Outline comparison — MassiveGRID](https://massivegrid.com/blog/xwiki-vs-bookstack-vs-outline-comparison/) — MEDIUM confidence (vendor blog)
- [BookStack vs Wiki.js — Canadian Web Hosting](https://blog.canadianwebhosting.com/bookstack-vs-wikijs-choosing-self-hosted-team-wiki/) — MEDIUM
- [Wiki.js vs Outline — DEV Community](https://dev.to/selfhostingsh/wikijs-vs-outline-which-to-self-host-lo1) — MEDIUM
- [Outline vs BookStack — elest.io](https://blog.elest.io/outline-vs-bookstack-which-self-hosted-wiki-for-your-team/) — MEDIUM
- [LeafWiki — single Go binary, SQLite, Markdown on disk (GitHub)](https://github.com/perber/leafwiki) — HIGH (closest architectural analog)
- [LeafWiki devlog: backlinks, search, SQLite FTS5 — DEV](https://dev.to/perber/leafwiki-devlog-7-v061-introducing-backlinks-better-search-sqlite-fts5-1llo) — MEDIUM
- [Top 12 Self-hosted Wiki Engines 2024 — Medevel](https://medevel.com/12-self-hosted-wiki-engines-for-2024/) — MEDIUM
- [Karpathy LLM Wiki pattern (gist)](https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f) — MEDIUM (validates AI-maintained, file-native KB pattern)
- [Dify Knowledge Pipeline / Human Input node](https://dify.ai/blog/introducing-knowledge-pipeline) — MEDIUM (validates human-approval AI workflow pattern)
- [LlamaIndex — PDF text extraction challenges](https://www.llamaindex.ai/glossary/pdf-text-extraction) — HIGH (validates extraction/OCR pitfall)
- [Elastic Workplace Search — content extraction (Apache Tika)](https://www.elastic.co/guide/en/workplace-search/current/content-sources-content-extraction.html) — HIGH (text-extraction expectations)
- SPEC.md (project source of truth) and .planning/PROJECT.md — HIGH (authoritative scope)

---
*Feature research for: self-hosted OKF-native internal team wiki with hidden Git + approval-gated AI agent*
*Researched: 2026-06-17*
