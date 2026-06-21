---
phase: 04-eino-agent
reviewed: 2026-06-21T00:00:00Z
depth: deep
files_reviewed: 19
files_reviewed_list:
  - cmd/okf-workspace/main.go
  - internal/agent/agent.go
  - internal/agent/chatmodel.go
  - internal/agent/deps.go
  - internal/agent/prompts.go
  - internal/agent/propose.go
  - internal/agent/stream.go
  - internal/agent/tools.go
  - internal/attachments/service.go
  - internal/audit/audit.go
  - internal/server/handlers_agent.go
  - internal/server/handlers_auth.go
  - internal/server/router.go
  - web/src/api/client.ts
  - web/src/components/AgentAnswer.tsx
  - web/src/components/AgentPanel.tsx
  - web/src/components/DiffReviewDialog.tsx
  - web/src/components/PromptBar.tsx
  - web/src/routes/AppShell.tsx
  - web/src/stores/agentPanel.ts
findings:
  critical: 1
  warning: 5
  info: 3
  total: 9
status: clean
fix:
  fixed_at: 2026-06-22
  critical_fixed: 1
  warning_fixed: 5
  info_fixed: 2
  info_deferred: 1
  cr01_resolution: body-only propose/apply contract (frontmatter server-owned, attached once at apply)
  cr01_roundtrip_test: internal/pages.TestApplyPatchBodyOnlyRoundTrip
  deferred:
    - IN-03 (SSE reserved-keyword delta): no change required — the review documents
      this as a safe invariant (server never emits model text on the `event:` line);
      kept as documentation only.
  follow_up:
    - WR-01: editor selection capture is not yet plumbed — Rewrite is exposed
      disabled ("select text first") and selection-scope Ask is not yet wired.
    - WR-02: per-page ACLs not yet implemented — agent retrieval stays workspace-wide
      (acceptable while all authed users read everything); TODO gates the ACL work.
---

# Phase 4: Code Review Report

**Reviewed:** 2026-06-21
**Depth:** deep
**Files Reviewed:** 19
**Status:** clean (CR-01 + all 5 warnings fixed 2026-06-22; IN-01/IN-02 fixed, IN-03 deferred as documented-invariant)

## Summary

I reviewed the Phase 4 Eino agent against the AI-SPEC §1 Critical Failure Modes and the focus-security-boundary contract. **The load-bearing structural safety boundary is sound** and I want to state that explicitly:

