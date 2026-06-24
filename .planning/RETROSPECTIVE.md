# Retrospective: OKF Workspace

A living retrospective across milestones. Newest milestone first.

## Milestone: v1.0 — Knowledge Graph & LLM Auto-Tagging

**Shipped:** 2026-06-24
**Phases:** 5 (8–12) | **Plans:** 14 | **Tasks:** 34 | **Commits:** ~85 (over 2026-06-23→24)

### What Was Built
A derived link/tag adjacency store (`internal/graph`, rebuildable from files, never source of truth) kept fresh at every page-mutation site; three authed graph read endpoints + a page-view backlinks panel; an Obsidian-style global graph view and per-page local-graph dock (single Canvas-only `react-force-graph-2d` dep, code-split out of the editor bundle); a byte-stable `okf.SetTags` frontmatter primitive feeding an on-demand per-page suggest→approve tagging chain; and an admin bulk sweep (`internal/tagsweep`) that enqueues per-page suggestion jobs into a review queue, approved through the same byte-stable apply path with batched commits.

### What Worked
- **Mirroring shipped seams.** Every v1.0 subsystem cloned an existing v0.9.9 pattern: the graph job cloned `search.KindIndex`; tag-apply rode the existing single-writer `EnqueueCommit` spine; the admin reindex copied "Rebuild search index"; the tag suggest→approve flow mirrored the agent's propose→approve→apply. Net new architecture was minimal and review was fast because reviewers already knew the shape.
- **Derived-store discipline.** Holding "SQLite is a cache, never source of truth" as an exit gate (delete-and-rebuild reproduces byte-identical adjacency, `-race` clean) kept the files-as-truth constraint intact and made the admin rebuild backstop a natural drop-in.
- **Byte-stable writes + server-side re-validation.** Routing all tag writes through one `SetTagsFrontmatter` builder (used by both per-page and batch) meant no drift, and the security review confirmed no YAML/frontmatter injection vector. Never trusting LLM or client tag lists at the write was the right default.
- **Front-loading the testable core (Phase 10-01).** Building the non-visual graph foundation (helpers, hooks, store slice) with full unit coverage before the canvas components kept the visual layer thin and vitest-assertable.

### What Was Inefficient
- **Live UAT against the worker model.** The Phase-12 "bulk-approve commit didn't land live" scare cost real time. Root cause was worker saturation (≈126 queued suggest jobs ahead of the commit), not a durability bug — the in-process drain tests already proved the one-commit invariant. A code-inspection of the resolve-vs-commit ordering up front would have settled it faster than repeated live attempts (each killed before the queue drained).
- **SUMMARY frontmatter drift.** Phases 11/12 SUMMARYs used a `requires:` field instead of `requirements-completed:`, so the milestone-complete CLI scraped malformed one-liners into MILESTONES.md (had to be hand-curated at close). Coverage was real; only the YAML key was wrong.
- **Live-validation harness friction.** The Bash sandbox kills foreground `sleep`/`pkill`/retry-loops (exit 144), so live server UAT needed the awkward "server with `&` inside one foreground call" pattern. This pushed several visual checks to deferred human-browser UAT.

### Patterns Established
- **Derived cache + rebuild backstop** as the standard shape for any new index over the Markdown files (search, now graph/tags).
- **One shared byte-stable frontmatter builder** consumed by both single-item and batch write paths — no second implementation allowed to exist.
- **Suggest→review→approve, output re-validated server-side** as the safety contract for any LLM-driven write (extends the agent's propose→approve model to tagging).
- **Milestone close = audit + independent integration check + independent security review** before tag (security review run even though GSD's complete-milestone doesn't mandate it).

### Key Lessons
- When a live test contradicts passing unit tests, **inspect the ordering in code before re-running the flaky live path** — the cheap deterministic check usually settles it.
- **Match the SUMMARY frontmatter field the close tooling reads** (`requirements-completed:`) so accomplishment extraction stays clean.
- A derived store's **rebuild-parity test is the cheapest guarantee** that you haven't quietly made SQLite the source of truth.

### Cost Observations
- Model mix: all standing agents on opus (lead + delegated engineer/qa/security spans).
- Notable: two delegated independent passes at close (cross-phase integration ≈140k tokens, security review ≈135k tokens) caught nothing blocking but confirmed the seams and mitigations with evidence — worth it for a release-tagged milestone touching the untrusted-input surface.

---

## Cross-Milestone Trends

| Milestone | Phases | Plans | Shipped | Audit | Security |
|-----------|--------|-------|---------|-------|----------|
| v0.9.9 MVP | 8 (0–7) | 36 | 2026-06-23 | passed | — |
| v1.0 Knowledge Graph & LLM Auto-Tagging | 5 (8–12) | 14 | 2026-06-24 | passed | passed (0 HIGH/CRITICAL) |

**Recurring strengths:** cloning shipped seams keeps new work cheap to build and review; files-as-truth held across both milestones.
**Recurring friction:** live browser/server UAT is harness-constrained → visual checks repeatedly deferred to human UAT. Worth a dedicated, sandbox-friendly live-validation recipe.
