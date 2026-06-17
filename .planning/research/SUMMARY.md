# Project Research Summary

**Project:** OKF Workspace
**Domain:** Self-hosted, single-binary, files-as-truth internal wiki with hidden Git versioning and an approval-gated Eino AI agent
**Researched:** 2026-06-17
**Confidence:** HIGH

## Executive Summary

OKF Workspace is a layered modular monolith: one Go process, a React/Vite SPA embedded in the binary, two persistence substrates (Git-backed filesystem = source of truth; SQLite = disposable operational cache), and an async job worker. The architecture is a deliberate inversion of the mainstream wiki pattern — instead of a database backed by a filesystem, the filesystem IS the database and everything else is a derived projection. Bleve and SQLite are rebuildable caches; deleting them loses nothing except sessions and job-queue state. This is not a preference but a hard architectural invariant from which every component decision follows.

The recommended build order is forced by dependency, not preference: a safe-path resolver and an OKF frontmatter parser must exist before any page handler; a single-writer Git service with stale-lock self-heal must exist before commits are wired; attachments and their text-extraction pipeline must exist before search indexes attachment text; all of the above must exist before the Eino agent can use them as tools. This maps directly onto the SPEC's Phases 0–5 and no shortcut in that order is safe. The two highest-risk phases are Phase 1 (the single-writer Git service and Markdown round-trip byte-stability are easy to get subtly wrong and expensive to retrofit) and Phase 4 (Eino is pre-1.0 and the agent's approval-gate boundary is the load-bearing safety mechanism for the whole product).

The principal risks are: (1) Markdown round-trip rot — a parse→re-serialize cycle silently corrupts files and must be blocked by a golden-corpus byte-stable test in Phase 1; (2) Git index.lock collisions and crash recovery — mitigated by the single-writer queue and startup self-heal in Phase 0/1; (3) indirect prompt injection via page/attachment content reaching the agent — mitigated structurally by capability-scoped Go tools (not prompt rules) and the human-approval write gate, which must never be bypassed; (4) two open decisions that must be forced in Phase 2 before uploads ship: large-binary-in-Git policy and PDF/DOCX OCR/fidelity spike. Eino API pinning must be verified at Phase 4 implementation time.

---

## Key Findings

### Recommended Stack

All core technologies are locked with validated versions (verified against Go module proxy and npm registry on 2026-06-17). The backend is Go 1.26 (single static binary, `CGO_ENABLED=0`), chi v5.3.0, Goldmark v1.8.2, Bleve v2.6.0, and Eino v0.9.9 with eino-ext at `@latest` (pseudo-version). SQLite uses the pure-Go `modernc.org/sqlite` v1.52.0 driver to preserve the cross-compile/no-C-toolchain guarantee. The frontend is React 19.2.7 + Vite 8.0.16 + TypeScript 6.0.3, with `@uiw/react-md-editor` 4.1.1 as the MVP editor (raw Markdown + preview, NOT a rich block editor — TipTap deferred and gated on round-trip tests). Git versioning shells out to the git CLI; go-git is not used in MVP.

The open choices are all prescribed: `gopkg.in/yaml.v3` v3.0.1 with `yaml.Node` for surgical frontmatter edits (preserves key order and comments); `github.com/gabriel-vasile/mimetype` v1.4.13 for magic-byte MIME sniffing on upload; `github.com/alexedwards/argon2id` v1.0.0 over `golang.org/x/crypto@v0.53.0` for password hashing; `github.com/alexedwards/scs/v2` v2.9.0 for sessions; `github.com/justinas/nosurf` v1.2.0 for CSRF; `github.com/ledongthuc/pdf` (pure-Go, text-layer only) and `github.com/fumiama/go-docx` for extraction. Frontend state is split: `@tanstack/react-query` 5.101.0 for server state, `zustand` 5.0.14 for ephemeral UI state.

