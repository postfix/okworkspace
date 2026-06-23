---
phase: 6
slug: live-preview-editor-obsidian-style
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-06-21
---

# Phase 6 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | vitest 3.2.4 + jsdom 26.1.0 + @testing-library/react 16.3.0 (frontend); Go `testing` (backend okf corpus gate) |
| **Config file** | `web/vitest.config.ts` (jsdom env, `src/test/setup.ts`, globals) |
| **Quick run command** | `cd web && npx vitest run <touched cm test>` |
| **Full suite command** | `cd web && npm test` (`vitest run`) + `go test ./internal/okf/...` |
| **Estimated runtime** | ~20–40 seconds (frontend), ~5s (okf gate) |

---

## Sampling Rate

- **After every task commit:** Run `cd web && npx vitest run <touched cm test>` (sub-30s)
- **After every plan wave:** Run `cd web && npm test` + `go test ./internal/okf/...`
- **Before `/gsd-verify-work`:** Full `npm test` green AND `go test ./internal/okf/ -run TestGoldenRoundTrip` green
- **Max feedback latency:** 40 seconds

---

## Per-Task Verification Map

| Req | Behavior | Test Type | Automated Command | File Exists | Status |
|-----|----------|-----------|-------------------|-------------|--------|
| EDIT-01 | live-preview decorations render for each construct (headings, bold/italic, lists, links, inline code, code blocks, images, GFM tables) | unit (state+jsdom) | `cd web && npx vitest run src/lib/cm/livePreview.test.ts` | ❌ W0 | ⬜ pending |
| EDIT-02 | toggling Live⇄Source never mutates `doc.toString()` | unit (EditorState) | `cd web && npx vitest run src/lib/cm/mode.test.ts` | ❌ W0 | ⬜ pending |
| EDIT-03 | type→save cycle ships verbatim bytes; backend corpus stays green | unit + go | `cd web && npx vitest run src/components/LivePreviewEditor.test.tsx` && `go test ./internal/okf/ -run TestGoldenRoundTrip` | ⚠️ backend ✅ / frontend ❌ W0 | ⬜ pending |
| EDIT-04 | save machinery untouched; no raw HTML; image-src allowlist | unit | `cd web && npx vitest run src/lib/cm/sanitizeSrc.test.ts src/routes/PageEditor.test.tsx` | ⚠️ PageEditor.test.tsx exists (retarget) / sanitizeSrc ❌ W0 | ⬜ pending |
| SRCH-06 (preserve) | unified read surface stamps heading id == `slug(text)` matching `okf.ScanHeadings`; `#hash` deep-link scrolls | unit | `cd web && npx vitest run src/lib/cm/headingAnchors.test.ts` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `web/src/lib/cm/mode.test.ts` — EDIT-02: build an `EditorState` with each okf corpus fixture string, toggle the mode compartment, assert `state.doc.toString()` identical before/after (state-level, no DOM).
- [ ] `web/src/lib/cm/livePreview.test.ts` — EDIT-01: assert the ViewPlugin emits expected decoration kinds for bold/heading/link/code/image/table inputs.
- [ ] `web/src/lib/cm/sanitizeSrc.test.ts` — EDIT-04: assert `javascript:`, exec `data:`, and path-escape srcs are blocked; safe `http(s)`/app-relative pass.
- [ ] `web/src/lib/cm/headingAnchors.test.ts` — SRCH-06: assert rendered heading id equals github-slugger `slug(text)` for corpus headings (mirrors `okf.ScanHeadings`).
- [ ] `web/src/components/LivePreviewEditor.test.tsx` — EDIT-03/04: type into the editor, assert `onChange` receives verbatim bytes; value/onChange contract parity with old MDEditor.
- [ ] **Update** `web/src/routes/PageEditor.test.tsx` — retarget from `<MDEditor>` to `LivePreviewEditor`, preserving autosave/conflict assertions.
- [ ] Optional: import `internal/okf/testdata/corpus/*.md` fixture strings into the frontend tests so EDIT-02/03 exercise the same byte-stability fixtures as the backend gate.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Obsidian "feel" — markers reveal smoothly on the active line without reflow jump | EDIT-01 | Perceptual/visual quality not assertable in jsdom (no layout engine) | Open a page in Live mode; move the cursor through bold/heading/link lines; confirm markers reveal on the active line and text below does not visibly jump |
| Inline image + table widgets render and reveal-to-source on edit | EDIT-01 | jsdom lacks image layout; visual confirmation needed | Edit a page with an image and a GFM table; confirm both render inline in Live, and editing the line shows raw source |

*Backend round-trip (EDIT-03) and toggle byte-stability (EDIT-02) are fully automated; only perceptual quality is manual.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 40s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
