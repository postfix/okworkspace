# Phase 1: OKF Pages, Navigation & Hidden Git - Research

**Researched:** 2026-06-18
**Domain:** Byte-stable Markdown/YAML round-trip · single-writer Git batching over the `git` CLI · file-tree navigation · per-file optimistic concurrency · hidden Git history/restore
**Confidence:** HIGH (locked stack + read the actual Phase-0 spines this phase extends; round-trip mechanics verified against Goldmark/yaml.v3 behavior)

## Summary

Phase 1 is a **vertical-slice MVP** built entirely on Phase-0 spines that already exist and compile in this repo: `internal/repo` (safe-path resolver + file primitives), `internal/gitstore` (single-writer `Commit(ctx, CommitSpec)` + stale-lock self-heal), `internal/jobs` (single-goroutine async worker), `internal/store` (migration-driven `*sql.DB`), and `internal/server` (chi router with session/CSRF/RBAC/audit middleware already wired). Almost nothing here is greenfield infrastructure — the work is (a) a **new `internal/okf` package** for byte-stable OKF Markdown parse/edit/emit, (b) a **`CommitJob` handler** registered on the existing worker, (c) **page/tree/folder chi handlers** mounted into the existing authenticated route group, and (d) **frontend page Read/Edit/History modes** replacing the `PLACEHOLDER_TREE` in `AppShell.tsx`.

The phase has exactly one load-bearing risk surface, flagged repeatedly by the ROADMAP and PITFALLS.md: **round-trip rot**. Any parse→re-serialize cycle that touches more than the changed bytes will, over weeks, churn every file and silently degrade the "copy plain Markdown off the server" promise. The defense is structural: the **body is opaque text** (never re-rendered through an AST on the write path), and **frontmatter is edited surgically via `yaml.Node`** (mutate only changed/missing fields, never full re-marshal). The golden-corpus byte-stable round-trip test in `internal/okf` is the **phase exit gate** and must exist before the editor is considered done.

The save model (locked in CONTEXT D-01..D-04) is **autosave-draft-in-SQLite + batched CommitJob**: the canonical `.md` file is written to disk *only* when the CommitJob fires, so the working tree is always byte-equal to the last Git commit. This is the single cleanest invariant in the phase — it protects the round-trip gate (no saved-but-uncommitted files), keeps "files are truth" honest, and makes optimistic-concurrency reasoning simple (revision = hash of the last committed bytes).

**Primary recommendation:** Build `internal/okf` first with the golden-corpus round-trip test as a hard gate; treat the page body as opaque text and frontmatter as surgical `yaml.Node` edits; route every repo mutation (save, rename+link-rewrite, trash/restore, version-restore) through a `CommitJob` on the existing `jobs.Worker` calling the existing `gitstore.Commit` — never call `git` or write `.md` files from an HTTP handler.

## User Constraints (from CONTEXT.md)

### Locked Decisions

**Save & commit model**
- **D-01:** Autosave-draft + batched-commit model (SPEC §14.2 shape). NOT one-commit-per-keystroke, NOT bare explicit-Save=commit.
- **D-02:** Autosaved draft persists in **SQLite** (`internal/store`), keyed by page path + user. The canonical `.md` on disk is **only written when the batched CommitJob fires**. Consequence (load-bearing): the working tree is always byte-equal to the last Git commit.
- **D-03:** Batched commit triggers on **explicit Save OR a short idle fallback** (~5–10s of no edits with a dirty draft).
- **D-04:** Commit goes through the **existing single-writer Git spine** (`gitstore.Commit` + a `CommitJob` on `internal/jobs`). Do NOT bypass with raw `git` calls. Message carries user identity + action + source per SPEC §14.2.

**Page links & rename/move integrity**
- **D-05:** Canonical on-disk link format is a **standard Markdown relative `.md` path** (e.g. `[Deploy](../runbooks/deploy.md)`). No wiki `[[...]]`, no app-only ID links.
- **D-06:** Links inserted via a **"link to page" picker** emitting the relative path; typing Markdown links directly also supported. In read mode, clicking a page link **navigates within the app**.
- **D-07:** On rename/move, **eagerly rewrite every inbound link** across the repo to the new path, committed in the **same commit (or paired commit)** as the move. Rewrites MUST go through the **round-trip-safe edit path** (`internal/okf`). Accept the cost of a repo-wide link scan per rename/move.

**Delete-to-trash & restore**
- **D-08:** Delete = **`git mv` of the page into `.okf-workspace/trash/`** via the single-writer service (a real commit). Restore = move back (another commit). No `git rm` + history-resurrection.
- **D-09:** **Pages only** are trashable in MVP; **folders are implicit** (disappear from the tree when empty). No explicit "delete folder" action.
- **D-10:** Trash records **original path + deleted-by + timestamp**. Restore-target collisions handled (suffix or prompt).
- **D-11 (refinement, not blocking):** Deleting a page that others link to leaves inbound links **dangling** — acceptable for MVP. "Warn before deleting a linked page" is a nice-to-have the planner may add but is not required.

**New-page creation flow**
- **D-12:** Create opens a **small modal asking for the title**; backend **slugifies title → filename** (e.g. "Deploy Staging" → `deploy-staging.md`) in the selected folder, with **collision suffixing** (`-2`, `-3`, …). Filenames/paths stay hidden from the user. No transient `untitled.md` inline-create.
- **D-13:** New pages **pre-filled with valid required frontmatter**: `type: Page` (default), generated `title`, ISO-8601 `timestamp`, empty `tags`, empty `description`. No type-picker in MVP.