**Core technologies:**
- `Go 1.26 + chi v5.3.0`: single static binary, idiomatic net/http middleware — locked
- `Goldmark v1.8.2`: CommonMark + GFM Markdown→HTML; edit path NEVER re-serializes AST — locked
- `Bleve v2.6.0`: pure-Go full-text search with faceting + per-field analyzers over pages + extracted attachment text — locked
- `Eino v0.9.9 + eino-ext @latest`: ReAct agent via `flow/agent/react.NewAgent`; tools via `utils.InferTool`; write tools EXCLUDED from agent graph — locked (pre-1.0, re-verify at Phase 4)
- `modernc.org/sqlite v1.52.0`: pure-Go SQLite for operational metadata only (users, sessions, jobs, refs, audit mirror) — picked
- `gopkg.in/yaml.v3 v3.0.1 (yaml.Node)`: surgical frontmatter edit without key reorder/comment loss — picked (critical for round-trip safety)
- `git CLI shell-out (os/exec)`: single-writer serialized queue in `internal/gitstore`; no go-git in MVP — locked
- `React 19.2.7 + Vite 8.0.16 + TS 6.0.3 + @uiw/react-md-editor 4.1.1`: SPA embedded in binary; raw Markdown editor; TipTap deferred — locked

### Expected Features

The full SPEC MVP (Phases 0–5) is the target — PROJECT.md explicitly confirms this, not the §24 first-prototype slice.

**Must have (table stakes):**
- Local login/logout, sessions, admin/editor/reader RBAC (Phase 0)
- Left-side page tree, expand/collapse, recent pages (Phase 1)
- Page CRUD: create, rename, move, delete-to-trash, restore (Phase 1)
- Markdown editor with live preview + render (Phase 1)
- Page metadata: title, tags, description in YAML frontmatter (Phase 1)
- Internal page links + rename/move link integrity (Phase 1)
- Full-text + title + tag search (Phase 3)
- Upload/download original attachments + attachment cards (Phase 2)
- Page version history + restore (Phase 1)
- Audit log (cross-cutting)

**Should have (competitive differentiators):**
- Files-as-truth: plain Markdown + YAML frontmatter on disk; SQLite is never content store
- Attachments as immutable originals + metadata sidecar + extracted-text sidecar (three-part model)
- Hidden Git versioning: automatic batched commits, history/restore, zero Git jargon to users
- Approval-gated Eino agent: propose→review diff→approve→apply→commit; read/write boundary structural
- Attachment Q&A / summarize from extracted text (PDF/DOCX/TXT)
- Provider-agnostic LLM via `config.yaml` (local Ollama or remote OpenAI-compatible)
- Single Go binary + data dir (no Postgres/Redis/Elasticsearch/K8s)
- Soft locks + presence + conflict-diff resolution (overwrite/merge/save-as-copy)

**Defer to v1.x / v2+:**
- XLSX/ZIP text extraction, TipTap rich editor (gated on round-trip corpus), real-time Yjs/CRDT, OCR for scanned PDFs, Notion-style databases, comments, public sharing, SSO, mobile app

**The load-bearing anti-feature:** direct agent writes without approval. This must be actively defended against "auto-apply" feature creep — it is a non-goal, not just a deferral.

### Architecture Approach

The system is a layered modular monolith with strict one-way data flow: files are written first, then SQLite/Bleve are updated as derived projections. Two key chokepoints: `internal/repo` (safe-path resolver — every filesystem access must pass through it, including agent tool reads) and `internal/gitstore` (single-writer Git service — all git ops serialized through one goroutine/queue). The async job worker decouples slow/serial work from request handlers. The agent reaches the system only through registered tools; write tools return proposals, never mutations.

