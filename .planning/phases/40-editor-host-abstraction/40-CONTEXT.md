---
author: architect
responsible: architect
phase: 40
phase_type: implementation
status: Ready for planning
hard_bar: false
security_relevant: false
design_fork: false
parent_artifacts:
  - .planning/ROADMAP.md
  - .planning/REQUIREMENTS.md
  - .planning/codebase/MAP-cm-editor-subsystem.md
---

# Phase 40: Editor-Host Abstraction (safe refactor, no behavior change) - Context

**Gathered:** 2026-07-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Extract a **stable editor-surface seam** from `PageEditor.tsx` so a TipTap surface can later mount
beside CM6 through it — with **ZERO behavior change** to the shipping CM6 path. Extend the `editorMode`
store to three-way. Isolate this structural refactor from TipTap-feature risk (red-team MAJ-03) and
unblock RE-03's real-save materialization (BLK-01).

**Requirements:** RE-05

**The grounded key fact (shapes the whole scope):** `runSaver` — the save / tag-reconcile / soft-lock /
revision-409 machinery — **already operates on `body`/`frontmatter` React state (via refs), NOT on the
editor instance** (`PageEditor.tsx:246-376`). So the save path is ALREADY surface-agnostic. The ONLY
editor-specific coupling is the **imperative insert-at-cursor**: `insertTemplateAtCursor`
(`:529-542`) and `insertLink` reach into `.cm-editor`'s `cmView.view` and call
`view.dispatch(view.state.replaceSelection(...))`. RE-05's real work is to abstract THAT coupling
behind a surface interface — not to rewrite `runSaver` (touching it would risk the very byte-stable
save this phase must preserve).

**Non-goals (out of this phase):**
- Any TipTap editor component, `@tiptap/react`, mounting a real TipTap surface — Phase 41 (RE-03).
- Exposing `"tiptap"` as a user-selectable mode / the toolbar toggle / the load guard — Phase 42 (RE-04).
- Rewriting `runSaver`/conflict/lock internals — they stay byte-identical (already surface-agnostic).
- Code-split / `assertLazyChunks` work — Phase 42.

**Parallelization:** independent of Phases 38/39 (they touch only `web/src/lib/tiptap/*`; this touches
`web/src/routes/PageEditor.tsx` + `web/src/components/LivePreviewEditor.tsx` + `web/src/stores/editorMode.ts`
+ a new host module). MAY run in parallel with Phase 39 — disjoint files (qa-confirmed).
</domain>

<decisions>
## Implementation Decisions

### D-01: The surface interface + imperative insert handle (MED-06)
- Define an `EditorSurface` contract: the props a mountable editor needs — `value: string`,
  `onChange: (v: string) => void`, `mode` (see D-03), `readOnly: boolean`, `currentPath: string` —
  PLUS an imperative handle `EditorSurfaceHandle { insertAtCursor(text: string): void }`.
- `LivePreviewEditor` implements it: add `forwardRef` + `useImperativeHandle` exposing
  `insertAtCursor(text)` that does today's `view.dispatch(view.state.replaceSelection(text))` on its
  OWN `EditorView` (it already holds the view internally — no more reaching in from PageEditor).
- `PageEditor` stops reaching into `.cm-editor` `cmView.view`: `insertTemplateAtCursor` + `insertLink`
  call `surfaceRef.current?.insertAtCursor(...)` instead (`editorSurfaceRef` DOM reach-in removed).
  This is the MED-06 fix — the interface exposes insert, so a TipTap surface can implement it later.

### D-02: The mount seam (host)
- Introduce a thin host seam so the surface is chosen by mode behind the stable interface. Minimal
  form: a `renderEditorSurface(mode)` decision in `PageEditor` (or a small `<EditorHost>` wrapper)
  that mounts `<LivePreviewEditor>` for `"live"`/`"source"` today. `"tiptap"` has NO surface yet
  (Phase 41 adds it) — for now it must be unreachable (not user-selectable, D-03) so the seam never
  needs a TipTap branch this phase.
