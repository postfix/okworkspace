// resolveRelativeMdLink resolves a Markdown link href (as authored on disk by the
// LinkPicker, D-05) against the CURRENT page's directory, producing a clean
// workspace-relative `.md` path suitable for an in-app `/app/page/<path>` route
// (D-06). It is a pure function so it can be unit-tested in isolation (WR-02).
//
// Returns null when the href is NOT an internal `.md` link — external links
// (http/https/mailto/protocol-relative `//`) and non-`.md` targets are left for
// the caller to render unchanged, preserving the existing external-link behavior.
//
// currentPath is the workspace-relative path of the linking page (e.g.
// "docs/guide.md"); href is the raw link target from the rendered Markdown.
export function resolveRelativeMdLink(
  currentPath: string,
  href: string | undefined,
): string | null {
  if (href == null || href === "") {
    return null;
  }
  // External / protocol links are never rewritten to in-app routes.
  // Matches `http://`, `https://`, `mailto:`, any `scheme:` and protocol-relative `//`.
  if (/^[a-z][a-z0-9+.-]*:/i.test(href) || href.startsWith("//")) {
    return null;
  }

  // Split off a trailing #fragment (and any ?query) so the `.md` gate sees the
  // bare path; the fragment is carried back through onto the resolved path.
  let path = href;
  let suffix = "";
  const hashIdx = path.indexOf("#");
  if (hashIdx !== -1) {
    suffix = path.slice(hashIdx);
    path = path.slice(0, hashIdx);
  }

  // Only `.md` targets are internal page links (preserve the existing gate).
  if (!path.toLowerCase().endsWith(".md")) {
    return null;
  }

  // Determine the base segments to resolve against:
  //  - leading "/"  → resolve from workspace root (drop the current dir)
  //  - otherwise    → resolve from dirname(currentPath)
  let baseSegments: string[];
  if (path.startsWith("/")) {
    baseSegments = [];
    path = path.replace(/^\/+/, "");
  } else {
    baseSegments = currentPath.split("/").slice(0, -1).filter((s) => s !== "");
  }

  const out = [...baseSegments];
  for (const seg of path.split("/")) {
    if (seg === "" || seg === ".") {
      continue;
    }
    if (seg === "..") {
      // Clamp at root: never escape above the workspace root.
      if (out.length > 0) {
        out.pop();
      }
      continue;
    }
    out.push(seg);
  }

  return out.join("/") + suffix;
}
