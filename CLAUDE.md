<!-- GSD:project-start source:PROJECT.md -->

## Project

**OKF Workspace**

OKF Workspace is a lightweight, self-hosted, OKF-native internal wiki built for the agent era. Human-readable Markdown files (with YAML frontmatter) are the source of truth, Git provides hidden version history, uploaded attachments stay downloadable as their original files, and a CloudWeGo Eino agent can read, search, summarize, and propose edits that a human approves before they are applied. It targets a small internal team (~5 people) who want Notion-like simplicity without vendor lock-in, monthly cost, or a proprietary knowledge database.

**Core Value:** A non-technical teammate can create, edit, and find knowledge — and get useful AI help on it — while every byte stays as plain Markdown + original files on disk, versioned in Git, with no proprietary store to escape. If everything else fails, *the data must remain open, portable, and the wiki usable without Git knowledge.*

### Constraints

- **Tech stack — Backend:** Go, single process, **chi** router, Goldmark + YAML for Markdown/frontmatter, **shell out to git CLI first** for versioning, slog/zerolog logging — must compile to one binary.
- **Tech stack — Search:** **Bleve** (pure-Go full-text index) — chosen over SQLite FTS5 for richer relevance/faceting.
- **Tech stack — Frontend:** **React** + Vite + TypeScript, bundled into static assets served/embedded by the Go backend; MVP editor is a Markdown editor with preview (not a rich block editor).
- **Tech stack — Agent:** CloudWeGo **Eino**, used only for orchestration/agentic flows (Q&A, summarize, patch proposal, attachment Q&A) — not for plain CRUD. LLM access is **provider-agnostic / configurable** via `config.yaml` (OpenAI-compatible endpoint; local Ollama or remote both supported).
- **Tech stack — DB:** SQLite for operational metadata only.
- **Deployment:** must self-host as one Go binary + a data directory on a small VPS/homelab/VM — no PostgreSQL, Redis, Elasticsearch, or Kubernetes required; portable by copying the repo + config; Docker + systemd packaging supported.
- **Team/scale:** small internal team (~5 users) — informs the "soft locks beat realtime CRDT" tradeoff.
- **Security:** untrusted input surface (uploads, paths, agent tools, auth) must be hardened per SPEC §21 — path-safety resolver, upload validation, agent sandbox, CSRF/session safety, audit logging.
- **Data openness:** all content must remain plain files copyable off the server; Git history must stay readable/useful.

<!-- GSD:project-end -->

<!-- GSD:stack-start source:research/STACK.md -->

## Technology Stack

## Recommended Stack

### Core Technologies (LOCKED — versions validated)

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Go | 1.26.0 | Backend language, single static binary | Locked. 1.26 toolchain present in env; gives `embed` for shipping the SPA, mature `log/slog`, generics (used by Eino tool helpers). |
| `github.com/go-chi/chi/v5` | v5.3.0 | HTTP router + middleware | Locked. Idiomatic `net/http`-compatible, composable middleware, zero heavy deps. v5.3.0 released 2026-05-22. |
| `github.com/yuin/goldmark` | v1.8.2 | Markdown → HTML rendering | Locked. CommonMark-compliant, extensible (GFM tables, autolinks via `extension` package), the de-facto Go Markdown engine (used by Hugo). |
| `github.com/blevesearch/bleve/v2` | v2.6.0 | Full-text search index | Locked. Pure-Go, embeddable, supports faceting + per-field analyzers + highlighting (gives the page/heading/attachment result types the SPEC needs). No external service. |
| `github.com/cloudwego/eino` | v0.9.9 | Agent orchestration (ReAct, tools, graph) | Locked. Core framework. Released 2026-06-17 (today). Provides `flow/agent/react.NewAgent` and typed tool helpers. |
| `github.com/cloudwego/eino-ext/components/model/openai` | v0.0.0-20260616080858-ab17b7308bf8 (pseudo-version; `@latest`) | OpenAI-compatible ChatModel | Locked. `ChatModelConfig{BaseURL, APIKey, Model}` drives Ollama or any OpenAI-compatible endpoint from `config.yaml`. eino-ext is versioned by pseudo-version — pin with `go get ...@latest` then commit `go.sum`. |
| React | 19.2.7 | Frontend UI library | Locked. Current stable React 19.x. |
| React DOM | 19.2.7 | DOM renderer | Locked. Must match React major. |
| Vite | 8.0.16 | Frontend build/dev server | Locked. Vite 8 is current; build output embedded into the Go binary via `embed.FS`. |
| TypeScript | 6.0.3 | Frontend language | Locked. TS 6.x current. |

