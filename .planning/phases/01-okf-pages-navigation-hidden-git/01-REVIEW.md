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
  - web/src/routes/AppShell.tsx
  - web/src/routes/PageEditor.tsx
  - web/src/routes/PageView.tsx
  - web/src/stores/recent.ts
findings:
  critical: 2
  warning: 7
  info: 4
  total: 13
status: issues_found
---

# Phase 1: Code Review Report

**Reviewed:** 2026-06-19
**Depth:** standard
**Files Reviewed:** 45
**Status:** issues_found

## Summary

Reviewed the OKF round-trip model (`internal/okf`), the page lifecycle service and
its single-writer commit path (`internal/pages`), the git store reads/history/push,
the chi HTTP surface, and the React/TS frontend. The hardened areas the scope note
flagged hold up well: the SEC-01 path resolver is consistently routed through
(`repo.*`), the argv flag-smuggling guard on the version token is enforced in both
`gitstore` and `pages`, the hidden-Git contract is respected (no SHA leaks to the
wire), Markdown render keeps raw HTML off (`rehype-sanitize`, no `dangerouslySetInnerHTML`),
and mutating routes are editor-gated from the session role.

The defects worth blocking on are correctness gaps rather than the obvious security
sinks: a **route-dispatch collision** that mis-handles any page living under a folder
named `version` (CR-01), and **client-emitted frontmatter that produces invalid YAML**
for a class of ordinary titles, which the server then rejects with a 500 and which can
corrupt the title field (CR-02). Several WARNING-level consistency and navigation bugs
follow.

## Critical Issues

### CR-01: Page paths containing a `version` (or `history`) folder segment are mis-dispatched

**File:** `internal/server/handlers_pages.go:98-110`, `internal/server/handlers_history.go:69-89`
**Issue:** `handleGetPageOrHistory` routes the `/pages/*` catch-all purely on
substring/suffix matching of the wildcard:

```go
case strings.HasSuffix(wild, "/history"):  // -> history
case strings.Contains(wild, "/version/"):  // -> view-version
default:                                    // -> plain read
```

A legitimate page whose repo-relative path contains a `version` folder segment —
e.g. `docs/version/notes.md` — produces `wild = "docs/version/notes.md"`, which
satisfies `strings.Contains(wild, "/version/")`. It is therefore routed to
`handleViewVersion`, which then computes `pathPart = "docs"` and
`version = "notes.md"`. `validVersionToken("notes.md")` fails, so the user gets a
generic 500/400 and **can never read or edit that page**. There is no guard that the
folder names `version`/`history` are reserved, and `slugify` happily produces them
(`slugify("Version") == "version"`). This silently bricks a content path.

**Fix:** Do not overload the read wildcard with suffix/substring dispatch on
user-controlled path segments. Anchor the version/history sub-resource on a token the
content namespace cannot collide with — e.g. require the suffix to be the *final two*
segments AND that the remaining path ends in `.md`:

```go
func (h *authHandlers) handleGetPageOrHistory(w http.ResponseWriter, r *http.Request) {
	wild := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	if base, ok := strings.CutSuffix(wild, "/history"); ok && strings.HasSuffix(base, ".md") {
		h.handleHistory(w, r); return
	}
	if i := strings.LastIndex(wild, "/version/"); i >= 0 && strings.HasSuffix(wild[:i], ".md") {
		h.handleViewVersion(w, r); return
	}
	h.handleGetPage(w, r)
}
```

Apply the same `.md`-anchored check in `handleRenamePage` for the `/rename` and
`/restore` suffixes (a page named so its path ends `…/restore` is impossible because of
`.md`, but a `restore`/`rename` folder segment is not the issue there — the suffix form
is safe; the `version` substring is the dangerous one). Add a reserved-segment reject in
`slugify`/`uniquePath` as defense in depth.

### CR-02: Client `frontmatter.ts setField` emits invalid YAML for ordinary titles, breaking save and corrupting the title field

