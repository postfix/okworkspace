// linkNav — internal `.md` link click-navigation on the CM6 live-preview surface
// (EDIT-01 / D-06, RESEARCH Pattern 4). Rendered Markdown links are CM6
// `Decoration.mark`s (a `<span class="cm-md-link" data-href="…">` wrapping the link
// text — see livePreview.ts), NOT `<a onClick>` elements, so a click must be
// intercepted at the editor DOM level and routed through react-router.
//
// SECURITY (T-06-07): the href is read from the `data-href` DATA attribute the link
// mark stored — it is never an executed action. resolveRelativeMdLink owns the
// scheme allowlist (it returns null for any `scheme:` href, protocol-relative `//`,
// or non-`.md` target), so a `javascript:`/`data:` href can NEVER reach `navigate`:
//   • non-null target → an internal `.md` page → preventDefault + SPA navigate.
//   • null            → external / non-`.md` / unsafe → default behavior (no SPA
//                        navigation); the browser handles a normal external link.
import { EditorView } from "@codemirror/view";
import type { Extension } from "@codemirror/state";

import { resolveRelativeMdLink } from "../mdlink";

// linkNav returns a CM6 extension (a DOM event handler) that routes internal `.md`
// link clicks to `navigate('/app/page/<target>')`.
//
// `currentPath` may be a plain string OR a getter `() => string` — the EditorView
// is created once but its linking page can change, so LivePreviewEditor passes a
// ref-reading getter to keep the resolve base live without recreating the view.
// `navigate` is react-router's navigate fn (LivePreviewEditor passes a ref-reading
// wrapper for the same reason).
export function linkNav(
  currentPath: string | (() => string),
  navigate: (to: string) => void,
): Extension {
  const pathOf = () =>
    typeof currentPath === "function" ? currentPath() : currentPath;
  return EditorView.domEventHandlers({
    mousedown(event) {
      // Find the nearest rendered link element (the mark span) under the click.
      const target = event.target;
      if (!(target instanceof HTMLElement)) return false;
      const linkEl = target.closest(".cm-md-link");
      if (!linkEl) return false;

      // The original Markdown href is carried as data (never executed).
      const href = linkEl.getAttribute("data-href") ?? undefined;
      const resolved = resolveRelativeMdLink(pathOf(), href);
      if (resolved == null) {
        // External / non-`.md` / unsafe scheme → let default behavior happen; no
        // SPA navigation and (because resolveRelativeMdLink rejected it) no
        // javascript:/data: scheme is ever navigated.
        return false;
      }
      // Internal `.md` link → route within the SPA.
      event.preventDefault();
      navigate(`/app/page/${resolved}`);
      return true;
    },
  });
}
