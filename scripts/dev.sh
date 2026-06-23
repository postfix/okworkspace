#!/usr/bin/env bash
# dev.sh — rebuild the SPA + Go binary and (re)start the OKF Workspace server.
#
# The SPA is embedded into the binary via //go:embed internal/web/dist, so a
# frontend change is NOT live until BOTH the SPA and the binary are rebuilt.
# This script does the full cycle so you never test a stale binary again.
#
# Usage:
#   ./scripts/dev.sh                       rebuild, then run the server (foreground)
#   ./scripts/dev.sh --build-only          rebuild only; don't start the server
#   ./scripts/dev.sh --config ./other.yaml use a different config file
#   ./scripts/dev.sh --help                show this help
#
# Ctrl-C stops the server (it runs in the foreground so you see the logs —
# including the one-time admin password on first run).
set -euo pipefail

# Resolve repo root from this script's location so it works from any cwd.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

CONFIG="./config.yaml"
SERVE=1
BIN="./okf-workspace"

while [ $# -gt 0 ]; do
  case "$1" in
    --build-only)   SERVE=0 ;;
    --config)       CONFIG="${2:?--config needs a path}"; shift ;;
    --config=*)     CONFIG="${1#*=}" ;;
    -h|--help)      sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *)              echo "dev.sh: unknown argument '$1' (try --help)" >&2; exit 2 ;;
  esac
  shift
done

# 1. Stop any running server (graceful TERM, then force KILL stragglers).
#    Matches only the running 'okf-workspace serve' process, not the build.
echo "==> stopping any running okf-workspace serve ..."
pkill -TERM -f 'okf-workspace serve' 2>/dev/null || true
for _ in 1 2 3 4 5 6; do
  pgrep -f 'okf-workspace serve' >/dev/null 2>&1 || break
  sleep 0.3
done
pkill -KILL -f 'okf-workspace serve' 2>/dev/null || true

# 2. Build the SPA (outputs to internal/web/dist, the //go:embed root).
#    Install deps first if node_modules is missing.
if [ ! -d web/node_modules ]; then
  echo "==> web/node_modules missing — running npm ci ..."
  npm --prefix web ci
fi
echo "==> building SPA (web) ..."
npm --prefix web run build

# Vite wipes internal/web/dist on build, deleting the tracked .gitkeep — restore it.
if [ ! -e internal/web/dist/.gitkeep ]; then
  git checkout -- internal/web/dist/.gitkeep 2>/dev/null || touch internal/web/dist/.gitkeep
fi

# 3. Build the single, cgo-free binary with the SPA embedded.
echo "==> building binary (CGO_ENABLED=0) ..."
CGO_ENABLED=0 go build -o "$BIN" ./cmd/okf-workspace

# 4. First-run config: copy the example if none exists yet.
if [ ! -f "$CONFIG" ]; then
  echo "==> no $CONFIG found — copying from config.example.yaml"
  cp config.example.yaml "$CONFIG"
fi

if [ "$SERVE" -eq 0 ]; then
  echo "==> build complete (--build-only); server not started."
  exit 0
fi

# 5. (Re)start in the foreground; exec so Ctrl-C cleanly stops the server.
echo "==> starting: $BIN serve --config $CONFIG"
echo "    (Ctrl-C to stop. On first run, watch the log for the one-time admin password.)"
exec "$BIN" serve --config "$CONFIG"
