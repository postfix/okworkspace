# Architecture Research

**Domain:** Self-hosted single-binary Go service (chi) + React/Vite SPA — files-as-truth Markdown wiki with hidden Git versioning, Bleve search, a background job worker, and a CloudWeGo Eino agent
**Researched:** 2026-06-17
**Confidence:** HIGH (SPEC §7/§9/§11/§14/§16 is explicit and internally consistent; Eino orchestration model verified against CloudWeGo docs; remaining items are standard Go service patterns)

> **Mandate:** This is a *validation and refinement* of the SPEC's proposed architecture, not a new design. The SPEC's package layout (§16), repo layout (§9), and data flows (§14.2, §15.4, §19) are sound. Below they are confirmed, the component boundaries and interfaces are made explicit, the three key data flows are traced end-to-end, a dependency-driven build order is derived, and the four hard design tensions are addressed with concrete recommendations.

---

## Standard Architecture

### System Overview

The system is a **layered modular monolith**: one HTTP process, clear internal package seams, two persistence substrates (the Git-backed file repo = source of truth; SQLite = derived/operational cache), and an async job worker decoupling slow work (extraction, indexing, commits) from request handlers.

```
┌──────────────────────────────────────────────────────────────────────┐
│  React/Vite SPA (embedded static assets)                              │
│  AppShell · LeftTree · PageView/Editor · AttachmentPanel · PromptBar  │
└───────────────┬──────────────────────────────────┬───────────────────┘
                │ REST /api/v1 (JSON, cookie auth)   │ SSE (agent stream + job status)
┌───────────────▼──────────────────────────────────▼───────────────────┐
│  internal/server  (chi router, middleware: session, CSRF, RBAC, audit)│
│  internal/web     (embed.FS static assets + SPA fallback)             │
├────────────────────────────────────────────────────────────────────── ┤
│                         SERVICE LAYER (plain Go)                       │
│  ┌────────┐ ┌────────┐ ┌────────┐ ┌────────────┐ ┌────────┐ ┌───────┐│
│  │ auth   │ │ okf    │ │ repo   │ │ attachments│ │ search │ │ agent ││
│  │ users  │ │(parse/ │ │(safe   │ │(upload/    │ │(Bleve  │ │(Eino, ││
│  │        │ │ render)│ │ paths) │ │ metadata)  │ │ query) │ │ tools)││
│  └───┬────┘ └───┬────┘ └───┬────┘ └─────┬──────┘ └───┬────┘ └───┬───┘│
│      │          │          │            │            │          │     │
│      │   ┌──────▼──────────▼────────────▼────────────▼──────┐   │     │
│      │   │   internal/jobs (async worker + queue)            │◄──┘     │
│      │   │   text-extraction · index · git-commit · cleanup  │         │
│      │   └──────┬───────────────────────────────┬───────────┘         │
│      │          │                               │                     │
│  ┌───▼──────────▼───┐                  ┌─────────▼──────────┐          │
│  │ internal/audit   │                  │ internal/gitstore  │          │
│  │ (writes to both) │                  │ (shell-out git CLI)│          │
│  └───────┬──────────┘                  └─────────┬──────────┘          │
├──────────┼─────────────────────────────────────┼──────────────────────┤
│          ▼                                       ▼                      │
│  ┌───────────────┐                    ┌────────────────────────────┐   │
│  │  app.db       │  derived/cache     │  repo/  = SOURCE OF TRUTH   │   │
│  │  (SQLite)     │◄───── reflects ────│  *.md + assets/{originals,  │   │
│  │  users·sess·  │      (one-way)     │  extracted,metadata} +      │   │
│  │  jobs·index·  │                    │  .okf-workspace/{trash,     │   │
│  │  attach-refs· │                    │  locks,manifest}            │   │
│  │  audit-mirror │                    │  (a Git working tree)       │   │
│  └───────────────┘                    └────────────────────────────┘   │
└──────────────────────────────────────────────────────────────────────┘
```

