# Feature Research

**Domain:** Self-hosted Markdown wiki — Knowledge Graph visualization + LLM auto-tagging (v1.0 milestone)
**Researched:** 2026-06-24
**Confidence:** MEDIUM (Obsidian graph behavior + AI-tagging patterns well-documented in product/help docs and plugin ecosystem; exact thresholds/limits are product choices, not standards)

> Scope: ONLY the two NEW features (Obsidian-style knowledge graph and LLM auto-tagging) plus the supporting **backlinks** capability the graph requires. Existing capabilities (page CRUD, byte-stable round-trip, internal `.md` link tracking + rewrite-on-rename, YAML `tags` frontmatter, Bleve search, the Eino propose→approve→apply→commit agent, Git versioning, soft locks, CM6 editor) are treated as **dependencies**, not re-researched.

---

## Feature Landscape

### Table Stakes (Users Expect These)

The team are ex-Obsidian users (per project memory: "make the UI mimic Obsidian"). For them, "knowledge graph" *means* the Obsidian graph; anything materially less will read as broken.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| **Global graph: pages as nodes, `.md` page-links as edges** | The baseline meaning of "knowledge graph" in a wiki | MEDIUM | Edge set already exists — reuse the internal page-link tracker. Render with a force-directed layout. |
| **Force-directed layout that settles** | Obsidian graphs animate then stabilize; a static or jittering layout feels wrong | MEDIUM | Use a library's built-in simulation (e.g. d3-force / a force-graph wrapper / Sigma+graphology). At ~5 users / hundreds of pages, perf is a non-issue. |
| **Click node → open that page** | Primary point of a graph in a wiki: navigation | LOW | Wire node-click to the existing router (`/app/page/:path`). |
| **Zoom + pan** | Any non-trivial graph needs it; Obsidian has it | LOW | Provided by every mainstream graph lib out of the box. |
| **Node sizing by link count (degree)** | Obsidian sizes nodes by connection count; hubs should look like hubs | LOW | Degree = in+out links; precompute from the link index. Optional min/max clamp. |
| **Local (per-page) graph panel** | Obsidian's local graph is core; shows "what connects to THIS page" | MEDIUM | Active page + its direct neighbors. Dock in a side panel that **auto-updates when the active page changes**. |
| **Local graph depth/hops control** | Obsidian's depth slider (depth 1 = direct neighbors, +1 each hop) | MEDIUM | BFS to depth N from the active page over the link graph. Default depth 1. |
| **Backlinks tracking + page-view backlinks panel** | Obsidian shows "Linked mentions"; a wiki without backlinks feels one-directional | MEDIUM | **Reverse index** of the existing forward-link tracker. Feeds BOTH the graph (backlink edges) and a page-side "Referenced by" list. |
| **Hover node → highlight its neighbors** | Obsidian dims the rest on hover/focus to show a node's connections | LOW–MEDIUM | Highlight adjacent nodes/edges, fade the rest. Pure client-side. |
| **Empty/disconnected handling (orphans visible but distinguishable)** | A page with no links must still appear; users hunt orphans to connect them | LOW | Render orphans (optionally as a toggle); don't silently drop them. |
| **LLM tag suggestion for the current page, on demand** | The headline v1.0 feature; "suggest tags for this page" button | MEDIUM | Send page title+body to the Eino agent; return a ranked tag list. |
| **Suggest → approve, never silent write** | Mandated by the project's agent safety model (no direct agent writes) | MEDIUM | Suggestions are a proposal; user accepts/rejects per tag before anything touches frontmatter. Mirrors the existing patch-approve flow. |
| **Approved tags written to YAML frontmatter `tags`, byte-stable** | Tags must land in the existing `tags` field and survive round-trip | MEDIUM | Reuse the frontmatter writer that preserves unknown fields + byte-stability. Append/merge into `tags`, dedupe. |
| **Reuse existing vocabulary over inventing new tags** | Without this, every sweep invents near-duplicates → tag soup | MEDIUM | Feed the workspace's existing tag set into the prompt; instruct the model to prefer existing tags. This is the single biggest quality lever. |

