---
author: codebase-archaeologist
responsible: codebase-archaeologist
phase: 29
milestone: v1.5
status: verified
confidence: direct_read
grounding: direct file read (SMTC daemon offline this session — no semantic/blast-radius certs)
head: f03cffc
---

# Codebase Map: Server-Authoritative Tag-Rewrite Subsystem

## Subsystem: workspace-wide `#tag` rename/merge/delete that rewrites BODY + frontmatter

The v1.5 milestone's central new subsystem (Phase 29, TR-01..06). Before v1.5 the admin
rename/merge/delete ops (v1.3 / TM-01..03) rewrote only the frontmatter `tags:` key; the
inline `#tag` left in the page BODY went stale, and because the editor re-materializes body
`#tags` into frontmatter on the next save (ED-02b), the "renamed" tag silently came back.
This subsystem makes those ops **server-authoritative for the body**: a rename/merge/delete
rewrites the body `#tags` AND the frontmatter in ONE byte-stable batched commit, and the
affected-set enumeration is widened to include body-only pages.

The design mirrors the existing links subsystem: `okf.FindHashtags`/`RewriteHashtags` are the
`#tag` counterpart to `links.go`'s `FindLinks`/`RewriteLinks` (byte scanners, not Markdown
parsers, preserving the round-trip invariant).

### Files

| File | Role |
|------|------|
| `internal/okf/hashtag.go` | The server-side `#tag` scanner + rewriter (468 ln). `FindHashtags(body) []Hashtag` — the SINGLE region-aware recognizer (two-pass: `classifyBlocks` marks excluded ranges + line content-start offsets, then a `FindLinks`-style byte walk applies `hashStartOK` + `scanTagBody`). `RewriteHashtags(body, old, new) ([]byte, bool)` — byte-stable splice/remove of matched spans. `Hashtag{Start, End, Name}` byte-offset span type. Helpers: `classifyBlocks`, `htmlBlockAt`, `blockquoteContentStart`, `isATXHeadingLine`, `indentedCodeLine`, `scanTagBody`, `isBodyByte`/`isNonDigitByte`/`isWsByte`. |
| `internal/okf/validate.go` | `ValidateInlineTag(tag) error` — the body-splice gate: validates a BARE name against the inline-`#tag` grammar SUBSET (`[A-Za-z0-9_/-]`, ≥1 non-digit, no leading/trailing/doubled `/`). STRICTER than and DISTINCT from `ValidateTagCharset`. |
| `internal/okf/repair.go` | `ValidateTagCharset(tag)` (`^[a-zA-Z0-9_ -]+$`, permits space + all-digit, forbids `/`) — the pre-existing frontmatter charset gate, UNCHANGED. `SetTags` — byte-stable frontmatter tags-region editor. |
| `internal/pages/batch.go` | `ApplyTagsBatch` — the batched single-commit apply. v1.5 additions on `TagApplyItem`: `BodyOld`/`BodyNew` (body-rewrite intent) + `SkipFrontmatter` (body-only page). The **body channel**: when `BodyOld != ""`, rewrite `doc.Body` via `okf.RewriteHashtags` from the batch's OWN freshly-read body (TOCTOU-safe). `SetTagsFrontmatter` (shared byte-stable tags-region write). |
| `internal/pages/tagscan.go` | `PagesWithBodyTag(ctx, tag)` (TR-04 body-inclusive enumeration) + `DetectStaleBodyTags(ctx)` (TR-06 detect+report). Both use `okf.FindHashtags` and read bodies through the SEC-01 resolver. `bodyHasTag`, `isLivePage`, `frontmatterTags` (local YAML-node walk). |
| `internal/server/handlers_tags_admin.go` | The rename/merge/delete/list handlers. `buildTagApplyItems` (widened inclusion predicate + `body_affected` preview mark). The SEC-v1.5-F01 intersection gate (`ValidateInlineTag` ∩ `ValidateTagCharset` on rename/merge targets). |
| `internal/okf/hashtag_test.go` + `internal/okf/testdata/hashtag_corpus.json` | The Lezer↔Go offset-parity test + its single-sourced corpus (TR-02). |
| `internal/okf/validate_test.go` | `ValidateInlineTag` grammar-subset corpus + `TestValidateInlineTag_DoesNotLoosenCharset` (proves the two gates stay complementary). |
| `internal/pages/tagscan_test.go` | `TestPagesWithBodyTag` (body-only + fenced-exclude + trash-skip), `TestPagesWithBodyTag_CaseAndEmpty`, `TestDetectStaleBodyTags`. |
| `internal/pages/batch_bodyrewrite_test.go` | Body-rewrite goldens + 409 stale-page cases (per 29-SUMMARY). |
| `web/src/lib/cm/hashtag.parity.test.ts` + `web/src/lib/cm/hashtag.fixtures.ts` | The TS side of the parity contract: runs the REAL Lezer parser (`inlineTagSpansFromBody`) over the shared fixtures and regenerates the Go corpus JSON; drift FAILS CI on either side. |

