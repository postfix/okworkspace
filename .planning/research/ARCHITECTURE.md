# Architecture Research

**Domain:** Self-hosted Markdown wiki (Go + React) — v1.0 milestone: Knowledge Graph + LLM Auto-Tagging
**Researched:** 2026-06-24
**Confidence:** HIGH (grounded in the actual v0.9.9 codebase, not assumed)

> This is an INTEGRATION study for a subsequent milestone. The existing architecture is fixed; the question is *where the two new features attach*. Every recommendation below names a concrete existing seam (package, function, job kind, endpoint group) that the new code reuses or mirrors, so the roadmapper can derive phases with explicit new-vs-modified boundaries and a dependency-aware build order.

---

## Existing Architecture (the surface we integrate WITH)

```
┌──────────────────────────────────────────────────────────────────────┐
│  React 19 SPA (go:embed)  — react-query (server state) · zustand (UI) │
│  routes: /app/page/:path · ⌘K search · agent panel · diff dialog      │
└───────────────┬──────────────────────────────────────────────────────┘
                │  /api/v1/*  (chi: authed group = reads, editor group = CSRF writes)
┌───────────────▼──────────────────────────────────────────────────────┐
│  internal/server  (HTTP handlers, role gating, nosurf CSRF)           │
├───────────────────────────────────────────────────────────────────────┤
│  internal/pages   SINGLE-WRITER mutation service                      │
│    Save/Create/Rename → okf.Parse/Repair/Emit (byte-stable)           │
│    → EnqueueCommit (jobs) ─┐         → enqueueIndexUpsert (fire&forget)│
│  internal/agent   Eino ReAct + single-shot modes; READ-ONLY 5 tools;  │
│    ProposePatch (no write) → /apply-patch endpoint → pages.Save        │
│  internal/okf     byte-stable Doc model: FindLinks · Field/SetField · │
│                   Repair · Emit  (frontmatter = surgical yaml.Node)    │
│  internal/search  Bleve index; readTags; KindIndex jobs; RebuildIndex │
├──────────────────────────┬────────────────────────────────────────────┤
│  internal/jobs  SINGLE background goroutine, SQLite FIFO queue         │
│    Register(kind, handler) · Enqueue (fire&forget) · EnqueueAndWait    │
├──────────────────────────┼────────────────────────────────────────────┤
│  internal/gitstore  single-writer git CLI commits  │  internal/repo    │
│  Markdown files on disk (truth)  │  SQLite app.db (operational only)   │
└──────────────────────────────────────────────────────────────────────┘
```

**Five load-bearing invariants the new features MUST NOT break:**

1. **Files are truth; SQLite is operational-only.** A links/backlinks/tag-edge table is a *derived cache* — it must be fully rebuildable from the `.md` files, exactly like the Bleve index. Never the source of truth.
2. **Byte-stable frontmatter.** `okf.Doc` re-emits `RawFront` verbatim unless `FrontDirty`. Tag writes must go through a surgical `yaml.Node` edit (à la `okf.SetField`), never a re-marshal of the whole file. The golden-corpus round-trip test is the gate.
3. **Single-writer commit path.** Every content write flows `pages.Save → EnqueueCommit → gitstore.Commit`. There is no second writer. Tag application reuses this exact path.
4. **Agent never writes.** `internal/agent` has a 5-tool read-only allow-list enforced by a set-equality build test (`tools_test.go`). Tagging is a *suggestion* mode; the write happens at a separate CSRF+editor-gated HTTP endpoint, mirroring `/agent/propose-patch` → `/agent/apply-patch`.
5. **Fire-and-forget derived-index maintenance.** `pages.Save` calls `enqueueIndexUpsert` (worker.Enqueue, never EnqueueAndWait — CR-01 deadlock lesson) after the commit. The graph index follows the identical pattern.

---

## Feature 1 — Knowledge Graph

### Q1: Where is the page-link graph computed and stored?

