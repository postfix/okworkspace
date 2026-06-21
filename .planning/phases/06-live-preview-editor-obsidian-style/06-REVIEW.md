---
phase: 06-live-preview-editor-obsidian-style
reviewed: 2026-06-21T00:00:00Z
depth: standard
files_reviewed: 13
files_reviewed_list:
  - web/src/lib/cm/sanitizeSrc.ts
  - web/src/lib/cm/widgets.ts
  - web/src/lib/cm/linkNav.ts
  - web/src/lib/cm/livePreview.ts
  - web/src/lib/cm/headingAnchors.ts
  - web/src/lib/cm/mode.ts
  - web/src/lib/cm/markdown.ts
  - web/src/lib/cm/theme.ts
  - web/src/components/LivePreviewEditor.tsx
  - web/src/routes/PageEditor.tsx
  - web/src/routes/PageView.tsx
  - web/src/stores/editorMode.ts
  - web/src/test/cmCorpus.ts
findings:
  critical: 1
  warning: 2
  info: 0
  total: 3
status: issues_found
---

# Phase 06: Code Review Report

**Reviewed:** 2026-06-21T00:00:00Z
**Depth:** standard
**Files Reviewed:** 13
**Status:** issues_found

## Summary

The Phase 06 implementation delivers a solid CM6 live-preview Markdown editor with
correct application of the EDIT-02/EDIT-03 byte-stability invariants, proper CM6
lifecycle management (single EditorView, destroy-on-cleanup, StrictMode-safe), and a
well-structured security boundary for image src (sanitizeSrc.ts allowlist) and link
navigation (resolveRelativeMdLink allowlist). No `innerHTML` of page content is used
anywhere; all widget DOM is built via `createElement`/`textContent`/explicit attributes.
The `data-href` attribute path is safe (setAttribute handles any string; linkNav never
executes an unsafe href). The CRLF normalization by CM6 is acknowledged and deliberately
out of scope for the frontend tests.

Three issues were found, one critical:

1. **CRITICAL:** `headingText()` in `headingAnchors.ts` does not strip the ATX trailing
   closing `#` sequence. The Go backend's `atxHeading()` does strip it (`trimATXClosing`).
   For any heading like `## Title ##`, the frontend produces a different slug than the
   backend, breaking `#anchor` search deep-links.

2. **WARNING:** `runSaver` in `PageEditor.tsx` does not handle `getPage()` failure after
   a successful `savePage()`. The exception propagates out of the unguarded `await
   getPage(path)` call, leaves the UI frozen at "Saving…" with no error message, and
   leaves `baseRevision.current` stale — causing the next autosave to 409 with a false
   conflict banner despite the save having succeeded.

