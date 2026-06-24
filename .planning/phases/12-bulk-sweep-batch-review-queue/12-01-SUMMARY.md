---
phase: 12-bulk-sweep-batch-review-queue
plan: 01
subsystem: backend
status: complete
tags: [tagging, bulk-sweep, staging-queue, jobs, single-drain, safety-gate, admin-rbac, csrf, key-free]
requires:
  - "agent.SuggestTags(ctx, path) ([]tags, []existing, baseRev, err) — Phase-11 single-shot tag-suggestion mode (11-02)"
  - "graph page_tags table — distinct tagged-page set for untagged detection (08)"
  - "jobs.Worker single-drain (Enqueue/Register/Handler) + jobs.Handler signature (Phase 1)"
  - "store.Migrate numbered-migration loader + page_tags (0009) / schema_migrations"
  - "repo.Repo Tree()/Root() — live-page enumeration via the SEC-01 resolver"
provides:
  - "tag_suggestions staging table (migration 0010) — pending tag proposals, operational/derived, never source of truth"
  - "tagsweep.Store — StagePending (supersede-on-restage), ListPending (deterministic), Targets(untagged|all) over live page set ∩ page_tags"
  - "tagsweep.KindTagSuggest job kind + SuggestPayload + SuggestHandler — calls SuggestTags for ONE page, stages a pending row, NO write/commit"
  - "POST /api/v1/admin/tags/sweep (admin) — enqueues one job per target, 202 {ok,queued:N}, writes nothing"
  - "GET /api/v1/admin/tags/suggestions (admin) — pending review-queue read"
  - "audit.ActionTagSweep constant"
affects:
  - internal/store/migrations/0010_tag_suggestions.sql
  - internal/tagsweep/store.go
  - internal/tagsweep/job.go
  - internal/server/handlers_tagsweep.go
  - internal/server/router.go
  - internal/server/handlers_auth.go
  - internal/audit/audit.go
  - cmd/okf-workspace/main.go
tech-stack:
  added: []
  patterns:
    - "new job kind on the EXISTING single-drain worker (mirror graph.KindGraph): kind + JSON payload builder + jobs.Handler with defer recover()"
    - "narrow one-method consumer interface (suggester) satisfied structurally by *agent.Service — no internal/agent import in internal/tagsweep (mirrors agent's vocabularyReader)"
    - "Store over the shared *sql.DB + SetRepo(*repo.Repo) for live-page enumeration (mirror graph.Store)"
    - "supersede-on-restage via DELETE-then-INSERT in one tx + a partial-unique index (page_path) WHERE status='pending'"
    - "admin enqueuer interface (Enqueue only) + fire-and-forget enqueue per target + 202 + hidden-infra generic copy (mirror handleGraphReindex)"
    - "go/no-go safety gate test: real worker drain proves N pending rows + working-tree & Git HEAD byte-identical (zero writes/commits) across kill+restart"
key-files:
  created:
    - internal/store/migrations/0010_tag_suggestions.sql
    - internal/tagsweep/store.go
    - internal/tagsweep/store_test.go
    - internal/tagsweep/job.go
    - internal/tagsweep/job_test.go
    - internal/server/handlers_tagsweep.go
    - internal/server/handlers_tagsweep_test.go
  modified:
    - internal/server/router.go
    - internal/server/handlers_auth.go
    - internal/audit/audit.go
    - cmd/okf-workspace/main.go
decisions:
  - "KindTagSuggest is registered AFTER agentSvc is constructed (its suggester dependency), which is post-worker.Start — UNLIKE the other kinds (registered before Start). Worker.Register is mutex-safe (write lock vs handlerFor's read lock) and no KindTagSuggest job can be enqueued until the admin sweep endpoint is reachable (after full wiring), so registering a brand-new kind post-Start is race-free. This is a justified deviation from the plan's 'register before Start' instruction, forced by the real dependency ordering (agentSvc depends on pagesSvc/attachSvc built after Start)."
  - "The suggester interface lives in internal/tagsweep and is satisfied structurally by *agent.Service — tagsweep does NOT import internal/agent at all (cleaner than importing it only for sentinels; the handler returns the suggester error verbatim and lets the worker retry/backoff, so it never needs to inspect ErrTagsInvalid)."
  - "Empty request body on POST /admin/tags/sweep is allowed (defaults all=false); only malformed JSON is a 400 — keeps the 'just start it' admin UX simple."