### Supporting Libraries — Backend (OPEN choices, prescribed)

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `gopkg.in/yaml.v3` | v3.0.1 | YAML frontmatter parse/emit | **Pick.** Stable, ubiquitous, struct-tag based. Use `yaml.Node` for the "preserve unknown/optional fields on round-trip" requirement (SPEC §10). See *What NOT to Use* re: goccy/go-yaml. |
| `modernc.org/sqlite` | v1.52.0 | SQLite driver (operational metadata only) | **Pick.** Pure-Go (no cgo) → keeps the "single static binary, cross-compile, no C toolchain" promise. Slightly slower than cgo but irrelevant at 5 users / metadata-only load. |
| `github.com/gabriel-vasile/mimetype` | v1.4.13 | Content-sniffing MIME detection | **Pick** for upload validation (SPEC §11/§21.2). Detects by magic bytes, not extension — exactly what "don't trust the filename" requires. Returns extension + MIME; supports a configurable allow-list. |
| `golang.org/x/crypto/argon2` | v0.53.0 (module `golang.org/x/crypto`) | Argon2id password hashing primitive | **Pick** (low-level). The reference Argon2id implementation. |
| `github.com/alexedwards/argon2id` | v1.0.0 | Argon2id hash/verify wrapper | **Pick** (ergonomic layer). Thin wrapper over `x/crypto/argon2` producing PHC-format strings with embedded params + `ComparePasswordAndHash`. Avoids hand-rolling salt/encoding (a classic footgun). |
| `github.com/alexedwards/scs/v2` | v2.9.0 | Server-side session management | **Pick** for SPEC §6.5/§21.4 session cookies. HTTPOnly + SameSite + secure flags built-in; pluggable stores — use the SQLite store so sessions live in `app.db` (operational data). 168h TTL maps to config. |
| `github.com/justinas/nosurf` | v1.2.0 | CSRF protection middleware | **Pick** for SPEC §21.4. `net/http`-native, double-submit cookie, composes with chi. Lighter than gorilla/csrf and actively maintained (v1.2.0 2025-05). |
| `github.com/ledongthuc/pdf` | v0.0.0-20250511090121-... | PDF → plain text extraction | **Pick (default).** Pure-Go, no cgo, no external binary — preserves single-binary deploy. Good enough for text-layer PDFs (the SPEC only needs extracted text for search/agent, originals are immutable). See *Alternatives* for scanned-PDF/OCR escalation. |
| `github.com/fumiama/go-docx` | v0.0.0-20250506085032-... | DOCX → text extraction | **Pick.** Pure-Go, actively maintained (2025), reads paragraphs/tables. DOCX is a zip of XML; this avoids the heavy/commercial unioffice. |
| `github.com/xuri/excelize/v2` | v2.10.1 | XLSX read (deferred-but-cheap) | **Optional.** SPEC says XLSX is upload/download-only in MVP; include only if you want XLSX cell-text in search later. Pure-Go. |
| stdlib `encoding/csv` | — (Go std) | CSV text extraction | **Pick.** No dependency needed; flatten rows to text for indexing. |
| stdlib `archive/zip` | — (Go std) | ZIP handling (no extraction in MVP) | **Pick.** Only needed to validate/inspect; SPEC keeps ZIP as upload/download-only. |
| `log/slog` | — (Go std) | Structured logging | **Pick.** Locked choice (slog *or* zerolog). Prefer stdlib `slog` — zero deps, JSON handler, context-aware; matches single-binary minimalism. Use zerolog only if you need its allocation-free hot path (not relevant at this scale). |
| `github.com/spf13/cobra` | v1.10.2 | CLI (`serve --config`, admin bootstrap) | **Pick.** SPEC §20.1 uses `okf-workspace serve --config ...` and admin bootstrap — cobra gives subcommands cleanly. (Optional; stdlib `flag` also fine for a 1–2 command CLI.) |

