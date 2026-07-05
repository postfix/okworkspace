---
author: architect
responsible: architect
phase: 42
phase_type: implementation
status: Ready for planning
hard_bar: false
security_relevant: true
design_fork: true
design_fork_resolution: "informed-consent (co-driver, 2026-07-02): load guard = tree-equality; TipTap offered on any semantically-round-trippable page + a one-time notice that it normalizes modeled syntax for all viewers"
parent_artifacts:
  - .planning/ROADMAP.md
  - .planning/REQUIREMENTS.md
  - .planning/phases/41-tiptap-tag-suggest-slash/41-SUMMARY.md
---

# Phase 42: Toggle + Load Guard + Code-Split (user-facing opt-in) - Context

**Gathered:** 2026-07-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Make TipTap a **user-selectable** opt-in editing mode: expose the persisted 3-way toggle, gate entry
with a load guard (warn AND fall back), prove the whole TipTap tree code-splits out of the entry chunk,
and implement the co-driver-chosen shared-content fidelity resolution. This is the milestone's final
build phase — it turns the Phase-41 surface (built + mounted but not user-reachable) into a shipped feature.

**Requirements:** RE-04 · **SECURITY-adjacent:** the load guard is the byte-stability boundary.

**DESIGN FORK — RESOLVED (co-driver, 2026-07-02): (i) informed consent.** The load guard uses
**tree-equality** (`parse(roundTrip(md)) ≡ parse(md)`); TipTap is offered on any semantically-round-
trippable page (most pages); a **one-time notice** tells the user TipTap normalizes modeled syntax
(bullets `*`→`-`, `__`→`**`, loose lists) for all viewers. Option (ii) byte-delta was NOT chosen.

**Grounded state (Phases 38-41 complete):**
- `renderEditorSurface()` (`PageEditor.tsx:748-774`) ALREADY mounts `<TipTapEditor>` for
  `mode==="tiptap"` via `React.lazy` + `<Suspense>`. The `"tiptap"` branch exists; it's just not
  user-reachable (no UI sets `mode="tiptap"`).
- `editorMode` store (`stores/editorMode.ts`) is already 3-way (`"live"|"source"|"tiptap"`); `setMode`
  accepts all three; `toggle()` (Cmd/Ctrl-E) stays live⇄source only.
- The codec exports `roundTrip(md)` + `parse(md)` (`lib/tiptap/codec.ts:26-43`) — `isLosslessRoundTrip`
  composes from these.
- `assertLazyChunks` forbidden set (`vite.config.ts:41-58`) currently has `MERMAID_FORBIDDEN` (incl.
  `marked`, mislabeled "mermaid-UNIQUE"); TipTap already code-splits (Phase 41 verified entry chunk
  free of tiptap), but the FORBIDDEN set does NOT yet assert the TipTap tree stays out.

**Non-goals (out of this phase):**
- Modeled/editable tables or math — Future (preserved verbatim).
- Any new editor feature — RE-03 shipped the surface; this only EXPOSES + GUARDS it.
- Making TipTap the default — CM6 stays the default (SPEC: opt-in only, byte-exact default).

**Depends on:** Phases 40 (mode store + host seam), 41 (the TipTap surface + lazy mount). Both complete.
</domain>

<decisions>
## Implementation Decisions

### D-01: The 3-way toggle (user-selectable TipTap)
- Replace the 2-way live⇄source toolbar flip (`PageEditor.tsx` toolbar button ~`:759`) with a **3-way
  mode control** (segmented control or a cycling button with 3 states) that sets `editorMode` to
  `"live" | "source" | "tiptap"`. CM6 stays the DEFAULT (`"live"`).
- Selecting `"tiptap"` routes through the LOAD GUARD (D-02) before mounting — it does NOT blindly set
  the mode. `"live"`/`"source"` switch as today (CM6, byte-identical).

### D-02: The load guard — tree-equality, warn AND fall back (BLK-03 + MED-07), on BOTH entries
- **`isLosslessRoundTrip(md): boolean`** (new, in `lib/tiptap/` — composes the exported codec):
  `parse(roundTrip(md))` deep-equals `parse(md)` (tree-equality — the informed-consent semantics).
  Preserved-tier constructs (tables/math/HTML) are byte-exact so they pass; modeled-syntax
  normalization ALSO passes (it's semantically lossless — that's the informed-consent choice).
