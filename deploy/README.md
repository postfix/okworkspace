# Deploying OKF Workspace

OKF Workspace ships as **one static Go binary + a data directory** — no
PostgreSQL, Redis, Elasticsearch, or Kubernetes. You can run it from a release
binary, under systemd, or in Docker. A `git` binary must be on `PATH` at runtime
(versioning shells out to the git CLI — a locked decision).

The data directory (`storage.data_dir`, SPEC §20.2) holds everything portable:

```
<data_dir>/
  repo/        # the Markdown wiki (the source of truth) + hidden .git history
  app.db       # operational SQLite: users, sessions, jobs, audit_log mirror
  logs/        # (reserved)
  tmp/         # (reserved)
```

Copy `<data_dir>` + `config.yaml` to move an entire workspace.

## First-run admin password

On the very first start against an empty `app.db`, OKF Workspace creates the
admin account and prints a **one-time password exactly once** to the log:

```
... "msg":"admin user created — save this password, it will NOT be shown again"
    "username":"admin" "one_time_password":"<28 chars>" "must_change_password":true
```

Save it, sign in, and you will be forced to set a new password. The plaintext is
never logged again. (Lost it? Recover from a shell: `okf-workspace admin
reset-password <username>`.) The bootstrap is recorded in the audit log.

---

## Option A — systemd (`deploy/okf-workspace.service`)

```bash
# 1. Dedicated non-root user + directories
sudo useradd --system --home /var/lib/okf-workspace --shell /usr/sbin/nologin okf
sudo mkdir -p /etc/okf-workspace /var/lib/okf-workspace
sudo chown -R okf:okf /var/lib/okf-workspace

# 2. Binary + config
sudo install -m 0755 okf-workspace /usr/local/bin/okf-workspace
sudo cp ../config.example.yaml /etc/okf-workspace/config.yaml
sudo $EDITOR /etc/okf-workspace/config.yaml         # set storage.data_dir=/var/lib/okf-workspace

# 3. Secrets (only needed once the agent is enabled, Phase 4)
echo 'OKF_LLM_API_KEY=changeme' | sudo tee /etc/okf-workspace/okf-workspace.env >/dev/null
sudo chmod 600 /etc/okf-workspace/okf-workspace.env

# 4. Install + start
sudo cp okf-workspace.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now okf-workspace

# 5. Read the one-time admin password
sudo journalctl -u okf-workspace | grep one_time_password
```

The unit runs as the non-root `okf` user, restarts on failure, and confines
writes to `/var/lib/okf-workspace` (`ProtectSystem=strict` +
`ReadWritePaths=`). `OKF_DATA_DIR` / `OKF_LISTEN` may override the config.

---

## Option B — Docker (`deploy/Dockerfile`)

Multi-stage build: stage 1 `npm ci && npm run build` (SPA), stage 2
`CGO_ENABLED=0 go build` (static binary with the SPA embedded), final stage a
pinned minimal Alpine that ships `git` and runs as the **non-root `okf`** user.

```bash
# Build (run from the repo root so the build context includes web/ + internal/)
docker build -f deploy/Dockerfile -t okf-workspace:latest .

# Run with a persisted data volume on port 8080
docker run -d --name okf \
  -p 8080:8080 \
  -v okf-data:/data \
  -e OKF_LLM_API_KEY=changeme \
  okf-workspace:latest

# Read the one-time admin password from the container log
docker logs okf 2>&1 | grep one_time_password
```

`OKF_DATA_DIR=/data` and `OKF_LISTEN=0.0.0.0:8080` are set in the image; the
LLM API key is supplied with `-e OKF_LLM_API_KEY=...` at run time and is never
baked into the image or logged.

> **Why Alpine and not distroless/scratch?** The wiki versions content by
> shelling out to the `git` CLI (locked decision), and scratch/distroless-static
> images contain no `git`. The runtime base is a pinned, minimal Alpine with
> `git` + `ca-certificates`, running as a non-root user — keeping the
> single-binary, non-root deploy promise while satisfying the hard git dependency.

---

## Building the binary yourself (SPEC §20.1)

```bash
cd web && npm ci && npm run build && cd ..        # SPA -> internal/web/dist (embed root)
CGO_ENABLED=0 go build -o okf-workspace ./cmd/okf-workspace
cp config.example.yaml config.yaml                # then edit storage.data_dir etc.
./okf-workspace serve --config ./config.yaml
```