**Version history & restore (Claude's discretion — locked defaults)**
- **VER-02:** History view lists the page's Git commits (timestamp, author/display name, action); no raw SHAs surfaced.
- **VER-03:** Restore writes the chosen old version's content as a **new forward commit** (never rewrites/`reset`s history). Flows through the single-writer commit path.
- **VER-04:** Push reuses Phase-0 config keys (`git.remote_enabled`, `git.push_on_commit`, `git.pull_on_startup`, ff-only pull, branch). When enabled, push happens after commit; on divergence, alert and refuse to auto-merge (Phase-0 D-12). Push is the deferred-from-Phase-0 piece landing here.

**Navigation (Claude's discretion — locked defaults)**
- **NAV-01/02:** Tree from `GET /api/v1/tree` (SPEC §17.2); folders expand/collapse; page rows titled by frontmatter `title` (fall back to filename). Current page highlighted (NAV-04).
- **NAV-05:** Recent pages **client-side** (localStorage / zustand) — no server round-trip.
- **NAV-03:** `POST /api/v1/folders`; a folder with a seeded/blank `index.md` is acceptable so empty folders are representable.

**Optimistic-concurrency floor (scaffold; hardened in Phase 5)**
- Page GET returns `revision` (content hash); PUT carries `base_revision`; backend returns **409** on mismatch rather than silently overwriting. Full conflict UX is Phase 5 — only revision check + 409 now.

### Claude's Discretion
History-view rendering, restore mechanics (forward commit), recent-pages storage (client-side), tree titling/ordering, folder-create representation, and the exact idle-debounce interval are left to research/planning within the above constraints.

### Deferred Ideas (OUT OF SCOPE)
- Full conflict-resolution UX (overwrite / manual-merge / save-as-copy), presence, soft locks — **Phase 5**.
- DiffReviewDialog / agent-proposed-patch page mode (§18.3 Diff review) — **Phase 4**.
- Warn-before-deleting-a-linked-page (dangling-link guard) — nice-to-have, not required (D-11).
- Folder delete/move-to-trash as a unit — out of scope (D-09); pages-only.
- Server-side / cross-device recent pages — Phase 1 keeps recents client-side.
- Folder-inferred or picker-selected page `type` — MVP defaults to `type: Page` (D-13).
- Configurable idle-debounce interval / draft retention policy — sensible default now; expose later.

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PAGE-01 | Create a page in the selected folder | D-12 slug+collision flow; `POST /api/v1/pages`; `okf.Emit` with D-13 scaffolded frontmatter → CommitJob |
| PAGE-02 | Edit title/tags/description/body | Edit mode (`@uiw/react-md-editor` for body + frontmatter form); autosave-draft (D-02) → SQLite drafts table |
| PAGE-03 | Save and view rendered Markdown | CommitJob writes `.md`; read mode renders via `react-markdown`+`remark-gfm`+`rehype-sanitize` (Pattern: render matches Goldmark GFM) |
| PAGE-04 | Rename a page | `POST /pages/{path}/rename`; `git mv` + eager inbound-link rewrite (D-07) in one commit |
| PAGE-05 | Move a page to another folder | Same rename handler with a new parent dir; identical link-rewrite path |
| PAGE-06 | Delete a page to trash | `DELETE /pages/{path}` → `git mv` into `.okf-workspace/trash/` (D-08) + trash record (D-10) |
| PAGE-07 | Restore a page from trash | Move back via single-writer commit; collision suffixing (D-10) |
| PAGE-08 | Link from one page to another | Relative `.md` link (D-05); link picker (D-06); in-app navigation on click |
| PAGE-09 | Fill missing required frontmatter on save | `okf.Repair` — surgical `yaml.Node` add of only missing required fields; byte-safe (Pitfall: round-trip rot) |
| NAV-01 | Browse pages in left tree | `GET /api/v1/tree` from `repo.Tree()` walk; tree response shape SPEC §17.2 |
| NAV-02 | Expand/collapse folders | Client tree component over the nested tree response |
| NAV-03 | Create a folder | `POST /api/v1/folders`; seeded blank `index.md` so empty folders are representable |
| NAV-04 | Highlight current open page | Route `/app/page/:path` drives active-row state in tree |
| NAV-05 | Recently visited pages | Client-side (zustand + localStorage), no server round-trip |
| VER-01 | Auto-commit on save with user identity | CommitJob → `gitstore.Commit` (author = user; Action/Source in message body, already implemented) |
| VER-02 | View page version history | `GET /pages/{path}/history` → `git log --follow` parsed into commit list (no SHAs surfaced) |
| VER-03 | Restore a previous version | `POST /pages/{path}/restore` → read old blob via `git show`, write as new forward commit |
| VER-04 | Push to configured remote | After-commit push (config-gated); reuse Phase-0 ff-only/diverged semantics; **NEW: implement push** |

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Markdown render (read mode) | Browser (React) | — | `react-markdown` renders to React elements; keep server out of the hot read path. Body never re-serialized on write. |
| Markdown/frontmatter parse+repair (`okf`) | API / Backend | — | Byte-stable round-trip is a correctness invariant; must run server-side where the canonical bytes live. |
| Autosave draft | API / Backend (SQLite) | Browser (debounce) | Draft is operational state (D-02); SQLite holds it. Browser triggers autosave on a debounce. |
| Commit / `.md` write | API / Backend (single-writer worker) | — | All repo mutation funnels through `gitstore.Commit` via `CommitJob` (D-04). Never the browser, never an HTTP handler directly. |
| Link rewrite on rename/move | API / Backend | — | Repo-wide scan + round-trip-safe edit; must be server-side and inside the same commit. |
| Tree / folder listing | API / Backend (`repo.Tree`) | — | Filesystem is truth; tree is derived from a walk, not from SQLite. |
| Recent pages | Browser (zustand/localStorage) | — | NAV-05 locked client-side; no server state for a 5-user tool. |
| Optimistic-concurrency check | API / Backend | Browser (carry `base_revision`) | Server computes revision from committed bytes; 409 on mismatch. |
| Remote push | API / Backend (worker, after commit) | — | Network mutation must serialize through the single-writer too. |

## Standard Stack

> Stack is **LOCKED** in CLAUDE.md. This section records what THIS phase actually pulls in (most is already in `go.mod` / `package.json`; the frontend render/editor/diff libs are new installs).

### Core (already present in go.mod / used here)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go | 1.26.0 | Backend | `[VERIFIED: go version]` `os.Root` (used by `repo.Resolve`), `log/slog`. |
| `github.com/go-chi/chi/v5` | v5.3.0 | Router/middleware | `[VERIFIED: go.mod]` Page/tree routes mount into the existing authed group. |
| `github.com/yuin/goldmark` | v1.8.2 | Markdown→HTML (server, read-mode optional) | `[VERIFIED: go list -m -versions]` latest is v1.8.2. **AST is position-based, NOT a lossless source store** — never use its Markdown renderer on the write path. |
| `gopkg.in/yaml.v3` | v3.0.1 | Frontmatter parse/emit via `yaml.Node` | `[VERIFIED: go list -m -versions]` latest is v3.0.1. `yaml.Node` preserves key order + unknown fields → the round-trip-safe frontmatter primitive. |
| `modernc.org/sqlite` | v1.52.0 | Drafts + revision cache (operational only) | `[VERIFIED: go.mod]` Pure-Go, single binary. |
| `git` CLI | 2.47.3 | Versioning (shell-out, LOCKED) | `[VERIFIED: git --version]` All commits via `gitstore` single-writer. |

### Supporting — Frontend (NEW installs this phase)
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `@uiw/react-md-editor` | 4.1.1 | Edit-mode body editor (Markdown + live preview) | `[VERIFIED: npm view]` Edits **raw Markdown string** (CodeMirror under the hood) → protects round-trip (SPEC §8.2 "editor with preview, NOT rich block"). |
| `react-markdown` | 10.1.0 | Read-mode render to React elements | `[VERIFIED: npm view]` No `dangerouslySetInnerHTML` by default. Pair with the two plugins below. |
| `remark-gfm` | 4.0.1 | GFM tables/strikethrough/task-lists/autolinks | `[VERIFIED: npm view]` Must match Goldmark's GFM so client render agrees with any server render. |
| `rehype-sanitize` | 6.0.0 | Sanitize rendered HTML (REQUIRED) | `[VERIFIED: npm view]` Multi-user wiki content can include user/agent HTML/links → prevent stored XSS. Keep `rehype-raw` OFF. |
| `react-diff-viewer-continued` | 4.2.2 | History compare / old-version view | `[VERIFIED: npm view]` Used by HistoryDialog; reused by Phase-4 DiffReviewDialog. (Optional this phase if history shows full versions, not diffs.) |

### Already installed (consumed, not added)
`@tanstack/react-query` 5.101.0 (page/tree/history fetch + optimistic updates), `zustand` 5.0.14 (editor mode, recent-pages, dirty-draft state), `react-router-dom` 7.18.0 (`/app/page/:path` route), `lucide-react` 0.469.0 (tree icons — already in AppShell).

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `@uiw/react-md-editor` | `@uiw/react-codemirror` + `@codemirror/lang-markdown` | More control, less bundled CSS, slower to ship. md-editor is faster for MVP and still edits the raw string. |
| `react-diff-viewer-continued` | `diff2html` + `diff` (jsdiff) | If the backend returns a unified-diff string. Not needed in Phase 1 (history shows versions, not server diffs). |
| `git log --follow` for history | libgit2 / go-git | LOCKED to CLI; go-git only on a hard constraint. CLI `--follow` handles rename history. |
| Repo-wide link scan on rename (D-07) | Alias-redirect frontmatter (`aliases:`) | **Decided: eager rewrite (D-07).** Alias-redirect keeps links portable-but-stale and pushes resolution into every reader; eager rewrite keeps on-disk links literally correct (success criterion 2) at the cost of one repo walk per rename — fine at ~5 users. |

**Installation:**
```bash
# Frontend (run in web/)
npm install @uiw/react-md-editor@4.1.1 react-markdown@10.1.0 remark-gfm@4.0.1 rehype-sanitize@6.0.0 react-diff-viewer-continued@4.2.2
# Backend: goldmark + yaml.v3 are the only new-ish backend deps; add goldmark if read-mode HTML is rendered server-side:
go get github.com/yuin/goldmark@v1.8.2   # yaml.v3 already in go.mod
```

## Package Legitimacy Audit

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| `@uiw/react-md-editor` | npm | latest pub 2026-05-21 | ~699k/wk | github.com/uiwjs/react-md-editor | **SUS (too-new)** | **Approved with note** — false positive: 699k weekly downloads + established repo; "too-new" reflects a fresh patch release of a mature package. Planner need NOT gate, but may add a `checkpoint:human-verify` if policy requires. |
| `react-markdown` | npm | 2025-03-07 | ~25.3M/wk | github.com/remarkjs/react-markdown | OK | Approved |
| `rehype-sanitize` | npm | 2023-08-26 | ~6.6M/wk | github.com/rehypejs/rehype-sanitize | OK | Approved |
| `remark-gfm` | npm | 2025-02-10 | ~28.5M/wk | github.com/remarkjs/remark-gfm | OK | Approved |
| `react-diff-viewer-continued` | npm | 2026-04-23 | ~891k/wk | github.com/Aeolun/react-diff-viewer-continued | OK | Approved |

**Packages removed due to [SLOP] verdict:** none.
**Packages flagged as suspicious [SUS]:** `@uiw/react-md-editor` — flagged "too-new" only; high downloads + real repo + zero postinstall (`postinstall: null` confirmed) make this a recency false-positive. CLAUDE.md already locks this package. The planner may optionally add a `checkpoint:human-verify` before install; not strictly required.

*All five packages confirmed via the legitimacy seam with `postinstall: null` (no install-time scripts) and real GitHub source repos. Backend `goldmark`/`yaml.v3` are already-vetted, locked deps verified current on the Go module proxy.*

## Architecture Patterns

### System Architecture Diagram

```text
                         ┌─────────────────────────── Browser (React SPA) ──────────────────────────┐
  user edits body/fm ───▶│ PageEditor (@uiw/react-md-editor raw string + frontmatter form)           │
                         │   │ debounce ~1s                                                          │
                         │   ▼                                                                       │
  explicit Save / idle ──┼─▶ PUT /pages/{path}  (body, frontmatter, base_revision)                   │
   navigate page link ──▶│  GET /pages/{path} ─▶ Read mode: react-markdown+remark-gfm+rehype-sanitize│
                         │  GET /tree ─▶ LeftTree (expand/collapse, highlight current)               │
                         │  recent pages ◀── zustand/localStorage (client only)                      │
                         └────────────────────────────────┬─────────────────────────────────────────┘
                                                          │ JSON over chi (session+CSRF+RBAC+audit)
        ┌─────────────────────────────────────────────────▼──────────────────────────────────────────┐
        │ internal/server handlers (NEW): tree / folders / pages / rename / history / restore         │
        │   • every path → repo.Resolve (SEC-01 chokepoint)  • editor/reader gated by RequireRole      │
        └───┬───────────────────────────┬───────────────────────────┬───────────────────────────────┘
            │ read path                  │ write path (mutations)     │ history/restore (read git)
            ▼                            ▼                            ▼
  ┌──────────────────┐      ┌──────────────────────────┐   ┌──────────────────────────┐
  │ internal/okf     │      │ autosave draft → SQLite   │   │ git log --follow (parse) │
  │ Parse / Repair / │      │ (drafts table)            │   │ git show <rev>:<path>    │
  │ Emit (byte-safe) │      │        │ on Save/idle      │   └──────────────────────────┘
  │ + link rewrite   │      │        ▼                   │
  └────────┬─────────┘      │ enqueue CommitJob ────────┐│
           │ reads/writes   └──────────────────────────┘│
           ▼                                             ▼
  ┌──────────────────┐                     ┌──────────────────────────────────────────┐
  │ internal/repo    │  resolve+read/write │ internal/jobs Worker (single goroutine)   │
  │ (.md files=truth)│◀────────────────────│  CommitJob handler:                       │
  └──────────────────┘                     │   okf.Emit → repo.Write → gitstore.Commit │
                                           │   → (config) push                          │
                                           └───────────────────┬──────────────────────┘
                                                               ▼
                                              ┌────────────────────────────────────────┐
                                              │ internal/gitstore (single-writer mutex)  │
                                              │ git add/commit (author=user) [+ push]    │
                                              └────────────────────────────────────────┘
```

Trace the primary use case (edit → save → render): user types → debounced PUT writes a draft row → explicit Save (or ~5–10s idle) enqueues a CommitJob → worker calls `okf.Emit` then `repo.Write` then `gitstore.Commit` → working tree now byte-equal to HEAD → next `GET /pages/{path}` reads the committed file → React renders it.

### Recommended Project Structure
```
internal/
├── okf/              # NEW — byte-stable OKF parse/repair/emit + golden corpus (exit gate)
│   ├── okf.go        #   Parse(bytes) -> Doc{Frontmatter yaml.Node, Body []byte, Delim, Trailing}
│   ├── repair.go     #   Repair(doc) -> add ONLY missing required fields (surgical)
│   ├── links.go      #   FindLinks / RewriteLinks (relative .md path rewrites)
│   ├── emit.go       #   Emit(doc) -> bytes (re-attach unchanged frontmatter bytes when possible)
│   └── golden/       #   corpus + *_test.go  (load→save-no-edit→bytes-equal)
├── pages/            # NEW — page/tree/folder service (slug, collision, trash, history, restore)
│   ├── service.go
│   ├── tree.go
│   ├── trash.go      #   trash record store + restore-collision handling
│   └── commitjob.go  #   CommitJob payload codec + handler (registered on jobs.Worker)
├── server/           # extend router.go: mount /tree /folders /pages routes in the authed group
├── store/migrations/ # NEW: 0004_drafts.sql (+ optional revisions cache, trash table)
├── repo/ gitstore/ jobs/ store/   # EXISTING spines — reuse, do not fork
web/src/
├── routes/
│   ├── AppShell.tsx  # replace PLACEHOLDER_TREE with live LeftTree
│   ├── PageView.tsx  # NEW — Read mode
│   └── PageEditor.tsx# NEW — Edit mode (md-editor + frontmatter form + autosave)
├── components/
│   ├── LeftTree.tsx  # NEW — tree (expand/collapse, highlight)
│   ├── HistoryDialog.tsx  # NEW — version list + restore
│   ├── CreatePageModal.tsx# NEW — title-only modal (D-12)
│   └── LinkPicker.tsx     # NEW — emits relative .md link (D-06)
└── stores/recent.ts  # NEW — zustand + localStorage recent pages
```

### Pattern 1: Byte-stable OKF document model (the round-trip core)
**What:** Split a `.md` file into (frontmatter region, body region) by detecting a leading `---\n ... \n---\n` fence; keep the **raw body bytes opaque**; parse frontmatter into a `yaml.Node` ONLY to inspect/repair; on emit, prefer re-attaching the **original frontmatter bytes verbatim** when no field changed, and only re-render the YAML when a field was actually added/changed.
**When to use:** Every read-modify-write of a page (save, repair, link rewrite).
**Example:**
```go
// internal/okf/okf.go (illustrative — Source: yaml.v3 yaml.Node docs + PITFALLS.md Pitfall 1)
type Doc struct {
    HasFrontmatter bool
    RawFront       []byte    // exact original frontmatter bytes (between the --- fences)
    Front          yaml.Node // parsed for inspection/repair only
    Body           []byte    // OPAQUE — never re-rendered through a Markdown AST
    FrontDirty     bool      // set true only when Repair/RewriteLinks changed a field
}

// Repair adds ONLY missing required fields, leaving every existing byte intact.
func Repair(d *Doc, now time.Time) {
    required := requiredFieldSet(&d.Front) // type,title,description,tags,timestamp
    for _, missing := range required.Missing() {
        appendScalarOrSeq(&d.Front, missing, defaultFor(missing, now))
        d.FrontDirty = true
    }
}

// Emit re-attaches unchanged frontmatter verbatim; re-marshals ONLY when dirty.
func (d *Doc) Emit() ([]byte, error) {
    var front []byte
    if d.FrontDirty {
        b, err := yaml.Marshal(&d.Front) // yaml.Node preserves order + unknown keys
        if err != nil { return nil, err }
        front = b
    } else {
        front = d.RawFront
    }
    // reassemble: ---\n + front + ---\n + Body  (preserve original trailing newline)
    return assemble(d.HasFrontmatter, front, d.Body), nil
}
```
**Key landmines this guards against:** a code block containing a literal `---` line (must NOT be mistaken for a frontmatter fence — only a fence at byte 0 counts); CRLF vs LF (normalize only if you decide to, and decide ONCE — the golden test pins the choice); trailing-newline presence; YAML scalar quoting / date reformatting (avoided by not re-marshaling clean frontmatter).

### Pattern 2: CommitJob on the existing single-writer worker (D-04)
**What:** Handlers never touch git or write `.md`. They enqueue a `CommitJob` whose payload is a JSON `CommitSpec`-plus-content; the worker's single goroutine deserializes, calls `okf.Emit` → `repo.Write` → `gitstore.Commit`, then optionally pushes.
**When to use:** Every mutation (save, rename+rewrite, trash, restore, version-restore).
**Example:**
```go
// internal/pages/commitjob.go (illustrative — reuses internal/jobs + internal/gitstore)
const KindCommit = "commit"

type commitPayload struct {
    Writes []fileWrite       // path + new bytes (already okf.Emit'd or computed in handler)
    Spec   gitstore.CommitSpec // Paths/Message/User/Action/Source (existing type)
    Push   bool
}

func (s *Service) registerCommitJob(w *jobs.Worker) {
    w.Register(KindCommit, func(ctx context.Context, payload string) error {
        var p commitPayload
        if err := json.Unmarshal([]byte(payload), &p); err != nil { return err } // non-retryable shape error -> still retried then failed
        for _, fw := range p.Writes {
            if err := s.repo.Write(fw.Path, fw.Bytes); err != nil { return err } // resolver-gated
        }
        if err := s.git.Commit(ctx, p.Spec); err != nil { return err }
        if p.Push { return s.git.Push(ctx) } // NEW Push method (config-gated, ff semantics)
        return nil
    })
}
```
*Note:* The worker already serializes (one goroutine) and `gitstore.Commit` already holds the single-writer mutex — so even direct callers can't race. Routing through the worker additionally gives retry/backoff and decouples the HTTP response from disk latency (the UI shows "Saved" off the draft, not off the commit).

### Pattern 3: Optimistic-concurrency floor (revision = content hash)
**What:** `GET /pages/{path}` returns `revision` = a stable hash of the **committed file bytes** (e.g. `sha256` hex of the on-disk `.md`, OR the git blob SHA from `git rev-parse HEAD:<path>` — the blob SHA is free and already content-addressed). `PUT` carries `base_revision`; the handler recomputes the current revision and returns **409** if they differ, before enqueuing the CommitJob.
**When to use:** Every page save.
**Recommendation:** Use the **git blob SHA** (`git rev-parse HEAD:<path>`) as the revision — it is exactly "hash of the committed bytes," zero extra hashing, and stable across processes. Because of D-02 (working tree == HEAD), there is never an uncommitted-but-newer state to confuse this.

### Pattern 4: Hidden Git history & restore (forward-commit)
**What:** History = `git log --follow --format=...` for the page path, parsed into `{when, who(display name), action}` (Action recovered from the message trailer `gitstore.buildMessage` already writes). Restore = `git show <rev>:<path>` to read old bytes, then a normal CommitJob writing those bytes as a **new** commit (VER-03 — never `reset`/rewrite). `--follow` keeps history continuous across renames and the trash `git mv` (D-08).
**When to use:** HistoryDialog and restore action.

### Anti-Patterns to Avoid
- **Re-serializing the body through Goldmark's Markdown renderer on save.** Goldmark's AST is position-based and its Markdown renderer explicitly warns output "may not be textually identical." Body is opaque text. (Pitfall 1.)
- **Full `yaml.Marshal` of frontmatter on every save.** Reorders keys, drops comments, reformats dates/scalars. Only re-marshal when a field actually changed; otherwise re-attach original bytes.
- **Calling `git` or writing `.md` from an HTTP handler.** Breaks the single-writer guarantee and the "working tree == HEAD" invariant. Always enqueue a CommitJob.
- **Reading page body/frontmatter from SQLite.** SQLite holds drafts + (optional) revision cache only — operational/derived, never content (Pitfall 3, files-as-truth).
- **Treating a `---` line inside a fenced code block as a frontmatter fence.** Only a fence at byte offset 0 is frontmatter.
- **Surfacing raw git SHAs in the history UI.** VER-02 locks "no raw SHAs."

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| YAML frontmatter round-trip | Custom key=value parser / full re-marshal | `yaml.v3` `yaml.Node` + surgical field add | Hand-rolled parsers mangle anchors, multiline scalars, quoting; `yaml.Node` preserves order + unknown fields. |
| Markdown render (read mode) | Regex/string HTML builder | `react-markdown` + `remark-gfm` + `rehype-sanitize` | Stored-XSS-safe, GFM-correct, matches Goldmark. |
| Markdown editor | Plain `<textarea>` + custom preview | `@uiw/react-md-editor` | Raw-string editing (protects round-trip) + preview for free; LOCKED. |
| Git commit/serialization | New goroutine + raw `exec.Command("git"...)` | Existing `gitstore.Commit` via `CommitJob` | Single-writer + stale-lock self-heal already built; reuse it. |
| Async retry/backoff for commits | New worker | Existing `internal/jobs.Worker` | FIFO single-drain, retry-with-backoff, observability already present. |
| Path safety | New path checks in handlers | Existing `repo.Resolve` / `repo.Read|Write` | Fuzz-tested SEC-01 chokepoint; every path MUST route through it. |
| Diff/compare UI (history) | Custom diff renderer | `react-diff-viewer-continued` | Side-by-side/inline; reused by Phase-4 DiffReviewDialog. |
| Content revision/hash | Custom hashing scheme | `git rev-parse HEAD:<path>` (blob SHA) | Already content-addressed by Git; zero extra work; stable. |

**Key insight:** ~90% of this phase is *wiring existing spines together*. The genuinely new, subtle code is confined to `internal/okf` (round-trip) and the rename link-rewrite scan. Spend the engineering budget there; everything else is reuse.

## Runtime State Inventory

> Greenfield-leaning phase, but it INTRODUCES persistent state and an on-disk layout. Not a rename/refactor of existing data, so most categories are empty — recorded explicitly.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | NEW: `drafts` table in `app.db` (autosave, D-02); OPTIONAL `revisions` cache + `trash` record table (D-10). No existing data is migrated. | Add migration `0004_drafts.sql` (+ optional tables). Idempotent `CREATE TABLE IF NOT EXISTS` per existing migration pattern. |
| Live service config | None new beyond Phase-0 `config.yaml` git keys (`remote_enabled`/`push_on_commit`/`pull_on_startup`/`branch`) — already parsed in `internal/config`. VER-04 push READS them; no new keys. | None — reuse existing `config.GitConfig`. |
| OS-registered state | None — no scheduler/daemon registration. | None. |
| Secrets/env vars | None new. (LLM key is Phase-4.) | None. |
| Build artifacts | NEW frontend deps change `web/package-lock.json`; embedded SPA (`internal/web/dist`) rebuilds. | `npm install` + `npm run build` before `go build` (existing embed flow). |

**On-disk repo layout introduced/used this phase (verified against SPEC §9):** `.okf-workspace/trash/` (D-08 delete target) and folder `index.md` files (NAV-03). `.okf-workspace/manifest.json` and `.okf-workspace/locks/` exist in the SPEC layout but are NOT used this phase (locks = Phase 5). Confirm the Phase-0 seed already created `.okf-workspace/` (it seeds a starter layout) — if not, the trash handler must create `trash/` on first delete via `repo.MkdirAll`.

## Common Pitfalls

### Pitfall 1: Round-trip rot (THE phase risk)
**What goes wrong:** Any parse→re-serialize that touches more than the changed bytes churns every file over time; code blocks containing `---`, YAML comments, date quoting all get corrupted/reformatted.
**Why it happens:** Goldmark AST is position-based (not a source store); generic `yaml.Marshal` reorders keys and reformats scalars; devs assume "Markdown in, Markdown out" is lossless. It isn't.
**How to avoid:** Body is opaque text (no AST round-trip). Frontmatter edited surgically via `yaml.Node`, re-marshaled ONLY when a field changed; clean frontmatter re-attached verbatim. **Golden-corpus byte-stable round-trip test is the exit gate** — build it before the editor is "done."
**Warning signs:** Whitespace-only git diffs on pages nobody edited; `git blame` becomes noise; a page truncated after a code block containing `---`; frontmatter dates change quoting style.

### Pitfall 2: Concurrent Git writes collide on `index.lock`
**What goes wrong:** Two mutations shell out to `git` at once → `fatal: Unable to create '.../index.lock': File exists`; a killed process orphans the lock and wedges every later commit.
**Why it happens:** Multiple commit triggers (save, rename, trash, restore) + the job worker; Git's index is a single global lock.
**How to avoid:** Already solved by Phase-0 spines — **route everything through `CommitJob` → `gitstore.Commit`** (single mutex + single worker goroutine). `gitstore.SelfHealStaleLock` clears a stale lock on startup. Do NOT add a second writer path.
**Warning signs:** `index.lock` errors under light concurrent use; a "Saved" toast with no commit; a wedged repo after a container restart.

### Pitfall 3: Reading content from SQLite (files-as-truth violation)
**What goes wrong:** A read-for-edit path trusts the draft/cache instead of the committed file; a stale read → save overwrites newer disk state.
**Why it happens:** Drafts live in SQLite (D-02); easy to blur "draft" with "content."
**How to avoid:** Page body/frontmatter is ALWAYS read from the `.md` file via `repo.Read`. Drafts are a *resumable editing buffer*, surfaced as "you have unsaved changes," never as the canonical page. Revision is computed from committed bytes, not the draft.
**Warning signs:** Edits appear/disappear after a refresh; two tabs show different bodies; a save lands content the user never typed.

### Pitfall 4: Rename link-rewrite corrupts unrelated bytes
**What goes wrong:** A naive find/replace of the old path string rewrites occurrences inside code blocks or partial-path false matches, OR a re-serialize during rewrite churns the rest of the file.
**Why it happens:** Repo-wide string replace is tempting; it ignores Markdown structure and the round-trip invariant.
**How to avoid:** Rewrite links through `okf` (parse → operate on the body's link tokens → emit body bytes with only the matched links changed). Match links structurally (Markdown link destination ending in the old `.md` relative path), not bare substring. Commit the move + rewrites in one commit (D-07).
**Warning signs:** A code sample mentioning the old filename gets mangled; large diffs across many files after a single rename.

### Pitfall 5: Trash/restore loses provenance or collides
**What goes wrong:** Restore doesn't know the original folder; or restoring onto an existing same-named page clobbers it.
**Why it happens:** `git mv` into trash discards the original path unless recorded; restore target may now be occupied.
**How to avoid:** Record original path + deleted-by + timestamp at delete (D-10) — store in a SQLite `trash` table and/or a sidecar so restore can reconstruct the target. On restore collision, suffix (`-2`) or prompt (D-10).
**Warning signs:** "Restore" puts a page at the repo root; a restore silently overwrites a live page.

## Code Examples

### Detect frontmatter fence (only at byte 0)
```go
// Source: CommonMark frontmatter convention + PITFALLS.md Pitfall 1
func splitFrontmatter(src []byte) (hasFM bool, rawFront, body []byte) {
    // A fence is ONLY recognized at the very start of the file.
    if !bytes.HasPrefix(src, []byte("---\n")) && !bytes.HasPrefix(src, []byte("---\r\n")) {
        return false, nil, src
    }
    rest := src[len("---"):]
    rest = bytes.TrimLeft(rest, "\r")
    rest = rest[1:] // consume the newline
    // find the closing fence: a line that is exactly --- (optionally \r) then newline/EOF
    idx := indexClosingFence(rest)
    if idx < 0 {
        return false, nil, src // unterminated -> treat whole file as body (no FM)
    }
    return true, rest[:idx], rest[idxAfterClosingFence(rest, idx):]
}
```

### Tree response from a repo walk (NAV-01, SPEC §17.2)
```go
// Source: SPEC §17.2 tree shape + existing repo.Tree()
type Node struct {
    Type     string `json:"type"`     // "folder" | "page"
    Path     string `json:"path"`     // repo-relative, slash-separated
    Title    string `json:"title"`    // frontmatter title, fallback to base name
    Children []Node `json:"children,omitempty"`
}
// Build by walking repo.Tree(), skipping .git and .okf-workspace, reading ONLY the
// frontmatter region of each .md (not the whole body) to get the title cheaply.
```

### Push (NEW — VER-04, config-gated, ff-aware)
```go
// internal/gitstore/push.go (illustrative — extends the existing single-writer service)
func (g *GitStore) Push(ctx context.Context) error {
    if !g.cfg.RemoteEnabled || !g.cfg.PushOnCommit || g.cfg.Remote == "" {
        return nil // no-op when disabled (no network)
    }
    g.mu.Lock(); defer g.mu.Unlock()
    if _, err := g.git(ctx, "push", g.cfg.Remote, g.cfg.Branch); err != nil {
        // On rejection (non-ff / diverged), do NOT force; surface via Health like PullOnStartup.
        g.diverged = true
        return nil // alert, never auto-merge/force (Phase-0 D-12 parity)
    }
    return nil
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Re-render Markdown body through an AST on save | Treat body as opaque text; AST only for read-mode HTML | Established best practice for "files are truth" wikis | Eliminates round-trip rot |
| `dangerouslySetInnerHTML` for rendered Markdown | `react-markdown` → React elements + `rehype-sanitize` | unified v11 ecosystem | Removes stored-XSS class |
| One-commit-per-save in the HTTP handler | Single-writer worker + batched/debounced CommitJob | This project's locked model (D-01..D-04) | No lock contention, decoupled UI latency |

**Deprecated/outdated:** none locked-in is outdated. `react-diff-viewer` (unmaintained) → use `react-diff-viewer-continued` (already locked).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Phase-0 seed already created `.okf-workspace/` (trash dir may still need creating on first delete) | Runtime State Inventory | Low — handler can `repo.MkdirAll(".okf-workspace/trash")` defensively; verify in planning by inspecting the Phase-0 seed code. |
| A2 | Using the git blob SHA (`git rev-parse HEAD:<path>`) as `revision` is acceptable to the Phase-5 hardening | Pattern 3 | Low — Phase 5 only adds conflict UX on top of the 409 floor; a content-addressed revision is the natural base. Confirm during Phase-5 planning. |
| A3 | `@uiw/react-md-editor` "too-new" SUS verdict is a recency false positive (mature package, fresh patch) | Package Legitimacy Audit | Low — 699k weekly downloads + real repo + null postinstall; package is CLAUDE-locked. |
| A4 | LF (not CRLF) is the canonical on-disk newline; the golden corpus pins this | Pattern 1 / Pitfall 1 | Medium — if Windows authors commit CRLF, the round-trip test must either preserve CRLF per-file or normalize once. Decide explicitly in `internal/okf` and encode in the corpus. |
| A5 | `git log --follow` reliably traces history across the trash `git mv` and renames | Pattern 4 | Low–Medium — `--follow` is heuristic for renames; for the trash move it should hold (single 100%-similar rename). Verify in the spike. |

**If A4 is the only Medium:** it is a *decision to make explicit in planning*, not an unknown — both options (preserve-per-file vs normalize-once) are viable; the corpus locks whichever is chosen.

## Open Questions

1. **CRLF/newline policy for the byte-stable corpus**
   - What we know: LF is the natural default; the working tree must be byte-equal to HEAD (D-02).
   - What's unclear: whether to preserve a file's original line endings verbatim or normalize to LF on first write.
   - Recommendation: **Preserve per-file verbatim** (read bytes, edit surgically, write bytes) — strongest "files are truth" guarantee and simplest round-trip proof. Pin it in the golden corpus with at least one CRLF fixture. Decide in planning.

2. **Where the autosave debounce + idle-commit timer lives**
   - What we know: D-03 says explicit Save OR ~5–10s idle. Recents are client-side.
   - What's unclear: client-driven idle (browser timer → PUT that commits) vs. server-driven (a draft TTL the worker sweeps).
   - Recommendation: **Client-driven** — the browser already debounces autosave; on explicit Save or after N seconds idle, the client issues a "commit now" PUT. Server stays stateless about timers. Avoid a server sweep loop in MVP.

3. **Does history fetch need a body diff, or just version list + full-version view?**
   - What we know: VER-02 = list; VER-03 = restore. `react-diff-viewer-continued` is locked but optional for Phase 1.
   - Recommendation: MVP shows the version list + "view this version" (full content) + Restore. Defer side-by-side diff rendering unless cheap; it returns for free in Phase 4's DiffReviewDialog.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| `git` CLI | All versioning (gitstore single-writer) | ✓ | 2.47.3 | none (LOCKED requirement; already used in Phase 0) |
| Go toolchain | Backend build | ✓ | 1.26.0 | none |
| Node.js | Frontend build (Vite 8) | ✓ | 20.19.6 | none (Vite 8 needs Node 20.19+ — met) |
| npm registry access | Install 5 new frontend deps | ✓ (verified via legitimacy seam) | — | none |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** none.

## Validation Architecture

> Nyquist validation is ENABLED (`workflow.nyquist_validation: true`).

### Test Framework
| Property | Value |
|----------|-------|
| Backend framework | Go stdlib `testing` (+ table tests; existing pattern across `internal/*_test.go`) |
| Frontend framework | Vitest 3.x + @testing-library/react (existing: `AppShell.test.tsx`, `UserMenu.test.tsx`) |
| Config file | `web/vitest` via `web/package.json` scripts; Go needs none |
| Quick run command | `go test ./internal/okf/...` (round-trip gate) · `npm --prefix web test` |
| Full suite command | `go test ./...` · `npm --prefix web run test` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PAGE-09 / round-trip gate | load→save-no-edit→bytes-equal across corpus | unit (golden) | `go test ./internal/okf -run TestGoldenRoundTrip` | ❌ Wave 0 |
| PAGE-09 | repair adds ONLY missing required fields, byte-safe | unit | `go test ./internal/okf -run TestRepair` | ❌ Wave 0 |
| PAGE-08 / D-07 | rename rewrites inbound links without corrupting other bytes | unit | `go test ./internal/okf -run TestRewriteLinks` | ❌ Wave 0 |
| PAGE-01/03 | create→save→read returns rendered content; commit recorded | integration | `go test ./internal/pages -run TestCreateSaveRead` | ❌ Wave 0 |
| PAGE-04/05 | rename/move keeps links valid + history continuous | integration | `go test ./internal/pages -run TestRenameMove` | ❌ Wave 0 |
| PAGE-06/07 / D-08,10 | delete→trash→restore round-trips with provenance + collision | integration | `go test ./internal/pages -run TestTrashRestore` | ❌ Wave 0 |
| Optimistic floor | stale `base_revision` → 409 | integration | `go test ./internal/server -run TestPagePutConflict` | ❌ Wave 0 |
| VER-01/02/03 | save commits w/ identity; history lists; restore = forward commit | integration | `go test ./internal/pages -run TestHistoryRestore` | ❌ Wave 0 |
| VER-04 | push fires when configured; diverged → alert, no force | integration | `go test ./internal/gitstore -run TestPush` | ❌ Wave 0 |
| NAV-01..04 | tree renders, expand/collapse, current highlight | component | `npm --prefix web test LeftTree` | ❌ Wave 0 |
| NAV-05 | recent pages persist client-side | component | `npm --prefix web test recent` | ❌ Wave 0 |
| Single-writer | concurrent saves serialize, no `index.lock` corruption | integration (concurrency) | `go test ./internal/pages -run TestConcurrentSaves -race` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/okf/...` (the gate) + the package under edit.
- **Per wave merge:** `go test ./... -race` + `npm --prefix web run test`.
- **Phase gate:** full suite green AND **golden-corpus round-trip test green** before `/gsd-verify-work`.

### Wave 0 Gaps
- [ ] `internal/okf/golden/` corpus fixtures — headings, nested lists, fenced code (incl. code containing `---` and frontmatter-looking lines), GFM tables, inline + reference links, image + attachment links, frontmatter with comments/odd scalars, a CRLF fixture (A4). **This corpus IS the exit gate.**
- [ ] `internal/okf/*_test.go` — round-trip, repair, link-rewrite.
- [ ] `internal/pages/*_test.go` — create/save/read, rename/move, trash/restore, history/restore, concurrency.
- [ ] `internal/store/migrations/0004_drafts.sql` (+ optional `revisions`/`trash`).
- [ ] Frontend tests for `LeftTree`, `PageEditor` autosave, recent-pages store.
- [ ] No framework install needed (Go `testing` + existing Vitest cover everything).

*Spike (per ROADMAP Notes): prototype the single-writer Git batching + stale-lock recovery and the `internal/okf` byte-stable round-trip early — both have subtle failure modes. The spike feeds directly into the Wave 0 corpus.*

## Security Domain

> `security_enforcement: true`, ASVS L1.

### Applicable ASVS Categories
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V1 Architecture | yes | Single-writer Git; files-as-truth; SQLite operational-only (no content) |
| V4 Access Control | yes | `RequireRole` — readers view; editors create/edit/delete (this phase makes RBAC meaningful per CONTEXT code_context); authorize from session, never client input |
| V5 Input Validation | yes | Every path → `repo.Resolve` (SEC-01); title slugification rejects traversal; reject `..`/absolute/NUL before slug |
| V5 Output Encoding (XSS) | yes | `rehype-sanitize` on rendered Markdown; `rehype-raw` OFF; React escaping for titles/tree labels |
| V3 Session Mgmt | inherited | SCS sessions + nosurf CSRF already wrap all mutating routes (Phase 0) |
| V7 Logging | yes | Audit page create/edit/delete via existing `audit.Logger` (SPEC §21.5) |
| V6 Cryptography | no (new) | No new crypto; revision uses git blob SHA, not a security boundary |

### Known Threat Patterns for Go + chi + git-CLI + React-Markdown
| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Path traversal via page path / slug / link target | Tampering | `repo.Resolve` on EVERY path (incl. rename target, trash path, restore target); validate slug pre-write |
| Stored XSS via page body / agent-authored HTML | Tampering | `rehype-sanitize`, `rehype-raw` disabled; never `dangerouslySetInnerHTML` |
| Command injection into `git` | Tampering/EoP | Already mitigated: `gitstore` uses `exec.Command` arg slices, never a shell string. New rename/restore args must follow the same slice pattern. |
| Privilege bypass (reader mutating) | EoP | `RequireRole(editor)` on POST/PUT/DELETE page routes; authorize from session |
| CSRF on save/rename/delete | Tampering | nosurf already covers all mutating methods (Phase 0); new routes inherit it via the same group |
| Symlink escape on rename/move target | Tampering | `repo.Resolve` EvalSymlinks + `os.Root` (already enforced) — applies to the destination too |
| Audit gap on content changes | Repudiation | Record page create/edit/delete/rename/restore via `audit.Logger` (SEC-05) |

**Note:** Phase 1 does NOT introduce the agent or uploads, so V12 (file upload) and prompt-injection (Pitfall 4 in PITFALLS.md) are out of scope here — they arrive in Phases 2/4.

## Sources

### Primary (HIGH confidence)
- Existing Phase-0 source read in this session: `internal/gitstore/{commit,git,health}.go`, `internal/repo/{path,files}.go`, `internal/jobs/{queue,worker}.go`, `internal/store/migrations.go`, `internal/server/{router,middleware}.go`, `internal/auth/rbac.go`, `web/src/routes/AppShell.tsx`, `web/package.json`, `go.mod`, migrations `0001/0002`.
- `SPEC.md` §9 (repo layout), §10 (OKF format + required-field repair), §14 (commit triggers/batching/remote), §17.2–17.3 (tree/pages API), §18.3 (page modes), §21 (path/auth/audit) — read directly.
- `.planning/research/PITFALLS.md` Pitfalls 1–3 — round-trip rot, index.lock, cache-drift — read directly.
- `CONTEXT.md` D-01..D-13 + VER/NAV defaults — locked decisions.
- Version verification: `go list -m -versions github.com/yuin/goldmark` (v1.8.2), `gopkg.in/yaml.v3` (v3.0.1); `npm view` for all 5 frontend packages; `git --version` (2.47.3); `go version` (1.26.0); `node --version` (20.19.6).

### Secondary (MEDIUM confidence)
- Package legitimacy seam (`gsd-tools query package-legitimacy check --ecosystem npm ...`) — verdicts + download counts + repo URLs + `postinstall: null` for all 5 frontend packages.

### Tertiary (LOW confidence)
- None — all claims are sourced from repo code, SPEC, locked CONTEXT, or tool verification.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — versions verified on Go proxy + npm; most already in `go.mod`/`package.json`.
- Architecture: HIGH — built on Phase-0 spines read directly; patterns are reuse + one new package.
- Pitfalls: HIGH — taken from this project's own PITFALLS.md and verified against Goldmark/yaml.v3 behavior.
- Security: HIGH — reuses Phase-0 SEC-01/CSRF/RBAC/audit; new surfaces mapped to existing controls.

**Research date:** 2026-06-18
**Valid until:** 2026-07-18 (stable locked stack; re-verify only if Goldmark/yaml or the frontend libs bump majors)
