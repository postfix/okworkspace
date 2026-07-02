# OKF Workspace Documentation

**OKF Workspace** is a lightweight, self-hosted, OKF-native internal wiki for small teams (~5 people), built for the agent era. Human-readable Markdown files with YAML frontmatter are the source of truth, Git provides hidden version history, and a CloudWeGo Eino agent can read, search, summarize, and propose edits that a human approves before they are applied.

This directory contains contributor-facing architecture documentation, covering the graph,
tag, and editor subsystems and their HTTP contracts.

## Quick Links

- [Architecture Overview](architecture/overview.md) -- system architecture, data flow, component diagram
- [Editor Subsystem](architecture/editor-subsystem.md) -- the CM6 live-preview surface, decoration pipeline, syntax highlight, mermaid, hashtags
- [Graph Subsystem](architecture/graph-subsystem.md) -- derived link/tag adjacency store, rebuild, query layer
- [Tag Subsystem](architecture/tag-subsystem.md) -- LLM tag suggestion, bulk sweep, review queue, byte-stable apply
- [Tag Metadata Subsystem](architecture/tag-metadata-subsystem.md) -- tag color/description/hierarchy store (files-as-truth), usage analytics, tag-op undo (v1.6)
- [API: Authoring](api/authoring.md) -- page templates, page create, LLM-generated description endpoint contracts
- [API: Graph](api/graph.md) -- global graph (GraphData), Backlinks endpoint contracts (the per-page local graph was removed in v1.5)
- [API: Tagsweep](api/tagsweep.md) -- tag suggestion, bulk sweep, review queue endpoint contracts
- [API: Tag Admin](api/tag-admin.md) -- workspace-wide rename/merge/delete/bulk (frontmatter + body), tag metadata, analytics, undo, tag vocabulary

## Tech Stack

| Layer | Technology | Purpose |
|-------|-----------|---------|
| Backend | Go 1.26, chi v5 | Single static binary, HTTP router |
| Frontend | React 19 + Vite 8 + TypeScript 6 | SPA, embedded via `//go:embed` |
| Editor | CodeMirror 6 + `@lezer/markdown` | Hand-built live-preview Markdown surface (edit + read); syntax highlight + mermaid loaded as lazy chunks |
| Markdown | Goldmark v1.8 + YAML v3 | Markdown render, frontmatter parse/emit |
| Search | Bleve v2.6 | Full-text search index (pure Go) |
| Agent | CloudWeGo Eino v0.9.9 | LLM orchestration (provider-agnostic) |
| Database | SQLite (modernc.org/pure-Go) | Operational metadata only |
| Versioning | git CLI | Hidden history behind the backend |

## How to Contribute

### Prerequisites

- Go 1.26
- Node 20.19+
- `git` binary on PATH

### Build and Run

```bash
# Build the SPA
cd web && npm ci && npm run build && cd ..

# Build the single binary
CGO_ENABLED=0 go build -o okf-workspace ./cmd/okf-workspace

# Configure and run
cp config.example.yaml config.yaml
./okf-workspace serve --config ./config.yaml
```

### Code Conventions

- **Files are truth.** SQLite holds only operational metadata; all content is plain Markdown on disk.
- **Safe-path resolver.** All filesystem access goes through `internal/repo.Repo`, never raw `os.*` calls.
- **Single-writer commit.** Mutations route through `pages.Save` which serializes through a single-writer Git commit spine.
- **Derived caches are rebuildable.** Both the graph cache and the search index can be rebuilt from on-disk files at any time.
- **Error handling.** Server-facing errors return generic copies to clients (no infrastructure vocabulary leaked). Details go to structured logs via `slog`.

### Testing

Tests use the standard Go testing package. Key patterns:

- **Graph:** `internal/graph/harness_test.go` sets up an in-memory SQLite store with a mock repo for unit tests.
- **Tagsweep:** `internal/tagsweep/job_test.go` and `store_test.go` test the staging and job dispatch.

Run all tests:

```bash
CGO_ENABLED=0 go test ./...
```
