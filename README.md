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

## Security floor (Phase 0)

- Argon2id password hashing; server-side sessions (HttpOnly + SameSite); CSRF on
  every mutating route; role-based access control read only from the session.
- A safe-path resolver is the single filesystem chokepoint for all repo access.
- An **audit log** records key actions (login, logout, admin account changes,
  bootstrap/seed, password resets) to both a SQLite mirror and a structured log
  line — never recording any password or token.

## License

See the repository for license details.