**Recommendation: maintain a derived adjacency in SQLite (`page_links` table), updated by the SAME fire-and-forget job mechanism that updates Bleve — NOT derived on-demand per request.**

Rationale (decisive, not wishy-washy):

- **The structural scanner already exists.** `okf.FindLinks(body)` is a byte scanner that skips code fences/inline code and returns resolved link destinations. The rename/move flow (`pages.rewriteFolderInboundLinks`) already walks every `.md` once via `filepath.WalkDir` + `repo.Read` + `okf.Parse` and resolves links to repo-relative targets via `path.Clean(path.Join(fromDir, dest))`. That resolution logic is the *exact* forward-edge extractor a graph needs — reuse it, do not reinvent.
- **Derive-on-demand for the GLOBAL graph does not scale even at this size with acceptable latency margins.** A global graph render would re-walk + re-parse every page on every request. At ~5 users / a few hundred pages it would *work*, but it duplicates the rebuild cost on a hot path and gives no incremental freshness for the local-graph panel. A small adjacency table is O(edges) to read and trivially incremental.
- **Backlinks are the reverse index of forward links** — they fall out of the same table for free (`SELECT src FROM page_links WHERE dst = ?`). A page-view backlinks panel and the local-graph neighborhood both read it.

**Proposed `page_links` schema (operational metadata, fully rebuildable):**

```
page_links(src_path TEXT, dst_path TEXT, PRIMARY KEY(src_path, dst_path))
  -- one row per resolved internal link edge; src links to dst.
  -- index on dst_path for O(backlinks) reverse lookup.
```

Only **internal, resolvable** links become edges: skip `isAbsoluteOrExternal` destinations (the same predicate `okf.RewriteLinks` already uses), resolve the rest with `path.Clean(path.Join(dir(src), dest))`, and keep an edge only if the target `.md` exists in the repo (a dangling link is not a graph edge — optionally surface "unresolved links" separately later, out of v1.0 scope).

**Freshness — reuse the KindIndex pattern exactly.** Register a new job kind on the *existing* `jobs.Worker`:

- New `const KindGraph = "graph"` with op `upsert|delete|rebuild` (mirror `search.indexPayload`).
- `pages.Service` already has `enqueueIndexUpsert`/`enqueueIndexDelete` called after every mutation. Add a sibling `enqueueGraphUpsert`/`enqueueGraphDelete` right beside them (same fire-and-forget, same Warn-and-swallow on enqueue failure because the rebuild backstop reconciles).
- The graph handler's `upsert(path)` = re-scan that one page's outbound links and replace its `src_path` rows (delete-then-insert the page's edge set). `delete(path)` = delete rows where `src=path OR dst=path`. `rebuild` = walk the whole repo (clone `RebuildIndex`'s `repo.Tree()` loop) and rewrite the table — the startup-drift + admin-reindex backstop.

**Create/rename/move/delete freshness specifics (the question's emphasis):**

| Mutation | Existing hook | Graph action |
|----------|---------------|--------------|
| Create | `Create → enqueueIndexUpsert` | + `enqueueGraphUpsert(newPath)` (a new page may already contain links) |
| Edit/Save | `Save → enqueueIndexUpsert` | + `enqueueGraphUpsert(path)` (links added/removed in body) |
| Rename/Move | `relocate` rewrites inbound links across many files, then upserts each | + `enqueueGraphUpsert` for the moved page AND every file whose body was rewritten (the `rewriteFolderInboundLinks` result set already names them) **and** a graph `delete(oldPath)` so the stale `src=oldPath` rows go. Because rename rewrites *destinations* in linkers, those linkers' edges change → they must be re-scanned, which the existing per-file upsert set already covers. |
| Delete-to-trash | `enqueueIndexDelete` | + `enqueueGraphDelete(path)` — removes the page's outbound edges AND inbound edges pointing at it (the now-dangling links remain in linkers' bodies but produce no edge until/unless that linker is re-saved; the rebuild backstop reconciles). |
| Restore | restore re-adds the file + upserts index | + `enqueueGraphUpsert(restoredPath)` |