### Differentiators (Competitive Advantage)

Where OKF Workspace can feel as good as Obsidian (or better, by tying graph + tags + agent together).

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **User-toggleable edge types: page-links / backlinks / shared-tags** | Explicitly in the milestone scope; lets users see *different* relationship structures | MEDIUM | Three independent edge layers over the same node set. Shared-tag edges = pages sharing ≥1 tag (can explode — see anti-features; gate behind toggle, off by default or thresholded). |
| **Tag/group coloring in the graph** | Obsidian "color groups" — color nodes by tag/folder/query; turns the graph into a map | MEDIUM | Color by folder or by tag membership. Ties the new auto-tagging feature directly into graph legibility (better tags → better-colored graph). |
| **In-graph search/filter** | Type to filter which nodes show; declutters large graphs | LOW–MEDIUM | Reuse Bleve or simple client-side title/tag filter to dim/hide non-matching nodes. |
| **Shared-tag edges that visualize the auto-tagging output** | Closes the loop: auto-tag pages → shared-tag edges reveal new clusters | MEDIUM | The synergy story of this milestone. Only as good as the tag hygiene (controlled vocabulary matters). |
| **Bulk "tag untagged pages" sweep with batch review** | Onboard an existing wiki's backlog of untagged pages in one pass | HIGH | Iterate untagged (or all) pages → propose tags per page → **batch review UI** (approve/reject per page or per tag). Long-running → reuse the existing jobs subsystem + audit log. |
| **Per-page tag-count and confidence limits** | Keeps suggestions tight (e.g. max 3–5 tags, drop low-confidence) | LOW | Prompt + post-filter. Prevents the model from spraying 12 tags on one note. |
| **Local graph "neighbor links" toggle** | Obsidian shows interlinks *among* the displayed neighbors, revealing hidden structure | MEDIUM | After BFS, also draw edges between non-active nodes that link each other. |
| **Backlinks panel with link context (the sentence around the link)** | Obsidian shows the surrounding line; far more useful than a bare list | MEDIUM | Requires capturing link position/line during indexing. Nice-to-have on top of the basic list. |

### Anti-Features (Commonly Requested, Often Problematic)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **Silent auto-apply of LLM tags (no review)** | "Just tag everything for me" | Violates the project's locked agent-safety model (no direct writes); mis-tags pollute frontmatter + Git history with no human gate | Always suggest→approve. Batch review for bulk, but still a human gate before write/commit. |
| **Unconstrained new-tag invention every run** | Feels "smart"/comprehensive | Tag explosion + near-synonyms (`api` / `apis` / `rest-api`) make tags useless for filtering and pollute shared-tag graph edges | Bias hard toward existing vocabulary; map free-form output to nearest existing tag; cap tag count; normalize case/separators. |
| **Shared-tag edges drawn for EVERY shared tag, by default** | "Show me everything connected" | A common tag (`meeting`, `runbook`) creates a near-complete subgraph (O(n²) edges) → hairball, slow, unreadable | Edge type OFF by default; only connect on rarer tags, or require ≥2 shared tags, or cap edges per tag. Make it a deliberate toggle. |
| **3D / WebGL graph, GPU effects, animations** | Looks impressive | Overkill for ~5 users / hundreds of pages; adds deps, perf tuning, and a11y problems for zero real value | 2D force-directed in SVG/Canvas is plenty. Match Obsidian's clean 2D default. |
| **Real-time graph that re-simulates on every keystroke** | "Live" feel | Expensive, distracting; the graph is a navigation/overview tool, not a live editor surface | Recompute on page save / link change / explicit refresh. Cache the layout. |
| **Tags as full ontology (hierarchy, relations, weights) in MVP** | Knowledge-management ambition | Massive scope; YAML `tags` is a flat list; hierarchy needs new data model + UI | Keep flat `tags`. Optional later: nested tags via `parent/child` string convention (Obsidian-style) — defer. |
| **Embedding/vector store for tag similarity in MVP** | "Proper" near-synonym dedup | Adds a vector index + model + storage; breaks "single binary, SQLite for metadata only" simplicity | String normalization + existing-vocabulary prompting + simple edit-distance/case-fold dedup. Defer embeddings unless tag soup proves unsolvable. |
| **Auto-tagging attachments / non-Markdown files in MVP** | "Tag my PDFs too" | Attachments don't carry frontmatter; tag storage location undefined | Scope auto-tagging to pages (frontmatter `tags`) only. |

