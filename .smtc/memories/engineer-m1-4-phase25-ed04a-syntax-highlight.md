---
slug: engineer-m1-4-phase25-ed04a-syntax-highlight
created_at: 2026-06-30T17:53:43Z
modified_at: 2026-06-30T17:53:43Z
---
# Phase 25 (ED-04a) — Code Syntax Highlighting — engineer build notes (milestone m1.4)

Frontend-only (web/). Built + gated + green. Commits: 98eb4ca 93ad433 2734fe6 9bbb88c ff6b226.

## Durable patterns / gotchas learned
- **lang-markdown drags lang-html -> lang-css + lang-javascript into the ENTRY chunk.**
  @codemirror/lang-markdown (the editor's base language, statically imported in
  markdown.ts) hard-depends on lang-html (embedded-HTML highlighting), which pulls
  lang-css + lang-javascript. So those FOUR grammars are unavoidably in the entry chunk
  and were there pre-phase. Any "no lang-* in entry" build assertion MUST exclude those
  four. We use an anchored negative lookahead: lang-(?!(?:markdown|html|css|javascript)/)
  (trailing slash = path-segment boundary so a future lang-cssinjs stays forbidden).
- **Vite 8 uses rolldown types, not rollup.** Do NOT import OutputChunk from "rollup" in a
  plugin -- it mismatches Vite's Plugin.generateBundle (rolldown). Let the bundle param be
  typed by the Plugin type and narrow with file.type === "chunk".
- **@types/node** was NOT installed; needed for Buffer in a build plugin. tsconfig.node.json
  now owns vite-plugins/ with "types": ["node"]. tsc -b builds only app+node projects
  (root references) -- the test project is for vitest/IDE.
- **?raw import** for "source has no static grammar import" structural test -- robust under
  vitest (vite/client types via src/vite-env.d.ts); import.meta.url+fileURLToPath FAILS
  (not a file:// scheme under vitest transform).
- **emptyOutDir deletes internal/web/dist/.gitkeep** on every build -- touch to restore;
  dist artifacts are gitignored (only .gitkeep tracked).

## Async widget security pattern (reusable for Phase 26 mermaid widget)
CodeBlockWidget.toDOM renders plain textContent FIRST (sync), then async highlight:
- matchLanguageName(codeLanguages, lang, false) -- fuzzy=false is REQUIRED, else
  substrings match (jsonl->json). Exact name/alias only, still case-insensitive.
- Guard the WHOLE post-load body in one try/catch (not just load()); build tokens into a
  DETACHED DocumentFragment, then swap atomically (clear+append) only on success. This
  prevents (a) blanking the node if parse/emit throws and (b) unhandled rejection from the
  void this.highlight() fire-and-forget. Teardown guard: destroyed || !codeEl.isConnected.
- Token spans: createElement + textContent ONLY, never innerHTML (T-06-08).
  Adversarial suite (widgets.adversarial.test.ts, 45 tests) proves no-innerHTML +
  teardown-safety hold under bidi/surrogate/fence-breakout payloads.

## Numbers
Entry chunk measured 1,180,566 B; budget maxEntryBytes=1,420,000 (~20% headroom).
Full suite 685 tests / 59 files green. Both assertion arms proven e2e.

## Gate process note
Both anvil-code-reviewer and anvil-adversarial-tester independently flagged the same
unguarded-post-load defect (MED). Adversarial worker delivered an it.fails-encoded suite on
a worktree branch; I did NOT merge the branch (it pinned buggy behavior) -- instead extracted
the file, fixed the defects, and converted the defect-pinning tests to assert the corrected
behavior. Convert it.fails -> it() when you fix what an adversarial worker reproduced.
