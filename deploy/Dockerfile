# OKF Workspace multi-stage build (SPEC §20.5).
#
# Stage 1 (node):  build the React SPA into internal/web/dist (the //go:embed root).
# Stage 2 (golang): CGO_ENABLED=0 static build with the SPA embedded.
# Stage 3 (final):  minimal, NON-root runtime image (T-00.04-04).
#
# NOTE on the runtime base (T-00.04-SC):
#   The app shells out to the `git` CLI (a LOCKED decision — single-writer Git
#   versioning), so the runtime image MUST contain a `git` binary. A pure
#   distroless/static or scratch image has no git, so we use a pinned minimal
#   Alpine that ships git and runs as a non-root user. All base images are
#   pinned by tag; the SPA is built with `npm ci` against the committed lockfile.

# ---- Stage 1: build the SPA ----
FROM node:20.19-bookworm-slim AS web
WORKDIR /src/web
# Copy only the manifest first for a cached dependency layer.
COPY web/package.json web/package-lock.json ./
RUN npm ci
# Build into ../internal/web/dist (the embed root); copy the rest of the repo
# that the build references (vite config, tsconfig, src, and the embed target).
COPY web/ ./
COPY internal/web/ /src/internal/web/
RUN npm run build

# ---- Stage 2: build the static Go binary ----
FROM golang:1.26-bookworm AS build
WORKDIR /src
# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download
# Bring in the source and the freshly built SPA (so //go:embed finds dist/*).
COPY . .
COPY --from=web /src/internal/web/dist/ ./internal/web/dist/
# Pure-Go, statically linked single binary (CLAUDE.md: CGO_ENABLED=0).
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/okf-workspace ./cmd/okf-workspace

# ---- Stage 3: minimal non-root runtime ----
FROM alpine:3.21
# git is a hard runtime dependency (shell-out single-writer Git, LOCKED).
# ca-certificates for any future https git remote / LLM endpoint.
RUN apk add --no-cache git ca-certificates \
    && addgroup -S okf && adduser -S -G okf -h /data okf \
    && mkdir -p /data && chown -R okf:okf /data
COPY --from=build /out/okf-workspace /usr/local/bin/okf-workspace
COPY config.example.yaml /etc/okf-workspace/config.example.yaml

# Run as the non-root okf user with a scoped, persisted data directory.
USER okf
WORKDIR /data
VOLUME ["/data"]
EXPOSE 8080

# OKF_DATA_DIR/OKF_LISTEN can override the config for container deployment.
# OKF_LLM_API_KEY is supplied at `docker run -e ...` time (never baked in).
ENV OKF_DATA_DIR=/data \
    OKF_LISTEN=0.0.0.0:8080

ENTRYPOINT ["/usr/local/bin/okf-workspace"]
CMD ["serve", "--config", "/etc/okf-workspace/config.yaml"]
