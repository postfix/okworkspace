---
author: architect
responsible: architect
phase: 29
phase_type: implementation
status: Ready for planning
hard_bar: true
security_relevant: true
design_fork: false
parent_artifacts:
  - .planning/ROADMAP.md
  - .planning/REQUIREMENTS.md
---

# Phase 29: Tag-Regression Fix (server-authoritative) - Context

**Gathered:** 2026-07-01
**Status:** Ready for planning

<domain>
## Phase Boundary

Make workspace tag **rename / merge / delete** durable against v1.4's inline-`#tag`
materialization by rewriting affected pages' body `#tags` **server-side, in the same
byte-stable batched commit** as the frontmatter change — via a new code-region-aware
`okf.RewriteHashtags` primitive proven in lock-step with the TS Lezer grammar. This closes
the confirmed regression at the source for **all future operations**, including pages never
reopened after the operation. Historical corruption already written by pre-fix ops is handled
by TR-06 (detect+report), **not silently claimed away**.

**Requirements:** TR-01, TR-02, TR-03, TR-04, TR-05, TR-06

**Non-goals:** a second full Markdown parser in Go (`RewriteHashtags` is a conservative,
corpus-pinned `#tag`-boundary scanner like `RewriteLinks`, NOT a general Lezer re-impl);
removing the client `reconcileFrontmatterTags` (the frontmatter-canonical sync-at-save model
ED-02b stays — the server fix makes it a natural no-op on renamed pages); new tag features
(bulk multi-tag, metadata, hierarchy); touching the Phase-28 reading-shell surface.

**This phase builds on committed HEAD** — Phase 28 lands the reading-shell fold-in as a clean
baseline first, so these surgical commits are isolable and independently verifiable.
</domain>

<decisions>
## Implementation Decisions

### Build order (risk-first, within the phase) — non-negotiable ordering
- **D-01:** The central risk is the Phase-21-warned "second `#tag` parser" drift. It is retired
  FIRST, before any handler depends on the primitive, in this order:
  0. **`ValidateInlineTag` validator FIRST** (TR-03 blocker-fix) — a strict inline-`#tag`
     grammar-subset validator gates rename `new_name` AND merge `target` at the handler (400 on
     fail). Nothing may splice an invalid token into a body.
  1. **TR-01 + TR-02 together** — land `okf.RewriteHashtags` AND its offset-level, whole-document
     Lezer-parity test in the same step. The parity test is the anti-drift guarantee; the
     primitive is not "done" until the test pins it.
  2. **TR-04** — body-inclusive enumeration in the **pages service** (reads bodies through the
     SEC-01 resolver), NOT in `graph/query.go`.
  3. **TR-03** — wire the primitive + enumeration into rename/merge/delete (widened change
     predicate + body channel through the batch).
  4. **TR-05** — surface body-rewrite scope in the preview/admin UI.
  5. **TR-06** — stale-body remediation (detect+report) for pre-fix historical corruption.

### The validator (D-02, TR-03 blocker)
- **D-02:** `okf.ValidateInlineTag(tag string) error` is a NEW validator (in `internal/okf/`,
  patterned on `repair.go:ValidateTagCharset`) enforcing the inline-`#tag` grammar SUBSET the
  Lezer body grammar can represent: body alphabet `[A-Za-z0-9_/-]`, **≥1 non-digit**, **no
  space**, nested `#a/b` slash policy (slash separates segments; not leading/trailing/doubled;
  a tag is never all-digits-and-slashes). It is applied to rename `new_name` AND merge `target`
  BEFORE any write (400 at the handler). **The existing `ValidateTagCharset` (`^[a-zA-Z0-9_ -]+$`,
  repair.go:159) is INSUFFICIENT and must NOT be the sole gate:** it permits a space and an
  all-digit token and forbids `/` — the exact inverse of what body injection needs. `ValidateInlineTag`
  is a stricter, additional gate; `ValidateTagCharset`'s existing role (frontmatter charset,
  SEC-02 HTML-metachar defense) is unchanged. Merge and delete today call **no** validator at the
  handler (handlers_tags_admin.go:318,382) — merge MUST now validate `target`; delete needs no
  new-token validation (it only removes).