### Supporting Libraries — Frontend (OPEN choices, prescribed)

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `react-markdown` | 10.1.0 | Render Markdown body to React (read mode) | **Pick.** Renders to React elements (no `dangerouslySetInnerHTML` by default → safer). Pair with the plugins below. |
| `remark-gfm` | 4.0.1 | GFM tables/strikethrough/task-lists/autolinks | **Pick.** Matches Goldmark's GFM extensions so server render and client render agree. |
| `rehype-sanitize` | 6.0.0 | Sanitize rendered HTML | **Pick (required).** Wiki content can include agent- or user-authored HTML/links; sanitize to prevent stored XSS. Keep `rehype-raw` **off** unless you must render raw HTML — if you enable raw HTML, sanitize is mandatory. |
| `@uiw/react-md-editor` | 4.1.1 | MVP Markdown editor with live preview | **Pick.** Textarea-based Markdown editor (CodeMirror under the hood) with split preview — exactly the SPEC §8.2 "Markdown editor with preview, NOT a rich block editor" requirement. Edits raw Markdown → protects round-trip (no lossy block model). TipTap deferred to Phase 2 per locked decision. |
| `react-diff-viewer-continued` | 4.2.2 | Diff review UI (agent patch + version restore) | **Pick.** Side-by-side / inline diff component for the DiffReviewDialog (SPEC §18.3) and history compare. Maintained fork of the abandoned `react-diff-viewer`. |
| `diff` (jsdiff) | 9.0.0 | Compute diffs / apply when needed client-side | **Optional.** Backend produces the authoritative unified diff (SPEC §17.6); use jsdiff only if you want client-side preview of unsaved local edits. |
| `react-dropzone` | 15.0.0 | Drag-and-drop file upload | **Pick.** Powers the SPEC §19 drag-file-into-page UX; gives file-type/size pre-checks before the multipart POST. |
| `@tanstack/react-query` | 5.101.0 | Server-state/data fetching cache | **Pick.** Tree, page, search, agent calls are all server state; react-query handles caching, revalidation, optimistic updates (maps to SPEC §13 optimistic concurrency). |
| `zustand` | 5.0.14 | Client UI state (editor mode, presence, prompt) | **Pick.** Tiny, unopinionated store for ephemeral UI state (current mode, soft-lock/presence banner, prompt bar). Redux is overkill for 5 users. |
| `react-router-dom` | 7.18.0 | Routing (`/login`, `/app/page/:path`, `/admin`) | **Pick.** Matches SPEC §18.1 routes. |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `@vitejs/plugin-react` 6.0.2 | React Fast Refresh + JSX in Vite | Match Vite 8 major. |
| ESLint 10.5.0 + `@types/react` 19.2.17 | Lint + React types | Flat config (`eslint.config.js`) is the only format in ESLint 10. |
| Go `embed` (stdlib) | Embed built SPA into binary | `//go:embed web/dist` served by chi as static fallback → single-binary deploy (SPEC §20.1). |
| `github.com/golang-migrate/migrate/v4` v4.19.1 | SQLite schema migrations | Optional; or hand-roll idempotent `CREATE TABLE IF NOT EXISTS` for a 5-user metadata DB. Migrate is worth it once you have >3 tables. |
| Docker multi-stage + systemd | Packaging | SPEC §20.4/§20.5. Stage 1 builds SPA, stage 2 builds Go binary, final scratch/distroless image, non-root. |

## Installation

# --- Backend (go.mod) ---

# optional: go get github.com/xuri/excelize/v2@v2.10.1

# optional: go get github.com/golang-migrate/migrate/v4@v4.19.1

# --- Frontend (web/) ---