This means **NO new write path** — graph maintenance rides entirely on the mutation hooks that already exist, and the single background goroutine keeps it strictly serialized (no concurrent-writer races on `page_links`).

### Q2: Graph-serving endpoints and payload sizing

Two read endpoints, both in the `authed` (any-authenticated) chi group beside `/tree` and `/search` — graph reads are not mutations:

| Endpoint | Scope | Shape | Payload control |
|----------|-------|-------|-----------------|
| `GET /api/v1/graph` | Global | `{nodes:[{path,title}], edges:[{src,dst,type}]}` | At a few hundred pages this is small (KB-scale). Title comes from a cheap `okf.Field(doc, FieldTitle)` or — better — a `pages` metadata cache so the endpoint doesn't re-parse every file. Cap node count defensively; if the workspace ever grows, paginate/cluster later (out of v1.0). |
| `GET /api/v1/graph/local?path=…&depth=1` | Neighborhood | Same shape, filtered to the page + its direct (depth-1) neighbors | depth-1 default keeps it tiny; allow `depth=2` as an opt-in. This is the local-graph side panel AND backs the page-view backlinks list (`edges where dst=path` = backlinks). |

**Edge `type` is computed at query time, not stored per-type-as-separate-tables.** The endpoint returns each edge tagged `link` (from `page_links`) or `tag` (from the shared-tag join, see Q3). The UI's Obsidian-style toggles (page links / backlinks / shared tags) then **filter client-side** — react-query caches the full edge set, zustand holds the toggle state. Backlinks are not a separate edge type in storage; they are forward `link` edges *rendered* as inbound when viewed from the target. Keeping payloads reasonable is mostly "don't re-parse files on the hot path" (read from the cache table) + depth-limit the local view.

### Q3: Shared-tag edges

**Source:** frontmatter `tags`, read with the **already-existing** `search.readTags(doc)` (sequence-aware — it handles `tags: [a,b]`, block lists, and a single scalar; `okf.Field` alone returns "" for sequences, Pitfall 7). Do not write a second tag reader.

**Computation:** shared-tag edges are derived, not stored as adjacency. Maintain a `page_tags(path, tag)` table populated by the **same KindGraph upsert job** (when a page is re-scanned, replace its tag rows alongside its link rows — one job, two tables). Then a shared-tag edge between A and B exists iff they share ≥1 tag:

```
-- pages sharing tag T are mutually connected via T
SELECT a.path AS src, b.path AS dst, a.tag AS via
FROM page_tags a JOIN page_tags b
  ON a.tag = b.tag AND a.path < b.path;
```

This is computed in the `/graph` endpoint (or a small materialized helper). **Caveat for the roadmapper:** a popular tag creates an O(n²) clique. Cap fan-out per tag (e.g. skip tag-cliques above a threshold, or only connect within the neighborhood for the local graph) — flag this for the graph-edges plan. `page_tags` doubles as a tag-autocomplete/source-of-truth for the auto-tagging UI (Feature 2), so build it once and both features consume it.

---

## Feature 2 — LLM Auto-Tagging

### Where suggestion lives (new Eino mode / endpoint)

**Recommendation: a new SINGLE-SHOT agent mode `SuggestTags`, mirroring `agent.Rewrite`/`agent.Draft` — NOT a ReAct tool, NOT a new tool in the read-only allow-list.**

- `internal/agent` already has the single-shot pattern: `generateOnce(ctx, msgs, maxTokens, temp)` with a hard `singleShotTimeout`, per-mode token caps, and a validate-and-retry harness (`proposeWithRetry`). Add `func (s *Service) SuggestTags(ctx, path) ([]string, error)`:
  - Fetch body via the role-scoped `deps.Pages.Get` (never `os.ReadFile`).
  - Single-shot Generate with a tags-extraction prompt; the page text is supplied as **untrusted DATA** (reuse `delimitUntrusted`).
  - **Validate the model output into a clean `[]string`** (lowercase, dedup, strip empties, cap count, reject prompt-injection-y junk) — the tagging analog of `validateProposedBody`. Retry on malformed output, never return garbage.