### The single-recognizer invariant (no second `#tag` parser)

```
web/src/lib/cm/hashtag.ts (tagParserExtensions — Lezer grammar)
    │  render badges (livePreview.ts)  ── inlineTagsFromBody (materializer)  ── inlineTagSpansFromBody (byte spans)
    │                                                                             │
    │  hashtag.parity.test.ts runs the REAL parser over hashtag.fixtures.ts,     │
    │  regenerates ↓ (byte from/to spans)                                        │
internal/okf/testdata/hashtag_corpus.json  ←── asserted byte-equal ──┘
    │
internal/okf/hashtag_test.go: FindHashtags MUST match the corpus spans (offset parity, not name-set)
    │
internal/okf/hashtag.go: FindHashtags  ── the ONE Go recognizer, reused by:
    ├─ RewriteHashtags (hashtag.go)             — the body rewriter
    ├─ bodyHasTag / PagesWithBodyTag (tagscan)  — the affected-set enumeration
    └─ DetectStaleBodyTags (tagscan)            — the corruption report
```

`FindHashtags` recognizes a `#tag` exactly where the Lezer parse emits a `Hashtag` node, and
SKIPS every region the parse excludes: fenced code, inline code spans, indented (4-space/tab)
code, HTML blocks (CommonMark type-1/type-6), `\#` escapes, and the ATX-heading leading `#`
run. A `#` abutting a blockquote marker (`>#tag`) is a valid start (the `cx.offset` rule).
**Discipline: skip-when-uncertain** — an unclassifiable region leaves the `#` UNCHANGED (a
missed rewrite self-heals; an over-eager one corrupts a code region).

### The two rewrite rules (RewriteHashtags)

- **D-03a (delete whitespace):** removing a token collapses exactly ONE adjacent space, and
  ONLY when the token was space-bounded on BOTH sides (` #ops ` → ` `); otherwise whitespace
  is untouched, a newline is NEVER merged across.
- **D-03b (merge adjacent-duplicate):** a merge yielding `#b #b` leaves both tokens as-is (no
  dedupe — the frontmatter reconcile dedupes the set).

On no match, `RewriteHashtags` returns the ORIGINAL slice (byte-identical, same backing array —
`TestRewriteHashtags_NoMatchSameSlice`).

### The two validators (complementary, NOT interchangeable)

| Gate | Regex/rule | Space | All-digit | `/` | Role |
|------|-----------|-------|-----------|-----|------|
| `ValidateTagCharset` (repair.go) | `^[a-zA-Z0-9_ -]+$` | allow | allow | **reject** | frontmatter charset / SEC-02 HTML-metachar defense |
| `ValidateInlineTag` (validate.go) | `[A-Za-z0-9_/-]`, ≥1 non-digit, slash policy | **reject** | **reject** | allow (nested `a/b`) | body-splice safety (round-trips as the same badged tag) |