- **`runSaver` and all save/lock/409/reconcile state stay in `PageEditor` UNCHANGED.** The host seam is
  about the *surface mount + insert*, not the save loop (which is already surface-agnostic). Do NOT
  extract or rewrite `runSaver` — that would risk the byte-stable save with no benefit this phase.

### D-03: 3-way editorMode — code, not just docs (MED-05, BLK-02)
- **Extend** `web/src/stores/editorMode.ts` (do NOT create a new file — BLK-02): `EditorMode = "live"
  | "source" | "tiptap"`. Keep the persisted key `okf.editor.mode`.
- The CM6 `toggle()` (Cmd/Ctrl-E) + `mode.ts` `setMode(view, mode)` keep cycling **`live` ↔ `source`
  ONLY** (they are CM6-internal and inactive when a non-CM6 surface mounts). `setMode`'s type is
  narrowed so it can never receive `"tiptap"`.
- `"tiptap"` is selectable ONLY via a future React toolbar control (Phase 42) that remounts the
  surface — NOT via the CM6 keymap. This phase adds the type + store support; it does NOT expose a UI
  to pick `"tiptap"` (guard: if `mode === "tiptap"` somehow, fall back to CM6 `live` — Phase 42 wires
  the real load-guarded selection).
- The existing PageEditor toolbar toggle (`:759` `setMode(mode === "live" ? "source" : "live")`) stays
  a 2-way live⇄source flip this phase.

### D-04: Zero behavior change is THE bar
- No TipTap import in this phase (keeps the bundle unchanged; RE-04 code-splits TipTap later).
- CM6 renders and behaves byte-identically. The full existing editor suites pass unchanged
  (MIN-10: the `PageEditor` suites `vi.mock('../components/LivePreviewEditor')` — the mock seam MAY
  move to the surface interface; that re-point is allowed, behavior byte-identical).

### The agent's discretion
- Whether the host seam is an inline `renderEditorSurface` in `PageEditor` or a small extracted
  `<EditorHost>`/`useEditorSurface` — recommend the MINIMAL form that abstracts the surface+insert
  without moving `runSaver` (lowest regression risk).
- The exact `forwardRef`/`useImperativeHandle` shape on `LivePreviewEditor`.
- Commit slicing (recommend: surface interface + LivePreviewEditor ref, then PageEditor rewire, then
  editorMode 3-way — each independently green).
</decisions>

<disciplines>
## Lenses surfaced for this phase

~~~yaml
disciplines:
  - section: "D-04 zero behavior change"
    name: "refactor-preserves-observable-behavior"
    scope: ["PageEditor.tsx", "LivePreviewEditor.tsx", "editorMode.ts"]
    guidance: "This phase changes STRUCTURE, not BEHAVIOR. The CM6 edit/read surface, autosave,
      soft-lock, 409 conflict flow, tag reconcile, and template/link insert must be byte-identical
      before and after. qa verifies exactly this: the full existing editor + PageEditor suites pass
      UNCHANGED (a moved vi.mock seam aside). If a test needs a real assertion change to pass, that is
      a behavior change — stop and surface it, do not adjust the test to fit."
    rationale: "PageEditor is 949 lines of load-bearing save/lock/conflict machinery. The red-team
      isolated this refactor (MAJ-03) precisely so TipTap risk is not entangled with it. A silent
      behavior change here would surface as data loss / conflict mishandling under TipTap later."
    override_signals:
      - "None. A behavior change is out of scope for RE-05 by definition — it is a no-behavior-change refactor."
  - section: "D-02 do not touch runSaver"
    name: "leave-the-byte-stable-save-alone"
    scope: ["runSaver", "the conflict/lock/reconcile flow"]
    guidance: "runSaver already reads body/frontmatter state via refs — it is surface-agnostic. Do NOT
      extract, move, or rewrite it. The host seam abstracts the surface MOUNT + INSERT only. Touching
      the save loop risks the byte-stable ApplyTagsBatch/revision-409 invariant for zero benefit."
    rationale: "The milestone's whole value rests on byte-stable saves. The safest RE-05 leaves the
      proven save machinery exactly as it is and only abstracts the editor-surface coupling."
    override_signals:
      - "If a genuine surface-specific hook is needed in the save path, surface it to the architect — do not silently refactor runSaver."