**Major components:**
1. `internal/repo` — safe-path resolver + Markdown file read/write/move/trash; fuzz-tested first
2. `internal/okf` — pure, I/O-free frontmatter parse/serialize/render/repair; isolated for round-trip testing
3. `internal/gitstore` — single-writer Git service; serialized commit queue with batching + debounce; stale-lock self-heal on startup
4. `internal/jobs` — async worker + SQLite-persisted queue for extract/index/commit/cleanup jobs
5. `internal/attachments` — immutable original storage, metadata sidecars, extraction enqueueing, ref-count GC
6. `internal/search` — Bleve index over pages + extracted attachment text; disposable, rebuildable from files
7. `internal/agent` — Eino ReAct agent; read tools execute live; write tools return proposals only; every file read via `repo.Resolve`
8. `internal/server` — chi router + middleware (session, CSRF, RBAC, audit); REST + SSE handlers
9. `internal/store` — centralized SQLite (`app.db`): users, sessions, jobs, attachment refs, audit mirror — never content

**Three canonical data flows:**
- **Flow A (page save):** safe-path gate → per-file revision check → frontmatter repair → `repo.Write` (truth) → enqueue IndexJob + CommitJob → audit → return before commit lands
- **Flow B (attachment upload):** MIME/size validation → write original (immutable, truth) → write metadata JSON (truth) → enqueue ExtractJob → SSE status updates
- **Flow C (agent patch):** ReAct loop → `propose_page_patch` returns diff (no write) → DiffReviewDialog → user approves → re-enters Flow A with `action="approved_agent_patch"`

### Critical Pitfalls

1. **Markdown round-trip rot** (Phase 1 gate) — Any parse→re-serialize cycle silently rewrites files, churns Git diffs, corrupts edge cases (code blocks containing `---`, frontmatter with comments). Prevention: edit path keeps raw Markdown bytes; frontmatter edited surgically via `yaml.Node`; golden-corpus byte-stable test required as Phase 1 exit gate. TipTap forbidden until corpus passes through the rich editor too.

2. **Concurrent Git writes / orphaned index.lock** (Phase 0/1 foundation) — Multiple goroutines or a process killed mid-commit wedge the repo. Prevention: single-writer model (one goroutine owns all repo mutations); startup self-heal that clears stale locks, runs `git fsck --quick`; batched/debounced commits from one worker queue only.

3. **SQLite/Bleve drift from files** (Phase 1 rule, Phase 3 rebuild) — Index diverges via pull-on-startup, version restore, crash between file-write and projection, direct disk edit. Prevention: content reads always from files; Bleve/SQLite rebuilt from files on startup HEAD mismatch; index updates idempotent and keyed by path; index failure non-fatal to the write.

4. **Indirect prompt injection via page/attachment content** (Phase 4, depends on Phase 0/1) — Uploaded PDFs or page bodies can embed instructions steering agent tool calls. Prevention: approval gate is the load-bearing defense (UI must show concrete diff, never prose summary); capability sandbox enforced in Go tool implementations; safe-path resolver in every agent tool; proposed patch validation before display.

5. **Path traversal / symlink escape** (Phase 0/1 foundation) — `filepath.Join(root, userPath)` is not safe against `..`, encoded variants, absolute paths, or symlinks. Prevention: one safe-path resolver for every file op (repo, attachments, agent tools) that rejects `..` after decoding, resolves symlinks, and asserts the result is within repo root prefix; generated attachment storage names; resolver fuzz-tested before anything else depends on it.

6. **Optimistic concurrency mishandled** (Phase 1 floor, Phase 5 UX) — Per-repo revision produces false conflicts; last-write-wins silently loses edits. Prevention: revision = per-file content/blob hash; on mismatch return 409 + diff + all three choices (overwrite, manual-merge, save-as-copy) — partial implementation defeats the guarantee.

---

## Implications for Roadmap

Suggested phases: 6 (Phases 0–5 per SPEC)