**SEC-v1.5-F01:** a rename/merge target is written to BOTH surfaces, so `handleTagRename` /
`handleTagMerge` require the INTERSECTION. A nested target `a/b` passes `ValidateInlineTag` but
fails `ValidateTagCharset` → **clean 400 at the gate**, not a 500 at the frontmatter write.
(`e0fedaa`; `TestTagRename_FrontmatterCharsetGate` / `TestTagMerge_FrontmatterCharsetGate`.)
Delete needs no target gate (the body op is a removal, `BodyNew==""`).

### The body channel in ApplyTagsBatch (byte-stable, TOCTOU-safe)

Per item, under `lockMutation` (serializes vs namespace mutations), for the whole batch:

```
Exists? no → notfound
Revision != it.BaseRevision → stale (excluded, batch continues)
raw = repo.Read; doc = okf.Parse(raw)
body = it.BodyOld != "" ? RewriteHashtags(doc.Body, BodyOld, BodyNew) : doc.Body   ← body channel
front = it.SkipFrontmatter ? doc.RawFront : SetTagsFrontmatter(RawFront, body, it.Tags)
out = emitForWrite(front, body)
bytes.Equal(out, raw) → applied (no-op guard: skip the write, still report applied)
else stage fileWrite → ONE commitPayload → EnqueueCommit (single-writer worker)
after commit lands: mark applied + enqueue index/graph upserts (fire-and-forget)
```

`BaseRevision` is CALLER-supplied and already server-sourced: the sweep-approve handler stages
it from `tag_suggestions` (T-12-07); the admin handlers read it fresh via `pages.Get`. The
batch trusts the field — it does not re-read the queue. A `stale`/`notfound` page never sinks
the batch. A non-nil error returns ONLY on infrastructure failure (read/emit/commit-enqueue).

### Affected-set enumeration (widened, TR-04)

The admin handler unions frontmatter pages with body-only pages:

```
pagePaths = uniqueStrings( graph.PagesByTag(tag)  ∪  pages.PagesWithBodyTag(tag) )
```

`PagesWithBodyTag` lives in the PAGES service (not `internal/graph/query.go`) BY DESIGN:
`query.go` holds a "reads NO bodies — only path enumeration" invariant (query.go:256). A
body-only `#tag` is invisible to the frontmatter-only `page_tags` cache (`PagesByTag`), so the
body scan must read bodies, which it does through the SEC-01 resolver (`repo.Tree()` +
`repo.Read`) with the graph-rebuild skip rules (`isLivePage`: live `.md` only, no
trashed/non-`.md`). Merge enumerates body-`#source` pages only, NOT body-`#target`.

### Preview `body_affected` (TR-05)

`buildTagApplyItems` includes a page when `changed || bodyWillChange`. It sets `SkipFrontmatter
= !changed` (body-only page → tags region byte-identical) and marks `BodyAffected` in the
`tagPreviewPage`. The preview response carries per-page `body_affected` (JSON `body_affected`);
`web/src/api/client.ts` types it (`body_affected?: boolean`) and `TagManagement.tsx` renders a
`prose` badge + a "N page(s) also have body #tags that will be rewritten" note.

## Undocumented invariants & landmines (per-claim confidence)

1. **`FindHashtags` is the ONE `#tag` recognizer server-side — reused by RewriteHashtags AND
   tagscan.** (HIGH — hashtag.go:70-73, tagscan.go:22-24, grep of `FindHashtags` callers.)
   Adding a second `#tag` scanner reopens the render⇄server drift the whole parity corpus
   exists to prevent. New body-`#tag` logic MUST consume `FindHashtags`.

2. **Offset parity is byte-level and CI-enforced on BOTH sides.** (HIGH — hashtag_test.go:43-72
   asserts `from`/`to`; hashtag.parity.test.ts:63-77 regenerates-and-diffs the JSON.) A grammar
   change committed to `hashtag.ts` but not to `hashtag_corpus.json` fails the TS side; a Go
   scanner that finds a different span fails the Go side. Regenerate the corpus with
   `UPDATE_CORPUS=1 npx vitest run src/lib/cm/hashtag.parity.test.ts` after an intentional
   grammar/fixture change — never hand-edit the JSON.