**Rule that governs everything:** data flows *one way* — files are written first, then SQLite/Bleve are updated to reflect them. SQLite and the Bleve index are rebuildable caches; deleting `app.db` and `bleve/` must lose nothing except sessions and job queue state. This is the non-negotiable invariant from SPEC §8.1.

### Component Responsibilities

| Package | Owns / Responsibility | Depends on | Exposes (interface shape) |
|---------|----------------------|------------|---------------------------|
| `cmd/okf-workspace` | Process entrypoint, `serve` command, wiring/DI of all services, startup sequence (config → db → repo/git init → search open → job worker → http) | everything | `main()` |
| `internal/config` | Load+validate `config.yaml`, defaults, env overrides (`api_key_env`); typed config struct | — | `Load(path) (Config, error)` |
| `internal/server` | chi router, middleware stack (recover, request-id, session, CSRF, RBAC, audit), REST handlers, SSE endpoints | all services | `New(deps) http.Handler` |
| `internal/web` | `embed.FS` of built SPA, static serving + SPA history-fallback | — | `Handler() http.Handler` |
| `internal/auth` | Login/logout, password hash (Argon2id/bcrypt), session create/validate, cookie issuance, role checks | `users`, db | `Authenticate`, `Session`, `RequireRole(role)` middleware |
| `internal/users` | User CRUD, admin bootstrap on first start, roles (admin/editor/reader) | db | `Create/Get/List`, `BootstrapAdmin` |
| `internal/repo` | **Safe path resolver** (no `../`, abs, symlink escape), read/write Markdown, create folder, rename/move, delete-to-trash/restore, list tree | filesystem | `Resolve(rel) (abs, error)`, `Read/Write/Move/Trash`, `Tree()` |
| `internal/okf` | Parse/serialize YAML frontmatter (Goldmark + yaml), validate+repair required fields, render Markdown→HTML, new-page templates | — (pure) | `Parse([]byte) (Doc, error)`, `Serialize(Doc)`, `Render(body)` |
| `internal/attachments` | Accept upload, validate size/MIME/ext, generate safe stored name + sha256, write original, write metadata JSON, link/unlink to page, serve download, **enqueue** extraction job, ref-count GC | `repo`, `jobs`, db | `Store`, `Get`, `Download`, `Link/Unlink`, `Replace`, `Delete` |
| `internal/search` | Open/own Bleve index, index page & attachment docs, query by title/body/tag/filename/extracted-text, return typed results | Bleve | `Index(doc)`, `Delete(id)`, `Query(q, filter) []Result` |
| `internal/gitstore` | Detect/init repo, pull-on-startup, stage+commit (with identity+action metadata), push, read history, restore version; **serializes all git ops** | git CLI, `repo` dir | `Commit(spec)`, `History(path)`, `Restore(path, rev)`, `Push()` |
| `internal/agent` | Build Eino ReAct agent, register read/write tools, run chat/summarize/propose-patch, stream tokens, enforce read-vs-write approval boundary, never see secrets | `repo`, `okf`, `search`, `attachments` (read paths only) | `Chat(ctx, req) stream`, `ProposePatch`, tools call back into services |
| `internal/jobs` | In-process worker pool + persisted queue (SQLite-backed), job kinds: extract / index / commit / cleanup, retry+backoff, status for SSE | `attachments` (extractors), `search`, `gitstore`, db | `Enqueue(job)`, `Status(id)`, worker loop |
| `internal/audit` | Append audit events (login, page/attachment/agent actions) to log + SQLite mirror | db | `Record(event)` |

---

## Recommended Project Structure

Confirmed exactly as SPEC §16, with internal sub-structure made explicit. **No deviation recommended.**