- The 5-tool read-only boundary is **untouched** — `tools_test.go` set-equality still passes because tagging adds no tool. (Adding a 6th tool would fail the build gate; we deliberately do not.)

New endpoints in the chi `editor` (CSRF + editor-gated) group, beside `/agent/propose-patch`:

| Endpoint | Purpose |
|----------|---------|
| `POST /api/v1/agent/suggest-tags` | Body `{path}` → returns `{suggested:[…], current:[…], base_revision}` — the suggest step. **Writes nothing.** Capture `base_revision` via `pages.Revision` at suggestion time, exactly like `ProposePatch`. |
| `POST /api/v1/agent/apply-tags` | Body `{path, tags:[…], base_revision}` → the approve→apply step. |

### How suggest→approve→apply reuses the EXISTING propose→apply→commit path (byte-stable)

This is the crux. The tag write must **NOT** introduce a new commit path and must keep frontmatter byte-stable.

**The clean reuse:** apply-tags assembles the new page source by **surgically editing only the `tags` key on the parsed `okf.Doc`**, then routes through `pages.Save` (the single-writer path) — identical in spirit to how `/apply-patch` calls `pages.Save`.

Concrete flow for `POST /agent/apply-tags`:

1. Re-validate the tag list server-side (defense in depth — same posture as `/apply-patch` re-running `ValidateProposedBody`).
2. `pages.Get(path)` → gives `Frontmatter` (raw region), `Body`, current `Revision`.
3. **New surgical setter in `okf`: `SetTags(d *Doc, tags []string)`** — mirrors `okf.SetField` but writes a `yaml.SequenceNode` for the `tags` key (SetField only does scalars). It sets `FrontDirty=true` so `Emit` re-marshals **only** the frontmatter `yaml.Node` (key order + unknown fields preserved) while the opaque `Body` is passed through **byte-for-byte untouched**. This is the ONE genuinely new okf primitive the milestone needs.
4. Call `pages.Save(path, body, newFrontmatter, base_revision, actor)` — the SAME method `/apply-patch` uses. It enforces the 409 stale-revision floor, re-`Repair`s, `Emit`s byte-stably, and `EnqueueCommit`s through the single-writer worker. The fire-and-forget `enqueueIndexUpsert` (and the new `enqueueGraphUpsert`) fire automatically, so search facets AND shared-tag graph edges refresh with no extra wiring.

**Why this preserves every invariant:** the body never goes through the model or an AST (only the body is read for suggestion, never rewritten); frontmatter is edited as a `yaml.Node`, not regenerated; the write is the single-writer git commit; the agent itself still never writes (apply is the gated HTTP endpoint); and the 409 floor blocks a stale tag-apply if the page changed since suggestion.

> **Decision point for the roadmapper:** `pages.Save` currently takes `frontmatter string` (the raw region) + `body string` and re-assembles. apply-tags can either (a) build the new raw frontmatter string itself via `okf.SetTags`+`Emit`-style and pass it through `Save` as today, or (b) add a thin `pages.SaveTags(path, tags, baseRev, user)` that does the parse→SetTags→Emit internally and reuses `enqueueWrite`. Option (b) is cleaner and keeps the frontmatter manipulation inside `pages`/`okf` where the byte-stability tests live. Recommend (b).

### How the bulk sweep reuses the async jobs worker

**Recommendation: the bulk sweep is a fan-out of per-page jobs on the EXISTING worker, with suggestions staged for batch review — NOT a single long-running request and NOT silent auto-apply.**