---

## Feature Dependencies

```
[Global graph view]
    └──requires──> [Page-link index]            (EXISTS: internal .md link tracking)
    └──requires──> [Graph render lib + layout]   (NEW)

[Local graph panel]
    └──requires──> [Page-link index]            (EXISTS)
    └──requires──> [Global graph render lib]     (NEW, shared)
    └──requires──> [Active-page context/router]  (EXISTS)

[Backlinks panel] ──and── [backlink edge type]
    └──requires──> [Reverse-link index]          (NEW: invert existing forward-link tracker)

[Shared-tag edge type] ──and── [tag/group coloring]
    └──requires──> [Frontmatter tags]            (EXISTS)
    └──enhanced-by──> [LLM auto-tagging]          (better tags → better edges/colors)

[LLM tag suggestion (per-page)]
    └──requires──> [Eino agent + provider config] (EXISTS)
    └──requires──> [Frontmatter tags read/write, byte-stable] (EXISTS writer; NEW merge logic)
    └──requires──> [Existing-vocabulary collection] (NEW: gather all tags across pages)
    └──requires──> [Suggest→approve UI]           (NEW; mirrors EXISTING patch-approve pattern)

[Bulk tagging sweep]
    └──requires──> [LLM tag suggestion (per-page)] (NEW, above)
    └──requires──> [Jobs subsystem]               (EXISTS)
    └──requires──> [Batch review UI]              (NEW)
    └──requires──> [Audit log]                    (EXISTS)

[Shared-tag edges] ──conflicts (perf)──> [naive O(n²) edge generation on common tags]
```

### Dependency Notes

- **Graph (both global + local) requires the existing page-link index:** the edge set for the "page links" layer is already maintained by the existing link tracker / rename-rewrite system — the graph is a *new view* of existing data, not new data capture.
- **Backlinks require a reverse index:** the forward-link tracker maps page → outgoing links; the graph's backlink edges and the page-view backlinks panel both need the inverse (page → incoming links). This is the one genuinely new index. Building it well unblocks both the panel and the backlink edge toggle.
- **LLM auto-tagging leans on the EXISTING agent + approval pattern:** reuse the Eino read-only tool boundary and the propose→review→approve→apply→commit flow. Tag suggestion is "another kind of proposal"; do not invent a second write path.
- **Auto-tagging requires gathering existing vocabulary:** to bias the model toward reuse, the backend must collect the union of all `tags` across pages and pass it (or a relevant subset) into the prompt. Without this, tag explosion is near-certain.
- **Shared-tag edges + auto-tagging are mutually reinforcing but perf-coupled:** good tags make shared-tag edges meaningful; but naive shared-tag edge generation is O(n²) on hub tags. Toggle off by default and threshold.

---

## MVP Definition

### Launch With (v1.0)

Minimum to satisfy the milestone (ex-Obsidian team gets a credible graph + low-effort tagging).

