---
phase: 01-okf-pages-navigation-hidden-git
reviewed: 2026-06-19T00:00:00Z
depth: standard
files_reviewed: 45
files_reviewed_list:
  - cmd/okf-workspace/main.go
  - internal/audit/audit.go
  - internal/gitstore/history.go
  - internal/gitstore/push.go
  - internal/gitstore/read.go
  - internal/okf/emit.go
  - internal/okf/links.go
  - internal/okf/okf.go
  - internal/okf/repair.go
  - internal/pages/commitjob.go
  - internal/pages/history.go
  - internal/pages/rename.go
  - internal/pages/service.go
  - internal/pages/trash.go
  - internal/pages/tree.go
  - internal/repo/files.go
  - internal/server/handlers_auth.go
  - internal/server/handlers_history.go
  - internal/server/handlers_pages.go
  - internal/server/handlers_trash.go
  - internal/server/handlers_tree.go
  - internal/server/router.go
  - internal/store/migrations/0004_drafts.sql
  - internal/store/migrations/0005_trash.sql
  - web/src/api/client.ts
  - web/src/App.tsx
  - web/src/components/AutosaveStatus.tsx
  - web/src/components/CreateFolderModal.tsx
  - web/src/components/CreatePageModal.tsx
  - web/src/components/DeleteConfirmDialog.tsx
  - web/src/components/HistoryPanel.tsx
  - web/src/components/LeftTree.tsx
  - web/src/components/LinkPicker.tsx
  - web/src/components/MarkdownProse.tsx
  - web/src/components/MoveDialog.tsx
  - web/src/components/PageActionMenu.tsx
  - web/src/components/RecentList.tsx
  - web/src/components/RenameModal.tsx
  - web/src/components/RestoreConfirmDialog.tsx
  - web/src/components/TrashView.tsx
  - web/src/lib/frontmatter.ts
  - web/src/lib/mdlink.ts
  - web/src/routes/AppShell.tsx
  - web/src/routes/PageEditor.tsx
  - web/src/routes/PageView.tsx
  - web/src/stores/recent.ts
findings:
  critical: 0
  warning: 6
  info: 4
  total: 10
status: issues_found
---

# Phase 1: Code Review Report

**Reviewed:** 2026-06-19T00:00:00Z
**Depth:** standard
**Files Reviewed:** 45
**Status:** issues_found

## Summary

This is a re-review of Phase 1 (OKF Workspace: Go + chi backend, React/TS frontend)
after an initial review + fix cycle. The four prior Critical/Medium items were
re-verified as correctly resolved (details below). No new Critical issues were
found this pass.