- New job kind `KindTagSuggest` on `jobs.Worker`. A `POST /agent/bulk-tag-sweep` (editor-gated) enumerates target pages (untagged, or all — read `page_tags` to find untagged) and enqueues one `KindTagSuggest{path}` job per page, fire-and-forget. The single drain goroutine processes them serially (respecting LLM rate limits naturally — no concurrent provider hammering), with the worker's built-in backoff/retry/MaxAttempts handling a flaky provider.
- Each `KindTagSuggest` job calls `agent.SuggestTags(path)` and **persists the suggestion to a `tag_suggestions(path, suggested_json, base_revision, status='pending')` table** — it does NOT apply. This is the "no silent frontmatter writes" requirement: the sweep produces a review queue.
- A `GET /agent/tag-suggestions` (authed) lists pending suggestions; the UI shows a batch approve/reject surface. Approving one calls the same `apply-tags` path (per-page `pages.Save`, single-writer, 409-checked). Because each apply re-checks `base_revision`, a page edited between sweep and approval is safely skipped/flagged, never clobbered.

This reuses: the worker (serialization + retry/backoff), the suggestion mode, the apply endpoint, and the commit path — the sweep adds only a job kind + a staging table + a list/approve UI.

---

## New vs Modified Components (explicit for the roadmapper)

### New components

| Component | Location | What it is |
|-----------|----------|------------|
| `page_links` table + maintenance | `internal/graph` (new) or extend `internal/search` | Derived link adjacency (rebuildable) |
| `page_tags` table + maintenance | same | Derived tag membership (rebuildable); feeds shared-tag edges + untagged-page query |
| `KindGraph` job + handler | new package / jobs registration | upsert/delete/rebuild of `page_links`+`page_tags` for one page or whole repo |
| `GET /graph`, `GET /graph/local` | `internal/server/handlers_graph.go` (new) | Global + neighborhood JSON (authed group) |
| `okf.SetTags(d, tags)` | `internal/okf/repair.go` or new file | Surgical `yaml.SequenceNode` frontmatter setter (the one new byte-stable primitive) |
| `agent.SuggestTags` + `validateTags` | `internal/agent` | Single-shot tag suggestion mode + output validator (mirrors Rewrite/Draft + validateProposedBody) |
| `KindTagSuggest` job + `tag_suggestions` table | jobs + new table | Bulk sweep fan-out → staged review queue |
| `POST /agent/suggest-tags`, `/agent/apply-tags`, `/agent/bulk-tag-sweep`, `GET /agent/tag-suggestions` | `internal/server/handlers_agent.go` | Suggest/apply/sweep/list endpoints (editor-gated except the GET) |
| Graph view (global) + local panel + backlinks panel + tag-suggestion review UI | `web/src/` | React: react-query for `/graph`, zustand for edge-type toggles; a force-graph lib for rendering |

### Modified components

| Component | Modification | Risk |
|-----------|-------------|------|
| `pages.Service` mutation methods | Add `enqueueGraphUpsert`/`enqueueGraphDelete` calls beside the existing `enqueueIndexUpsert`/`Delete` in Create/Save/relocate/trash/restore | Low — additive, same fire-and-forget pattern |
| `pages.Service` (optional `SaveTags`) | Thin tag-write method reusing `enqueueWrite` | Low — wraps existing path |
| `jobs` wiring (cmd/server startup) | `Register(KindGraph, …)`, `Register(KindTagSuggest, …)`; enqueue graph rebuild on startup-drift (mirror search rebuild) | Low |
| `internal/server/router.go` | Register the new routes in the correct chi group (authed vs editor) | Low |
| Startup drift recovery | Add graph-table rebuild alongside the existing search rebuild-on-drift | Low — same trigger |
| `agent.Service`/`Deps` | No new tool; reuse `Pages` reader. Possibly inject a tag-suggestion store | Low — boundary unchanged |

**The agent's 5-tool read-only allow-list and `tools_test.go` set-equality test are NOT modified** — a structural guarantee the roadmapper should call out as an explicit non-goal/constraint.

