---
author: architect
---

# Phase 22: Model-B Materialization + Tag UI — Context

**Gathered:** 2026-06-30
**Status:** Ready for planning

<domain>
## Phase Boundary

Carry the milestone's load-bearing decision: on **save**, reconcile body `#tags` into the
canonical frontmatter `tags:` **client-side**, as a **diff (add AND remove)** — reusing the
Phase 21 Lezer parse so there is **no second parser** and **zero indexer change**. Then replace
the `InlineTagEditor` chip field with a minimal read-only **badge bar** whose ✕ removes only
non-inline (LLM/imported) tags. Unblocks Phase 23 (LLM suggestions feed the same frontmatter set).

**In scope (ED-02b + ED-02c):**
- A code-region-aware `inlineTagsFromBody(body): string[]` (runs the shared Lezer parser, collects
  `Hashtag` nodes) — supersedes the Phase-21 `parseInlineTags` string scanner for materialization.
- Reconciliation woven into `runSaver` (PageEditor) just before the PUT.
- A `TagBadgeBar` component replacing `InlineTagEditor` in `PageEditor.tsx`.
- Removal (and deletion if orphaned) of `InlineTagEditor`.
- Golden reconciliation test (add + remove + LLM-tag-persists in one save) + badge-bar tests +
  PageEditor save-integration coverage.

**Out of scope (later / Future):**
- LLM tag suggestion UI/flow → **Phase 23** (ED-03a). This phase only makes the frontmatter set
  the canonical sink they will land in.
- Any server-side `#tag` parser / indexer change — **explicitly forbidden** (locked). Search,
  graph (`page_tags`), tag-sweep, v1.3 tag-management keep reading frontmatter ONLY.
- Rendering `#tag` in react-markdown (agent chat/diff) — Future.
</domain>