### Phase 0: Skeleton, Auth, and Foundations
**Rationale:** Nothing runs without auth; no file operation is safe without the path resolver; no commit history without a Git repo. These are the load-bearing prerequisites for every subsequent phase.
**Delivers:** Single binary serving embedded React shell; local login/logout/sessions; admin/editor/reader RBAC; admin bootstrap; data dir + Git repo init + pull-on-startup; stale-lock self-heal; safe-path resolver with fuzz test suite; SQLite schema + migrations.
**Addresses:** SPEC §6.5, §20, §21.1, §21.4.
**Avoids:** Concurrent Git index.lock corruption (single-writer in place from day one); path traversal (resolver before any file op).
**Research flag:** No phase research needed. Chi middleware, Argon2id, SCS sessions, nosurf CSRF are all well-documented standard patterns.

### Phase 1: OKF Pages, Navigation, and Hidden Git
**Rationale:** The core wiki loop. `internal/okf` parser must be round-trip tested before any page can be read or written; the Git commit pipeline must exist before "save" is fully meaningful; history/restore complete the vertical slice.
**Delivers:** Page tree + folder navigation; page CRUD (create/rename/move/delete-to-trash/restore); Markdown editor with live preview; YAML frontmatter parse + surgical repair; automatic batched commits on save; page history + restore; internal page links + link integrity on rename/move; per-file optimistic concurrency (409 on conflict).
**Addresses:** SPEC §6.1, §6.2, §8.2, §10, §13, §14; full §24 first-milestone vertical slice.
**Avoids:** Round-trip rot (golden corpus test is Phase 1 exit gate); orphaned index.lock (single-writer already in place); per-repo revision bug.
**Research flag:** Spike recommended (not full phase research). The single-writer Git batching + stale-lock recovery and the `internal/okf` byte-stable round-trip both have subtle failure modes worth prototyping early in Phase 1.

### Phase 2: Attachments and Text Extraction
**Rationale:** Depends on safe-path resolver (Phase 0) and repo paths (Phase 1). Text-extraction pipeline is a gate for both attachment search (Phase 3) and agent attachment Q&A (Phase 4). Two open decisions must be forced before attachments ship.
**Delivers:** Upload/download immutable originals; MIME/size validation + magic-byte sniffing; generated storage names; metadata JSON sidecars; attachment cards with extraction status; replace/delete with ref-count GC; async text extraction for PDF/DOCX/TXT; SSE extraction status; `Content-Disposition: attachment` for risky types.
**Addresses:** SPEC §5.2, §6.3, §11, §19, §21.2.
**Avoids:** Upload security mistakes (MIME sniffing, stored-XSS via inline SVG/HTML); large-binary Git bloat (policy forced before uploads ship).
**Research flag:** NEEDS PHASE RESEARCH — two open decisions must be spiked: (1) large-binary-in-Git policy (accept versioned binaries vs. keep originals outside Git; history rewrite is expensive after the fact, decide before first upload lands); (2) PDF/DOCX extraction fidelity spike (test representative sample files against `ledongthuc/pdf` + `fumiama/go-docx` before Phase 2 ships; "no text extracted" UX path must be solid).