3. **`RewriteHashtags`/`FindHashtags` are byte scanners, NOT Markdown parsers.** (HIGH —
   hashtag.go:9-16.) They preserve the round-trip invariant by touching only matched `#tag`
   byte spans; prose, code regions, and other tags are byte-identical. The conservative
   skip-when-uncertain bias is the control against corrupting a code region.

4. **`PagesWithBodyTag`/`DetectStaleBodyTags` MUST stay out of `internal/graph/query.go`.**
   (HIGH — tagscan.go:16-20; query.go's "no bodies" invariant.) They read bodies; putting them
   in the graph query layer would violate its path-only-enumeration contract. They live in the
   pages service and route every read through the SEC-01 resolver.

5. **`ValidateInlineTag` and `ValidateTagCharset` are complementary — do NOT collapse them.**
   (HIGH — validate.go:14-18, `TestValidateInlineTag_DoesNotLoosenCharset`.) Each rejects what
   the other permits (space/all-digit vs `/`). A rename/merge target must pass BOTH; a
   frontmatter-only or body-only op uses the relevant one. Repurposing the weaker gate reopens
   SEC-v1.5-F01 (500 on `a/b`) or a body-injection hole.

6. **`DetectStaleBodyTags` is DETECT+REPORT only — no writer, no HTTP/CLI caller.** (HIGH —
   grep: only its test references it; tagscan.go:107-118.) It surfaces historical corruption
   (a body `#tag` absent from frontmatter — the pre-fix half-reverted signature); it NEVER
   auto-fixes. The ROADMAP "for all pages including never-reopened" over-claim was honestly
   softened to "for all FUTURE operations" + this surfaced set. Do not wire it to a mutating
   path without a deliberate decision.

7. **The batch body rewrite recomputes from the batch's OWN fresh read (TOCTOU-safe).** (HIGH —
   batch.go:145-153.) `BodyOld`/`BodyNew` are the op INTENT, not a precomputed body. The
   handler's `bodyWillChange` (buildTagApplyItems) is only for inclusion + preview; the
   authoritative rewrite happens in the batch against the freshly-read, revision-checked body.

8. **The no-op guard prevents byte-identical writes.** (HIGH — batch.go:173-179.) If neither
   frontmatter nor body changed, no `fileWrite` is staged, but the page is still reported
   `applied` (its state already matches). Prevents an empty-diff commit churn.

## Blast radius (direct-read estimate; SMTC unavailable this session)

- `okf.FindHashtags` → `RewriteHashtags`, `bodyHasTag`/`PagesWithBodyTag`, `DetectStaleBodyTags`.
  Its recognition is pinned to the Lezer grammar by the parity corpus — a change here without a
  matching `hashtag.ts` + corpus change fails CI. Highest-fan-out in the subsystem.
- `okf.ValidateInlineTag` → `handleTagRename`, `handleTagMerge` (target gate). A loosening is a
  body-injection security regression (route to security-engineer).
- `TagApplyItem` fields (`BodyOld`/`BodyNew`/`SkipFrontmatter`) → `ApplyTagsBatch` +
  `buildTagApplyItems`. Both admin (body channel on) and sweep-approve (body channel off,
  `BodyOld==""`) callers share the struct; defaults keep the sweep path unchanged.
- `tagPreviewPage.body_affected` → `client.ts` TS type + `TagManagement.tsx` render.
- `pages.PagesWithBodyTag` → the three admin handlers' affected-set union.

### Test corpus (where the invariants are pinned)

`internal/okf/hashtag_test.go` (+ `testdata/hashtag_corpus.json`), `internal/okf/validate_test.go`,
`internal/pages/tagscan_test.go`, `internal/pages/batch_bodyrewrite_test.go`,
`internal/server/handlers_tags_admin_test.go`, and the TS `hashtag.parity.test.ts` /
`hashtag.fixtures.ts`. Treat a parity-corpus failure as a render⇄server drift (or an
un-regenerated corpus), not a flaky test.