<decisions>
- **D-01 — Inline extraction = the SHARED Lezer parser (code-region-aware; the locked "no second
  parser" goal).** Add `inlineTagsFromBody(body: string): string[]` that runs the **same**
  `@lezer/markdown` extension set used by the CM6 render over the body string and collects
  `Hashtag` node text (`tree.iterate` / cursor; slice `from+1..to`; lowercase; de-dupe). Confirmed
  API: `import { parser } from "@lezer/markdown"` → `parser.configure(extensions).parse(body):
  Tree`. This is code-region/heading/escape-aware **by construction**, so materialization can
  never disagree with what badges on screen — which is the entire point of the frontmatter-
  canonical model and directly closes the Phase-21 `parseInlineTags` "render⇄scanner divergence"
  handoff (`21-VERIFICATION.md`).
  - **D-01b — One source of truth for the extension list.** Refactor `markdown.ts` to export the
    shared `[Table, Strikethrough, TaskList, Autolink, hashtagExtension]` array (or a pre-configured
    standalone `markdownTagParser`); BOTH the CM6 `markdownExtension` and `inlineTagsFromBody`
    consume it, so the parser can never fork between render and materialize.
  - The Phase-21 pure `parseInlineTags` scanner is **retired from the materialize path**; keep it
    only if a non-CM consumer still needs it, else delete it (minimalism — the code-reviewer gate
    confirms no live consumer).

- **D-02 — Reconcile in `runSaver`, before the PUT (PageEditor.tsx ~228–236).** Inside the saver
  loop, after capturing `sentBody = bodyRef.current` / `sentFrontmatter = frontmatterRef.current`
  and BEFORE `savePage`, compute the diff against the **last saved** body:
  ```
  prevInline = inlineTagsFromBody(lastSavedBody.current)
  curInline  = inlineTagsFromBody(sentBody)
  removedInline = prevInline − curInline
  reconciled = normalize( (readTags(sentFrontmatter) − removedInline) ∪ curInline )
  if (setChanged(readTags(sentFrontmatter), reconciled))
      sentFrontmatter = setTags(sentFrontmatter, reconciled)
  ```
  PUT the (possibly rewritten) `sentFrontmatter`; on success `lastSavedFrontmatter.current =
  sentFrontmatter` (the reconciled value) as the code already does. `lastSavedBody.current` is the
  prior saved body at this point (advanced only after the PUT) — so the diff window is exactly
  right: additions come from `curInline`, removals from `prevInline − curInline`, and an LLM tag
  that was never inline survives (`readTags − removedInline` keeps it). NOT a monotonic union.

- **D-03 — Byte-stability guard (criterion 4).** Only call `setTags` when the **tag SET actually
  changed** (case-insensitive compare of `readTags(sentFrontmatter)` vs `reconciled`). `setTags`
  rewrites the whole tags block (and normalizes flow→block form), so calling it on an unchanged set
  would churn bytes and could 409-thrash autosave — guard against it. When nothing changed, the
  save proceeds with the untouched frontmatter (true byte-stable round-trip).

- **D-04 — Normalization matches existing tag rules.** `reconciled` is trimmed, lower-cased, and
  de-duped case-insensitively (the inline parser already lower-cases; frontmatter/LLM tags must be
  folded the same way so a case-collision yields one tag). The **server re-validates** on save
  (existing path) — the client normalization is the first pass, not the only one. No indexer change.

- **D-05 — `TagBadgeBar` replaces `InlineTagEditor` (PageEditor.tsx 644–655).** A minimal
  read-only bar. **Effective tags = union(inlineTagsFromBody(body), readTags(frontmatter))** shown
  as badges (reuse the `.cm-hashtag` visual or a sibling token). **Inline-origin** tags (∈
  `inlineTagsFromBody(body)`) carry **no ✕** (managed by editing `#tag` in the body). **Non-inline**
  tags (`readTags(frontmatter) − inlineTagsFromBody(body)`) each get an **✕** that does a
  byte-stable `setTags` removal write — mirroring the old `onTagsChange` (update `frontmatter`
  state + `frontmatterRef.current` + `setSaveState("idle")`). The bar reflects the LIVE body (so
  typing `#x` shows an un-removable badge immediately, before save).

- **D-06 — Remove + delete `InlineTagEditor`.** It is imported only by `PageEditor` (verified).
  After D-05 replaces it, delete `InlineTagEditor.tsx` + `.css` + `.test.tsx` (orphan — minimalism).
  The engineer re-greps for any consumer before deleting; the code-reviewer gate confirms.

- **D-07 — Coordinate the shared surface, don't fight it.** `PageEditor.tsx` is also edited by
  Phase 24 (removes the Description field at 632–643). Phase 22 touches the tag region (644–655)
  and `runSaver` (208–305) — disjoint from the description input. Additive; sequence 22 before 24
  (it is). No `markdown.ts` conflict with Phase 25 (codeLanguages) — different export.

- **D-08 — No design fork.** The model (frontmatter-canonical, client reconcile-as-diff), the
  reconciliation formula, and the badge-bar contract are all locked by REQUIREMENTS/ROADMAP; the
  one real implementation choice (Lezer parse vs hardened string scanner) is decided for us by the
  "no second parser / code-region-aware" requirement + the Phase-21 handoff → D-01. Proceed in-band.
</decisions>

<canonical_refs>
- `web/src/routes/PageEditor.tsx` — `runSaver` (208–305; reconcile insert before `savePage` at
  ~232); refs `lastSavedBody`/`bodyRef`/`frontmatterRef`/`baseRevision`/`lastSavedFrontmatter`
  (121–135); seed effect (140–150); `InlineTagEditor` wiring + `onTagsChange` (644–655).
- `web/src/lib/tag-frontmatter.ts` — `readTags` / `setTags` (byte-stable block rewrite; no
  normalization — the reconcile supplies it).
- `web/src/lib/cm/hashtag.ts` — `parseInlineTags` (retire from materialize) + new
  `inlineTagsFromBody`; `hashtagExtension`.
- `web/src/lib/cm/markdown.ts` — `markdownExtension`; refactor to export the shared extension list
  (D-01b) for the standalone tag parser.
- `web/src/components/InlineTagEditor.tsx` (+ `.css` / `.test.tsx`) — the chip field to remove/delete.
- `web/node_modules/@lezer/markdown/dist/index.d.ts` — `parser`, `MarkdownParser.configure`,
  `parse` (AUTHORITATIVE standalone-parser surface).
- `.planning/REQUIREMENTS.md` §ED-02b, §ED-02c.
- `.planning/phases/21-inline-tag-parser-badge-render/21-VERIFICATION.md` — the render⇄scanner
  divergence this phase closes; the pinned divergence tests to update/retire.
</canonical_refs>