## Eino wiring (validated against current source)

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| `modernc.org/sqlite` (pure-Go) | `github.com/mattn/go-sqlite3` v1.14.45 (cgo) | If you measure SQLite as a bottleneck (you won't at 5 users) and accept a C toolchain + harder cross-compile. Pure-Go wins here for the single-binary goal. |
| `gopkg.in/yaml.v3` | `github.com/goccy/go-yaml` v1.19.2 | If you need comment preservation / better error positions in frontmatter. Trade-off: larger API surface and you'd own more edge cases. yaml.v3 + `yaml.Node` covers the round-trip requirement. |
| `github.com/ledongthuc/pdf` (pure-Go) | `github.com/gen2brain/go-fitz` v1.24.15 (MuPDF, OCR-capable) | If you need text from **scanned** PDFs (image-only) or robust layout extraction. go-fitz bundles MuPDF (cgo/large) — breaks pure single-binary; gate behind a build tag / optional sidecar if a deployment needs it. |
| `github.com/fumiama/go-docx` | `github.com/unidoc/unioffice` v1.39.0 | unioffice is more complete but **commercially licensed** (AGPL/paid) — avoid for an open self-hosted tool. |
| `github.com/justinas/nosurf` | `github.com/gorilla/csrf` v1.7.3 | If you prefer gorilla's API/`csrf.Token` template helper. Either works; nosurf is lighter and SPA-friendly. |
| stdlib `log/slog` | `github.com/rs/zerolog` v1.35.1 | If you want allocation-free logging on a hot path. Not needed at this scale; slog keeps deps at zero. |
| `@uiw/react-md-editor` | `@uiw/react-codemirror` 4.25.10 + `@codemirror/lang-markdown` 6.5.0 | If you want to build a custom editor surface (more control over the SPA, fewer bundled styles) and render preview with react-markdown yourself. md-editor is faster to ship for MVP. |
| `react-diff-viewer-continued` | `diff2html` 3.4.56 + `diff` 9.0.0 | If the backend already returns a unified-diff string and you just want to render it (diff2html renders unified-diff text directly). Good fit since SPEC §17.6 returns a `diff` string. |
| shell-out `git` CLI (LOCKED) | `github.com/go-git/go-git/v5` | Only if a deployment forbids a `git` binary. Locked decision is CLI-first; revisit only on a hard constraint. |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `github.com/unidoc/unioffice` for DOCX | Commercial/AGPL license — conflicts with an open, self-hostable tool. | `github.com/fumiama/go-docx` (pure-Go, permissive). |
| `github.com/nguyenthenguyen/docx` | Designed for find/replace in templates, not clean text extraction; unmaintained (2023). | `github.com/fumiama/go-docx`. |
| `github.com/h2non/filetype` | Last release 2021; smaller signature set than mimetype. | `github.com/gabriel-vasile/mimetype` v1.4.13. |
| `rehype-raw` without `rehype-sanitize` | Renders raw HTML from page content → stored XSS in a multi-user wiki. | Keep raw HTML off, or always pair with `rehype-sanitize`. |
| bcrypt as the *default* | bcrypt caps password input at 72 bytes and is weaker than Argon2id; SPEC §21.4 lists Argon2id first. | Argon2id via `alexedwards/argon2id`. (bcrypt acceptable only as a fallback if a deploy env can't afford Argon2 memory cost.) |
| TipTap / rich block editor in MVP | Locked: deferred to Phase 2, gated on Markdown round-trip tests. Block model risks corrupting canonical Markdown. | `@uiw/react-md-editor` (raw Markdown + preview). |
| SQLite FTS5 for search | Locked decision chose Bleve for richer relevance/faceting and to keep content out of SQLite. | Bleve v2.6.0. |
| Storing wiki content in SQLite | Violates the files-are-truth principle (SPEC §5.1, §8.1). | Markdown files on disk; SQLite for operational metadata only. |

## Stack Patterns by Variant

- Use `modernc.org/sqlite`, `github.com/ledongthuc/pdf`, `github.com/fumiama/go-docx` (all pure-Go, no cgo).
- Build with `CGO_ENABLED=0`. One artifact, no C toolchain.
- Add `github.com/gen2brain/go-fitz` (MuPDF) behind a build tag, or run extraction as an optional out-of-process step — do not make it the default (breaks static binary).
- `agent.base_url: http://localhost:11434/v1`, model e.g. `llama3.1` / `qwen2.5`; `OKF_LLM_API_KEY` can be any non-empty placeholder (Ollama ignores it).
- Same `openai.ChatModelConfig`, set real `BaseURL` + `APIKey` from `OKF_LLM_API_KEY`. No code change — provider-agnostic per locked decision.

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| React 19.2.7 | react-dom 19.2.7, @types/react 19.2.17 | Keep React/ReactDOM/types on the same major (19). |
| Vite 8.0.16 | @vitejs/plugin-react 6.0.2, Node 20.19+ | Env has Node v20.19.6 — meets Vite 8's Node requirement. |
| `react-markdown` 10 | remark-gfm 4, rehype-sanitize 6 | All on the unified v11+ ecosystem; align plugin majors with react-markdown 10. |
| `eino` v0.9.9 | `eino-ext` (pseudo-version `@latest`) | eino-ext tracks eino's interfaces; pin both via go.sum after `go get`. eino moves fast (v0.9.9 dated today) — re-verify the `react.NewAgent`/`openai.NewChatModel` signatures at implementation time. |
| `modernc.org/sqlite` v1.52.0 | Go 1.26 | Pure-Go; no cgo flags needed. |
| `alexedwards/scs/v2` v2.9.0 | chi v5, modernc sqlite | Use the SQLite session store so sessions persist in `app.db`. |

## Confidence Assessment

| Area | Confidence | Reason |
|------|------------|--------|
| Locked Go libs (chi, goldmark, bleve, eino) | HIGH | Versions from Go module proxy (`proxy.golang.org/.../@latest`), 2026-06-17. |
| Open Go libs (mimetype, argon2id, scs, nosurf, sqlite, pdf, docx) | HIGH | All version-verified via proxy; license/maintenance status checked. PDF/DOCX extraction quality is MEDIUM (pure-Go libs handle text-layer docs well; scanned PDFs need the go-fitz escape hatch). |
| Eino API (NewChatModel / InferTool / react.NewAgent) | HIGH | Verified against current source on GitHub `main` for paths, config structs, and constructor signatures. NOTE: eino is pre-1.0 and fast-moving — re-confirm at build time. |
| React/Vite/TS + frontend libs | HIGH | Versions from npm registry, 2026-06-17. |

## Sources

- `proxy.golang.org/<module>/@latest` — authoritative latest versions for all Go modules (chi v5.3.0, goldmark v1.8.2, bleve v2.6.0, eino v0.9.9, eino-ext pseudo-version, modernc.org/sqlite v1.52.0, mimetype v1.4.13, x/crypto v0.53.0, argon2id v1.0.0, scs v2.9.0, nosurf v1.2.0, ledongthuc/pdf, fumiama/go-docx, excelize v2.10.1, cobra v1.10.2) — HIGH
- `npm view <pkg> version` — authoritative npm versions (react/react-dom 19.2.7, vite 8.0.16, typescript 6.0.3, react-markdown 10.1.0, remark-gfm 4.0.1, rehype-sanitize 6.0.0, @uiw/react-md-editor 4.1.1, react-diff-viewer-continued 4.2.2, react-dropzone 15.0.0, @tanstack/react-query 5.101.0, zustand 5.0.14, react-router-dom 7.18.0) — HIGH
- `github.com/cloudwego/eino-ext/components/model/openai` source + `github.com/cloudwego/eino/flow/agent/react` + `components/tool/utils` — Eino import paths, `ChatModelConfig`, `NewChatModel`, `react.NewAgent`/`AgentConfig`, `utils.InferTool` signatures — HIGH (verified against `main`)
- SPEC.md §8/§10/§11/§14/§15/§20/§21 and PROJECT.md locked decisions — requirements mapping — HIGH

<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->

## Conventions

Conventions not yet established. Will populate as patterns emerge during development.
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->

## Architecture

Architecture not yet mapped. Follow existing patterns found in the codebase.
<!-- GSD:architecture-end -->

<!-- GSD:skills-start source:skills/ -->

## Project Skills

No project skills found. Add skills to any of: `.claude/skills/`, `.agents/skills/`, `.cursor/skills/`, `.github/skills/`, or `.codex/skills/` with a `SKILL.md` index file.
<!-- GSD:skills-end -->

<!-- GSD:workflow-start source:GSD defaults -->

## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:

- `/gsd-quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd-debug` for investigation and bug fixing
- `/gsd-execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->

<!-- GSD:profile-start -->

## Developer Profile

> Profile not yet configured. Run `/gsd-profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
