# Stack Research

**Domain:** Self-hosted Markdown wiki (single Go binary + React 19 SPA) — v1.0 additions: Obsidian-style knowledge graph + LLM auto-tagging
**Researched:** 2026-06-24
**Confidence:** HIGH (graph libs verified via npm registry today; tagging path verified against existing `internal/agent` source)

> **Scope:** This file covers ONLY what is NEW for the two v1.0 features. The existing stack (Go 1.26 CGO-free / chi / Goldmark / Bleve / modernc.org/sqlite / git CLI / Eino v0.9.9 + eino-ext openai / React 19.2 / Vite 8 / TS 6 / CM6 / react-query / zustand / react-router 7) is LOCKED and not re-evaluated.

## Executive Recommendation (one-liner each)

- **Graph rendering:** `react-force-graph-2d` (Canvas 2D, built on `force-graph` + d3-force). Best fit for hundreds-to-low-thousands of nodes, smallest dependency footprint of the React-friendly options, React-19-clean (`peerDependencies: { react: "*" }`), and gives the imperative handle needed for click-to-navigate + zoom/pan + Obsidian-like styling. **Edge-type toggles are computed in your own code** (you filter the link array), not a library feature in any candidate — so the library choice is about render perf/footprint, not "does it have toggles."
- **Backlinks / link graph:** Build the graph **server-side in Go from existing data** — no new backend dependency. Parse page-link + tag data you already have (Goldmark AST for links; YAML frontmatter tags) and expose a `GET /api/graph` JSON endpoint. Persist the adjacency in the existing SQLite operational DB as a derived cache (never source of truth).
- **LLM auto-tagging:** **No new agent dependency.** Reuse the existing `openai.ChatModel.Generate` path and the already-built **validate-and-retry structured-output pattern** (`internal/agent/propose.go`, "validate-and-retry, not ResponseFormat" per AI-SPEC §4b) plus the existing propose→approve→apply→commit flow. The only "new" pieces are a Go-side prompt + a JSON-tag-list validator. **Do NOT add embeddings, a vector store, or a structured-output/JSON-schema library.**

---

## Recommended Stack

### Core Technologies (NEW)

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| `react-force-graph-2d` | 1.29.1 | Interactive force-directed graph (global view + local panel) in the React SPA | Canvas-2D renderer (handles ~thousands of nodes at 60fps far better than SVG; no WebGL/three.js needed at this scale). Built on the mature `force-graph` (vasturiano) + `d3-force` simulation. Exposes imperative ref (`zoomToFit`, `centerAt`, `d3Force`, node/link click & hover, custom `nodeCanvasObject`) → gives the Obsidian-style label-on-zoom, click-to-navigate, hover-highlight UX. `peerDependencies: { react: "*" }` → clean on React 19. Small dep tree (`force-graph`, `react-kapsule`, `prop-types`). |
| **Server-side graph builder (Go)** | — (stdlib + existing Goldmark) | Compute nodes/edges (page links, backlinks, shared-tag edges) from files | No new dependency. You already parse Markdown (Goldmark) and frontmatter (yaml.v3). Walk the AST for links, read `tags` from frontmatter, invert for backlinks. Emit one JSON graph payload. Keeps the heavy lifting in Go (fast, testable, no client-side full-repo parse). |
| **Existing Eino `openai.ChatModel`** | eino v0.9.9 / eino-ext openai (pinned pseudo-version) | LLM tag suggestion (per-page + bulk sweep) | Already wired (`internal/agent/chatmodel.go`). Tag suggestion is a single-shot `Generate` (no ReAct loop needed) — same shape as the existing Rewrite/Draft modes. Reuses the validate-and-retry contract and the propose→approve→commit safety model verbatim. Zero new agent deps. |

### Supporting Libraries (NEW, frontend)

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `react-force-graph-2d` | 1.29.1 | (see Core) | **Pick.** Install the `-2d` subpackage ONLY — not the umbrella `react-force-graph` (1.48.2), which bundles 2D + 3D(three.js) + VR + AR and balloons the bundle. The `-2d` package is the right granular import. |
| `d3-force` | 3.0.0 | Force simulation tuning (charge/link/center) | **Transitive** via `force-graph`; you normally don't import it directly. Pull it in explicitly only if you need to set custom forces via the `d3Force()` accessor (e.g. weaker charge for the dense global graph). |
| `@types/d3-force` | 3.0.10 | Types for the above | Dev-only, add if you touch `d3Force()` directly. |

