// sanitizeImageSrc gates the `src` of an inline image (`![alt](src)`) before it is
// mounted into a live-preview `<img>` widget (T-06-01, RESEARCH Security Domain /
// Pattern 3). Page bodies are user- and agent-authored and therefore untrusted, so
// an image src can carry an XSS vector (`javascript:`, `vbscript:`, executable
// `data:` URLs). This is the one audited chokepoint every image widget MUST call
// before setting `img.src` — the 06-03 image widget falls back to rendering the raw
// Markdown text when this returns null (never implying the bytes changed).
//
// Policy (allowlist, not denylist):
//   - empty / whitespace-only           → null (nothing to render)
//   - has a URI scheme (`name:`)        → allow ONLY `http:` and `https:`; all
//                                         other schemes (javascript, vbscript,
//                                         data, file, …) → null
//   - no scheme (app-relative / root-   → allowed (the app's attachment + relative
//     relative / protocol-relative-free)  path convention; resolved by the caller)
//
// Scheme detection mirrors mdlink.ts's `/^[a-z][a-z0-9+.-]*:/i` so "is this
// schemed?" is classified identically across the codebase. Pure + unit-tested.

// SCHEME_RE matches a leading RFC-3986 scheme followed by ':' — same shape as
// resolveRelativeMdLink uses. The capture group is the scheme name (sans colon).
const SCHEME_RE = /^([a-z][a-z0-9+.-]*):/i;

// Schemes permitted on a schemed src. Everything else schemed is rejected.
const ALLOWED_SCHEMES = new Set(["http", "https"]);

export function sanitizeImageSrc(src: string): string | null {
  if (src == null) {
    return null;
  }
  const trimmed = src.trim();
  if (trimmed === "") {
    return null;
  }

  // Protocol-relative ("//host/x") has no scheme but is effectively external and
  // ambiguous in this app's relative-path world — reject it.
  if (trimmed.startsWith("//")) {
    return null;
  }

  const m = SCHEME_RE.exec(trimmed);
  if (m) {
    const scheme = m[1].toLowerCase();
    return ALLOWED_SCHEMES.has(scheme) ? trimmed : null;
  }

  // No scheme: an app-relative / attachment / root-relative path. Allowed.
  return trimmed;
}