```
cmd/okf-workspace/
  main.go                  # serve command, DI wiring, startup order

internal/
  config/      config.go   # YAML load + validate + env
  server/      router.go    handlers_*.go  middleware.go  sse.go
  web/         embed.go     # embed.FS of ../web/dist
  auth/        auth.go      session.go     password.go    middleware.go
  users/       users.go     bootstrap.go
  repo/        path.go      # safe resolver — most security-critical file
               files.go     tree.go        trash.go
  okf/         frontmatter.go  render.go    template.go    validate.go
  attachments/ store.go     metadata.go    extract/       # pdf, docx, txt extractors
               download.go  refcount.go
  search/      index.go     query.go        mapping.go     # Bleve doc mapping
  gitstore/    git.go       commit.go       history.go     restore.go
  agent/       agent.go     tools.go        prompts.go     patch.go
  jobs/        queue.go     worker.go       jobs_*.go      # one file per job kind
  audit/       audit.go
  store/       db.go        migrations/      # SQLite + schema (shared by several pkgs)

web/                        # React/Vite SPA; `npm run build` → web/dist (embedded)
  src/ ...
  dist/                     # build output, embedded by internal/web
```

### Structure Rationale

- **`internal/repo` is the security chokepoint.** `path.go` (the safe resolver) is the single function through which *every* filesystem access for content must pass. Build and fuzz-test it first (SPEC §21.1, §22.1). No other package may construct absolute repo paths itself.
- **`internal/okf` is pure** (no I/O) — frontmatter parse/serialize/render. This makes round-trip testing (SPEC §22.3) trivial and keeps Markdown integrity logic isolated and reusable by both the page handlers and the agent's patch tooling.
- **`internal/store`** (added to the SPEC list, implied by "SQLite"): centralizes the `*sql.DB`, migrations, and the operational-data schema so `users`, `auth`, `jobs`, `audit`, `attachments`(refs), search-cache all share one well-managed connection rather than each opening SQLite.
- **`attachments/extract/`** subpackage isolates per-format extractors (pdf/docx/txt) so adding formats is additive and so the extractors run only inside the job worker, never in a request handler.
- **`agent/tools.go`** is the *only* surface where the agent touches the rest of the system — it calls the same services as the HTTP handlers, never the filesystem directly (SPEC §15, §21.3).

---

## Architectural Patterns

### Pattern 1: Files-as-truth with one-way derived caches

**What:** The Git working tree under `repo/` is authoritative. SQLite (`app.db`) and the Bleve index are *projections* of that tree, updated **after** a successful file write. A reconcile/reindex pass can rebuild both from files alone.

**When to use:** Always, for any content mutation. Read paths may serve from the file (page body) and/or the cache (tree listing, search) but writes go file-first.

**Trade-offs:** + Portability and the "copy the folder = full backup" guarantee (SPEC §3.3, §25). + Agents read the same files humans edit. − Two stores can drift on crash between write and projection; mitigated by making projection idempotent + a startup reconcile + treating the cache as disposable.

```go
// Canonical write path (page save). Order is load-bearing.
func (s *PageService) Save(ctx, path, doc, baseRev string) error {
    abs, err := s.repo.Resolve(path)            // 1. safe-path gate
    if err != nil { return err }
    if err := s.checkRevision(path, baseRev); err != nil {
        return ErrConflict                        // 2. optimistic concurrency
    }
    doc = s.okf.RepairRequired(doc)               // 3. frontmatter repair
    if err := s.repo.Write(abs, s.okf.Serialize(doc)); err != nil {
        return err                                // 4. FILE WRITTEN = truth
    }
    s.jobs.Enqueue(IndexJob{Path: path})          // 5. project → Bleve (async)
    s.jobs.Enqueue(CommitJob{Path: path, User: u, Action: "page_edit"})
    s.audit.Record(...)                           // 6. audit
    return nil                                     // returns BEFORE commit lands
}
```

### Pattern 2: Async job worker decoupling slow/serial work

**What:** A single in-process worker (goroutine pool) draining a SQLite-persisted queue handles text extraction, (re)indexing, git commits, and cleanup. Handlers `Enqueue` and return immediately; the SPA polls/streams status via SSE.

