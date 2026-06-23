# Pitfalls Research

**Domain:** Knowledge graph + LLM auto-tagging on a files-as-truth Markdown wiki (Go single-binary + React SPA, OKF Workspace v1.0)
**Researched:** 2026-06-24
**Confidence:** HIGH (system-specific reasoning grounded in PROJECT.md/CLAUDE.md constraints; graph-library and force-simulation facts verified against current library docs)

> Scope note: these are pitfalls for **adding a graph + LLM tagging to *this* system** — not generic graph/LLM advice. Every pitfall is tied to one of the v1.0 hard constraints: CGO-free single binary, files-as-truth Markdown+frontmatter in Git, agent writes require human approval, byte-stable round-trip, SQLite = operational metadata only, single-writer Git commit path, ~5-user scale.

---

## Critical Pitfalls

### Pitfall 1: Byte-stable frontmatter round-trip broken when writing `tags`

**What goes wrong:**
The approved tag-write path re-serializes the whole YAML frontmatter (via `yaml.Marshal` of a struct) and silently rewrites the entire file: it reorders keys, drops unknown/optional frontmatter fields, normalizes quoting (`'foo'` → `foo`), reflows a block-style tag list into flow style (`tags: [a, b]`), strips comments, or changes indentation. The body Markdown may also get re-emitted by Goldmark instead of preserved verbatim. The result is a giant noisy Git diff on a page where the user only approved adding two tags — and possibly *data loss* of frontmatter fields the struct doesn't model.

**Why it happens:**
The v0.9.9 page-edit path already solved byte-stable round-trip (PROJECT.md: "byte-stable round-trip" is a shipped, validated requirement), but tag-writing is tempting to implement as a *new* shortcut: "I have a `Page` struct, just set `.Tags` and marshal." `yaml.v3`'s `Marshal` does not preserve key order, comments, or fields absent from the struct, and `yaml.Node`-based surgical editing is more work. The "preserve unknown/optional fields on round-trip" requirement (SPEC §10, CLAUDE.md) is exactly the footgun.

**How to avoid:**
Reuse the **existing** v0.9.9 frontmatter writer that already passes byte-stability tests — do NOT introduce a second serialization path. Edit `tags` surgically: operate on the `yaml.Node` tree (or the existing round-trip-safe editor), touch *only* the `tags` key, leave every other key/comment/field/quoting byte-identical, and never touch the body bytes. Add a golden-file test: take a page with rich frontmatter (comments, extra fields, mixed quoting, a flow-style list elsewhere), add a tag, assert the diff touches *only* the `tags` lines. Decide and pin the canonical tag style once (block list vs flow) and only emit that style when *creating* the `tags` key on a page that lacks it.

**Warning signs:**
Git diffs for a tag approval show changes to unrelated frontmatter lines or body; `git diff --stat` shows far more changed lines than tags added; users complain "the AI rewrote my whole page"; frontmatter fields disappear after tagging.

**Phase to address:**
The tag-**write** phase (the phase that lands the approved-tag persistence path). Gate that phase's verification on a frontmatter golden-file round-trip test before any bulk sweep is allowed to run.

---

### Pitfall 2: Loading the umbrella `react-force-graph` drags three.js into the embedded bundle

**What goes wrong:**
A developer imports `react-force-graph` (the umbrella package) or `ForceGraph3D` "to keep options open." The umbrella package pulls in the 3D variant, which depends on **three.js** — adding hundreds of KB (gzipped) of WebGL code to the SPA. Because the SPA is embedded into the single Go binary via `//go:embed web/dist`, this bloats the *binary itself*, slows `vite build`, and ships a 3D/VR/AR rendering engine the product never uses (the milestone is 2D Obsidian-style graph only).

**Why it happens:**
The umbrella `react-force-graph` re-exports 2D/3D/VR/AR; the 2D-only standalone (`react-force-graph-2d`) is a separate package. The default "npm install react-force-graph" path is the bloated one. Tree-shaking does **not** save you if you import from the umbrella entry — the 3D path with three.js comes along.

