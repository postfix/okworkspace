# Project Research Summary

**Project:** OKF Workspace — v1.0 Knowledge Graph & LLM Auto-Tagging
**Domain:** Self-hosted Markdown wiki (Go single-binary + React SPA) — additive milestone on top of shipped v0.9.9
**Researched:** 2026-06-24
**Confidence:** HIGH

## Executive Summary

This milestone adds two closely coupled features to an already-shipped, well-constrained system: an Obsidian-style knowledge graph (global view + per-page local panel + backlinks) and LLM-driven tag suggestions routed through the existing agent safety model. The core research finding is that **no new backend Go module and only one new frontend package (`react-force-graph-2d` 1.29.1) are needed.** The graph is built server-side from existing Goldmark + yaml.v3 parsing, cached as derived adjacency in the existing SQLite operational DB, and served via two new read endpoints. Tag suggestion reuses the existing `openai.ChatModel.Generate` path and the validate-and-retry + propose→approve→apply→commit safety chain verbatim. The entire milestone is an integration exercise over v0.9.9 seams, not a greenfield build.

The recommended approach follows two parallel build chains after a shared foundation: (1) derived link/tag store → graph endpoints → graph UI; (2) `okf.SetTags` byte-stable primitive → tag suggestion mode → per-page approve flow → bulk sweep. Both chains plug into the same fire-and-forget job worker and single-writer commit path that already exist. The big design win: the graph backend has no new dependency, the tagging backend has no new dependency, and the frontend adds only the 2D-only Canvas graph library (no WebGL/three.js). This keeps the single CGO-free binary constraint and the files-as-truth constraint completely intact.

The primary risks are all about discipline on integration points that already exist: (a) tag writes must use the surgical `yaml.Node` editor, not re-marshal the whole frontmatter struct — one shortcut here silently corrupts every tagged page's Git history; (b) the bulk sweep must queue proposals for human review, never write autonomously — the safety model is a locked project constraint; (c) shared-tag edges are combinatorially explosive on popular tags and must be capped or modeled as a bipartite page↔tag graph. These risks are all preventable at design time with explicit acceptance criteria and a golden-file round-trip test.

---

## Key Findings

### Recommended Stack

The existing stack is fully reused. The only new runtime dependency is `react-force-graph-2d@1.29.1` (Canvas 2D, built on `force-graph` + `d3-force`, `peerDependencies: { react: "*" }` — clean on React 19, no three.js, small dep tree). The umbrella `react-force-graph` (1.48.2) must be explicitly avoided — it bundles 3D/VR/AR and drags three.js into the embedded binary. The graph view should be route-level lazy-loaded (`React.lazy`) to keep the initial editor bundle unaffected.

Backend: graph adjacency = Goldmark AST walk (`okf.FindLinks`, already used in rename/move) + `search.readTags` (already sequence-aware) → two new SQLite tables (`page_links`, `page_tags`) maintained by a new `KindGraph` job on the existing single-drain worker. LLM tagging = `ChatModel.Generate` (single-shot, same shape as Rewrite/Draft modes) + a new `validateTags` output validator + `okf.SetTags` (the one new primitive: a `yaml.SequenceNode` surgical frontmatter setter mirroring existing `okf.SetField`).

**Core technologies (new additions only):**
- `react-force-graph-2d` 1.29.1: 2D force-directed Canvas graph renderer — only frontend addition; install the `-2d` subpackage, never the umbrella.
- `page_links` + `page_tags` SQLite tables: derived link/backlink adjacency and tag membership cache — rebuildable from files, never source of truth.
- `okf.SetTags(d *Doc, tags []string)`: surgical `yaml.SequenceNode` setter — the single new byte-stable primitive gating all tag writes.
- `agent.SuggestTags` mode: single-shot `ChatModel.Generate` with vocabulary-aware prompt + `validateTags` harness — no new Eino dependency, no new agent tool.

### Expected Features