- **The guard fires on BOTH entries (MED-07):** opening a page in TipTap mode AND switching CM6→TipTap
  mid-session — checking the **CURRENT live (possibly unsaved) buffer**, not just saved bytes.
- **If `isLosslessRoundTrip` is false** (genuine content the bounded slice + preserved tier cannot
  round-trip — e.g. a dropped/mangled construct): the UI **warns AND falls back to CM6** for that
  content (BLK-03 — TipTap made unavailable for that page, not merely discouraged; a guard, not a toast).
  TipTap is never entered on non-round-trippable content.

### D-03: The informed-consent notice (the design-fork resolution)
- Because tree-equality PASSES modeled-syntax normalization, the guard alone won't inform the user that
  their `*` bullets / `__` emphasis will be reflowed for all viewers. So a **one-time notice** (a
  dismiss-persisted banner or a mode-switch confirmation the FIRST time a user opens TipTap) states:
  "TipTap normalizes bullet/emphasis/spacing style; saved changes reflow those bytes for everyone
  viewing this page." Persist the dismissal (localStorage, like `editorMode`). Not a per-save nag —
  informed consent, once.
- This is the explicit resolution of the shared-content fidelity fork. Do NOT also add a byte-delta
  fallback (that was option ii, not chosen).

### D-04: Code-split forbidden set — assert the WHOLE TipTap tree stays out of entry (MIN-08 + MIN-04)
- Extend `assertLazyChunks`' forbidden set (`vite.config.ts`) to assert the TipTap tree never leaks
  into the entry chunk: add `@tiptap/*` (`/core`, `/pm`, `/react`, `/starter-kit`, `/markdown`,
  `/extension-list`, `/suggestion`), and **`prosemirror-*`** (pulled via `@tiptap/pm`).
- **`marked`** is ALREADY in `MERMAID_FORBIDDEN` (`:48`) mislabeled "mermaid-UNIQUE" (`:37-40`) — it is
  now **dual-rooted** (mermaid@16 + `@tiptap/markdown`→marked@17). Move `marked` to a shared forbidden
  constant (or add a `TIPTAP_FORBIDDEN` array spread into `FORBIDDEN_IN_ENTRY`) and FIX the stale
  "mermaid-UNIQUE" comment to reflect the dual root. `maxEntryBytes` stays the backstop.
- Phase 41 already verified the entry chunk is tiptap-free; this phase makes that a BUILD-ENFORCED
  invariant (a future stray static import fails `npm run build`).

### D-05: Revision-checked save from TipTap (RE-04 crit) — already holds, confirm
- TipTap-mode save already routes through the RE-05 host's `runSaver` (revision-checked byte-stable
  `savePage(base_revision)` → single-writer commit; a stale 409 opens the conflict dialog). This phase
  adds no new save path — confirm the 409 flow works from TipTap mode (a test asserting a stale save 409s).

### The agent's discretion
- The toggle control shape (3-state segmented control vs a 3-cycle button) — match the existing UI idiom.
- The notice shape (a dismiss-persisted banner vs a confirm dialog on first TipTap open) — recommend a
  one-time dismiss-persisted banner (least intrusive; informed once).
- Where `isLosslessRoundTrip` lives (`lib/tiptap/codec.ts` or a new `lib/tiptap/guard.ts`).
- Whether the forbidden set is one array or `TIPTAP_FORBIDDEN` + `MERMAID_FORBIDDEN` spread.
- Commit slicing.
</decisions>

<disciplines>
## Lenses surfaced for this phase

~~~yaml
disciplines:
  - section: "D-02 the load guard"
    name: "guard-not-toast-on-both-entries"
    scope: ["the load guard", "the toggle"]
    guidance: "On BOTH opening-in-tiptap AND switching-to-tiptap, check the CURRENT live buffer with
      isLosslessRoundTrip (tree-equality). If false, WARN AND FALL BACK to CM6 — TipTap is made
      unavailable for that content, not merely discouraged with a dismissible toast. The byte-exact
      default is never entered-and-silently-lost. (BLK-03 + MED-07.)"
    rationale: "The load guard is the byte-stability boundary. A warn-only guard that still lets the
      user into TipTap on non-round-trippable content would silently lose bytes — the exact failure
      the two-tier contract exists to prevent."
    override_signals:
      - "None. Non-round-trippable content falls back to CM6; the guard is a guard."
  - section: "D-03 informed consent is the fork resolution"
    name: "notice-once-do-not-add-byte-delta-fallback"
    scope: ["the informed-consent notice"]
    guidance: "The co-driver chose informed consent (tree-equality guard + a one-time notice), NOT the
      byte-delta guard (option ii). Implement the notice; do NOT also tighten the guard to byte-equality
      (that would make TipTap unavailable on most pages — the rejected option). The notice is once
      (dismiss-persisted), not a per-save nag."
    rationale: "A locked co-driver design decision. Re-litigating it by quietly adding byte-delta
      behavior would violate the decision and cripple the feature."
    override_signals:
      - "Only a new co-driver decision changes this."