**How to avoid:**
Import the standalone 2D package only: `import ForceGraph2D from 'react-force-graph-2d'` (HTML5 canvas + d3-force, **no three.js**). Never install the umbrella `react-force-graph` or any `*-3d`/`*-vr`/`*-ar` package. Add a bundle-size budget check to the frontend build (fail CI if `web/dist` main chunk exceeds a threshold) and lazy-load the graph view as a route-level dynamic `import()` so it is code-split out of the initial app load. Verify `three` does **not** appear in `npm ls` / the lockfile.

**Warning signs:**
`three` shows up in `package-lock.json` / `npm ls three` resolves; `vite build` output chunk for the graph view is unexpectedly large; the Go binary grows by hundreds of KB after adding the graph; build time jumps.

**Phase to address:**
The graph-rendering (frontend) phase — pin the 2D-only dependency on day one and add the bundle budget + lazy-load there. Cheap to do up front, expensive to rip out later.

---

### Pitfall 3: Link/backlink index goes stale on rename / move / delete

**What goes wrong:**
The graph and backlinks panel are fed by a link index (page → outgoing links → resolved targets). When a page is renamed, moved between folders, or trashed/restored, edges aren't reconciled: you get **stale edges** pointing at the old path, **orphan nodes** for deleted pages that still appear in the global graph, **dangling backlinks** ("X links here" where X no longer links or no longer exists), and duplicate nodes when a move is treated as delete-old + create-new. Because the v0.9.9 page model already supports rename/move/delete-to-trash/restore (PROJECT.md "OKF pages"), every one of those operations is a chance to desync the link index.

**Why it happens:**
The link index is a *derived* structure (operational metadata, correctly NOT the source of truth). It's easy to build it once on startup and then forget to invalidate the right entries on every mutation. Backlinks are especially error-prone because a rename of page B must update the backlink entries of every page that links *to* B, not just B's own row. Trash/restore adds a third state (a page can be a valid link target again after restore).

**How to avoid:**
Treat the link index like the Bleve search index (which v0.9.9 already updates incrementally): hook **every** page mutation — create, edit, rename, move, trash, restore, and agent-approved apply — to reconcile affected edges in the same operation. On rename/move, rewrite both the moved page's outgoing edges *and* every inbound edge's target. Store links by stable page identity, and resolve display paths at query time so a move doesn't require touching inbound rows. Make the index **rebuildable from files** (a "reindex graph" admin action) since files are truth — that's your recovery hatch. Add an integration test that renames/moves/trashes/restores a linked page and asserts the graph + backlinks panel are correct after each step. Decide the policy for unresolved links explicitly (render as a distinct "missing target" node, or hide) rather than leaking stale edges.

**Warning signs:**
Backlinks panel shows pages that no longer link here; clicking a graph edge 404s or navigates to a moved page's old path; trashed pages still appear as nodes; node count in the global graph exceeds the live page count; restore doesn't bring a page's edges back.

**Phase to address:**
The backlinks/link-index phase (build the index + reconciliation hooks), with rename/move/trash/restore reconciliation as explicit acceptance criteria. The graph-rendering phase consumes this index and must not paper over staleness on the client.

---

### Pitfall 4: LLM tag explosion — near-synonyms, casing drift, ignoring the existing vocabulary

**What goes wrong:**
The LLM invents fresh tags per page instead of reusing what exists. Over a bulk sweep you get `Postgres`, `postgres`, `PostgreSQL`, `psql`, `database`, `databases`, `db` all coexisting; casing drifts (`API` vs `api`); multi-word tags arrive inconsistently (`ci-cd` vs `CI/CD` vs `ci cd`). The tag set bloats to hundreds of near-duplicates, which destroys the *entire point* of the shared-tag graph edge (every page becomes weakly connected to a different synonym, so the graph shows no real clusters) and makes tag-based navigation useless.

**Why it happens:**
A naive prompt is "suggest tags for this page" with no awareness of the workspace's existing tag vocabulary. The LLM is non-deterministic and will happily generate plausible-but-novel tags. Bulk sweep amplifies this: 200 pages tagged independently with no shared controlled vocabulary = combinatorial synonym sprawl.