### The primitive (D-03, TR-01/TR-02)
- **D-03:** `okf.RewriteHashtags(body []byte, old, new string) ([]byte, bool)` — `new==""` means
  delete. Modeled on `okf.FindLinks`/`RewriteLinks` (links.go:21-83,337) which already skips
  fenced (``` / ~~~) + inline-code + `\#`-escape. `RewriteHashtags` must skip the **FULL** Lezer
  exclusion set — those THREE **plus** indented (4-space/tab) code blocks, HTML blocks, and the
  ATX-heading leading `#`. Tag-boundary recognition reuses the exact predicates the grammar
  documents (`hashtag.ts:44-68`: `isWs` start boundary, `isBody` alphabet, `isNonDigit` ≥1 rule,
  trailing-`/` trim). **Conservative / skip-when-uncertain:** if a `#` run's region can't be
  confidently classified, leave it UNCHANGED — a miss leaves a stale tag (self-healing on the next
  correct op), never a corrupted code region or malformed doc. Byte-identical on no-match.
- **D-03a (delete whitespace rule, chosen):** delete removes `#`+token and collapses exactly ONE
  adjacent space **only when the token was space-bounded on both sides**; never touches whitespace
  otherwise; never merges across a newline. Corpus-tested (`see #ops here`, `foo #ops.`, a
  `#ops`-only line).
- **D-03b (merge adjacent-duplicate rule, chosen):** a merge yielding `#b #b` leaves both tokens
  as-is (byte-safer than collapsing; the frontmatter reconcile dedupes the set anyway).

### The parity test (D-04, TR-02) — the anti-drift contract
- **D-04:** A shared, checked-in **whole-document corpus fixture** consumed by BOTH the Go test and
  a TS side, with expected results **generated by the real Lezer parser** (not a hand-transcribed
  name list). The Go test asserts **byte offsets / spliced output**, not just tag-set membership —
  proving the Go scanner splices the correct span (trailing-`/` trim, nested `#a/b`, adjacent
  punctuation), since `inlineTagsFromBody` returns names WITHOUT offsets. The fixture covers the
  full `hashtag.test.ts` positive/negative corpus PLUS block contexts (indented code, HTML blocks,
  tables, lazy blockquote continuation, `>#tag` block-marker abut). **A Lezer-corpus addition the
  Go scanner would miss must fail CI** — single-sourced corpus is the mechanism. The planner
  decides the exact single-source shape (a JSON fixture emitted by a small Lezer-driven generator
  vs. a committed fixture + a TS test that regenerates-and-diffs); either satisfies "single-sourced,
  CI-enforced."

### Enumeration (D-05, TR-04)
- **D-05:** The body-inclusive affected-set scan lives in the **pages service** (which already reads
  bodies through the SEC-01 resolver — `service.go:Get`, `tree.go:Tree`, the `templates.go:42`
  `Tree()`+`Resolve` SEC-01 pattern), NOT in `internal/graph/query.go` (whose "reads NO bodies —
  only path enumeration" invariant, query.go:256, must hold). It reuses the SAME resolver + skip
  rules as the graph rebuild (live `.md` only; no trashed/non-`.md`; no path escape). One-shot
  per-operation scan (never per-keystroke). It finds a page whose `#tag` is body-only (absent from
  frontmatter → invisible to `PagesByTag`/`page_tags`). A merge whose source is in page A's body and
  target in page B's frontmatter resolves both.

### Wire-in coupling (D-06, TR-03) — the load-bearing blocker
- **D-06:** Two coupled changes, both blockers, or body-only pages are enumerated then silently
  dropped:
  1. **Widened change predicate:** `buildTagApplyItems` (handlers_tags_admin.go:221) today appends
     an item ONLY when the frontmatter tag set `changed`. It must become
     `(frontmatterChanged OR bodyWillChange)`, where `bodyWillChange` is computed by running
     `RewriteHashtags` on the batch's **freshly-read** body.
  2. **Body channel through the batch:** `pages.TagApplyItem` + `ApplyTagsBatch` (batch.go:47,133-137)
     today re-emit `doc.Body` **verbatim**. They gain a body-rewrite step keyed to the op so, per
     affected page, ONE write updates body (via `RewriteHashtags`) AND frontmatter (via
     `SetTagsFrontmatter`), committed through the single-writer path (N pages → 1 commit). The
     `bodyWillChange` predicate MUST be computed from the SAME freshly-read body the batch rewrites
     (avoid a stale-read TOCTOU where the handler's read and the batch's read disagree).
- **D-06a:** Byte-stability preserved: only the tags-region frontmatter lines and the matched body
  `#tag` bytes change; all other bytes identical. Per-page 409 (`ErrStaleRevision`, service.go:30)
  still excludes a stale page without sinking the batch; audit-log entries (`ActionTagRename/Merge/
  Delete`) still recorded, now reflecting body+frontmatter scope.

### TR-06 disposition (D-07) — honesty about historical corruption
- **D-07:** TR-06 is scoped to **detect + report** (NOT auto-fix) unless the planner finds a
  bounded, provably-safe auto-remediation. A one-shot workspace scan flags pages whose body `#tags`
  disagree with frontmatter in a way indicative of a pre-fix rename/delete (a body `#tag` whose
  frontmatter counterpart a prior op removed). The ROADMAP's "for all pages including never-reopened"
  is thereby softened to **"for all FUTURE operations"** + surfaced historical set — the corruption
  is reported, never silently left. Whichever path the planner chooses is stated in the plan.

### The agent's Discretion
- The exact single-source mechanism for the D-04 parity corpus (generator vs regenerate-and-diff).
- The `RewriteHashtags` internal structure (reuse/refactor of the `links.go` fence/inline/escape
  skip helpers vs. a parallel scanner) — as long as the exclusion set is complete and conservative.
- Whether TR-06 is a CLI/admin one-shot or an admin-panel surface (detect+report either way).
- Commit slicing within the risk-first order (validator+primitive+parity as one or two commits, etc.).
</decisions>

<disciplines>
## Lenses surfaced for this phase

> **Lenses, not rules.** Each entry is a perspective the architect surfaced. Downstream agents adopt, refine, or reason past them; divergence-with-reasoning is a successful interaction.

~~~yaml
disciplines:
  - section: "TR-01/TR-02"
    name: "offset-parity-not-set-parity"
    scope: ["internal/okf/**", "the parity test"]
    guidance: "The Go scanner and the TS Lezer grammar must agree on the SPLICED
      OUTPUT at the byte-offset level over whole documents — not merely on the SET of
      tag names. inlineTagsFromBody returns names without offsets, so the parity test
      must reconstruct/assert spans. A test that only compares tag-name sets does NOT
      catch a scanner that trims the wrong bytes."
    rationale: "The regression the milestone fixes is a byte-level body edit. A
      set-equal test would pass while the scanner corrupts adjacent bytes (trailing
      slash, nested segment, punctuation). Offset parity is the actual anti-drift guarantee."
    override_signals:
      - "A genuinely offset-free assertion is provably sufficient for a specific case
        (rare) — state why in the test."
  - section: "TR-01"
    name: "conservative-skip-when-uncertain"
    scope: ["okf.RewriteHashtags"]
    guidance: "When region classification is uncertain, SKIP (leave unchanged). A
      missed rewrite leaves a stale tag that self-heals on the next correct op; an
      over-eager rewrite corrupts a code region or produces a malformed document. Bias
      hard toward under-rewriting."
    rationale: "Corruption is unrecoverable and silent; a stale tag is visible and
      self-correcting. The asymmetry dictates the bias."
  - section: "TR-03/TR-04"
    name: "no-second-body-reader-in-graph-layer"
    scope: ["internal/graph/query.go"]
    guidance: "The body-inclusive enumeration goes in the pages service, through the
      SEC-01 resolver. graph/query.go's 'reads NO bodies — only path enumeration'
      invariant (query.go:256) MUST hold — do not bolt a body read into the lean graph
      query path."
    rationale: "The graph layer's request-path leanness is a deliberate invariant;
      breaking it to save a few lines trades a load-bearing property for convenience."
  - section: "security"
    name: "untrusted-tag-into-prose"
    scope: ["ValidateInlineTag", "RewriteHashtags", "the enumeration read path"]
    guidance: "A rename/merge target is untrusted input that gets SPLICED INTO PROSE.
      The validator (charset subset) + the conservative scanner (region-aware, never
      breaks out of a code region or injects Markdown) + the SEC-01-resolver read path
      (no traversal, no trashed/non-.md, no bypass) are the three guards. This is the
      surface Phase 30's security review will audit."
    rationale: "This is the milestone's new untrusted-input surface; v1.3 never got a
      security review, and body injection is a strictly larger surface than frontmatter."
~~~

</disciplines>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase spec & requirements
- `.planning/ROADMAP.md` §"Phase 29: Tag-Regression Fix" — goal, build order, success criteria, key files.
- `.planning/REQUIREMENTS.md` §TR-01…TR-06 — acceptance criteria (the authoritative bar).
- `.planning/STATE.md` §"Accumulated Context" — the confirmed-regression summary + substrate note (SMTC offline).

### The pattern to model (byte-scanner)
- `internal/okf/links.go:21-83` (`FindLinks` — structural byte scanner, skips fenced/inline/escape),
  `:337-386` (`RewriteLinks` — splice only matched bytes, byte-stable), `:85-194` (fence/inline-code
  skip helpers to reuse or mirror).

### The grammar to pin against (parity source of truth)
- `web/src/lib/cm/hashtag.ts:44-68` — `isWs`/`isBody`/`isNonDigit`/`scanBody` (the exact boundary
  predicates); `:106-133` — the Lezer extension set + `tagParser`; `:135-150` — `inlineTagsFromBody`.
- `web/src/lib/cm/hashtag.test.ts` — the positive/negative + adversarial corpus the fixture extends.

### The write surface to wire into
- `internal/server/handlers_tags_admin.go:194-230` (`buildTagApplyItems` — the widened predicate),
  `:243-315` rename, `:317-379` merge, `:381-440` delete (validators + compute funcs).
- `internal/pages/batch.go:47` (`TagApplyItem`), `:77-187` (`ApplyTagsBatch` — the `doc.Body`
  verbatim passthrough at :133-137), `:214-240` (`SetTagsFrontmatter`).
- `internal/pages/service.go:30,208-237` (`ErrStaleRevision` 409 floor), `:174` (`Get`).
- `internal/pages/tree.go:36` + `internal/pages/templates.go:38-46` (the `Tree()`+`Resolve` SEC-01
  enumeration pattern for TR-04).
- `web/src/lib/tag-reconcile.ts` — the client re-materialization the regression rides (the golden-test
  counterparty for TR-03 crit 5).

### Preview UI
- `web/src/components/admin/TagManagement.tsx:100-157` (rename), `:163-224` (merge), `:231-290`
  (delete) — the `affected_pages`/`total_affected` preview to extend with body scope.
- `web/src/api/client.ts` — the `renameTag`/`mergeTag`/`deleteTag` client + `TagPreview` type.

### No external specs
- No third-party specs/ADRs — requirements fully captured above.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `okf.FindLinks`/`RewriteLinks` (links.go) — the exact structural-byte-scanner pattern: skip
  code regions, splice only matched bytes, byte-stable on no-match. `RewriteHashtags` mirrors this
  with a LARGER exclusion set.
- `hashtag.ts scanBody` (`:70-88`) — the canonical body-alphabet + trailing-slash + ≥1-non-digit
  logic to port to Go faithfully.
- `SetTagsFrontmatter` (batch.go:214) — the byte-stable frontmatter tags-region write already shared
  by the per-page apply and the batch; the body channel sits ALONGSIDE it in the same write.
- `pages.Service` SEC-01 resolver reads (`Get`, `Tree`, templates.go's `Tree()`+`Resolve`) — the
  vetted body-read path for TR-04 enumeration.

### Established Patterns
- Byte-stable single-writer batched commit (N pages → 1 commit) via `EnqueueCommit` on the
  single-writer worker (batch.go:161-174). The body rewrite rides this UNCHANGED — it only changes
  what bytes each page's `fileWrite` carries.
- Per-page 409 exclusion without aborting the batch (batch.go:115-118) — preserved.
- Go tests: table-driven, corpus fixtures under the package; `go test ./...`.

### Integration Points
- `buildTagApplyItems` compute funcs (`computeRenameTags`/`computeMergeTags`/`computeDeleteTags`)
  currently drive the FRONTMATTER change only; the body-rewrite decision is a NEW parallel computation
  from the freshly-read body inside the batch (D-06).
- The client `reconcileFrontmatterTags` (tag-reconcile.ts:51) is the regression's other half — the
  golden test drives BOTH: server rewrite THEN client reconcile on the next (unchanged-body) save →
  the renamed-away tag must NOT reappear.

### Landmines
- **TOCTOU on the body read (D-06):** the handler's `pages.Get` read (for the preview) and the batch's
  `s.repo.Read` (at commit) are separate reads. The `bodyWillChange`/`base_revision` must be coherent —
  compute the body rewrite from the batch's freshly-read body, and let the existing `base_revision`
  409 floor reject a page that moved between preview and apply.
- **`page_tags` is frontmatter-only** — a body-only `#tag` is invisible to `PagesByTag` (the TR-04
  reason enumeration must be widened; do NOT try to fix this in the graph layer).
- **`ValidateTagCharset` is the WRONG gate for body injection** (permits space/all-digit, forbids
  slash). Add `ValidateInlineTag`; do not repurpose or loosen `ValidateTagCharset`.
- **The parity test must assert offsets, not just name sets** (see the discipline lens) — a set-equal
  test is a false-safe.
- SMTC daemon OFFLINE this session — every code claim here is grep/read confidence (Structural/Honest
  tier), never Decision-tier. The planner re-grounds line numbers against the Phase-28-committed HEAD.
</code_context>

<specifics>
## Specific Ideas

- The Go `RewriteHashtags` should share the fence/inline-code/escape skip helpers with `FindLinks`
  where clean, and ADD: indented-code (4-space/tab line-start), HTML-block, and ATX-heading `#`
  skips. The corpus already asserts indented-code exclusion (`\t#deploy → []`) so the Go side must
  match.
- The regression golden (TR-03 crit 5): rename `#ops`→`#devops` on a page with body `#ops` →
  server rewrites body to `#devops` + frontmatter `[devops]`; then feed the (unchanged) rewritten
  body through `reconcileFrontmatterTags` → byte-stable, `ops` does NOT reappear. Mirror for merge,
  delete, and a body-only page. Plus the stale-open-client 409 integration test.
- Preview (TR-05): the `affected_pages` entries need a body-affected flag/marker so the admin sees
  "prose will change on these pages," not just the frontmatter `current → new` diff.

## Deferred Ideas
- Auto-remediation of historical stale-body corruption beyond detect+report (TR-06) — only if the
  planner finds a provably-safe bounded fix; otherwise a Future item.
- A general Go Markdown parser — explicitly out of scope; the scanner is conservative + corpus-pinned.

*Phase: 29-tag-regression-fix*
*Context gathered: 2026-07-01*