**When to use:** Anything slow (PDF/DOCX extraction), anything that must be serialized (git), anything batchable (commits).

**Trade-offs:** + Fast request latency, natural batching point, retry/backoff in one place. + Survives restart (queue is persisted). − Eventual consistency (search/commit lag a save); acceptable for a 5-person tool. − Single-process only — fine given the single-binary deployment goal.

### Pattern 3: Serialized git via a single commit queue + batching

**What:** All git operations funnel through `gitstore` driven by one worker. Commits are **batched**: autosave writes the working tree continuously, but a `CommitJob` debounces (short idle window, e.g. 2–5s, or explicit save) and folds multiple pending changes into one commit with structured metadata (SPEC §14.2).

**When to use:** Every commit trigger (page/attachment/agent change). Never `git` from a request handler or from two goroutines concurrently.

**Trade-offs:** + No `.git/index.lock` contention, clean history, identity+action in every message. − Commit lags the save (acceptable; the file is already truth). − If `push_on_commit`, push failure must not fail the user write — push is a separate retryable step.

```go
// Commit message carries identity + provenance (SPEC §14.2).
spec := CommitSpec{
  Paths:   batched,                 // multiple files → one commit
  Message: "Update runbooks/deploy-staging.md",
  User:    "janis", Action: "page_edit", Source: "web-ui",
}
```

### Pattern 4: Eino agent as a sandboxed tool-caller (ReAct over compose.Graph)

**What:** The agent uses a `ToolCallingChatModel` (OpenAI-compatible provider per config — local Ollama or remote) inside Eino's ReAct loop (`compose.Graph` with ChatModel + Tools nodes). Tools are thin Go functions that call the *same services* the HTTP layer uses. **Read tools** execute immediately; **write tools** (`apply_page_patch`, `create_page`, `attach_file_to_page`) do not mutate — they return a *proposal* that the human approves, after which the normal page-save path runs. Eino's interrupt/resume (human-in-the-loop) maps directly onto this approval gate.

**When to use:** Agentic flows only (Q&A, summarize, propose-patch, attachment Q&A) — never plain CRUD (SPEC §8.3).

**Trade-offs:** + Single enforcement point for the safety boundary (no secrets, no shell, no path escape — §21.3). + Provider-agnostic. − ReAct loop cost/latency; mitigate with bounded `MaxStep` and scoping context to the current page/attachment.

```go
// Read tool: executes now. Write tool: returns a proposal, never writes.
proposePagePatch := tool.New("propose_page_patch", func(ctx, in PatchIn) (PatchOut, error) {
    cur := okf.Parse(repo.Read(in.Path))          // read via service, not FS
    proposed := applyModelEdit(cur, in.Instruction)
    return PatchOut{Diff: unifiedDiff(cur, proposed), RequiresApproval: true}, nil
})
// apply happens only after POST /agent/apply-patch with approval:true → PageService.Save
```

---

## Data Flow

### Flow A — Page edit → save → index → commit

```
User clicks Save (Editor)
   │ PUT /api/v1/pages/{path}  {frontmatter, body, base_revision}
   ▼
server handler → RBAC(editor) → CSRF check
   ▼
repo.Resolve(path)            ── safe-path gate (reject ../, abs, symlink)
   ▼
revision check vs base_revision  ── conflict? → 409 + diff (overwrite/merge/copy)
   ▼
okf.RepairRequired + Serialize  ── ensure required frontmatter fields
   ▼
repo.Write(file)              ★ SOURCE OF TRUTH UPDATED
   ▼
jobs.Enqueue(IndexJob) ──► worker: search.Index(page)      (async → Bleve)
jobs.Enqueue(CommitJob) ─► worker: debounce+batch ─► gitstore.Commit ─► [push]
   ▼
audit.Record(page_edit) ; 200 {new revision}   ── returns before commit lands
```

### Flow B — Attachment upload → store → extract → index

