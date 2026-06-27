# OKF Workspace

A lightweight, self-hosted, **OKF-native internal wiki built for the agent era**.

Human-readable Markdown files (with YAML frontmatter) are the source of truth,
Git provides hidden version history, uploaded attachments stay downloadable as
their original files, and a CloudWeGo Eino agent can read, search, summarize, and
propose edits that a human approves before they are applied.

It targets a small internal team (~5 people) who want Notion-like simplicity
without vendor lock-in, monthly cost, or a proprietary knowledge database.

> **Core value:** a non-technical teammate can create, edit, and find knowledge —
> and get useful AI help on it — while every byte stays as plain Markdown +
> original files on disk, versioned in Git, with no proprietary store to escape.

## Why

- **Files are truth.** Content is plain Markdown on disk; SQLite holds only
  operational metadata (users, sessions, jobs, the audit-log mirror) — never
  wiki content. Copy the data directory to move the whole workspace.
- **Single static binary.** Pure-Go, `CGO_ENABLED=0`, the React SPA embedded —
  deploy one binary + a data directory on a small VPS/homelab. No PostgreSQL,
  Redis, Elasticsearch, or Kubernetes.
- **Hidden Git history.** Versioning shells out to the `git` CLI behind the
  backend; users never need to know Git.

## Build & run (SPEC §20.1)

Prerequisites: Go 1.26, Node 20.19+, and a `git` binary on `PATH`.

```bash
# 1. Build the SPA (outputs to internal/web/dist, the //go:embed root)
cd web && npm ci && npm run build && cd ..

# 2. Build the CGO-free single binary with the SPA embedded
CGO_ENABLED=0 go build -o okf-workspace ./cmd/okf-workspace

# 3. Configure and run
cp config.example.yaml config.yaml      # adjust server.listen / storage.data_dir
./okf-workspace serve --config ./config.yaml
```

### First-run admin password

On the first start against an empty database, OKF Workspace creates the `admin`
account and prints a **one-time password exactly once** to the log:

```
"msg":"admin user created — save this password, it will NOT be shown again"
"username":"admin" "one_time_password":"<28 chars>" "must_change_password":true
```

Sign in with it, then set a new password when prompted. The plaintext is never
logged again. Locked out? Recover from a shell:

```bash
./okf-workspace admin reset-password <username>
```

## Configuration

`config.yaml` (SPEC §20.3) has `server`, `storage`, `git`, `auth`, `agent`,
`search`, and `attachments` sections — see [`config.example.yaml`](config.example.yaml).
Deployment env overrides: `OKF_DATA_DIR`, `OKF_LISTEN`, `OKF_ADMIN_USERNAME`.
The LLM API key is read at runtime from the variable named by `agent.api_key_env`
(default `OKF_LLM_API_KEY`) and is never stored in config or logged.

## Deploy

Run under **systemd** or **Docker** as a non-root single binary + data directory.
See [`deploy/README.md`](deploy/README.md) for both paths.

## v1.0 Features

OKF Workspace v1.0 adds a **derived knowledge graph** and **LLM auto-tagging** on top of the core wiki:

- **Global graph view** (`/app/graph`) -- a force-directed D3 visualization of every page and tag in the workspace as nodes, with typed edges for page-to-page links and page-to-tag membership. Nodes are sized by degree; orphan pages (no links, no tags) are visually distinct. Edge-type toggle chips filter by Links, Backlinks, and Shared Tags.
- **Local graph dock** -- an expandable panel on each page view showing the page's neighborhood (1/2/3-hop BFS traversal over link edges), with tag edges and popular-tag capping to prevent hub-node domination.
- **Backlinks panel** -- "Referenced by" listing on every page view showing all pages that link to the current page (reverse backlinks query) -- "Referenced by" listing on every page view showing all pages that link to the current page, with resolved titles.
- **Per-page LLM tag suggestion** -- a provider-agnostic one-shot mode that reads the page body, biases toward the workspace's existing tag vocabulary, and returns up to 5 normalized tags. The apply path is gated through the same byte-stable `pages.Save` single-writer commit with optimistic concurrency (409 on moved revision).
- **Bulk sweep & review queue** -- admin can sweep all untagged (or all) pages to enqueue one LLM tag-suggestion job per page. Results stage into a review queue; the admin approves per page through a batched apply that writes all approved tags in a single Git commit.
- **Admin graph rebuild** -- `POST /api/v1/admin/graph/reindex` rebuilds the derived link/tag adjacency from on-disk files (the correctness backstop).

For detailed architecture documentation, see [`docs/`](docs/README.md).

## v1.1 Features

OKF Workspace v1.1 adds **UX improvement, security hardening, and graph filtering**:

- **Graph Filter Sidebar** -- a flat panel of controls alongside the graph edge-type toggles: node search (live filter by name/path), visibility toggles (Tags: ON, Orphans: ON, Attachments: OFF, Existing files only: OFF), graph groups (named/colored/filtered), and display controls (arrows toggle, text fade threshold, node size, link thickness sliders). Every control updates the graph live. Settings persist in localStorage.
- **Sweep progress polling** -- the admin TagReviewView now polls `GET /admin/tags/sweep/status` every 2s while jobs are in-flight, showing a progress bar ("Processing sweep... N/M pages") and completion state. Non-blocking: user can navigate away and return.
- **Sweep fan-out cap** -- configurable `sweep.max_concurrent_jobs` (default 10) prevents resource exhaustion from a single bulk sweep run. The response reports both `queued` and `total`.
- **Tag charset validation** -- server-side input validation rejects tags containing characters outside `[a-zA-Z0-9_ -]` (no HTML metacharacters, Unicode, dots, or slashes). Defense-in-depth layered through `agent.ValidateTags`, `okf.ValidateTagCharset`, and `okf.SetTags`.
- **Security scanning** -- `govulncheck` reports 0 reachable vulnerabilities; `go vet` is clean; transitive vulnerabilities (19, none reachable) documented for tracking.

## Security floor (Phase 0)

- Argon2id password hashing; server-side sessions (HttpOnly + SameSite); CSRF on
  every mutating route; role-based access control read only from the session.
- A safe-path resolver is the single filesystem chokepoint for all repo access.
- An **audit log** records key actions (login, logout, admin account changes,
  bootstrap/seed, password resets) to both a SQLite mirror and a structured log
  line — never recording any password or token.

## License

See the repository for license details.