~~~

</disciplines>

<canonical_refs>
## Canonical References

### Phase spec & requirements
- `.planning/ROADMAP.md` §"Phase 40: Editor-Host Abstraction" — goal, 3 success criteria, parallelization.
- `.planning/REQUIREMENTS.md` §RE-05 (+ folded MED-05 3-way, MED-06 insert-at-cursor, MIN-10 mock re-point).

### The host + surface
- `web/src/routes/PageEditor.tsx` — the 949-line host. Key spans: `runSaver` (`:246-376`, DO NOT
  touch), `insertTemplateAtCursor` (`:529-542`, the `.cm-editor` cmView reach-in to replace), `insertLink`
  (`:507`, appends via `setBody`), the `<LivePreviewEditor>` mount (`:921-927`, props `value`/`onChange`/
  `currentPath`/`mode`/`readOnly`), the 2-way toolbar toggle (`:759`), `editorSurfaceRef` (`:80-84`).
- `web/src/components/LivePreviewEditor.tsx` — the ONE CM6 host component (make it conform to the
  surface interface + expose `insertAtCursor` via `forwardRef`/`useImperativeHandle`).
- `web/src/stores/editorMode.ts` — the 2-way store to EXTEND to 3-way (`EditorMode`, `setMode`,
  `toggle`, key `okf.editor.mode`).
- `web/src/lib/cm/mode.ts` — the CM6 `modeCompartment`/`setMode(view, mode)`/`toggleKeymap` (Mod-e).
  Stays `live`⇄`source` only (MED-05); narrow so it never receives `"tiptap"`.

### Tests (the zero-regression bar)
- `web/src/components/LivePreviewEditor.test.tsx`, `web/src/lib/cm/mode*.test.ts`,
  `web/src/routes/PageEditor.test.tsx` + `.conflict` + `.reconcile.adversarial` — must pass UNCHANGED
  (MIN-10: a moved `vi.mock` seam is allowed).
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable assets
- `runSaver` is already surface-agnostic (reads refs) — reuse as-is; do NOT extract.
- `editorMode.ts` — zustand+persist, extend the union + narrow `setMode`.
- `LivePreviewEditor` — already owns its `EditorView`; add an imperative `insertAtCursor` handle.

### Established patterns
- The `mode.ts` `modeCompartment` reconfigure-with-effects-only discipline (byte-identical doc across
  mode switches) — the model the 3-way toggle respects.
- zustand+persist store (`editorMode.ts`, `recent.ts`) — the 3-way extension pattern.

### Landmines
- **Do NOT touch `runSaver`** — byte-stable save/409/lock is load-bearing; it needs no change (already
  surface-agnostic). The refactor is the surface mount + imperative insert only.
- **BLK-02:** extend the EXISTING `editorMode.ts`, do NOT create `web/src/lib/editorMode.ts`.
- **MED-05:** the CM6 keymap + `setMode` stay 2-way (`live`⇄`source`); `"tiptap"` is React-toolbar-only
  (Phase 42) — do not let `"tiptap"` reach `setMode`.
- **MED-06:** replace the `.cm-editor` `cmView.view` reach-in with the imperative interface — the
  whole point (so a TipTap surface can implement insert too).
- **MIN-10:** `PageEditor` suites `vi.mock('../components/LivePreviewEditor')`; re-pointing the mock to
  the surface interface is allowed (behavior byte-identical).
- **Zero behavior change** — the hardest thing to fake and the whole point; qa runs the FULL existing
  editor + PageEditor suites and confirms byte-identical behavior (v1.6 lesson: shared-component change
  → run the component's full suite).
</code_context>

<deferred>
## Deferred Ideas

- **Full `runSaver` extraction into a `useEditorHost` hook** — not needed (runSaver is already
  surface-agnostic); a future cleanup, not RE-05.
- **Mounting the real TipTap surface** — Phase 41 (RE-03).
- **User-selectable `"tiptap"` + load guard + code-split** — Phase 42 (RE-04).
</deferred>

---

*Phase: 40-editor-host-abstraction*
*Context gathered: 2026-07-02*