```
User drags file into page (multipart POST /pages/{path}/attachments)
   ▼
server handler → RBAC(editor) → size/ext/MIME validation
   ▼
attachments.Store:
   ├─ generate safe stored name + sha256, choose assets/originals/YYYY/MM/<id>_<name>
   ├─ write ORIGINAL (immutable)                       ★ truth
   ├─ write metadata JSON (assets/metadata/<id>.json)  ★ truth
   ├─ insert attachment-ref row (SQLite, cache)
   └─ insert attachment link/card into page draft (okf body)
   ▼
jobs.Enqueue(ExtractJob)
   └─► worker: extract text → assets/extracted/<id>.txt ★ truth
            ├─ update metadata.extraction.status = done
            ├─ jobs.Enqueue(IndexJob: attachment text)  → Bleve
            └─ jobs.Enqueue(CommitJob: attachment_upload) → gitstore
   ▼
SSE pushes extraction_status: queued → done  ── UI updates card
Response (immediate): {id, download_url, extraction_status: "queued"}
```

Download path is independent and synchronous: `GET /attachments/{id}/download` → resolve id → metadata → stream original with `Content-Disposition: attachment` for risky types (§21.2). The original is **never** modified (§11).

### Flow C — Agent propose → review → approve → apply → commit

```
User prompt in PromptBar (POST /agent/chat or /agent/propose-patch, with context)
   ▼
agent.Chat: Eino ReAct loop (ChatModel ⇄ Tools), tokens streamed via SSE
   ├─ read tools run live: read_page, search_pages, read_attachment_text ...
   └─ propose_page_patch tool → builds unified diff   (NO write yet)
   ▼
Response: {summary, diff, requires_approval:true}
   ▼
UI DiffReviewDialog → user Approves
   ▼
POST /agent/apply-patch {page_path, diff, approval:true}
   ▼
server → RBAC(editor) → validate patch applies to current revision
   ▼
PageService.Save(...)  ── SAME path as Flow A (repo.Write → index → commit)
   ▼
gitstore.Commit(action="approved_agent_patch", agent="okf-assistant", prompt=...)
   ▼
audit.Record(agent_patch_approval)
```

The crucial boundary: **the agent never writes files.** It only produces proposals; the approved apply re-enters the ordinary, audited, revision-checked page-save flow. This satisfies SPEC §5.4 and §15.3.

### State Management (frontend)

```
React Query (server cache) ←─ REST /api/v1 ─→ Go services
   │  tree, page, search, attachment, history queries
SSE channel ──► agent token stream + job/extraction status ──► component state
Local UI state: editor buffer, page mode (read/edit/diff/history), soft-lock presence
```

---

## Build Order (dependency-driven → maps to SPEC Phases 0–5)

The order below is forced by dependencies, not preference. Each item names what it unblocks.

**Phase 0 — Skeleton (foundations everything else needs)**
1. `config` + `store`(SQLite+migrations) + `cmd` wiring + startup sequence.
2. `repo.path` **safe resolver first**, fuzz-tested — gate for all file access.
3. `gitstore` init + pull-on-startup (no commit consumers yet, but repo must exist).
4. `web` embed + `server` skeleton + `auth`/`users` + admin bootstrap + login.
   *Rationale:* auth must exist before any write API; safe-path must exist before any file op; SQLite/git/data-dir init are prerequisites for all services.

