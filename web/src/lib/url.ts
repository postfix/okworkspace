// encodePagePath percent-encodes a repo-relative page path for safe
// interpolation into an /api/v1/pages/... URL. The `/` are PATH SEPARATORS and
// must stay literal, so a bare encodeURIComponent(path) is WRONG (it would
// encode the slashes and break nested paths). Instead each `/`-separated
// segment is encoded on its own and re-joined with `/`.
//
// This makes a filename containing `#`, `?`, `%`, spaces, or unicode route
// correctly instead of the `#`/`?` starting a fragment/query or `%` mis-decoding
// server-side. Plain ASCII filenames are unchanged (no double-encoding).
export function encodePagePath(path: string): string {
  return path
    .split("/")
    .map((segment) => encodeURIComponent(segment))
    .join("/");
}