- [ ] **Reverse-link (backlinks) index** — foundational; unblocks panel + backlink edges
- [ ] **Backlinks panel in page view** ("Referenced by") — table-stakes, cheap once index exists
- [ ] **Global graph view** — nodes = pages, edges = page links, force-directed, settles, zoom/pan, node size by degree, click-to-open
- [ ] **Edge-type toggles** — page-links / backlinks / shared-tags (shared-tags OFF by default, thresholded)
- [ ] **Local graph panel** — active-page neighborhood, depth slider (default 1), auto-updates on page switch
- [ ] **Hover-highlight neighbors** + **orphans visible** — graph legibility table-stakes
- [ ] **Per-page LLM tag suggestion (on demand)** — suggest→approve per tag, biased to existing vocabulary, capped count
- [ ] **Approved tags merged into YAML `tags`** — byte-stable, deduped, committed via existing flow

### Add After Validation (v1.x)

- [ ] **Bulk untagged sweep** — trigger once per-page suggestion + approval are trusted; reuses jobs + batch review UI
- [ ] **Tag/group coloring** in graph — add once tags are clean enough to be worth coloring by
- [ ] **In-graph search/filter** — add when graphs grow enough to feel cluttered
- [ ] **Local graph "neighbor links" toggle** — refinement after the base local graph lands
- [ ] **Backlink context lines** in the backlinks panel — once link positions are captured

### Future Consideration (v2+)

- [ ] **Embedding-based tag near-synonym merge** — defer until string-normalization proves insufficient (adds vector store, breaks single-binary simplicity)
- [ ] **Nested/hierarchical tags** — defer; needs data-model + UI work, flat tags suffice for 5 users
- [ ] **Saved graph filters / multiple named color-group sets** — power-user nicety
- [ ] **Attachment tagging** — undefined storage model; defer

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Reverse-link (backlinks) index | HIGH | MEDIUM | P1 |
| Backlinks panel (page view) | HIGH | LOW | P1 |
| Global graph (nodes/edges/layout/click/zoom) | HIGH | MEDIUM | P1 |
| Node size by degree | MEDIUM | LOW | P1 |
| Edge-type toggles (links/backlinks/shared-tags) | HIGH | MEDIUM | P1 |
| Local graph panel + depth | HIGH | MEDIUM | P1 |
| Hover-highlight neighbors | MEDIUM | LOW | P1 |
| Orphans visible/toggle | MEDIUM | LOW | P1 |
| Per-page LLM tag suggestion (suggest→approve) | HIGH | MEDIUM | P1 |
| Reuse existing vocabulary (anti tag-explosion) | HIGH | MEDIUM | P1 |
| Merge tags into frontmatter (byte-stable) | HIGH | MEDIUM | P1 |
| Tag/group coloring in graph | MEDIUM | MEDIUM | P2 |
| In-graph search/filter | MEDIUM | LOW | P2 |
| Bulk untagged sweep + batch review | HIGH | HIGH | P2 |
| Per-page tag-count/confidence limits | MEDIUM | LOW | P2 |
| Local "neighbor links" toggle | MEDIUM | MEDIUM | P2 |
| Backlink context lines | MEDIUM | MEDIUM | P3 |
| Embedding-based synonym merge | LOW (at 5 users) | HIGH | P3 |
| Nested/hierarchical tags | LOW | HIGH | P3 |

**Priority key:** P1 = must have for the v1.0 milestone · P2 = should have, add when possible · P3 = future.

## Competitor Feature Analysis

| Feature | Obsidian (core) | AI-tagging plugins (e.g. AI Note Tagger / Auto Tag / Saner.AI) | Our Approach |
|---------|-----------------|---------------------------------------------------------------|--------------|
| Global graph | Force-directed, node size by links, color groups by query, tag/folder/link-type filters, search filter, orphan + attachment toggles | n/a | Match the useful 80%: force layout, degree sizing, edge-type toggles, hover highlight, orphans, click-to-open; defer color-by-query to P2 |
| Local graph | Depth slider, neighbor-links toggle, dockable auto-updating pane | n/a | Match: depth slider, auto-update, neighbor-links as P2 |
| Edge types | Page links + (via plugins) tag links | n/a | Page-links / backlinks / shared-tags toggles (shared-tags thresholded, off by default) |
| AI tag suggestion | n/a (core) | Scan content → suggest; many can preview/accept/ignore per tag; option to feed existing tags to bias reuse | Suggest→approve per tag, existing-vocabulary biased, capped count — routed through the existing agent approval flow |
| Bulk tagging | n/a | Some plugins batch over notes | Bulk untagged sweep via jobs + batch review (P2) |
| Apply to frontmatter | n/a | Insert into YAML frontmatter | Byte-stable merge into existing `tags`, deduped, committed via Git flow |
| Silent vs approved | n/a | Some default to instant tagging | **Never silent** — locked agent-safety model requires human approval |