metrics:
  duration: ~30m
  completed: 2026-06-24
  tasks: 3
  files: 11
---

# Phase 12 Plan 01: Bulk Sweep Backend Fan-out + Staging Spine Summary

Built the load-bearing safety half of the bulk tagging sweep (TAG-05): a new `KindTagSuggest` job on the EXISTING single-drain worker that calls Phase-11 `agent.SuggestTags` for ONE page and STAGES the result into a new `tag_suggestions` table with status `pending` — the job WRITES NO frontmatter and triggers NO commit. Plus the admin sweep-start endpoint (enumerates target pages server-side, enqueues one job per page fire-and-forget, returns 202 immediately, writing nothing) and the admin review-queue read endpoint. The serial single drain is the natural LLM rate-limiter — no parallel LLM caller was added. The go/no-go safety invariant (Pitfall 5) is proven by an explicit key-free test: draining the sweep jobs + simulating a worker kill/restart produces ONLY pending rows with the working tree and Git HEAD byte-identical (zero writes, zero commits).

## What Was Built

### Task 1 — migration 0010 + tagsweep store — commit 0da664e
- `internal/store/migrations/0010_tag_suggestions.sql`: `tag_suggestions` staging table (id, page_path, suggestions JSON `[{tag,existing}]`, base_revision, status DEFAULT 'pending', created_at) with a PARTIAL UNIQUE index `(page_path) WHERE status='pending'` (one pending row per page → supersede) + a `status` index. Header in the 0009 tone: operational/derived staging ONLY, never source of truth, a tag is never written to a file from this table without an explicit human approve.
- `internal/tagsweep/store.go`: `Store` over the shared `*sql.DB` + `SetRepo` (mirrors `graph.OpenStore`); `Suggestion{Tag,Existing}` + `PendingEntry{PagePath,Suggestions,BaseRevision}`; `StagePending` (DELETE-then-INSERT pending in one tx — supersede), `ListPending` (status='pending' ORDER BY page_path), `Targets(allPages)` — live pages (repo.Tree skip dirs/non-.md/trashed, mirroring graph.livePages) MINUS the distinct `page_tags` set when untagged, or all live when `all`; nil repo / zero targets → empty slice (no panic, no error).
- `store_test.go` (real temp SQLite + repo): migration idempotency (version 10 once), stage/list round-trip + supersede (exactly one pending row), Targets untagged vs all (excludes tagged-but-deleted `ghost.md`), zero-targets empty slice, nil-repo safety.

### Task 2 — KindTagSuggest job + key-free SAFETY GATE — commit 180b478
- `internal/tagsweep/job.go`: `const KindTagSuggest = "tag_suggest"` + `suggestPayload{Path}` + `SuggestPayload(path)` builder (mirror graph.UpsertPagePayload); narrow `suggester` interface (`SuggestTags(ctx,path)(tags,existing,baseRev,err)`) satisfied structurally by `*agent.Service` (NO internal/agent import). `SuggestHandler(store, s)` returns a `jobs.Handler` with `defer recover()` → returned error (mirrors GraphHandler verbatim): unmarshal payload → call SuggestTags → on error RETURN it (worker retries, stages nothing) → else zip tags+existing into `[]Suggestion` and `StagePending`. Handler holds NO writer — never `pages.Save`/`okf.SetTags`/a commit (grep-asserted SAFE).
- `job_test.go` (KEY-FREE, fake suggester): happy-path stages exactly one matching row; suggester error propagates + stages nothing; panic recovered to a non-nil error; **SAFETY GATE** `TestSafetyGate_NoAutoWrite` — a REAL `jobs.Worker` drains N enqueued jobs → N pending rows AND the content repo working tree + Git HEAD are byte-identical before/after (zero frontmatter writes / zero commits), then a Stop → re-enqueue → fresh-worker Start (kill+restart) re-stages but still never writes.