~~~

</disciplines>

<canonical_refs>
## Canonical References

### Phase spec & requirements
- `.planning/ROADMAP.md` §"Phase 42" — goal, 5 success criteria, the design fork.
- `.planning/REQUIREMENTS.md` §RE-04 (+ BLK-03 guard-not-toast, MED-07 both-entries, MIN-08/MIN-04
  forbidden set) + "The DESIGN FORK" section (resolved: informed consent).

### The seam (Phases 40-41)
- `web/src/routes/PageEditor.tsx:748-774` — `renderEditorSurface()` (TipTap already mounted for
  `mode==="tiptap"` via lazy+Suspense); the toolbar toggle (~`:759`, currently 2-way).
- `web/src/stores/editorMode.ts` — 3-way store (`setMode` accepts all three; `toggle` live⇄source only).
- `web/src/lib/tiptap/codec.ts:26-43` — `parse`/`roundTrip` (compose `isLosslessRoundTrip`).
- `web/src/lib/tiptap/preserved.ts` — the Tier-2 preserved tier (why most pages round-trip tree-equal).

### Code-split
- `web/vite.config.ts:41-58` — `MERMAID_FORBIDDEN` (incl. mislabeled `marked` `:48`) + `FORBIDDEN_IN_ENTRY`
  + the `assertLazyChunks({ forbidden, maxEntryBytes })` wiring (`:69-72`).
- `web/vite-plugins/assertLazyChunks.ts` — the guard (takes `forbidden: RegExp[]`).

### Save (RE-04 crit)
- `web/src/routes/PageEditor.tsx` `runSaver` (~`:246-376`) — the revision-checked byte-stable save
  TipTap already routes through (surface-agnostic).
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable assets
- `renderEditorSurface` "tiptap" branch (Phase 41) — mount exists; add the guarded toggle to reach it.
- `editorMode` 3-way store (Phase 40) — `setMode` accepts `"tiptap"`.
- `codec.roundTrip`/`parse` — compose `isLosslessRoundTrip` (tree-equality).
- `assertLazyChunks` + the forbidden-set pattern — extend for the TipTap tree.
- `runSaver` — the revision-checked save TipTap already uses (no new save path).

### Established patterns
- localStorage-persisted UI prefs (`editorMode`, notice-dismissal).
- The `MERMAID_FORBIDDEN` array spread into `FORBIDDEN_IN_ENTRY` — mirror for `TIPTAP_FORBIDDEN`.
- Tree-equality via `parse` deep-equal (the Phase-38 codec test pattern).

### Landmines
- **Guard on BOTH entries** (MED-07), current live buffer, warn AND fall back (BLK-03) — not a toast.
- **Informed consent is THE resolution** (D-03) — notice once; do NOT add byte-delta fallback (rejected option ii).
- **`marked` is dual-rooted** (MIN-08) — fix the "mermaid-UNIQUE" comment; the forbidden set must cover
  the whole TipTap tree incl. `prosemirror-*` (MIN-04).
- **CM6 stays default** — `"tiptap"` is opt-in; never auto-select it.
- **Shared component** (`PageEditor`) — run its full suite (v1.6 lesson); `npm run build` + assertLazyChunks green.
</code_context>

<deferred>
## Deferred Ideas

- **Modeled tables/math editing** — Future (preserved verbatim in v1.7).
- **TipTap as default / CM6 retirement** — a v2.0 question.
- **Per-save fidelity indicator** — the one-time notice suffices for v1.7 (informed consent).
</deferred>

---

*Phase: 42-toggle-load-guard-codesplit*
*Context gathered: 2026-07-02*