> **No third-party "edge toggle" or "graph config" library exists or is needed.** Obsidian's filter toggles (page links / backlinks / shared-tag edges, depth, orphans) are just **client state** over which edges you pass to the component. Manage that toggle state in the existing **zustand** store and recompute the filtered `links` array with a `useMemo`. react-query already handles fetching the `/api/graph` payload.

### Supporting Libraries (NEW, backend)

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| **(none required)** | — | — | Graph build = Goldmark AST walk + yaml.v3 frontmatter (both already present). Backlink/adjacency cache = a new SQLite table via the existing modernc.org/sqlite + migration path. Tagging = existing eino-ext openai. **The headline finding for the backend is: this milestone needs no new Go module.** |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| Existing Vite 8 + `embed.FS` | Bundle the graph component into the single binary | `react-force-graph-2d` is plain ESM, tree-shakeable, embeds via the existing `web/dist` → `//go:embed` path. No special build step. Verify the production bundle size delta after adding it (expect roughly low-hundreds of KB gzipped for force-graph+d3-force; acceptable, but lazy-load the graph route via `React.lazy` so it doesn't bloat the initial editor load). |
| Existing migration path (`CREATE TABLE IF NOT EXISTS` or golang-migrate) | Derived link/backlink cache table | Operational data only — derived from files, rebuildable from a full reindex, never authoritative. |

## Installation

```bash
# Frontend (web/) — the ONLY new runtime dep
npm install react-force-graph-2d@1.29.1

# Optional, only if you tune forces directly:
npm install d3-force@3.0.0
npm install -D @types/d3-force@3.0.10

# Backend: NO new go get. Graph + tagging reuse Goldmark, yaml.v3,
# modernc.org/sqlite, and the existing eino + eino-ext openai modules.
```

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| `react-force-graph-2d` (Canvas, 1.29.1) | `reagraph` 4.31.0 | reagraph is React-native and pretty, but it pulls the **entire three.js / @react-three/fiber / drei / @react-spring/three** stack (WebGL) + ~10 graphology packages → large bundle, heavier mental model, and WebGL is overkill for low-thousands of nodes. Choose reagraph only if you later need 3D, GPU-scale (10k+ nodes), or its built-in clustering/aggregation. Not for v1.0. |
| `react-force-graph-2d` | `sigma` 3.0.3 + `graphology` 0.26.0 (+ `@react-sigma/core` 5.0.6) | Sigma is WebGL and excellent at 10k–100k+ nodes; `@react-sigma/core` gives a React wrapper. But it's a steeper API (graphology model + sigma renderer + reducers for styling) and more bundle/setup than needed at this scale. Switch to sigma if the workspace graph ever exceeds ~5k nodes and Canvas starts to stutter. |
| `react-force-graph-2d` | `cytoscape` 3.34.0 + `react-cytoscapejs` 2.0.0 | Cytoscape is the gold standard for **analysis/bioinformatics** graphs (rich selectors, layout algorithms, compound nodes). Heavier and more "diagram editor" than "ambient knowledge graph." Use if you need advanced layout algorithms or graph-analysis features beyond force-directed display. `react-cytoscapejs` 2.0.0's React-19 status is less certain than force-graph's `react: "*"`. |
| `react-force-graph-2d` | `vis-network` 10.1.0 | Mature and capable, but Canvas-based with an older imperative API, no first-class React wrapper (you manage the instance yourself), and a larger footprint. No advantage over force-graph here. |
| Server-side Go graph builder | Client-side parse of all Markdown | Only if you wanted a purely static/offline export. Server-side is faster, testable in Go, reuses Goldmark, and avoids shipping/parsing the whole repo to the browser. |
| Validate-and-retry tagging (reuse) | OpenAI `response_format`/JSON-schema structured outputs | Only if you lock the deployment to a provider+model that reliably supports JSON-schema mode. The project is **provider-agnostic** (DeepSeek today, Ollama/local possible) — many endpoints don't honor `response_format` consistently, so the existing validate-and-retry approach is the portable, already-proven choice. |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| Umbrella `react-force-graph` (1.48.2) | Bundles 2D **+ 3D (three.js) + VR + AR** in one package → huge bundle, breaks the lightweight single-binary ethos. | The granular `react-force-graph-2d` (1.29.1) subpackage. |
| `reagraph` / `sigma` / WebGL stacks for v1.0 | Drag in three.js or graphology+WebGL renderer for a node count Canvas handles fine; more bundle + complexity for a 5-user wiki. | Canvas-based `react-force-graph-2d`. Revisit only if node count blows past a few thousand. |
| A graph **database** (Neo4j, Dgraph, SQLite graph extensions, RedisGraph) | Violates the "no PostgreSQL/Redis/Elasticsearch, single binary, files-as-truth" constraint. The graph is small and derivable from files on every reindex. | Compute in Go from files; cache adjacency in the existing SQLite operational DB (derived, rebuildable). |
| Embeddings + a vector store (pgvector, Qdrant, Chroma, sqlite-vss, a local embedding model) for tagging | Massive scope creep for "suggest a few tags." Adds a model/service/dependency, breaks single-binary + CGO-free, and isn't needed — an LLM `Generate` call with the page text + existing-tag vocabulary in the prompt is sufficient and provider-agnostic. | Plain `ChatModel.Generate` with the controlled tag vocabulary injected into the prompt. |
| A new Go JSON-schema / structured-output library wired to `response_format` for tagging | The project already standardized on **validate-and-retry, not ResponseFormat** (AI-SPEC §4b, `propose.go`) precisely because the LLM endpoint is provider-agnostic. Adding schema-mode plumbing fights that decision and isn't portable to Ollama/DeepSeek reliably. | Reuse the existing validate-and-retry pattern; write a small Go validator that parses the model's JSON tag array and rejects malformed/empty/out-of-vocabulary output, then retries (≤2) — mirroring `proposeWithRetry`. |
| Silent frontmatter writes from the tagging sweep | Violates the locked agent-safety model ("agent writes require explicit user approval"). | Route tag suggestions through the existing **propose→review→approve→apply→commit** flow (per-page diff for single pages; a batch-review UI for the bulk sweep). |
| `dangerouslySetInnerHTML` / unsanitized node labels in the graph | Node labels come from page titles (user/agent authored) → stored-XSS surface if rendered as HTML. | Render labels as Canvas text via `nodeCanvasObject` (text, not HTML) — Canvas drawing is inherently non-HTML, so labels are safe by construction. |

## Stack Patterns by Variant

**Global graph view (whole workspace, hundreds–low-thousands of nodes):**
- `react-force-graph-2d` with Canvas; lazy-load the route (`React.lazy`).
- Tune `d3Force('charge')` to weaker repulsion + use `cooldownTicks`/`warmupTicks` so the layout settles fast and stops (idle CPU instead of a perpetual simulation).
- Show labels only past a zoom threshold (Obsidian behavior) inside `nodeCanvasObject`.
- Compute node/edge data server-side and send a lean payload (id, title, path, degree; edges as id pairs with a `type` field for filtering).

**Local graph panel (current page + direct neighbors):**
- Same component, tiny pre-filtered subgraph (1-hop, optionally 2-hop) from the same `/api/graph` data or a focused `/api/graph?center=<path>&depth=1` endpoint.
- Fixed small size in a side panel; cheap to simulate.

**Edge-type toggles (page links / backlinks / shared-tag):**
- Backend tags each edge with `type: "link" | "backlink" | "tag"`.
- Frontend keeps three booleans in zustand; a `useMemo` filters `links` by enabled types before passing to the component. No library feature involved.
- Note: "page links" and "backlinks" are the same undirected edge set viewed two ways — decide whether to render link direction (arrows) or treat as undirected like Obsidian. Shared-tag edges can explode density (every page sharing a common tag becomes a clique) — gate them behind the toggle (default off) and consider a min-shared-tag threshold or rendering tags as their own node type.

**LLM tagging (per-page, on demand):**
- `ChatModel.Generate` with: page title + body (truncated to a token budget) + the **controlled vocabulary** = the set of tags already used across the workspace (read from the frontmatter/SQLite tag index). Ask for a JSON array of tags, "prefer existing tags, propose at most N new."
- Validate: JSON parses, items are strings, dedup, normalize case/slug, drop anything not matching tag rules; retry ≤2 on failure.
- Emit as a frontmatter diff through the existing approve→apply→commit flow.

**LLM tagging (bulk sweep over untagged pages):**
- Run as a **background job** via the existing `internal/jobs` machinery (don't block a request; the sweep is many `Generate` calls).
- Each page produces a *proposal*, not a write. Collect proposals into a batch-review queue the user approves page-by-page or all-at-once. Rate-limit/cap concurrency to respect the LLM endpoint and keep a 5-user box responsive.

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| `react-force-graph-2d` 1.29.1 | React 19.2.7 | `peerDependencies: { react: "*" }` → no React-19 peer conflict. It ships `prop-types` 15 as a normal dep; React 19 silently ignores propTypes (harmless, no action needed). |
| `react-force-graph-2d` 1.29.1 | `force-graph` ^1.51, `react-kapsule` ^2.5 | Transitive; Canvas renderer, no three.js. |
| `d3-force` 3.0.0 | `force-graph` ^1.51 | force-graph depends on d3-force internally; explicit install only if you call `d3Force()` directly — keep versions aligned (both d3 v3 line). |
| New graph code | Vite 8 / `embed.FS` | Plain ESM, tree-shakeable, embeds via existing pipeline. Lazy-load to keep editor's initial bundle lean. |
| Tagging code | eino v0.9.9 + eino-ext openai (pinned) | Reuses existing `openai.ChatModel.Generate`; no version bump, no new agent surface to re-verify. |
| Graph + tagging persistence | modernc.org/sqlite v1.52.0 | Derived link cache + tag index live in the existing operational DB (CGO-free, single binary preserved). |

## Sources

- `npm view <pkg> version` (2026-06-24) — `react-force-graph-2d` 1.29.1, `react-force-graph` 1.48.2, `force-graph` 1.51.4, `d3-force` 3.0.0, `d3-force-3d` 3.0.6, `sigma` 3.0.3, `graphology` 0.26.0, `@react-sigma/core` 5.0.6, `cytoscape` 3.34.0, `react-cytoscapejs` 2.0.0, `vis-network` 10.1.0, `reagraph` 4.31.0, `three` 0.184.0, `@types/d3-force` 3.0.10 — HIGH
- `npm view react-force-graph-2d peerDependencies dependencies` → `{ react: "*" }` + `force-graph`/`react-kapsule`/`prop-types` (Canvas-only, no three.js) — HIGH
- `npm view reagraph dependencies` → confirms three.js + @react-three/fiber + drei + ~10 graphology pkgs (heavy WebGL stack) — HIGH
- [vasturiano/react-force-graph (GitHub)](https://github.com/vasturiano/react-force-graph) + [react-force-graph-2d (npm)](https://www.npmjs.com/package/react-force-graph-2d) — granular 2D subpackage, Canvas renderer, React wrapper API — HIGH
- [React 19 Upgrade Guide](https://react.dev/blog/2024/04/25/react-19-upgrade-guide) — propTypes ignored in React 19 (so force-graph's prop-types dep is harmless) — HIGH
- `internal/agent/propose.go` + `chatmodel.go` + `prompts.go` (this repo) — existing `ChatModel.Generate` single-shot path and the validate-and-retry structured-output contract (AI-SPEC §4b) that tagging reuses — HIGH
- PROJECT.md / CLAUDE.md locked constraints (single CGO-free binary, files-as-truth, provider-agnostic LLM, agent-approval safety model) — requirement boundaries that rule out graph DBs, embeddings, and schema-mode plumbing — HIGH

---
*Stack research for: knowledge-graph + LLM-auto-tagging additions to a single-binary Go/React Markdown wiki*
*Researched: 2026-06-24*