**Must have (table stakes — v1.0):**
- Global graph: pages as nodes, page-link edges, force-directed layout that settles, click-to-navigate, zoom/pan, node size by degree
- Local (per-page) graph panel: active page + direct neighbors, depth slider (default 1), auto-updates on page switch
- Backlinks index + "Referenced by" panel in page view
- Edge-type toggles: page-links / backlinks / shared-tags (shared-tags OFF by default, thresholded to prevent hairballs)
- Hover-highlight neighbors; orphan nodes visible and distinguishable
- Per-page LLM tag suggestion on demand: suggest→approve per tag, biased to existing vocabulary, capped count (max ~5)
- Approved tags merged into YAML `tags` frontmatter: byte-stable, deduped, committed via single-writer flow

**Should have (differentiators — v1.x after validation):**
- Bulk untagged-pages sweep with batch review queue (P2)
- Tag/group coloring in graph by tag or folder (P2)
- In-graph search/filter (P2)
- Local graph "neighbor links" toggle (P2)
- Backlink context lines in the backlinks panel (P3)

**Defer to v2+:**
- Embedding-based tag near-synonym merge (vector store breaks single-binary constraint)
- Nested/hierarchical tags
- Attachment tagging (undefined frontmatter storage model)
- Saved graph filters / named color-group sets

### Architecture Approach

This milestone is a pure integration exercise over five existing v0.9.9 seams: (1) `okf.FindLinks` + `search.readTags` for the derived graph store; (2) the `jobs.Worker` fire-and-forget registration pattern; (3) `pages.Service` mutation hooks; (4) the `agent.ProposePatch`→`/apply-patch`→`pages.Save` single-writer pattern; (5) the chi `authed` vs `editor` route groups. The agent's 5-tool read-only allow-list and `tools_test.go` set-equality gate are explicitly NOT modified.

**Major components (new/modified):**
1. `internal/graph` (new): `page_links` + `page_tags` tables, `KindGraph` job, startup-drift rebuild — foundation.
2. `internal/server/handlers_graph.go` (new): `GET /api/v1/graph` + `GET /api/v1/graph/local` in the authed group.
3. `internal/okf` (extended): `okf.SetTags(d, tags)` surgical setter.
4. `internal/agent` (extended): `SuggestTags` mode + `validateTags`; `KindTagSuggest` bulk job + `tag_suggestions` staging table.
5. `internal/server/handlers_agent.go` (extended): suggest-tags / apply-tags / bulk-tag-sweep / tag-suggestions endpoints.
6. `web/src/` (new views): ForceGraph2D global + local panel, backlinks panel, tag-suggestion approval UI, bulk sweep review queue.

### Critical Pitfalls

1. **Frontmatter round-trip broken on tag write** — Use surgical `okf.SetTags` yaml.Node editor only. Gate tag-write phase on a golden-file test: add one tag → only `tags` lines change in the diff. Never re-marshal the whole frontmatter struct.

2. **Link/backlink index stale on rename/move/delete/restore** — Hook `enqueueGraphUpsert`/`enqueueGraphDelete` into every `pages.Service` mutation. On rename/move, re-scan all affected linker pages. Provide "reindex graph" admin backstop. Verify with integration tests across all mutation types.

3. **Bulk sweep auto-applies tags without human review** — Sweep produces `tag_suggestions(pending)` only; write fires only via human-approved `POST /agent/apply-tags`. This is a go/no-go criterion for the bulk-sweep phase.

4. **Tag explosion from unconstrained vocabulary** — Pass existing tag set into every prompt; normalize on write (lowercase, trim, dedupe); distinguish new-vs-existing in UI; new tags default-unchecked.

5. **K² shared-tag cliques blow up graph payload** — Use bipartite page↔tag node model or per-tag edge cap; shared-tag edges OFF by default; cache global graph payload server-side.

6. **`react-force-graph` umbrella drags three.js into the binary** — Import `react-force-graph-2d` only; verify `three` absent from lockfile; lazy-load graph route; add bundle-size CI check.

---

## Implications for Roadmap