**Phase 1 — OKF pages (the core loop)**
5. `okf` parse/serialize/render/repair (pure, round-trip tested) — needed before pages render or save.
6. `repo` files/tree/trash on top of the resolver → tree + page read/edit/create/delete.
7. `jobs` worker + `gitstore.Commit` (CommitJob) → automatic batched commits + history/restore.
   *Rationale:* okf before pages (can't read/write a page without parsing it); jobs+commit before "save" is fully done; this completes the §24 first-milestone vertical slice.

**Phase 2 — Attachments**
8. `attachments` store/metadata/download/refcount + ExtractJob (pdf/docx/txt extractors).
   *Rationale:* depends on `repo` (paths), `jobs` (extraction), `gitstore` (commit on upload). Extraction must exist before the agent can answer about attachments.

**Phase 3 — Search**
9. `search` (Bleve) mapping + IndexJob wired into page-save and extraction-done.
   *Rationale:* indexes both pages (Phase 1) and extracted text (Phase 2), so it lands after both produce content. Backfill via a reindex job over existing files.

**Phase 4 — Eino agent**
10. `agent` ReAct + read tools (list_tree, read_page, search_*, read_attachment_text) → Q&A + summarize; then propose_page_patch + apply flow + DiffReviewDialog.
    *Rationale:* agent tools are thin wrappers over `repo`/`okf`/`search`/`attachments`, all of which must exist first. Read tools before write/patch tools.

**Phase 5 — Collaboration improvements**
11. Soft locks (`.okf-workspace/locks/`) + presence (SSE) + conflict diff UI (overwrite/merge/save-as-copy). Optimistic-concurrency revision check was scaffolded in Phase 1; this hardens it.
    *Rationale:* layers onto the established save path; safely last.

---

## Key Design Tensions (explicit guidance)

### Tension 1 — Files-as-truth vs SQLite/Bleve cache consistency

**Resolution:** One-way projection (Pattern 1). Always write the file first; update SQLite/Bleve afterward via idempotent jobs. Treat `app.db` (minus sessions/queue) and the Bleve index as **disposable, rebuildable**. Provide a `reindex`/`reconcile` startup option and admin action that walks `repo/` and rebuilds projections — this is also the recovery path after a crash between file-write and projection, and after an out-of-band edit (someone `git pull`s or edits a file directly). Never let a read of stale cache mask the file: page *body* reads come from the file; only listings/search/refs come from cache.

### Tension 2 — Soft-lock concurrency (presence) vs optimistic concurrency (correctness)

**Resolution:** Two independent mechanisms, both needed. **Soft locks** (`.okf-workspace/locks/<path>` with user+heartbeat TTL, surfaced via SSE) are *advisory UX* — they show "Janis is editing" and allow force-edit; they never block a save. **Optimistic concurrency** (`base_revision` = content/git hash on every `PUT`) is the *correctness* guard: on mismatch return 409 + a diff and let the user choose overwrite / manual-merge / save-as-copy (§13.1). Build the revision check in Phase 1 (cheap, high value); add presence locks in Phase 5. Do not conflate them: a stale lock must never lose data, and the revision check must work even when locks are off.

### Tension 3 — Git commit batching vs history granularity

**Resolution:** Debounced single-writer commit queue (Pattern 3). Autosave writes the working tree frequently; a `CommitJob` waits a short idle window (config, ~2–5s) or fires on explicit Save, folding pending edits to the *same* page (and co-uploaded attachments) into one commit with structured `User/Action/Source` metadata. Agent-approved patches commit immediately with their own provenance (don't batch a human-approved change with unrelated autosaves — history clarity matters there). Keep `push` a *separate* retryable step after commit so remote failure never blocks a local save (§14.3). All commits serialized through one worker → no index-lock races.

### Tension 4 — Agent capability vs safety boundary

**Resolution:** The agent reaches the system *only* through registered Eino tools that call existing services (Pattern 4) — never the filesystem, never config/secrets, never shell, never git push (§15.3, §21.3). Read tools execute live; write tools return *proposals* and the actual mutation re-enters the human-approved, revision-checked, audited page-save path (Flow C). The safe-path resolver (`repo.Resolve`) is in the call chain of every agent file read, so path-escape is structurally impossible. Bound the ReAct loop with `MaxStep` and scope context to the current page/attachment to control cost and prevent runaway tool loops.

---

## Anti-Patterns

### Anti-Pattern 1: Treating SQLite as the content store
**What people do:** Cache page bodies in SQLite and serve/edit from there for speed.
**Why it's wrong:** Breaks the core promise (data must be copyable plain files; agents read files) and creates a second source of truth that drifts from Git.
**Do this instead:** SQLite holds only operational/derived data (users, sessions, jobs, refs, index cache, audit mirror). Page bodies are read from and written to files.

### Anti-Pattern 2: Committing on every keystroke / committing from handlers
**What people do:** Run `git add && git commit` synchronously inside the save handler, per autosave.
**Why it's wrong:** Index-lock contention, latency on the user's save, and a useless commit-per-keystroke history.
**Do this instead:** Debounced, batched commits via the single-writer commit queue; handlers only enqueue.

### Anti-Pattern 3: Giving the agent direct filesystem or write access
**What people do:** Let the Eino agent `os.WriteFile`/`os.ReadFile` against repo paths for convenience.
**Why it's wrong:** Defeats path safety, the approval boundary, and audit; an LLM can be coerced into traversal or unreviewed writes.
**Do this instead:** Agent calls services through tools; reads go through `repo.Resolve`; writes are proposals approved by a human before the normal save path runs.

### Anti-Pattern 4: Building features before the safe-path resolver and okf parser
**What people do:** Wire page CRUD against raw paths and add path validation "later."
**Why it's wrong:** Traversal/symlink-escape bugs and Markdown corruption become load-bearing and expensive to retrofit; they're the two most-tested units in §22.
**Do this instead:** `repo.path` and `internal/okf` are Phase 0/1 foundations, fuzz/round-trip tested before any page handler depends on them.

---

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| Git remote (Gitea/GitLab) | `gitstore` shells out to `git push`, post-commit, retryable | Optional (`remote_enabled`); push failure must not fail user writes; `pull_on_startup` for portability |
| LLM provider | Eino `ToolCallingChatModel`, OpenAI-compatible `base_url` (local Ollama or remote) | Provider-agnostic via config; API key from `api_key_env`, never exposed to the agent's tools/context |
| git CLI | `gitstore` execs the binary (SPEC: "shell out first") | Single-writer queue; can later swap to go-git behind the same interface without touching callers |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| handlers ↔ services | direct Go calls (interfaces) | RBAC/CSRF/audit applied in middleware before reaching services |
| services ↔ jobs | `Enqueue` (async) | Decouples slow/serial work; SSE relays status back |
| jobs ↔ gitstore/search | direct calls inside worker | Serialized git; idempotent indexing |
| agent ↔ services | via registered tools only | The one sandbox seam; same services as handlers, read-only for reads, proposal-only for writes |
| all writers ↔ repo files | only through `repo` (safe resolver) | Single chokepoint for path safety; nothing constructs repo abs-paths itself |
| files → SQLite/Bleve | one-way projection (post-write) | Caches are rebuildable; reconcile on startup |

---

## Sources

- SPEC.md §7 (architecture), §9 (repo layout), §11 (attachments), §14 (git versioning), §15 (agent/Eino), §16 (backend services), §21 (security) — primary source of truth (HIGH)
- .planning/PROJECT.md — confirmed stack decisions: chi, Bleve, React, shell-out git, provider-agnostic Eino, SQLite-operational-only (HIGH)
- [Eino: ReAct Agent Manual — CloudWeGo](https://www.cloudwego.io/docs/eino/core_modules/flow_integration_components/react_agent_manual/) — ReAct over compose.Graph (ChatModel + Tools nodes), ToolCallingChatModel, MessageModifier, MaxStep (HIGH)
- [eino package — pkg.go.dev/github.com/cloudwego/eino](https://pkg.go.dev/github.com/cloudwego/eino) — component model, OpenAI/Ollama implementations (HIGH)
- [Eino ADK: ChatModelAgent — CloudWeGo](https://www.cloudwego.io/docs/eino/core_modules/eino_adk/agent_implementation/chat_model/) — tool-use config, interrupt/resume for human-in-the-loop (maps to approval gate) (MEDIUM)

---
*Architecture research for: single-binary Go + React files-as-truth wiki with hidden Git, Bleve, job worker, and Eino agent*
*Researched: 2026-06-17*