### Task 3 — admin endpoints + worker/router/main wiring — commit c1ca4fa
- `internal/audit/audit.go`: `ActionTagSweep = "tag_sweep"`.
- `internal/server/handlers_tagsweep.go`: generic `tagSweepUnavailable` copy (names no internal); `tagSweepEnqueuer` interface (Enqueue only); `handleStartTagSweep` (admin) — empty body OK (default all=false), `Targets` server-side, enqueue one `KindTagSuggest` per target fire-and-forget (best-effort stop-on-first-error), audit `ActionTagSweep` `scope=…count=N`, 202 `{ok,queued:N}`, writes nothing, zero targets → queued=0; `handleListTagSuggestions` (admin) — `ListPending` → 200 JSON array.
- `internal/server/router.go` + `handlers_auth.go`: `tagSuggestions *tagsweep.Store` + `tagSweepJobs tagSweepEnqueuer` on `authHandlers` + `Deps`; `admin.Post("/admin/tags/sweep")` + `admin.Get("/admin/tags/suggestions")` under the existing `RequireRole(admin)` subgroup (+ global nosurf CSRF) — RBAC from the session, never client input.
- `cmd/okf-workspace/main.go`: construct `tagsweepStore := tagsweep.OpenStore(st.DB())` + `SetRepo(contentRepo)`; `worker.Register(tagsweep.KindTagSuggest, tagsweep.SuggestHandler(tagsweepStore, agentSvc))` on the SAME single worker (suggester = the already-built agentSvc); `Deps{… TagSuggestions: tagsweepStore, TagSweepJobs: worker}`.
- `handlers_tagsweep_test.go` (KEY-FREE, recording fake enqueuer + real store): admin-only 403/202 (sweep + list), per-page enqueue count + exact payloads (untagged only), all-scope (every live page), zero-targets queued:0, review-queue ordered pending entries.

## Deviations from Plan

### Adjustments

**1. [Adaptation - real ordering] KindTagSuggest registered after worker.Start (not before)**
The plan said "register BEFORE worker.Start, like every other kind." But the suggester is `agentSvc`, which is constructed AFTER `worker.Start` (it depends on `pagesSvc`/`attachSvc`, built post-Start). The other kinds register pre-Start because their handlers don't need agentSvc. `Worker.Register` is mutex-safe (write lock vs `handlerFor`'s read lock), and the FIRST `KindTagSuggest` job can only be enqueued via the admin endpoint once the server is fully wired and listening — strictly after registration. So registering this brand-new kind post-Start is race-free and correct.

**2. [Adaptation - cleaner decoupling] tagsweep imports internal/agent ZERO times**
The plan allowed importing `internal/agent` for the sentinel errors "or not at all if the interface fully decouples it; prefer the narrow interface." Implemented with the narrow `suggester` interface and NO agent import at all — the handler returns the suggester error verbatim for the worker to retry/backoff, so it never needs to inspect `ErrTagsInvalid`.

**3. [Adaptation - real signatures] store/test helpers mirror graph, not a generic helper**
Used `repo.New` + `store.Open`/`Migrate` + `repo.Root()` directly in the tagsweep test harness (mirroring `internal/graph/harness_test.go`) since there is no shared cross-package store test helper. The safety-gate "no write" proof uses a working-tree snapshot + `.git/HEAD` resolution (the handler holds no writer, so a byte-identical tree is the strongest possible proof).

No new dependency added; CGO-free single binary preserved; the read-only 5-tool agent boundary is untouched (KindTagSuggest reuses the existing SuggestTags MODE, not a new tool).

## Self-Check: PASSED

Files created (all FOUND):
- internal/store/migrations/0010_tag_suggestions.sql
- internal/tagsweep/store.go, store_test.go, job.go, job_test.go
- internal/server/handlers_tagsweep.go, handlers_tagsweep_test.go

Commits (all FOUND in git log): 0da664e, 180b478, c1ca4fa

Verification output (real, pasted):
- `CGO_ENABLED=0 go build ./...` → exit 0
- `env -u DEEPSEEK_API_KEY -u OKF_LLM_API_KEY CGO_ENABLED=0 go test ./internal/tagsweep/ ./internal/server/ ./internal/store/ ./internal/audit/` → ok (all four)
- `internal/tagsweep` `-v` → all 12 tests PASS incl. `TestSafetyGate_NoAutoWrite` (the Pitfall 5 go/no-go gate) and the migration-idempotency test
- `go vet ./internal/server/ ./internal/tagsweep/` → clean
- grep-assert `internal/tagsweep/job.go` contains zero `pages.Save`/`okf.SetTags`/`KindCommit` references → SAFE