### Phase 1: Derived Graph Store & Maintenance (Foundation)
**Rationale:** Everything depends on this. Graph UI has no data without `page_links`/`page_tags`. Tag suggestion needs `page_tags` for vocabulary prompts. Backlinks panel needs the reverse index.
**Delivers:** `page_links` + `page_tags` SQLite tables (rebuildable from files), `KindGraph` job on existing worker, `enqueueGraphUpsert`/`enqueueGraphDelete` wired into all `pages.Service` mutations, startup-drift rebuild, admin reindex endpoint.
**Avoids:** Pitfall 3 (stale index) — rename/move/trash/restore integration tests as explicit acceptance criteria.
**Research flag:** Standard patterns (mirrors `KindIndex`/`enqueueIndexUpsert`/`RebuildIndex` exactly) — skip research phase.

### Phase 2: Graph Endpoints & Backlinks Panel
**Rationale:** Exposes stored adjacency over HTTP; delivers first user-visible output (backlinks panel). Graph UI (Phase 3) depends on these endpoints existing.
**Delivers:** `GET /api/v1/graph` (global, typed edges), `GET /api/v1/graph/local` (neighborhood), backlinks "Referenced by" panel, shared-tag edge computation with popular-tag cap.
**Avoids:** Pitfall 8 (K² payload blow-up) — bipartite model or edge cap; server-side caching; lean JSON payload.
**Research flag:** Standard patterns. Shared-tag edge strategy (bipartite vs. threshold cap) requires a concrete product decision before planning.

### Phase 3: Graph UI (Global View, Local Panel, Edge Toggles)
**Rationale:** Headline visual feature; depends on Phases 1+2 for data. Can run in parallel with Phase 4.
**Delivers:** Global graph (Canvas, `react-force-graph-2d`, degree sizing, click-to-navigate, zoom/pan, orphans), local graph panel (docked, depth slider, auto-updates), edge-type toggles (zustand booleans + `useMemo`), hover-highlight.
**Uses:** `react-force-graph-2d@1.29.1` (2D only), React.lazy code-split, react-query for `/graph`, zustand for toggle state.
**Avoids:** Pitfall 2 (three.js bundle bloat); Pitfall 8 (main-thread jank — Canvas renderer, cooldown/freeze, incremental edge updates on toggle).
**Research flag:** Needs a short spike for force simulation tuning parameters and label-on-zoom threshold during planning.

### Phase 4: `okf.SetTags` + Per-Page Tag Suggestion (suggest→approve)
**Rationale:** LLM tagging chain starts here. `okf.SetTags` gates all tag writes and must be proven byte-stable before any write path ships. Per-page trust must be established before bulk sweep is activated.
**Delivers:** `okf.SetTags` with golden-file test, `agent.SuggestTags` with `validateTags`, `POST /agent/suggest-tags` + `POST /agent/apply-tags`, per-page approval UI (new-vs-existing distinction, new tags default-unchecked).
**Avoids:** Pitfall 1 (round-trip broken — golden-file test gates the phase); Pitfall 4 (tag explosion — existing vocabulary in prompt, write-time normalization); Pitfall 7 (hallucinated tags — cap + evidence + default-deny for new tags).
**Research flag:** Standard patterns (mirrors `ProposePatch`→`/apply-patch`→`pages.Save`). Prompt content and tag cap defaults require concrete product requirements.

### Phase 5: Bulk Sweep + Batch Review Queue
**Rationale:** High-value P2 feature; depends on Phase 4 (suggestion + apply primitives) and Phase 1 (`page_tags` for untagged-page enumeration). Only activatable once per-page tagging is trusted.
**Delivers:** `KindTagSuggest` job (serial drain = natural LLM rate-limiting), `tag_suggestions` staging table, `POST /agent/bulk-tag-sweep` (admin-gated; enqueues jobs, writes nothing), `GET /agent/tag-suggestions`, batch review UI, approvals route through Phase 4 apply, batched commits (not one-per-page).
**Avoids:** Pitfall 5 (sweep bypasses approval — go/no-go criterion); Pitfall 6 (single-writer contention — serial drain, batched commits, resumable on kill, 409 floor per page).
**Research flag:** Needs research-phase attention for batch review UX patterns and resumable job state machine design.