### Phase 3: Search
**Rationale:** Bleve must index both page content (Phase 1) and extracted attachment text (Phase 2), so it logically follows both. Backfill reindex job needed on first startup.
**Delivers:** Full-text + title + tag + attachment filename + extracted-text search; page/attachment/heading result types; incremental IndexJob wired to page-save and extraction-done events; full rebuild-from-files reindex job (admin action + startup HEAD-mismatch trigger).
**Addresses:** SPEC §6.3, §12.
**Avoids:** Index drift from files (rebuild machinery is Phase 3's primary engineering concern alongside the index itself).
**Research flag:** No phase research needed. Standard Bleve patterns; the rebuild-from-files logic is straightforward.

### Phase 4: Eino Agent
**Rationale:** All agent read tools are thin wrappers over `repo`, `okf`, `search`, and `attachments` — all must exist first. Read tools before write/patch tools. The DiffReviewDialog built here is reused in Phase 5.
**Delivers:** PromptBar with SSE token streaming; agent context modes (current page/selection/attachment/workspace); ask + summarize + rewrite + draft; propose-patch → DiffReviewDialog → approve → apply → commit; read tool suite; structural read/write boundary (write tools NOT in Eino graph); agent sandbox; diff validation before displaying proposed patches; audit log entries for prompt + approval.
**Addresses:** SPEC §5.4, §6.4, §7, §15, §18.2, §21.3.
**Avoids:** Prompt injection escalation (capability sandbox in Go tools, not prompts; approval gate shows real diff); agent path escape (every agent file read goes through `repo.Resolve`).
**Research flag:** NEEDS PHASE RESEARCH — Eino is pre-1.0 (v0.9.9 today, fast-moving). Before Phase 4 planning: re-verify `react.NewAgent`/`AgentConfig`/`utils.InferTool`/`openai.NewChatModel` signatures against current eino + eino-ext source; confirm interrupt/resume pattern for the approval gate; test chosen model/provider with `utils.InferTool`-generated tool schemas before building the full loop. Pin both `eino` and `eino-ext` via `go.sum` immediately after `go get`.

### Phase 5: Collaboration
**Rationale:** Soft locks and presence layer onto the save path from Phase 1; conflict UI reuses DiffReviewDialog from Phase 4; the optimistic concurrency floor (409 + per-file revision) was scaffolded in Phase 1. This phase hardens and completes those mechanisms.
**Delivers:** Soft lock files in `.okf-workspace/locks/` with user + heartbeat TTL; SSE presence feed; force-edit that still passes through the revision check; full conflict UX — overwrite / manual-merge / save-as-copy (save-as-copy creates a new page); soft lock expiry on session end/crash.
**Addresses:** SPEC §6 (collaboration), §13.1, §13.2.
**Avoids:** Soft lock without optimistic check (revision check must work even when force-editing past a soft lock; stale lock must never cause silent data loss).
**Research flag:** No phase research needed. Conflict UX is well-specified in SPEC §13.1; soft lock file format and TTL/heartbeat loop are straightforward.

### Phase Ordering Rationale

- Phases 0→1→2→3→4→5 is strictly dependency-ordered; each phase's features require all prior phases' outputs.
- Safe-path resolver and `internal/okf` parser are Phase 0/1 foundations that are also Phase 4 prerequisites — they cannot be retrofitted.
- Text extraction (Phase 2) gates both attachment search (Phase 3) and agent attachment Q&A (Phase 4).
- The DiffReviewDialog is built once in Phase 4, reused in Phase 5 — agent and collaboration share the diff rendering component.
- The single-writer Git service (Phase 0/1) is shared by page commits (Phase 1), attachment commits (Phase 2), and agent-approved patch commits (Phase 4).
- The `internal/jobs` async worker is introduced in Phase 1 (CommitJob) and extended for ExtractJob (Phase 2) and IndexJob (Phase 3) — the queue infrastructure is reused, not rebuilt.

### Research Flags

| Phase | Research Needed | Reason |
|-------|----------------|--------|
| Phase 0 | No | Standard Go auth/session/CSRF patterns; all libraries well-documented |
| Phase 1 | Spike recommended | Single-writer Git batching + stale-lock recovery and byte-stable round-trip have subtle failure modes worth prototyping early |
| Phase 2 | YES | Large-binary-in-Git policy + PDF/DOCX extraction fidelity spike must be resolved before attachments ship |
| Phase 3 | No | Standard Bleve patterns; rebuild-from-files logic is straightforward |
| Phase 4 | YES | Eino pre-1.0, fast-moving; re-confirm constructor signatures, tool schema generation, and interrupt/resume pattern at implementation time |
| Phase 5 | No | Conflict UX is well-specified; soft lock mechanism is standard |

---

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All versions verified against Go module proxy and npm registry on 2026-06-17. Eino API verified against GitHub `main` on same date — caveat: pre-1.0, re-verify at Phase 4. Pure-Go PDF/DOCX extraction quality is MEDIUM for complex/scanned documents. |
| Features | HIGH | Scope is fixed by SPEC.md as authoritative source of truth; competitor research confirms table-stakes vs. differentiator categorization. |
| Architecture | HIGH | SPEC §7/§9/§11/§14/§16 is explicit and internally consistent; Eino orchestration verified against CloudWeGo docs; component boundaries and data flows are well-defined. |
| Pitfalls | HIGH (domain mechanics) / MEDIUM (agent injection mitigations) | Git locking, Markdown round-trip, path safety — verified against Goldmark architecture, Git locking semantics, OWASP. Prompt injection mitigations: active research area, approval gate + capability sandbox chosen as primary defense. |

**Overall confidence:** HIGH

### Gaps to Address

- **Large-binary-in-Git policy (Phase 2):** Accept versioned binaries in Git (simple, `.git` bloat risk on re-uploads) vs. keep originals outside Git referenced from metadata (complex, cleaner history). History rewrite is expensive after the fact — force this decision at Phase 2 planning with a spike.
- **PDF/DOCX extraction fidelity (Phase 2):** `ledongthuc/pdf` and `fumiama/go-docx` handle text-layer documents; scanned/image-only PDFs yield nothing. Test against representative sample files before Phase 2 ships. "No text extracted" UX path must be solid.
- **Eino API pinning (Phase 4):** Eino v0.9.9 is pre-1.0. Re-verify `react.NewAgent`, `utils.InferTool`, and `openai.NewChatModel` signatures at Phase 4 implementation time. Pin both `eino` and `eino-ext` via `go.sum` after `go get` and commit the lockfile immediately.
- **Rename/move link integrity (Phase 1):** Internal Markdown links and attachment `linked_pages` metadata references must be updated (or resolved via `aliases` redirects) on page rename/move. Confirm the resolution strategy (eager rewrite vs. alias-redirect) during Phase 1 planning.
- **Remote Git push failure handling (Phase 0/1):** Push must be async and non-blocking on the user's save. `pull_on_startup` behavior on divergence (fast-forward-only vs. alert + refuse) must be defined. Recommend fast-forward-only pull with alert on divergence, followed by reindex.

---

## Sources

### Primary (HIGH confidence)
- `proxy.golang.org/<module>/@latest` — authoritative latest versions for all Go modules
- `npm view <pkg> version` — authoritative npm versions for all frontend packages
- `github.com/cloudwego/eino` + `eino-ext` source on GitHub `main` — constructor signatures, tool helpers, ReAct agent verified 2026-06-17
- `SPEC.md` — primary product and technical specification; authoritative for all feature and architecture decisions
- `.planning/PROJECT.md` — confirmed stack decisions, locked constraints, MVP target scope
- CloudWeGo Eino docs — ReAct agent, tool-use config, interrupt/resume pattern

### Secondary (MEDIUM confidence)
- MassiveGRID, Canadian Web Hosting, DEV Community, elest.io — wiki competitor feature comparisons (Outline, Wiki.js, BookStack)
- LeafWiki GitHub + devlog — closest architectural analog; differentiators confirmed
- OWASP LLM Prompt Injection Prevention Cheat Sheet + Lakera indirect prompt injection — agent security mitigations
- Karpathy LLM Wiki gist — validates AI-maintained, file-native knowledge-base pattern
- Dify Human Input node — validates human-approval AI workflow pattern

### Tertiary (LOW confidence)
- LlamaIndex PDF text extraction challenges — validates extraction/OCR pitfall; specific quality metrics need empirical testing
- Goldmark-markdown / pgavlin renderer notes on round-trip lossiness — validates Pitfall 1; exact behavior needs golden-corpus testing in Phase 1

---
*Research completed: 2026-06-17*
*Ready for roadmap: yes*