**How to avoid:**
Build and pass the **existing controlled vocabulary** into the suggestion prompt: enumerate current workspace tags (the link/tag index already knows them) and instruct the model to *prefer reusing* an existing tag and only propose a *new* tag when nothing fits, flagging new tags distinctly in the suggest→approve UI. Normalize on write: lowercase (or a single pinned casing rule), trim, collapse separators to one canonical form, dedupe against existing tags case-insensitively. Consider a similarity check (string distance / embedding) that warns "did you mean existing tag `postgres`?" before a near-duplicate is approved. Surface new-vs-existing tags differently in the approval UI so a human gatekeeps vocabulary growth. For bulk sweep, seed the prompt with the *current* (possibly growing) vocabulary so later pages reuse tags introduced earlier in the same sweep.

**Warning signs:**
Tag count grows roughly linearly with page count; the shared-tag graph view is a hairball or has no clusters; obvious synonym/casing pairs both present in the tag list; users say tag filtering returns the "wrong" subset because the concept is split across spellings.

**Phase to address:**
The LLM tag-suggestion phase (per-page) — bake in vocabulary-aware prompting + write-time normalization *before* the bulk-sweep phase exists, because the sweep multiplies any vocabulary mistake by the page count.

---

### Pitfall 5: Bulk sweep bypasses the suggest→approve safety model

**What goes wrong:**
The per-page flow correctly does suggest→approve (no silent writes), but the **bulk sweep** is implemented as "tag all untagged pages automatically" and writes frontmatter directly to dozens/hundreds of pages without per-item human approval. This silently violates the locked decision "Agent writes require explicit user approval / no silent frontmatter writes" (PROJECT.md Key Decisions; "Direct agent writes without user approval" is explicitly Out of Scope), produces a flood of low-quality/hallucinated tags across the workspace, and creates a massive Git commit no human reviewed.

**Why it happens:**
Per-item approval for 200 pages feels tedious, so "just apply them all" is the path of least resistance. The safety model is easy to honor for one page and easy to forget at scale.

**How to avoid:**
The bulk sweep must produce a **reviewable batch of proposals**, never direct writes — same safety invariant as the single-page flow, just batched. Present a review queue (per-page proposed tags, with the existing-vs-new distinction from Pitfall 4) where the human can approve/reject/edit in bulk or per page, then apply only approved tags. Make "apply" the human-gated step; "sweep" only *computes* proposals. Keep the read-only agent-tool boundary (v0.9.9: "read-only 5-tool boundary, no direct writes") — tag application goes through the same approved-write commit path as page edits, not through a new agent-write capability. Audit-log every proposal and approval (the audit log already exists).

**Warning signs:**
A sweep run mutates files with no human in the loop; a single commit touches many pages' frontmatter attributed to the agent; no proposal/approval entries in the audit log for swept tags; reviewers can't reject individual pages' tags.

**Phase to address:**
The bulk-sweep phase — its core design *is* the batched approval queue. This is a go/no-go acceptance criterion: if it can write without approval, the phase fails.

---

### Pitfall 6: Bulk sweep collides with the single-writer Git commit path

**What goes wrong:**
The sweep applies approved tags to many pages and triggers many Git commits (or fights the single-writer batched-commit serializer), while a human is simultaneously editing/saving a page. Symptoms: lock contention and timeouts on the single-writer path, a flood of one-commit-per-page noise in history, partial application (some pages committed, sweep dies mid-run leaving an inconsistent state), or the sweep starving interactive saves so the UI feels frozen for the other 4 users.

**Why it happens:**
v0.9.9 deliberately uses a **single-writer batched commit** model (PROJECT.md "Git versioning") sized for 5 users doing interactive edits — not for a burst of N programmatic writes. A bulk sweep is a fundamentally different, bursty write pattern the commit path wasn't designed for, and the git CLI shell-out has real latency per invocation.