## Open Questions / Notes for Requirements

- **Shared-tag edge thresholds** are a product decision, not a standard: pick a default (e.g. require ≥2 shared tags OR exclude tags appearing on >X% of pages) to avoid hairballs. Needs a requirement.
- **Tag-count cap and confidence handling** are product choices: a concrete default (e.g. max 5 tags/page, drop suggestions below a confidence/relevance bar) should be a testable requirement.
- **Vocabulary subset for prompting:** with hundreds of pages the full tag set may be large; "pass all tags" is fine at this scale, but the requirement should state the bound.
- **Graph data freshness:** define when the graph recomputes (on save / on link change / on explicit refresh) — do NOT re-simulate per keystroke.

## Sources

- [Graph view — Obsidian Help](https://obsidian.md/help/plugins/graph) — node size, filters (tag/folder/link type, search, orphans, attachments, existing-files-only), color groups by query, repel/link force — MEDIUM
- [Graph View | obsidianmd/obsidian-help — DeepWiki](https://deepwiki.com/obsidianmd/obsidian-help/4.5-graph-view) — settings breakdown — MEDIUM
- [5 features of Obsidian Graph View and how I use them](https://www.sivwuk.com/5-features-of-obsidian-graph-view-and-how-i-use-them/) — focus/highlight, search to declutter — MEDIUM
- [The Power of Obsidian's Local Graph — The Sweet Setup](https://thesweetsetup.com/the-power-of-obsidians-local-graph/) — depth, neighbor links, dockable auto-updating pane — MEDIUM
- [Extracting more understanding from the graph view: Local landscapes — Obsidian Forum](https://forum.obsidian.md/t/extracting-more-understanding-from-the-graph-view-local-landscapes/2073) — local graph depth/hops — MEDIUM
- [AI Note Tagger — Obsidian Stats](https://www.obsidianstats.com/plugins/ai-note-tagger) and [Auto Tag — Obsidian Stats](https://www.obsidianstats.com/plugins/auto-tag) — suggest tags, feed existing vocabulary, preview/accept per tag, write to frontmatter — MEDIUM
- [Using AI to create/suggest tags — Obsidian Forum](https://forum.obsidian.md/t/using-ai-to-create-suggest-tags/83853) — reuse vs invent, preview-and-approve — MEDIUM
- [Contextual Tagging with LLMs for KM](https://stackrundown.com/contextual-tagging-large-language-models/) and [LLM4Tag (arXiv 2502.13481)](https://arxiv.org/html/2502.13481v2) — controlled vocabulary, constrained assignment, embedding nearest-neighbor mapping, near-synonym merge, hierarchy — MEDIUM
- [Cytoscape.js vs vis-network vs Sigma.js 2026 — PkgPulse](https://www.pkgpulse.com/guides/cytoscape-vs-vis-network-vs-sigma-graph-visualization-2026) and [Best libraries for large force-directed graphs — Medium](https://weber-stephen.medium.com/the-best-libraries-and-methods-to-render-large-network-graphs-on-the-web-d122ece2f4dc) — graph render lib tradeoffs, React ref-lifecycle integration — MEDIUM

---
*Feature research for: Knowledge Graph + LLM auto-tagging on a self-hosted Markdown wiki*
*Researched: 2026-06-24*