3. **WARNING:** The `CSS.escape` fallback in `scrollToHash` (`headingAnchors.ts`) only
   escapes `"` and `\`, which is insufficient for a general CSS selector. In environments
   where `CSS.escape` is absent a manually-crafted hash containing `.`, `[`, `#`, etc.
   would produce an invalid or unintended `querySelector` string. Mitigated in practice
   (the slug algorithm drops all CSS-significant punctuation), but the fallback is
   demonstrably incomplete.

---

## Critical Issues

### CR-01: `headingText()` does not strip ATX closing `#` sequence — frontend and backend slugs diverge

**File:** `web/src/lib/cm/headingAnchors.ts:69`

**Issue:** `headingText()` strips the leading `# ` marker and trailing whitespace but
does NOT strip a trailing ATX closing sequence such as ` ##`. The Go backend's
`atxHeading()` calls `trimATXClosing()` which removes ` ##` before slugging. For any
CommonMark-valid heading that uses a closing marker (`## My Title ##`), the two
implementations produce different slugs:

- Backend (`okf.ScanHeadings`): `"My Title ##"` → `trimATXClosing` → `"My Title"` →
  slug `"my-title"`.
- Frontend (`headingText` + `slug`): `"My Title ##"` → slug → `"my-title-"` (the
  space before `##` becomes a trailing `-`; the `##` chars are dropped).

The Anchor stored in the search index (`#my-title`) differs from the DOM `id` stamped
by `headingAnchorField` (`my-title-`). A search result deep-link
`/app/page/foo.md#my-title` therefore fails to scroll to the correct heading.

The trailing `-` discrepancy is load-bearing because `slug()` does NOT trim hyphens
(intentional, to match github-slugger), so the trailing dash survives.

**Fix:** Add `trimATXClosing` to `headingText()`:

```typescript
// Strip an optional ATX closing '#' run preceded by whitespace (CommonMark §4.2).
// This mirrors Go's trimATXClosing so the frontend and backend slugs agree.
function trimATXClosing(s: string): string {
  const stripped = s.replace(/#+$/, "");
  if (stripped === s) {
    // No trailing '#' run — just trim trailing whitespace.
    return s.replace(/[ \t]+$/, "");
  }
  // There was a trailing '#' run; it is a valid closer only if preceded by
  // whitespace (or the line is only '#'s). Trim the preceding whitespace.
  const beforeHashes = stripped.replace(/[ \t]+$/, "");
  if (beforeHashes !== stripped) {
    // There was whitespace before the '#' run — valid closer, return trimmed.
    return beforeHashes;
  }
  // No whitespace before '#' run (e.g. "foo#") — not a closer; keep original
  // minus trailing whitespace.
  return s.replace(/[ \t]+$/, "");
}

export function headingText(line: string): string {
  const m = /^ {0,3}(#{1,6})(?:[ \t]+(.*))?$/.exec(line);
  if (!m) {
    return line;
  }
  return trimATXClosing(m[2] ?? "");
}
```

A test case to add to `headingAnchors.test.ts`:

```typescript
it("strips ATX closing '#' sequence (CommonMark §4.2)", () => {
  expect(headingText("## My Title ##")).toBe("My Title");
  expect(headingText("# Foo #")).toBe("Foo");
  expect(headingText("### Bar###")).toBe("Bar###"); // no space before — not a closer
  expect(headingText("## Baz ## ")).toBe("Baz");
});
```

---

## Warnings

### WR-01: `getPage()` after successful save is unguarded — network failure leaves UI frozen and baseRevision stale

**File:** `web/src/routes/PageEditor.tsx:131`

**Issue:** Inside `runSaver`, after `savePage()` succeeds, `getPage(path)` is called to
read back the fresh revision token. This call is NOT wrapped in a `try/catch`. If the
network request fails (transient error, server restart, timeout):

1. The exception propagates out of the `while` loop and the outer `try` block.
2. The `finally` block correctly resets `saving.current = false`.
3. The async `runSaver` function returns a rejected Promise.
4. The caller (`void runSaver(false)`) discards the Promise — the rejection is
   **unhandled** and surfaces as a browser console error.
5. `setSaveState` is never called with `"saved"` or `"idle"` — the UI is frozen
   displaying "Saving…" with no error message.
6. `baseRevision.current` is not updated. The next autosave fires with the old revision
   and may receive a `409 Conflict`, showing a **false conflict banner** to the user
   despite the page having been saved successfully.

**Fix:** Wrap the `getPage` call in its own `try/catch`:

```typescript
// Read back the advanced revision; on transient failure, fall back to a
// page reload rather than showing a false conflict.
try {
  const fresh = await getPage(path);
  baseRevision.current = fresh.revision;
} catch {
  // The save succeeded but we couldn't confirm the new revision.
  // Show a recoverable error rather than leaving the UI frozen or
  // triggering a false 409 on the next autosave.
  setSaveError(
    "Page saved, but we couldn't confirm the new version. Reload to continue editing safely.",
  );
  setSaveState("idle");
  return;
}
lastSavedBody.current = sentBody;
lastSavedFrontmatter.current = sentFrontmatter;
```

### WR-02: `CSS.escape` fallback in `scrollToHash` is under-escaped — selector injection possible when fallback is active

**File:** `web/src/lib/cm/headingAnchors.ts:177`

**Issue:** `scrollToHash` escapes the decoded hash before using it in a `querySelector`
call. When `CSS.escape` is available (all modern browsers, jsdom 20+) this is correct.
The fallback used when `CSS` is unavailable escapes only `"` and `\`:

```typescript
target.replace(/["\\]/g, "\\$&")
```

This is insufficient. Characters such as `.`, `#`, `[`, `]`, `(`, `)`, `:` are not
escaped; if unescaped in `#<value>`, they form compound CSS selectors (`#foo.bar`
matches `id="foo"` AND `class="bar"`; `#foo[attr]` is an attribute selector). A
user-crafted URL hash like `#foo.injection` would, when CSS.escape is absent, cause
`view.dom.querySelector("#foo.injection")` to match an unintended element.

The **practical exposure is low** because `slug()` drops all punctuation that is
CSS-significant (`.`, `[`, `#`, etc.), so anchors generated by the algorithm cannot
contain such characters. However, the raw `window.location.hash` is URL-decoded before
the lookup, and a manually typed URL could contain anything. If the query finds no
element the function returns `false` harmlessly, so this is not exploitable for
arbitrary DOM manipulation — but it can silently match an unexpected element.

**Fix:** Replace the fallback with the full CSS Identifiers escape algorithm specified
by the CSS spec (covering everything CSS.escape covers):

```typescript
// Full CSS.escape polyfill for environments where CSS.escape is absent.
// Implements https://drafts.csswg.org/cssom/#serialize-an-identifier
function cssEscape(s: string): string {
  if (typeof CSS !== "undefined" && typeof CSS.escape === "function") {
    return CSS.escape(s);
  }
  return s.replace(/[^\w-]/g, (ch) => `\\${ch}`);
}

// In scrollToHash, replace the inline escape with:
const escaped = cssEscape(target);
const el = view.dom.querySelector<HTMLElement>(`#${escaped}`);
```

The `\w` class (`[a-zA-Z0-9_]`) plus `-` are always safe unescaped in CSS identifiers;
every other character is backslash-escaped. This matches what CSS.escape produces for
non-empty, non-numeric-starting identifiers — which all slug-generated anchors satisfy.

---

_Reviewed: 2026-06-21T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