**How to avoid:**
Route sweep writes through the **same** single-writer path (do not open a second concurrent writer — that risks repo corruption/lock races). Batch the approved tag writes into **one commit** (or a few), not one-per-page, so history stays readable and the git CLI is invoked rarely. Run the sweep as a background job (the v0.9.9 `jobs` package exists) that yields to interactive edits, with bounded concurrency = 1 writer. Make application **idempotent and resumable** so a crash mid-sweep can re-run safely (already-applied pages are skipped). Ensure soft-locks/optimistic-concurrency (v0.9.9) are respected: skip or defer a page a human currently holds, surfacing the conflict rather than overwriting.

**Warning signs:**
Interactive saves time out or 409 during a sweep; history shows hundreds of single-page tag commits; a crashed sweep leaves some pages tagged and others not with no clean resume; git lock errors in logs.

**Phase to address:**
The bulk-sweep phase — the apply step must reuse the single-writer path and batch commits. Verify by running a sweep while a second session edits a page and asserting no corruption, no starvation, and a clean resumable state on kill.

---

### Pitfall 7: Hallucinated / off-topic tags accepted into the vocabulary

**What goes wrong:**
The LLM emits tags not grounded in the page (a generic `important`, a topic the page only mentions in passing, or a confidently-wrong domain term). Because tags are *structural* (they create shared-tag graph edges and drive navigation), a few hallucinated tags wire unrelated pages together and pollute clusters — worse than a hallucinated sentence in a summary because it's persistent metadata.

**Why it happens:**
Tag generation is an open-ended classification with no ground truth; models pad output, over-generalize, or latch onto incidental mentions. Without a confidence signal or a cap, low-quality tags flow into the approval set and tired reviewers rubber-stamp them.