---

## Data-Flow Changes

**Page mutation (after):**
```
PUT /pages → pages.Save → 409 check → okf.Repair/Emit (byte-stable)
   → EnqueueCommit ──────────────► gitstore.Commit (single writer)
   → enqueueIndexUpsert ─fire&forget─► KindIndex  ─► Bleve
   → enqueueGraphUpsert ─fire&forget─► KindGraph  ─► page_links + page_tags   ★NEW
```

**Tag apply (new, reuses the write path):**
```
POST /agent/apply-tags → re-validate tags → pages.Get → okf.SetTags (surgical yaml.Node)
   → pages.Save(newFrontmatter, body, baseRev)  ─► [identical single-writer flow above]
```

**Bulk sweep (new, reuses the worker):**
```
POST /agent/bulk-tag-sweep → enumerate untagged (page_tags) → enqueue N× KindTagSuggest
   worker drains serially → agent.SuggestTags (single-shot, validated)
       → INSERT tag_suggestions(status=pending)         (NO write to files)
   UI reviews → approve → POST /agent/apply-tags → [tag apply flow above]
```

**Graph read:**
```
GET /graph → read page_links + page_tags (no file re-parse) → tag edges by type
   → JSON {nodes, edges[type]} → react-query cache → zustand toggles filter client-side
```

---

## Suggested Build Order (dependency-aware)

The dependencies dictate this ordering; each phase is independently shippable/verifiable.

1. **Derived graph store + maintenance (foundation).** `page_links` + `page_tags` tables; `KindGraph` job (upsert/delete/rebuild) reusing `okf.FindLinks` + the rename link-resolution logic + `search.readTags`; wire `enqueueGraph*` into every `pages` mutation; startup-drift rebuild. *Verify: tables stay correct across create/edit/rename/move/delete vs a from-scratch rebuild.* **Must precede the graph view and the shared-tag edges — nothing renders without fresh adjacency.**

2. **Backlinks + graph endpoints.** `GET /graph` and `GET /graph/local`; backlinks = reverse query on `page_links`. *Verify: backlinks panel and neighborhood are correct.* Depends on (1).

3. **Graph UI (global view, local panel, backlinks panel, edge-type toggles).** Force-graph rendering, react-query + zustand toggles; shared-tag edges from `page_tags` (with the popular-tag fan-out cap flagged in (1)/(2)). Depends on (1)+(2).

4. **`okf.SetTags` + tag suggestion mode + suggest/apply endpoints.** The byte-stable frontmatter setter (with round-trip golden test), `agent.SuggestTags` + `validateTags`, `POST /agent/suggest-tags` (no write) and `/agent/apply-tags` (reuses `pages.Save`). *Verify: per-page suggest→approve writes only the tags key, body byte-identical, 409 floor holds.* The `okf.SetTags` byte-stability work gates everything that writes tags. **Tag suggestion must precede the bulk sweep** (the sweep is a fan-out over the same suggestion+apply primitives).

5. **Bulk sweep + review queue.** `KindTagSuggest` job, `tag_suggestions` staging table, `/agent/bulk-tag-sweep` + `GET /agent/tag-suggestions`, batch review UI; approvals reuse phase-4 apply. Depends on (4). Reuses `page_tags` from (1) to find untagged pages.

Phases (1→2→3) and (4→5) are two chains that can proceed in parallel after (1), since the tag chain only needs `page_tags` (built in 1) for "find untagged"; (4) can start once (1) lands.

---

## Anti-Patterns to Avoid (domain-specific, derived from the codebase)