- **Write boundary is structural and well-tested.** `internal/agent/tools.go` is the single tool-construction site; the five-tool read-only allow-list is enforced by a build-gate test (`tools_test.go` — exact set equality, plus a `tl.Info().Name` cross-check so the parallel name list can't lie). Apply/write is genuinely a separate HTTP endpoint (`handleApplyPatch`), never an Eino tool. No write/commit/push/shell tool is reachable from the ReAct graph. (D5 holds.)
- **Path safety holds.** Every tool routes through `pages.Service` / `search.Index` / `attachments.Service.ExtractedText` (the last via `repo.Exists`/`repo.Read`, the SEC-01 resolver) — no `os.ReadFile`, no client-path read in any tool. (Failure Mode #2 / D5.)
- **Secret handling holds.** The API key is read exactly once via `cfg.APIKey()` inside `newChatModel`, never logged, never returned, never placed in an error or audit Detail.
- **Optimistic concurrency holds.** `ProposePatch` captures `baseRev` via `pages.Revision` at proposal time; `handleApplyPatch` reuses `pages.Save(baseRevision)` which 409s on a moved revision before any write. (D8.)
- **Prompt-injection posture is correct at the structural level.** Untrusted content is delimited into the user turn; the real defense (non-tool apply + mandatory diff approval) is intact.
- **AuthZ is correct.** chat/summarize/rewrite/draft are any-authed; propose/apply are `RequireRole(RoleEditor)` + global nosurf CSRF; role is read from the session (`actorRole`/`actorUsername` read only from context), never the client.
- **DiffReviewDialog is a real trust gate.** It always renders the actual `ReactDiffViewer` diff (never prose), Reject is the deliberate initial-focus target, backdrop/Esc reject-without-applying, and the stale state removes Approve entirely. `MarkdownProse` keeps `rehype-raw` OFF with `rehype-sanitize` ON.

However, the propose→apply data path has a **confirmed patch-corruption bug** (the cardinal Failure Mode #4) that is not caught by the existing tests, plus several quality/robustness defects. Details below.

## Critical Issues

### CR-01: Propose→apply double-writes the frontmatter, corrupting the page (patch corruption, Failure Mode #4)

**File:** `internal/agent/propose.go:209-214`, `internal/server/handlers_agent.go:476-481` and `:517-523`; `web/src/routes/AppShell.tsx:184-194`

**Issue:** The propose/apply contract is internally inconsistent about whether `new_body` includes frontmatter, and the apply path ends up writing the frontmatter twice.

1. `proposePatchSystemPrompt` instructs the model to "Return the COMPLETE revised page — the full frontmatter region ... followed by the full Markdown body." `currentSource(pg)` (propose.go:296) builds the full `---\nfrontmatter\n---\nbody` source, and `validateProposedBody` compares the proposed *source's* frontmatter keys against it. So `newBody` returned by `ProposePatch` **contains the frontmatter region**. The `propose_test.go` fixtures confirm this — the "proposed" body is a variant of `d4Fixture`, which begins with `---\ntitle: ...`.
2. `handleProposePatch` returns `OldBody: pg.Body` (body **only**, no frontmatter) and `NewBody: newBody` (full source **with** frontmatter). The diff in `DiffReviewDialog` therefore shows the entire frontmatter block as "added" — a noisy, misleading diff (also violates D4 minimal-locality).
3. On approve, `AppShell.tsx:186-193` sends `new_body: p.new_body` (full source incl. frontmatter) **and** `frontmatter: frontmatterFromCache(...)` (the original frontmatter again). The AppShell comment claims "The proposal's new_body is the body only" — this is factually wrong given the prompt/`currentSource`.
4. `handleApplyPatch` → `pages.Save(path, req.NewBody, req.Frontmatter, ...)` → `assemble(frontmatter, body)` prepends `---\n{frontmatter}\n---\n` to `req.NewBody`. Result: `---\nFM\n---\n` + `---\nFM\n---\nbody`. `okf.Parse` consumes the first fence as frontmatter and the **rest** (including the literal second `---\nFM\n---` block) becomes the page body. The page is corrupted; the saved body now contains a stray YAML fence.
5. The defense-in-depth re-validation at handlers_agent.go:517 (`ValidateProposedBody(assembleSource(req.Frontmatter, ""), req.NewBody)`) does **not** catch this: the source has frontmatter, `req.NewBody` parses with frontmatter, and the key sets match — so validation passes and the double-write proceeds.

This is the exact "applied result differs from what the diff showed" / round-trip-rot failure the human gate is supposed to prevent. It is not caught by `apply_test.go` (which models the Save contract abstractly with a stub and never exercises the real `assemble`/`okf.Parse` round-trip) nor by `propose_test.go` (which only validates the proposal in isolation).

**Fix:** Make the frontmatter contract single-sourced. Pick one:

Option A (recommended) — `new_body` is the full source; do NOT prepend frontmatter again at apply. Split the proposed source into (frontmatter, body) on the server before Save:
```go
// handleApplyPatch — split the full proposed source, never re-prepend req.Frontmatter
doc, err := okf.Parse([]byte(req.NewBody))
if err != nil { writeError(w, http.StatusUnprocessableEntity, "..."); return }
fm, body := doc.FrontmatterText(), doc.Body // or equivalent accessors
err = h.pages.Save(r.Context(), path, body, fm, req.BaseRevision, actor)
```
and set `OldBody: oldSource` (full source via `assembleSource(pg.Frontmatter, pg.Body)`) in `proposePatchResponse` so the diff's two sides are comparable.

Option B — `new_body` is body-only. Change `proposePatchSystemPrompt` + `currentSource` so the model is given and returns only the body, validate frontmatter-preservation against `pg.Body` (no frontmatter to preserve), and keep `frontmatter: frontmatterFromCache` as the single frontmatter source at apply.

Either way, add a propose→apply round-trip test that feeds a frontmatter+body page through the real `assemble`/`okf.Parse`/`okf.Emit` and asserts byte-stability of the untouched regions (the D4 gate the AI-SPEC requires, currently only exercised on the proposal in isolation).

## Warnings

### WR-01: Summarize / Rewrite / Draft modes are non-functional — every submit silently runs an Ask

**File:** `web/src/routes/AppShell.tsx:105-157` and `:375-400`; `web/src/api/client.ts` (no `summarizePage`/`summarizeAttachment`/`rewrite`/`draft` functions exist)

**Issue:** `PromptBar` offers `ask | summarize | rewrite | draft | propose`, but `handleSubmit` ignores `mode` entirely and always calls `subscribeAgentChat` (the Ask SSE stream). There are no client functions for the `/agent/summarize-page`, `/agent/summarize-attachment`, `/agent/rewrite`, or `/agent/draft` endpoints (those handlers exist and are wired in the router, but nothing on the frontend calls them). `selection` is never captured from the editor nor sent, so selection-scoped Ask and Rewrite cannot work. A user selecting "Summarize" and submitting gets an Ask answer with no error — a silent wrong-action. Only Ask (any scope sans selection) and the Propose-from-answer flow are actually reachable.

**Fix:** Branch `handleSubmit` on `mode`: call dedicated client functions (`summarizePage`, `rewrite`, `draft`, …) for the non-Ask modes and route their results to the AgentPanel / DiffReviewDialog. Add the missing `client.ts` functions. Capture the editor selection and pass it as `selection` for selection-scope Ask and Rewrite. Until then, hide the unsupported modes in the selector so the bar can't fire a wrong action.

### WR-02: Misleading "role-scoped retrieval" comments — workspace Ask is not role-scoped (latent scope-leak when ACLs land)

**File:** `internal/agent/tools.go:198-201`, `internal/agent/agent.go:59-63`, `internal/server/handlers_agent.go:90-101`; wiring at `cmd/okf-workspace/main.go:247-252`

**Issue:** The code repeatedly asserts that workspace retrieval is bounded to "role-readable pages only" ("deps.Search is constructed role-scoped from the server session by the caller", D7/Failure Mode #2). In fact `search.Index.Query(ctx, q)` takes no role/identity argument, and `main.go` injects a single process-wide `searchIdx` into `agent.Deps.Search` — there is no per-request, role-scoped Search. Today this is **not an active leak** because the page-read model is "any authenticated user reads everything" (`roleSatisfies` gives no per-page ACL; the router mounts page reads in the any-authed group), so there is nothing private to leak. But the comments describe a control that does not exist, and the moment per-page ACLs land ("editor/reader content gating gains nuance in Phase 1", rbac.go:75) the workspace Ask and its citation frame will leak out-of-role pages with no error — exactly the §1b "silent permission/scope leak."

**Fix:** Either implement role-scoped retrieval (thread the session role into a `Query(ctx, role, q)` and filter, or post-filter hits against a role-readable set), or change the comments to state honestly that retrieval is NOT role-scoped at the MVP because all authenticated users can read all pages — and add a test/TODO gating the agent on the ACL work so the gap is caught when ACLs arrive.

### WR-03: `writeError` after the SSE stream has committed emits a superfluous WriteHeader and pollutes the stream

**File:** `internal/server/handlers_agent.go:122-137`

**Issue:** `AskStream` returns a non-nil error on a mid-stream provider failure or client-disconnect *after* it has already written SSE headers and body (and emitted its own `event: error` frame). The handler then falls to the `default` branch and calls `writeError(w, http.StatusBadGateway, ...)`, which calls `w.WriteHeader` again (a logged "superfluous WriteHeader" each time) and appends a JSON `{"error":...}` payload to the committed `text/event-stream` body. The comment acknowledges this ("writeError is a no-op on the status") but it is not a no-op — it writes bytes and triggers the warning.

**Fix:** Have `AskStream` distinguish pre-stream errors (return as now) from post-commit errors (return a sentinel like `errStreamAlreadyCommitted`, or return `nil` after it has emitted its own terminal SSE error frame). In the handler, skip `writeError` entirely once streaming has begun:
```go
_, err := h.agent.AskStream(r.Context(), w, prompt, sc)
if err == nil || errors.Is(err, agent.ErrStreamAlreadyCommitted) {
    return // AskStream already emitted a terminal SSE error frame
}
switch { /* only pre-stream errors reach here */ }
```

### WR-04: No length cap on `page_path` / `attachment_id` agent inputs (asymmetric input validation)

**File:** `internal/server/handlers_agent.go:79-88` (chat), `:206-210`, `:235-239`, `:423-440`

**Issue:** The handlers cap `prompt` (`maxPromptLen`), `selection`/`instruction` (`maxSelectionLen`/`maxPromptLen`) and reject NUL bytes, but `page_path` and `attachment_id` get only the NUL check — no length bound and no UTF-8 validation. A multi-megabyte `page_path` is spliced into the user turn (`buildScopedMessages` writes it into the prompt hint at prompts.go:134) and sent to the model, an unbounded input the focus-contract calls out (item 6: NUL/length/UTF-8 on agent inputs). It is bounded indirectly (the read tool resolves it server-side) but the prompt-splice path is not.

**Fix:** Add a modest length cap and a `utf8.ValidString` check on `page_path` and `attachment_id` in each handler (mirror the `maxPromptLen` pattern), e.g. reject paths over a few hundred bytes or non-UTF-8 with a 400 before building the scope.

### WR-05: `currentSource` and `assembleSource` are duplicated byte-assembly logic that can drift from `pages.assemble`

**File:** `internal/agent/propose.go:296-310`, `internal/server/handlers_agent.go:553-567`, and `internal/pages/service.go` (`assemble`)

**Issue:** Three independent functions reassemble `---\nfrontmatter\n---\nbody` with subtly different trimming rules (`currentSource` and `assembleSource` `TrimSpace` the frontmatter; `pages.assemble` is the authoritative one `Save` actually uses). Because the validators, the churn metric, the diff "old" side, and the actual write each pick a different one of these, any divergence in trailing-newline / trim behavior changes what is validated vs. what is written — and CR-01 is precisely a consequence of these three not agreeing about whether the body already carries frontmatter. This is the kind of duplicated-invariant that makes round-trip bugs hard to see.

**Fix:** Export one canonical assembler from `internal/pages` (e.g. `pages.AssembleSource(fm, body)`) and have the agent service and the HTTP layer call it, so "the exact bytes Save writes" is single-sourced. Add a test asserting `AssembleSource(fm, body)` round-trips through `okf.Parse`/`Emit` byte-stably.

## Info

### IN-01: `churnRatio` can index `oldBody` with edit offsets computed against a different string

**File:** `internal/agent/propose.go:317-336`

**Issue:** `churnRatio` calls `udiff.Lines(oldBody, newBody)` and then slices `oldBody[e.Start:e.End]` for each edit. This assumes `go-udiff`'s `Edit.Start`/`Edit.End` are byte offsets into `oldBody`; if the library's offsets are computed against a normalized/other buffer (or are rune offsets), the slice can panic on a multi-byte boundary or mis-count. It is only used for an audit metric (non-load-bearing), so worst case is a panic in the audit Detail path, but it is unguarded.

**Fix:** Confirm `udiff.Lines` returns byte offsets into the first argument; otherwise compute changed-line counts from the diff's own old/new segments. Add a bounds/recover guard since this feeds a non-critical metric and must never crash the propose handler.

### IN-02: `flattenPagePaths` recurses without a depth bound

**File:** `internal/agent/tools.go:227-242`

**Issue:** `walk` recurses the page tree with no depth limit. The tree is derived from on-disk folder structure (operator-controlled, shallow in practice), so this is low risk, but a pathologically deep tree (or a future cyclic node source) would blow the stack. Out-of-scope performance-wise, noted for robustness only.

**Fix:** Add a depth cap (e.g. 64) and stop recursing past it, returning what was collected.

### IN-03: SSE answer delta equal to a reserved event keyword is not specifically guarded

**File:** `internal/agent/stream.go:98` and `web/src/api/client.ts:594-624`

**Issue:** Answer deltas are framed as bare `data: <delta>` events with `name` defaulting to `"message"`. The parser keys behavior off the `event:` line, so a delta whose content is literally `done` or `citation` is treated as a token (correct) — there is no actual collision because the server never emits those words as an `event:` name from model content. This is fine as written; noting it only because the framing relies on the server never letting model text reach the `event:` line (it doesn't — `escapeSSE` only ever produces `data:` continuation lines). No change required; documenting the invariant.

---

_Reviewed: 2026-06-21_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