The remaining open items are 6 Warnings and 4 Info. The dominant themes are
DB/Git divergence on async commit failure (the trash delete/restore write a
SQLite row but the commit runs later in the worker, so a failed commit leaves
the two stores inconsistent), the autosave dual-timer design that can self-409
and has no in-flight guard, the version-token authorization gap (a hex token
from any page's history is accepted for any path), and the push failure
semantics that can fail an already-committed job and over-match "rejected".

### Previously-fixed items — re-verified RESOLVED

- **CR-01 (RESOLVED, b5f625a):** `/pages/*` sub-resource dispatch now anchors on
  the `.md` boundary. Verified in `handlers_pages.go:108-116` and
  `handlers_history.go:79-86`: a page at `docs/history.md` is not mis-routed
  (the `/history` branch additionally requires the trimmed remainder to end in
  `.md`), and a page under a folder named `version` (`docs/version/notes.md`) is
  not mis-routed (the marker is the literal `.md/version/`, not bare `/version/`).
  Fix is correct.
- **CR-02 (RESOLVED, 7bddef6):** `frontmatter.ts quoteIfNeeded`
  (`frontmatter.ts:72-87`) now quotes empty, whitespace-edged, indicator-leading,
  `": "`/` #`-bearing, quote-bearing, reserved-word, and numeric scalars before
  emitting them. YAML-unsafe titles round-trip safely. Fix is correct.
- **WR-02 / WR-06 (RESOLVED, 0c8421e):** `mdlink.ts resolveRelativeMdLink`
  (`mdlink.ts:12-67`) resolves relative `.md` links against the current page dir,
  clamps `..` at root (`out.length > 0` guard, line 58), leaves external/scheme/
  protocol-relative links untouched (line 21), and carries the fragment through.
  `MarkdownProse` takes `currentPath` and uses it (`MarkdownProse.tsx:32`). Fix is
  correct.
- **MEDIUM argv injection (RESOLVED, 2c66ef1):** Version token is hex-validated at
  both layers — `gitstore.isHexObjectName` gates `ShowAt` (`history.go:141-143`)
  and `pages.validVersionToken` gates `ViewVersion`/`RestoreVersion`
  (`history.go:96,130`). A flag-shaped token can no longer reach git as an object
  spec. Fix is correct.

## Warnings

### WR-01: Trash delete/restore write SQLite synchronously but commit asynchronously — a failed commit leaves DB and Git divergent

**File:** `internal/pages/trash.go:91-104` (Delete), `internal/pages/trash.go:188-195` (Restore)
**Issue:** `Delete` enqueues the trash commit via `EnqueueCommit` (which only
*queues* a job on the worker; the actual `r.Write`/`r.Remove`/`g.Commit` run later
in the drain goroutine — see `commitjob.go:51-97`), then **immediately and
synchronously** INSERTs the trash row. If the commit job later fails (write error,
git failure), the SQLite `trash` row persists pointing at a `trash_path` that was
never written to disk — `ListTrash` shows a phantom entry and `Restore` will fail
reading the missing file. `Restore` has the mirror-image bug: it enqueues the
restore commit, then synchronously `DELETE`s the trash row; a later commit failure
loses the row while the page is still physically in the trash directory, making it
unrecoverable through the UI. The async enqueue makes the SQLite write and the Git
write non-atomic.
**Fix:** Do not treat enqueue as success. Either (a) perform the trash/restore
commit synchronously and only touch SQLite after `g.Commit` returns, or (b) move
the `trash` row INSERT/DELETE into the CommitHandler so it runs in the same
goroutine after a successful commit (and is skipped on failure). Minimal option (a)
for Delete:
```go
// after EnqueueCommit succeeds, the job is only QUEUED — do not record the row
// until the commit actually lands. Prefer recording provenance from inside the
// commit handler, or run the commit synchronously here and INSERT only on success.
```
At minimum, document and reconcile on startup: a `trash` row whose `trash_path`
does not exist on disk must be pruned.

### WR-03: PageEditor autosave fires two overlapping saves per idle and has no in-flight guard — concurrent saves can self-409 and lose draft state

**File:** `web/src/routes/PageEditor.tsx:91-96` (scheduleAutosave), `57-87` (doSave)
**Issue:** `scheduleAutosave` arms BOTH a 1s `draftTimer` (`doSave(false)`) and a
6s `idleTimer` (`doSave(true)`) on every edit. After 1s of idle the draft save
fires; its body (`doSave`) is `async` and refetches to advance
`baseRevision.current`. There is no guard preventing a second `doSave` from
starting while the first is still in flight: if the 6s idle save (or an explicit
Save click, or a later keystroke's draft save) begins before the first save's
`getPage` refetch has updated `baseRevision.current`, the second PUT carries the
**stale** base revision and the server returns 409 against the user's own
just-committed write. The result is a spurious conflict banner on a single-user
edit session. `doSave` also closes over `body`/`frontmatter` via `useCallback`
deps, so a timer scheduled before a state update can save **stale** content.
**Fix:** Add an in-flight ref guard and collapse the dual timers:
```ts
const saving = useRef(false);
const doSave = useCallback(async (cutVersion: boolean) => {
  if (saving.current) return;          // drop overlapping saves
  saving.current = true;
  try { /* ...existing body... */ }
  finally { saving.current = false; }
}, [body, frontmatter, path, queryClient]);
```
And drop the always-armed `idleTimer`; escalate to a version save only after the
draft save settles (e.g. re-arm a single timer inside `doSave`'s success path), so
two PUTs never race on the same base revision.

### WR-04: View/restore accept any hex version token, not one belonging to THIS page's history — cross-page version disclosure / restore

**File:** `internal/pages/history.go:95-99` (ViewVersion), `129-144` (RestoreVersion)
**Issue:** `ViewVersion`/`RestoreVersion` validate only the *format* of `version`
(7–64 hex via `validVersionToken`) and then run `git show <token>:<path>`. They
never verify that `token` is actually a commit in `path`'s own history. Any
authenticated user who learns (or brute-forces a short prefix of) another commit
SHA can pass it with an arbitrary `path`: `git show <other-sha>:<path>` returns
that path's bytes *as they existed at that unrelated commit* if the path existed
there, and `RestoreVersion` will then write those bytes forward as a "restore."
This breaks the hidden-Git contract's intent (the version token is supposed to be
an opaque handle the server issued *for that page*) and lets a caller resurrect or
view content states that were never offered in that page's history panel.
**Fix:** Before `ShowAt`, confirm the token belongs to the page's `--follow`
history. Cheapest correct check: list the page's history tokens and require
membership.
```go
commits, err := s.git.History(ctx, path)
if err != nil { return ... }
ok := false
for _, c := range commits { if c.Token == version { ok = true; break } }
if !ok { return Page{}, fmt.Errorf("...: %w", ErrInvalidVersion) }
```
Apply the same membership check in both `ViewVersion` and `RestoreVersion`.

### WR-05: Push failure fails an already-committed job; isNonFastForward over-matches "rejected"

**File:** `internal/pages/commitjob.go:91-95` (push after commit), `internal/gitstore/push.go:29-39,47-52`
**Issue:** Two coupled problems:
1. In `CommitHandler`, `g.Push` runs *after* `g.Commit` has already succeeded.
   When `Push` returns a non-divergence error (transport, auth, DNS), the handler
   returns that error (`commitjob.go:92-94`), which marks the **whole job failed**
   even though the local commit is durable. Depending on the worker's retry policy
   this re-runs the handler, re-applying `r.Write`/`r.Remove`/`g.Commit` (creating
   duplicate/empty commits) and re-pushing — a failure loop driven by a transient
   network error.
2. `isNonFastForward` (`push.go:47-52`) matches the substring `"rejected"`. Git
   prints `! [rejected]` for non-fast-forward, but pre-receive/update hook denials
   and other server-side refusals also contain "rejected" — those would be
   silently swallowed as "divergence" (set `diverged`, return nil) when they are
   not divergence at all, masking a real push failure.
**Fix:**
1. Treat push failure as non-fatal to the commit job (the commit already
   succeeded): log/alert and return nil, mirroring the divergence path, OR push
   out-of-band rather than inside the commit handler.
```go
if p.Push {
    if err := g.Push(ctx); err != nil {
        // commit already landed; do not fail (and re-run) the whole job on a
        // transient push error — surface via Health instead.
        // log.Warn(...); return nil
    }
}
```
2. Tighten the divergence matcher to the non-fast-forward signals only and drop
   the bare `"rejected"`:
```go
return strings.Contains(msg, "non-fast-forward") ||
    strings.Contains(msg, "fetch first") ||
    strings.Contains(msg, "[rejected]")  // the NFF marker, not bare "rejected"
```

### WR-07: CreateFolder has no empty-slug fallback — a punctuation-only name yields an invalid path and a 500

**File:** `internal/pages/service.go:203-222` (CreateFolder)
**Issue:** `CreateFolder` rejects an empty/whitespace `name`, then computes
`dir := slugify(name)`. `slugify` drops every char outside `[a-z0-9-]`, so a
non-empty but punctuation-only name (e.g. `"!!!"`, `"***"`, `"##"`) slugs to `""`.
With `parent == ""` this makes `indexPath = "" + "/index.md"` → `"/index.md"`
(absolute), and with a parent it makes `parent + "//index.md"`. The resolver then
rejects it and the handler returns a generic 500 ("Something went wrong") instead
of the intended `ErrTitleRequired` 400 ("Give your folder a name"). Note `Create`
(page) already guards this exact case with a `base = "untitled"` fallback
(`service.go:262-264`); `CreateFolder` is missing the parallel guard.
**Fix:** Add the same empty-slug fallback used by `uniquePath`:
```go
dir := slugify(name)
if dir == "" {
    return ErrTitleRequired // or dir = "untitled" to match page-create behavior
}
if parent != "" {
    dir = strings.TrimSuffix(parent, "/") + "/" + dir
}
```
Returning `ErrTitleRequired` gives the clean 400; a silent `"untitled"` also
works but should match whatever `Create` does for consistency.

### WR-08: Move/relocate stages an unclean oldPath and trusts a raw newParent path — traversal-shaped move targets bypass the slug-based path safety

**File:** `internal/pages/rename.go:57-86` (Move), `92-127` (relocate)
**Issue:** Unlike `Create`/`CreateFolder` (which only ever build paths from
`slugify` output), `Move` accepts `newParentDir` directly from the request body
and only trims slashes/spaces (`rename.go:67`). It then builds
`newPath = newParentDir + "/" + base`, `path.Clean`s it, and passes it to
`uniqueExactPath` → `relocate`. The only safety net is `repo.Resolve` inside
`uniqueExactPath`/`enqueueWrite`. A `newParent` such as `"../../etc"` cleans to a
parent-escaping path; resolution *should* reject it, but the rejection surfaces as
a generic 500 rather than a validated 400, and the design relies entirely on the
resolver being airtight for an attacker-controlled multi-segment path — the
handler does NOT run `cleanPathString` on `newParent` (it only cleans the source
`path`). `relocate` also stages `oldPath` into `Spec.Paths` without re-cleaning it.
This is defense-in-depth: a single resolver regression would turn into a write
primitive.
**Fix:** Validate `newParent` at the handler or service boundary the same way page
paths are validated (reject `..` segments, absolute, NUL) before constructing
`newPath`, e.g. reuse `cleanPathString` for `new_parent` in `handleRenamePage`, or
add an explicit segment check in `Move` mirroring `cleanPathParam`. Fail with a 400
rather than relying solely on `repo.Resolve` to 500.

## Info

### IN-01: Folder-create audit target is malformed for root-level folders

**File:** `internal/server/handlers_pages.go:319-324`
**Issue:** The audit `Target` is built as `req.Parent + "/" + req.Name`. For a
root-level folder `req.Parent == ""`, producing a leading-slash target
`"/myfolder"` that does not match the actual created path (`myfolder/index.md`).
Minor audit-trail accuracy defect (not a security issue — audit is non-fatal).
**Fix:** Build the target consistently, e.g.
`target := strings.Trim(req.Parent+"/"+req.Name, "/")`, or have `CreateFolder`
return the created dir path and audit that.

### IN-02: readField/setField interpolate the field name into a RegExp without escaping

**File:** `web/src/lib/frontmatter.ts:21` (readField), `30` (setField)
**Issue:** `new RegExp(\`^${field}:...\`)` interpolates `field` unescaped. All
current callers pass literals (`"title"`, `"description"`), so this is not
currently exploitable, but it is a latent bug: a field containing regex
metacharacters would build a malformed or surprising pattern, and it is an easy
footgun for the next caller.
**Fix:** Escape the field before interpolation:
```ts
const esc = field.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
const re = new RegExp(`^${esc}:\\s*(.*)$`, "m");
```
Apply in both `readField` and `setField`.

### IN-03: Dead branch in isAbsoluteOrExternal — protocol-relative `//` is unreachable

**File:** `internal/okf/links.go:413-417`
**Issue:** The check `if dest[0] == '/' { return true }` (line 413) returns before
the protocol-relative test `len(dest) >= 2 && dest[0] == '/' && dest[1] == '/'`
(line 416) can ever be reached — any string whose first byte is `/` already
returned. The `//` branch is dead code. (Behavior is still correct — `//foo` is
treated as absolute/external either way — but the intent is misleading.)
**Fix:** Remove the unreachable `//` branch, or fold it into a comment on the
existing `dest[0] == '/'` check noting it covers both absolute and
protocol-relative.

### IN-04: ViewVersion returns the LIVE HEAD revision for a historical version — restore-from-view can never 409

**File:** `internal/pages/history.go:107-115` (ViewVersion), consumed by `web/src/components/HistoryPanel.tsx`
**Issue:** `ViewVersion` returns `Page.Revision = s.Revision(ctx, path)` — the
*current HEAD* blob revision, not the viewed historical version's identity. This
is intentional per the doc comment (so an editor still writes against HEAD), but
it means the `Revision` field is semantically ambiguous on a version-view
response: the same value is returned regardless of which old version is being
viewed, and any client that treated a version view as an editable base would write
against HEAD silently. Currently the History panel only renders the version
read-only (`HistoryPanel.tsx:124-128`), so there is no live bug — flagging for the
contract clarity since restore-from-history is in scope.
**Fix:** Either document on the API type that `revision` on a version response is
always the live HEAD (not the viewed version), or omit/zero `revision` for the
read-only version-view path so a future caller cannot mistake it for an editable
base.

---

_Reviewed: 2026-06-19T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