| Anti-pattern | Why it breaks this system | Do instead |
|--------------|---------------------------|------------|
| Storing the graph/tags as the source of truth in SQLite | Violates files-are-truth; the table is a cache | Always rebuildable from `.md`; rebuild-on-drift backstop |
| Re-marshaling the whole frontmatter to write tags | Reorders keys / drops unknown fields → round-trip rot (Pitfall 1) | Surgical `okf.SetTags` on the `yaml.Node` with `FrontDirty`; body untouched |
| Adding a write/tag tool to the Eino agent | Breaks the 5-tool read-only boundary + `tools_test.go` gate; agent must never write | Suggest = read-only mode; apply = separate CSRF+editor endpoint → `pages.Save` |
| Auto-applying swept tags | Violates "no silent frontmatter writes" / agent-safety model | Stage to `tag_suggestions(pending)`; human approves |
| Deriving the global graph on every request | Re-walks + re-parses every file on a hot path; no incremental local-graph freshness | Incremental `page_links`/`page_tags` via fire-and-forget jobs |
| `EnqueueAndWait` from inside a job handler (or from the sweep into apply) | CR-01 deadlock — the single drain goroutine waits on itself | Always `Enqueue` (fire-and-forget) for derived-index maintenance |
| Unbounded shared-tag cliques | A popular tag makes O(n²) edges, bloating the payload | Cap per-tag fan-out; for local graph, connect only within the neighborhood |
| Bypassing the 409 floor on tag apply | A stale apply silently overwrites a concurrent edit | Capture `base_revision` at suggest time; `pages.Save` enforces it |

---

## Confidence Assessment

| Area | Confidence | Reason |
|------|------------|--------|
| Link/backlink store location & maintenance | HIGH | `okf.FindLinks`, `rewriteFolderInboundLinks`, `KindIndex`/`enqueueIndexUpsert`, `RebuildIndex` all exist and verbatim model the pattern |
| Graph endpoints / payload control | HIGH | Mirrors existing `/tree`+`/search` in the authed group; sizing reasoning is sound at this scale |
| Shared-tag edges | HIGH | `search.readTags` is the existing sequence-aware reader; O(n²) clique caveat called out |
| Byte-stable tag write via `okf.SetTags` | HIGH | `okf.SetField`+`Emit`+`FrontDirty` is the proven surgical pattern; only the sequence-node variant is new |
| Suggest→approve→apply reuse of `pages.Save` | HIGH | `/apply-patch` already demonstrates the exact endpoint→`pages.Save`→single-writer reuse |
| Bulk sweep on the jobs worker | HIGH | `jobs.Worker` retry/backoff/single-drain is purpose-built for this; only a job kind + staging table are new |
| Force-graph rendering library choice (frontend) | MEDIUM | Not yet selected; not load-bearing for integration (a STACK-level decision for the graph UI phase) |

## Sources

- `internal/okf/{links.go,okf.go,emit.go,repair.go}` — structural link scanner, byte-stable Doc model, surgical frontmatter edit (`Field`/`SetField`/`Repair`), `FrontDirty` re-marshal — HIGH (read directly)
- `internal/pages/{service.go,rename.go}` — single-writer `Save`/`Create`/`relocate`, `enqueueIndexUpsert`/`Delete`, all-page walk + link rewrite, 409 floor — HIGH (read directly)
- `internal/search/{indexjob.go,rebuild.go,service.go}` — `KindIndex` fire-and-forget job pattern, `RebuildIndex` repo walk, `readTags` sequence-aware reader — HIGH (read directly)
- `internal/jobs/{queue.go,worker.go}` — single-writer worker, `Enqueue` vs `EnqueueAndWait`, retry/backoff, `Register(kind, handler)` — HIGH (read directly)
- `internal/agent/{agent.go,tools.go,propose.go}` + `internal/server/handlers_agent.go` — single-shot modes, read-only 5-tool boundary + set-equality gate, `ProposePatch`→`/apply-patch`→`pages.Save`, validate-and-retry — HIGH (read directly)
- `internal/server/router.go` — chi `authed` (reads) vs `editor` (CSRF writes) groups — HIGH (read directly)
- `.planning/PROJECT.md` v1.0 milestone scope + locked constraints — HIGH