**File:** `web/src/lib/frontmatter.ts:49-55` (`quoteIfNeeded`), used by `setField` (29-37)
**Issue:** When the editor changes the title/description, the SPA patches the raw YAML
text client-side and PUTs it. `quoteIfNeeded` only quotes when the value contains
`[:#"']`:

```js
function quoteIfNeeded(value: string): string {
  if (value === "") return '""';
  if (/[:#"']/.test(value)) { return `"${value.replace(/"/g, '\\"')}"`; }
  return value;
}
```

This leaves many YAML-significant inputs unquoted and therefore mis-parsed by the
server (`okf.Parse` → `yaml.Unmarshal`):

- A title starting with `[` or `{` (e.g. `[Draft] Plan`) → parsed as a YAML flow
  sequence/map → **parse error** → `Save` returns the generic 500 and the page cannot
  be saved.
- A title starting with `>`, `|`, `&`, `*`, `!`, `@`, `` ` `` → block-scalar / anchor /
  reserved indicators → mis-parse or error.
- A title with a leading/trailing space → silently trimmed on round-trip (data loss).
- A value containing `\` → the server-side double-quoted parse interprets it as an
  escape; combined with the client's own `\"` escaping this is lossy.

Because `okf.Field` then reads back a different (or empty) title, the navigation tree
and page header can silently show the wrong/blank title even when the save *does*
succeed. The byte-stable round-trip guarantee is defeated at its client entry point.

**Fix:** Quote far more conservatively (match YAML's plain-scalar rules), or — better —
do not hand-roll YAML on the client at all. Either:
1. Quote whenever the value is non-empty and not a safe plain scalar
   (`/^[A-Za-z0-9][\w .-]*$/` style allow-list), and emit double-quoted with full JSON
   escaping (`JSON.stringify(value)` produces a valid YAML double-quoted scalar); or
2. Send the structured title/description fields to the backend and let `okf.SetField`
   (already byte-safe, server-side) write them, rather than splicing raw YAML in the
   browser.

## Warnings

### WR-01: Trash/restore mutate SQLite synchronously but the file move is asynchronous — DB and Git can diverge

**File:** `internal/pages/trash.go:91-104` (Delete), `internal/pages/trash.go:188-195` (Restore)
**Issue:** `Delete` enqueues the move commit (async, runs later in the worker drain
goroutine) and then **synchronously** inserts the trash row. `Restore` enqueues the
restore commit and then **synchronously** `DELETE`s the trash row. If the enqueued
CommitJob later fails (git error, resolver reject, disk full), the SQLite side has
already been mutated:
- Failed delete: trash row exists but the page was never moved → `Restore` later reads
  a `trash_path` that does not exist (`repo.Read` error), and the live page still
  appears in the tree.
- Failed restore: the trash row is gone but the file was never written back → the page
  is unrecoverable from the UI even though its bytes still sit in `.okf-workspace/trash/`.

There is no compensating action and no transactional coupling between the job queue and
the metadata table.

**Fix:** Record the trash row (and delete it on restore) from inside the CommitJob
handler after `g.Commit` succeeds, or make the metadata write idempotent and reconcile
on startup. At minimum, document and handle the failure path so the two stores cannot
silently disagree.

### WR-02: MarkdownProse resolves only one level of `../` in internal links, breaking deep relative navigation

**File:** `web/src/components/MarkdownProse.tsx:24`
**Issue:** Internal `.md` links are rewritten to in-app routes via:

```js
const target = href!.replace(/^\.?\//, "").replace(/^\.\.\//, "");
```

This strips at most one leading `./` and one leading `../`. The canonical link format
(produced by `relativeMdLink`) emits multi-segment relatives like
`../../runbooks/deploy.md` for cross-folder links. After the replace, `target` is still
`../runbooks/deploy.md`, so the SPA navigates to `/app/page/../runbooks/deploy.md` —
a broken route that does not resolve to the linked page. Deep relative links (the common
case for a nested wiki) navigate to the wrong place or 404, defeating D-06.

**Fix:** Resolve the relative destination against the current page's directory properly
(mirror the server `relPath` / a `path.posix`-style join), not with two `replace`s. Use
the current route path + the destination to compute the absolute repo-relative target,
then route to `/app/page/<resolved>`.

### WR-03: Overlapping autosave timers can self-conflict and surface a spurious "changed elsewhere" banner

**File:** `web/src/routes/PageEditor.tsx:91-96, 57-87`
**Issue:** `scheduleAutosave` arms *both* a 1s draft timer and a 6s idle timer, each
calling `doSave`. `doSave` is not guarded against concurrent/overlapping invocations and
holds no in-flight lock. If the 1s draft save is still in flight (slow network/commit)
when the 6s idle timer fires, the second `doSave` reads `baseRevision.current`, which has
not yet advanced (it only updates after the first save's refetch completes). The second
save then races the first; once the first commit lands, the second save's
`base_revision` is stale and the backend returns 409 → the editor shows the
"This page was changed somewhere else" conflict banner to a single user editing alone.
There is also no dirty-check: the idle timer can fire a no-op version save with the same
bytes.

**Fix:** Track an in-flight flag (or use a single serialized save queue / react-query
mutation with `isPending` gating) so a new autosave never starts while one is running;
re-run once after the in-flight save resolves if the content changed. Skip the save when
the content is unchanged since the last successful save.

### WR-04: `handleViewVersion` / `RestoreVersion` accept any hex token for any path — cross-page version read

**File:** `internal/pages/history.go:95-116, 129-162`; `internal/gitstore/history.go:134-153`
**Issue:** `ViewVersion`/`RestoreVersion` validate that `version` is a hex object name
and that `path` resolves safely, then call `git show <ref>:<path>`. They never verify
that `ref` actually belongs to *this page's* history. A caller who knows (or guesses) a
40-char commit SHA can read the bytes of `path` at an arbitrary commit, including one in
which `path` was different content, or pair a SHA harvested from one page's history with
another page's path. Tokens are opaque and not enumerable from the UI, so exposure is
limited, but the authorization model is "valid hex + safe path" rather than "this token
is a version of this page." For `RestoreVersion` this lets a forward-commit be built from
an unrelated tree state.

**Fix:** Constrain the token to the page's own history: confirm `ref` appears in
`History(ctx, path)` (or that `git show ref:path` corresponds to a commit returned by
`git log --follow -- path`) before reading/restoring. Cache the page→tokens mapping if
the extra `git log` is a concern.

### WR-05: `Push` swallows all transport/auth failures as a hard error but divergence as success — silent staleness risk

**File:** `internal/gitstore/push.go:18-41`, `internal/pages/commitjob.go:91-95`
**Issue:** On a push failure that is *not* classified as non-fast-forward,
`CommitHandler` returns the error from `g.Push`, which fails the whole job *after the
local commit already succeeded and the response was already returned to the user*. The
user sees a successful save; the job then errors and (depending on the worker's retry
policy) may retry the entire commit batch — re-running `r.Write`/`g.Commit` against an
already-committed tree. Conversely, `isNonFastForward` is a substring match on
lowercased stderr (`"rejected"`, `"fetch first"`); an auth/transport error whose message
happens to contain "rejected" is misclassified as divergence and silently sets
`diverged` without alerting on the real cause.

**Fix:** Make push failures non-fatal to the (already-committed) job — log/alert via
Health and return nil, exactly as the divergence branch does — so a transient remote
problem never triggers a re-commit. Tighten `isNonFastForward` to match git's specific
non-ff phrasing rather than the broad `"rejected"` token.

### WR-06: `relativeMdLink` returns a bare filename for same-directory links, but `MarkdownProse` cannot round-trip the inverse for nested pages

**File:** `web/src/api/client.ts:367-382`, consumed by `web/src/components/LinkPicker.tsx:63-66`
**Issue:** For two pages in the same nested folder, `relativeMdLink("a/b/x.md","a/b/y.md")`
returns `"y.md"` (bare filename, correct on disk). But `MarkdownProse`'s link handler
(WR-02) routes a bare `y.md` to `/app/page/y.md` — the repo root — not `/app/page/a/b/y.md`.
So a link the LinkPicker itself inserts navigates to the wrong page whenever the editing
page is not at the repo root. This is the same root cause as WR-02 (no directory-aware
resolution) but is called out because the insert and render sides are inconsistent by
construction, so the feature is broken for any non-root page.

**Fix:** Resolve every internal link against the current page's directory in
`MarkdownProse` (see WR-02). After that fix, both bare and `../`-prefixed destinations
route correctly.

### WR-07: `slugify` collapses distinct titles to the same slug, and `.`/`/` are silently folded — surprising collisions

**File:** `internal/pages/service.go:293-312`
**Issue:** `slugify` maps `/`, `.`, `_`, spaces, and `-` all to a single hyphen and drops
everything else. Titles like `A/B`, `A.B`, `A B`, `A-B`, and `A_B` all slug to `a-b`,
silently colliding (then suffixed `-2`). More importantly a title that is entirely
non-`[a-z0-9]` (e.g. emoji-only, CJK-only, or `"..."`) slugs to `""`, and `Create`
falls back to `untitled` only in `uniquePath` — but `CreateFolder` (`service.go:208`)
calls `slugify(name)` directly with **no `untitled` fallback**, so a CJK-only or
symbol-only folder name produces `dir == ""` and `indexPath == "/index.md"`, which then
fails the resolver (or, if it didn't, would seed an index at the repo root). Non-ASCII
titles are common for an international team.

**Fix:** Apply the same empty-slug fallback in `CreateFolder` that `uniquePath` uses, and
consider transliterating or preserving Unicode letters/digits in `slugify` so non-ASCII
titles do not all collapse to `untitled`.

## Info

### IN-01: `CreateFolder` audit Target is malformed when parent is empty

**File:** `internal/server/handlers_pages.go:315`
**Issue:** `Target: req.Parent + "/" + req.Name` yields a leading-slash target like
`/Docs` for a root-level folder. Cosmetic, but the audit trail's target should match the
actual repo path.
**Fix:** Build the target the same way the service does (`path.Join(parent, name)` or skip
the separator when parent is empty).

### IN-02: `readField`/`setField` interpolate the field name into a RegExp without escaping

**File:** `web/src/lib/frontmatter.ts:21, 30`
**Issue:** `new RegExp(\`^${field}:\\s*(.*)$\`, "m")` trusts `field`. All current call
sites pass literals (`"title"`, `"description"`), so this is not currently exploitable,
but the exported signature invites a future caller to pass a field containing regex
metacharacters, producing a wrong match or a thrown error.
**Fix:** Escape `field` before building the pattern, or constrain it to a known set.

### IN-03: `isAbsoluteOrExternal` protocol-relative (`//host`) branch is dead code

**File:** `internal/okf/links.go:413-417`
**Issue:** The `dest[0] == '/'` check on line 413 returns true for any leading slash, so
the subsequent `len(dest) >= 2 && dest[0] == '/' && dest[1] == '/'` branch (lines 415-417)
is unreachable. The intent (treat `//host/path` as external) is already satisfied, but
the dead branch is misleading.
**Fix:** Remove the unreachable branch; the leading-slash check already covers it.

### IN-04: `ViewVersion` returns the *current* live revision as the version's revision

**File:** `internal/pages/history.go:107-115`
**Issue:** A viewed historical version carries `Revision: <current HEAD revision>`. This
is intentional (so the editor still writes against HEAD), and is documented, but it means
the frontend cannot tell from the payload that it is looking at an old, read-only
version — it could let the user "Save" stale bytes over HEAD if a code path reused this
Page for editing. Today the view path is read-only in the UI, so this is informational.
**Fix:** Consider a `readOnly`/`historical` flag on the version-view response so the
distinction is explicit rather than relying on caller discipline.

---

_Reviewed: 2026-06-19_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