### Phase Ordering Rationale
- Phase 1 must precede all others (shared foundation for both chains).
- Phases 2+3 and Phase 4 can run in parallel after Phase 1.
- Phase 3 depends on Phase 2 (endpoints must exist before UI renders).
- Phase 5 depends on both Phase 4 and Phase 1.
- Per-page tagging (Phase 4) is intentionally separated from bulk sweep (Phase 5): trust at small scale before multiplying by page count.
- Local graph panel is the UX lead feature (fast, meaningful, small); global graph is secondary and hardened in the same phase.

### Research Flags

Needs research during planning:
- **Phase 5 (Bulk Sweep):** batch review UX patterns and resumable job state machine — limited prior art in the codebase.
- **Phase 3 (Graph UI):** force simulation tuning (charge, link distance, cooldown/warmup ticks) and label-on-zoom threshold — empirical; short spike advisable.

Standard patterns (skip research-phase):
- **Phase 1:** exact mirror of `KindIndex`/`enqueueIndexUpsert`/`RebuildIndex`.
- **Phase 2:** mirrors `/tree` + `/search` in the authed chi group.
- **Phase 4:** exact mirror of `ProposePatch`→`/apply-patch`→`pages.Save`.

---

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | `react-force-graph-2d` version verified via npm 2026-06-24. Zero new backend deps — all integration paths verified against actual v0.9.9 source. |
| Features | MEDIUM | Obsidian graph behavior from help docs + community (not source). Exact thresholds (tag cap, shared-tag threshold) are product decisions, not research outputs. |
| Architecture | HIGH | Grounded entirely in actual v0.9.9 source files. Every seam named with a real function/package. |
| Pitfalls | HIGH | System-specific reasoning from actual code constraints. Graph library pitfalls verified against library docs. |

**Overall confidence:** HIGH

### Gaps to Address

- **Shared-tag edge strategy** (bipartite page↔tag nodes vs. per-tag cap with threshold): product decision required before Phase 2 planning.
- **Tag count cap and confidence handling**: a concrete default (e.g., max 5 tags/page) must be pinned in requirements.
- **Graph data freshness policy**: confirm "fire-and-forget on mutation" is the stated behavior (implies near-real-time freshness).
- **Bulk sweep role gate**: admin-only vs. editor — must be decided before Phase 5 planning.
- **`okf.SetTags` canonical tag style**: block list vs. flow style when creating `tags` key on a page that has none — pin once, enforce consistently.

---

## Sources

### Primary (HIGH confidence)
- `internal/okf/{links.go,okf.go,emit.go,repair.go}` — byte-stable Doc model, `FindLinks`, surgical frontmatter editor
- `internal/pages/{service.go,rename.go}` — single-writer path, mutation hooks, 409 floor
- `internal/search/{indexjob.go,rebuild.go,service.go}` — `KindIndex` pattern, `readTags`
- `internal/jobs/{queue.go,worker.go}` — single-drain worker, retry/backoff
- `internal/agent/{agent.go,tools.go,propose.go}` + `handlers_agent.go` — read-only boundary, propose→apply pattern
- `internal/server/router.go` — chi authed vs editor groups
- `npm view react-force-graph-2d` (2026-06-24) → v1.29.1, `peerDependencies: { react: "*" }`, no three.js
- PROJECT.md / CLAUDE.md locked constraints

### Secondary (MEDIUM confidence)
- Obsidian Help — Graph View, Local Graph: node size, depth slider, color groups, orphan toggles
- Obsidian Forum — local graph depth/hops, AI tag suggestion patterns, vocabulary reuse
- AI Note Tagger / Auto Tag Obsidian plugins — suggest→approve flows, existing-vocab prompting
- LLM4Tag (arXiv 2502.13481) — controlled vocabulary, constrained tag generation, near-synonym merge
- The Best Libraries for Large Force-Directed Graphs (Medium) — canvas vs SVG vs WebGL tradeoffs

---
*Research completed: 2026-06-24*
*Ready for roadmap: yes*