**How to avoid:**
Constrain generation: cap tags-per-page (e.g. a small N), require the model to justify each tag with evidence from the page (graspable in the approval UI), and strongly bias toward the existing vocabulary (Pitfall 4). Default the approval UI to *unchecked*/explicit-accept for **new** tags so the human must opt them in, not opt them out. Make rejection one click and remembered (a rejected tag for a page shouldn't be re-proposed every sweep). Keep humans as the gate (Pitfall 5) — the whole safety model exists precisely because the model will hallucinate.

**Warning signs:**
Tags appear that don't reflect page content; generic catch-all tags spread everywhere; shared-tag graph links pages that have nothing to do with each other; reviewers report "most suggestions are junk."

**Phase to address:**
The tag-suggestion phase (prompt constraints + evidence + caps) and the approval-UI phase (default-deny for new tags, sticky rejections).

---

### Pitfall 8: Global-graph payload too large / force simulation janks the main thread

**What goes wrong:**
The global graph view serializes the entire workspace (all nodes + all edges, possibly with redundant per-edge metadata) into one fat JSON payload, and runs the d3-force simulation **on the main thread**. Even though 5 users implies a modest page count, shared-tag edges are combinatorial: a popular tag on K pages creates ~K² shared-tag edges, so edge count can explode far beyond node count. The result is a slow initial fetch, a long layout "settling" period that pins the main thread (UI frozen, no input), and re-layout thrash every time an edge-type toggle (links/backlinks/shared-tags) flips and the whole simulation restarts.

**Why it happens:**
"It's only 5 users / a few hundred pages" hides the **edge** blow-up from shared tags. SVG rendering and main-thread d3-force are the defaults and are fine for tiny graphs but degrade non-linearly. Toggling edge types by re-running the full force layout from scratch (instead of incrementally) causes visible thrash.

**How to avoid:**
Render on **canvas**, not SVG (use `react-force-graph-2d`, which is canvas + d3-force — see Pitfall 2). Cap/curate the payload: send a lean node/edge list (ids + minimal display fields), and resolve heavy detail lazily on click. For shared-tag edges, **don't materialize the full K² clique** — cap edges per tag, or model the tag as its own node (page↔tag bipartite) so K pages on a tag cost K edges, not K². Compute the global graph payload server-side once and cache it (invalidate on the same hooks as Pitfall 3). Keep the simulation off the critical path: let it settle with a cooldown/alpha cap, freeze nodes after settling, and on edge-type toggles update the graph data **without a full cold restart** of the simulation. Treat the local (per-page neighborhood) graph as the common case — it's tiny by construction and should never hit these limits.

**Warning signs:**
Global graph takes seconds to appear or "boils" for a long time before settling; UI is unresponsive while the graph loads; edge-type toggles cause a visible re-shuffle/lag; the graph JSON payload is large relative to page count; CPU pegs on the graph route.

**Phase to address:**
The graph-rendering phase — canvas renderer + payload shape (bipartite/capped shared-tag edges) + server-side cached payload are core acceptance criteria, with the local graph delivered first (low-risk) and the global graph hardened against the edge blow-up.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Re-marshal whole frontmatter struct to write `tags` | Trivial to code | Breaks byte-stable round-trip, drops unknown fields, noisy diffs (Pitfall 1) | **Never** — reuse the v0.9.9 round-trip-safe writer |
| Bulk sweep writes tags directly (skip per-item approval) | "Done in one click" | Violates the locked no-silent-writes safety model; junk tags workspace-wide (Pitfall 5) | **Never** |
| Materialize full K² shared-tag clique edges | Simplest edge model | Global graph payload + layout blow up non-linearly (Pitfall 8) | Only for a tiny, capped tag, otherwise use page↔tag node model |
| Build link index once at startup, no incremental update | Fast to ship the first graph | Stale edges/orphans on every rename/move/delete (Pitfall 3) | Only as a throwaway spike; production must reconcile per-mutation |
| Run force simulation on main thread, SVG render | Easiest first render | Jank/frozen UI as edges grow (Pitfall 8) | OK for the tiny local graph; not for the global graph |
| One Git commit per swept page | Simple loop | History noise + git-CLI latency storm + single-writer contention (Pitfall 6) | **Never** at scale — batch into one commit |
| Import umbrella `react-force-graph` | "Keeps 3D option open" | three.js bloats embedded binary (Pitfall 2) | **Never** — use `react-force-graph-2d` |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| LLM endpoint (Eino, provider-agnostic, possibly local Ollama) | Bulk sweep fires N concurrent requests with no rate limit/backoff → hammers/times-out the endpoint (local Ollama is single-GPU; remote has rate limits) | Serialize/limit concurrency, add per-request timeout + retry-with-backoff, make the sweep resumable so a 429/timeout doesn't lose progress; show progress + allow cancel |
| Git CLI (single-writer, shell-out) | Sweep opens its own git access / commits per page → lock races, repo contention | Route all sweep writes through the existing single-writer path; batch commits; bounded writer=1 |
| Bleve / existing indexes | Building a *parallel* link index with different invalidation hooks than search → they drift | Hook link-index reconciliation into the **same** mutation points search already uses (create/edit/rename/move/trash/restore/agent-apply) |
| Frontmatter writer (yaml.v3) | New tag-write path instead of the proven round-trip-safe one | Surgically edit the `tags` node via the existing writer; golden-file test the diff |
| Agent tool boundary (read-only, 5 tools) | Adding a "write tags" agent tool that bypasses approval | Tag application stays on the human-approved write path, not a new agent-write capability |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| K² shared-tag edges | Global graph payload + layout grow non-linearly with pages-per-tag | Bipartite page↔tag node model, or cap edges per tag | A single common tag on many pages — possible even at this scale |
| Main-thread force layout + SVG | UI freezes while graph settles; toggles thrash | Canvas (`react-force-graph-2d`), cooldown/freeze after settle, incremental data updates on toggle (not cold restart) | Mid-hundreds of edges, well within reach via tags |
| Unbounded bulk sweep concurrency | Endpoint timeouts/429s; (local) GPU thrash; UI stall | Concurrency limit + timeout + backoff + resumable job | First sweep over the whole workspace |
| One-commit-per-page sweep | git-CLI latency storm, single-writer contention | Batch approved writes into one commit | First real bulk sweep |
| Whole-workspace graph JSON in one fat payload | Slow graph route, big transfer | Lean node/edge payload, server-side cache, lazy detail on click | Grows with page count; cache invalidation tied to mutations |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Rendering LLM-suggested tags / page titles in the graph or approval UI without sanitizing | Stored XSS via crafted page content / tag text reaching the DOM | Reuse the existing sanitize pipeline (rehype-sanitize is already in the stack); never `dangerouslySetInnerHTML` tag/node labels |
| Treating LLM tag output as trusted to write to disk | Prompt-injected page content steers the model to emit malicious/garbage tags that get committed | Human approval gate (Pitfall 5) + write-time normalization/validation (allowed chars, length cap) before frontmatter write |
| Bulk sweep exposed to non-admin roles | A reader/editor triggers an expensive workspace-wide LLM run | Gate sweep behind appropriate role (admin), respect existing RBAC; audit-log who triggered it |
| Graph/backlink endpoints leak trashed or otherwise non-visible pages | Information disclosure of deleted content | Filter the index by page visibility/trash state at query time (ties to Pitfall 3) |
| Sweep job has no resource ceiling | A triggered sweep is a self-inflicted DoS on the single binary | Bounded concurrency, cancelable job, per-run page cap |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| New tags look identical to reused tags in the approval UI | Reviewer can't see vocabulary growth; rubber-stamps synonyms | Visually distinguish "new tag" vs "existing tag" in suggest→approve (ties to Pitfall 4/7) |
| Default-checked tag suggestions | Reviewer accepts junk by clicking "approve all" | Default new tags to unchecked / explicit accept; one-click reject that sticks |
| Global graph as the headline feature | Hairball that's slow and unreadable; users bounce | Lead with the **local** per-page graph (small, fast, meaningful); make global secondary and curated |
| Edge-type toggles re-shuffle the whole layout | Disorienting "everything jumped" on each toggle | Update edges without cold-restarting the simulation; preserve node positions |
| Backlinks panel shows stale/dangling entries | Erodes trust in the whole feature | Reconcile on every mutation (Pitfall 3); render unresolved links as a distinct state |
| No progress/cancel on bulk sweep | User can't tell if it hung; can't stop a runaway run | Progress bar, cancel, resumable; show per-page status |

## "Looks Done But Isn't" Checklist

- [ ] **Tag write:** Often missing the byte-stable round-trip guarantee — verify a golden-file test where adding one tag changes *only* the `tags` lines (no reordered/dropped frontmatter, no body re-emit).
- [ ] **Backlinks/link index:** Often missing rename/move/trash/restore reconciliation — verify the graph + backlinks are correct after each of those operations on a linked page, and that "reindex from files" rebuilds it.
- [ ] **Bulk sweep:** Often missing the per-item approval gate — verify the sweep produces *proposals* and writes nothing until a human approves; verify it's audit-logged.
- [ ] **Bulk sweep:** Often missing resumability — verify killing the sweep mid-run leaves a consistent, resumable state (no half-applied page, no orphaned git lock).
- [ ] **Bulk sweep:** Often missing rate-limit/timeout handling — verify a slow/429ing endpoint doesn't crash the run or hammer the LLM.
- [ ] **Graph bundle:** Often missing the 2D-only import — verify `three` is absent from the lockfile and the graph view is lazy-loaded/code-split.
- [ ] **Shared-tag edges:** Often missing the K² guard — verify a tag on many pages doesn't explode edge count (bipartite/capped model).
- [ ] **Global graph:** Often missing payload leanness/caching — verify the payload is minimal and cached, invalidated on page mutations.
- [ ] **Vocabulary reuse:** Often missing existing-tags-in-prompt — verify suggestions prefer existing tags and new tags are flagged distinctly.

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Stale/orphan link index (Pitfall 3) | LOW | Files are truth — run a full "reindex graph from files" rebuild; ensure the admin action exists *before* you need it |
| Broken frontmatter round-trip already committed (Pitfall 1) | MEDIUM | Git history is intact — revert the offending commits, fix the writer, re-apply tags via the corrected path; add the golden-file test as a regression guard |
| Tag explosion / synonyms in the wild (Pitfall 4) | MEDIUM–HIGH | Build a tag-merge/alias tool (rename `postgres`/`PostgreSQL` → one canonical across all pages via the single-writer path); painful at scale — prevention >> cure |
| three.js shipped in the bundle (Pitfall 2) | LOW | Swap umbrella import for `react-force-graph-2d`, rebuild, confirm `three` gone; add bundle budget check |
| Half-applied crashed sweep (Pitfall 6) | LOW (if resumable) / HIGH (if not) | If idempotent/resumable: re-run, already-tagged pages skip. If not designed for it: manual git inspection + cleanup — design for resumability up front |
| Silent bulk writes already committed (Pitfall 5) | MEDIUM | Git revert the sweep commit(s), re-run through the approval queue; treat as a design defect, not a one-off |

## Pitfall-to-Phase Mapping

> Phase names are indicative for the roadmapper; the key is *ordering* — link index before graph render; per-page tagging (with vocabulary + round-trip safety) before bulk sweep.

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| 1. Frontmatter round-trip on tag write | Tag-write / persistence phase | Golden-file test: one tag added → only `tags` lines change |
| 2. three.js bundle bloat | Graph-rendering (frontend) phase | `npm ls three` empty; graph route code-split; bundle budget passes |
| 3. Stale link/backlink index | Backlinks / link-index phase (before graph render) | Rename/move/trash/restore integration test; reindex-from-files works |
| 4. Tag explosion / no vocabulary reuse | Per-page tag-suggestion phase (before sweep) | Existing tags passed to prompt; write-time normalization/dedupe; new-tag flagging |
| 5. Sweep bypasses approval | Bulk-sweep phase | Sweep writes nothing without human approval; audit-logged; reviewable queue |
| 6. Sweep vs single-writer Git | Bulk-sweep phase | Sweep during concurrent edit → no corruption/starvation; batched commit; resumable on kill |
| 7. Hallucinated/off-topic tags | Tag-suggestion + approval-UI phases | Tag cap + evidence; new tags default-deny; rejections sticky |
| 8. Graph payload / main-thread jank | Graph-rendering phase (local graph first, then global) | Canvas render; bipartite/capped shared-tag edges; cached lean payload; toggles don't cold-restart |

## Sources

- PROJECT.md (OKF Workspace v1.0 milestone scope, locked decisions, v0.9.9 validated requirements: byte-stable round-trip, single-writer batched commits, agent read-only/no-direct-writes, soft-locks/optimistic concurrency, incremental Bleve index, jobs package) — HIGH
- CLAUDE.md (locked stack: yaml.v3 + `yaml.Node` round-trip requirement SPEC §10, rehype-sanitize in stack, provider-agnostic Eino LLM incl. local Ollama, `//go:embed web/dist` single-binary, no-silent-frontmatter-writes safety model) — HIGH
- [react-force-graph (GitHub)](https://github.com/vasturiano/react-force-graph) — umbrella package re-exports 2D/3D/VR/AR; standalone `react-force-graph-2d` is canvas + d3-force; 3D variant depends on three.js — HIGH
- [force-graph (npm)](https://www.npmjs.com/package/force-graph) / [react-force-graph (npm)](https://www.npmjs.com/package/react-force-graph) — 2D = HTML5 canvas + d3-force; standalone packages for tree-shaking — HIGH
- [The Best Libraries and Methods to Render Large Force-Directed Graphs on the Web (Medium)](https://weber-stephen.medium.com/the-best-libraries-and-methods-to-render-large-network-graphs-on-the-web-d122ece2f4dc) — canvas/WebGL > SVG, run layout off main thread, mouse-tracking perf cost — MEDIUM
- [d3-force (D3 docs)](https://d3js.org/d3-force) — force simulation runs on the main thread by default; alpha/cooldown control settling — HIGH

---
*Pitfalls research for: knowledge graph + LLM auto-tagging on a files-as-truth Markdown wiki (OKF Workspace v1.0)*
*Researched: 2026-06-24*
