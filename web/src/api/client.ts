// API client for the OKF Workspace backend. Sends cookies (credentials) and
// echoes the nosurf CSRF token in the X-CSRF-Token header on every mutating
// request (SEC-04). The token is fetched from GET /api/v1/csrf and cached.

export interface Me {
  username: string;
  display_name: string;
  role: string;
  must_change_password: boolean;
}

// AdminUser mirrors the safe userView the admin API returns (no password hash).
export interface AdminUser {
  id: number;
  username: string;
  display_name: string;
  role: string;
  must_change_password: boolean;
  active: boolean;
}

export type UserRole = "admin" | "editor" | "reader";

// RepoHealth mirrors the backend GET /api/v1/health payload (SPEC §6.6).
export interface RepoHealth {
  ok: boolean;
  diverged: boolean;
  self_healed: boolean;
  detail: string;
}

const CSRF_HEADER = "X-CSRF-Token";
let csrfToken: string | null = null;

async function fetchCSRFToken(): Promise<string> {
  const res = await fetch("/api/v1/csrf", { credentials: "same-origin" });
  if (!res.ok) {
    throw new Error("Could not initialize the session.");
  }
  const data = (await res.json()) as { csrf_token: string };
  csrfToken = data.csrf_token;
  return csrfToken;
}

async function ensureCSRF(): Promise<string> {
  if (csrfToken) return csrfToken;
  return fetchCSRFToken();
}

async function mutate<T>(
  path: string,
  body: unknown,
  method: "POST" | "PUT" | "DELETE" = "POST",
): Promise<T> {
  const token = await ensureCSRF();
  const res = await fetch(path, {
    method,
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      [CSRF_HEADER]: token,
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) {
    // Try to surface the server's generic error message.
    let message = "Something went wrong. Check your connection and try again.";
    try {
      const data = (await res.json()) as { error?: string };
      if (data.error) message = data.error;
    } catch {
      // ignore parse errors; keep generic message
    }
    const err = new Error(message) as Error & { status?: number };
    err.status = res.status;
    throw err;
  }
  // Some success handlers return an empty body on a non-204 status (e.g.
  // createFolder → 201, savePage/deletePage → 204), while others return JSON
  // (login → Me, createPage → {path}). Read the body as text first and only
  // JSON.parse when it is non-empty, so an empty 2xx body resolves to undefined
  // instead of throwing "Unexpected end of JSON input" (UAT blocker).
  const text = await res.text();
  return text ? (JSON.parse(text) as T) : (undefined as T);
}

export async function login(username: string, password: string): Promise<Me> {
  return mutate<Me>("/api/v1/auth/login", { username, password });
}

export async function logout(): Promise<void> {
  await mutate<void>("/api/v1/auth/logout", undefined);
  csrfToken = null;
}

export async function me(): Promise<Me> {
  const res = await fetch("/api/v1/auth/me", { credentials: "same-origin" });
  if (res.status === 401) {
    const err = new Error("Your session expired. Sign in again to continue.") as Error & {
      status?: number;
    };
    err.status = 401;
    throw err;
  }
  if (!res.ok) {
    throw new Error("Something went wrong. Check your connection and try again.");
  }
  return (await res.json()) as Me;
}

// health fetches repository health. A GET, so no CSRF token is required.
export async function health(): Promise<RepoHealth> {
  const res = await fetch("/api/v1/health", { credentials: "same-origin" });
  if (!res.ok) {
    throw new Error("Something went wrong. Check your connection and try again.");
  }
  return (await res.json()) as RepoHealth;
}

// --- Self-service profile (D-06) ---

// updateProfile changes the current user's display name only (no role).
export async function updateProfile(displayName: string): Promise<void> {
  await mutate<void>("/api/v1/profile", { display_name: displayName }, "PUT");
}

// changePassword changes the current user's password (verifies the current one
// server-side, enforces >=12 chars, clears must_change_password).
export async function changePassword(
  currentPassword: string,
  newPassword: string,
): Promise<void> {
  await mutate<void>(
    "/api/v1/profile/password",
    { current_password: currentPassword, new_password: newPassword },
    "PUT",
  );
}

// --- Admin user management (D-05, admin-only and server-side RBAC-gated) ---

export async function listUsers(): Promise<AdminUser[]> {
  const res = await fetch("/api/v1/admin/users", { credentials: "same-origin" });
  if (res.status === 403) {
    const err = new Error("You don't have permission to do that.") as Error & {
      status?: number;
    };
    err.status = 403;
    throw err;
  }
  if (!res.ok) {
    throw new Error("Something went wrong. Check your connection and try again.");
  }
  return (await res.json()) as AdminUser[];
}

export interface CreatedUser extends AdminUser {
  one_time_password: string;
}

export async function createUser(
  username: string,
  displayName: string,
  role: UserRole,
): Promise<CreatedUser> {
  return mutate<CreatedUser>("/api/v1/admin/users", {
    username,
    display_name: displayName,
    role,
  });
}

// setUserRole changes a user's role. The server enforces RBAC (admin-only) and
// rejects demoting the last active admin with 409 (surfaced via the thrown error).
export async function setUserRole(id: number, role: UserRole): Promise<void> {
  await mutate<void>(`/api/v1/admin/users/${id}/role`, { role }, "PUT");
}

export async function resetUserPassword(
  id: number,
): Promise<{ one_time_password: string }> {
  return mutate<{ one_time_password: string }>(
    `/api/v1/admin/users/${id}/reset-password`,
    undefined,
  );
}

export async function deactivateUser(id: number): Promise<void> {
  await mutate<void>(`/api/v1/admin/users/${id}/deactivate`, undefined);
}

// --- Pages, tree & folders (PAGE-01..03, NAV-01..05) ---

// TreeNode mirrors the backend nested navigation tree (SPEC §17.2). A folder
// carries children; a page is a leaf. The user never sees the path/filename —
// only the title — but the path is the route token the SPA navigates to.
export interface TreeNode {
  type: "folder" | "page";
  path: string;
  title: string;
  children?: TreeNode[];
}

// Page mirrors the GET /pages response: the frontmatter region (raw YAML text),
// the opaque Markdown body, and the optimistic-concurrency revision.
export interface Page {
  frontmatter: string;
  body: string;
  revision: string;
}

// getTree fetches the nested navigation tree (a GET — no CSRF needed).
export async function getTree(): Promise<TreeNode[]> {
  const res = await fetch("/api/v1/tree", { credentials: "same-origin" });
  if (!res.ok) {
    throw new Error("Couldn't load your pages — try again.");
  }
  return (await res.json()) as TreeNode[];
}

// getPage fetches a single page by its repo-relative path (a GET).
export async function getPage(path: string): Promise<Page> {
  const res = await fetch(`/api/v1/pages/${path}`, { credentials: "same-origin" });
  if (res.status === 404) {
    const err = new Error(
      "This page no longer exists. It may have been moved or deleted.",
    ) as Error & { status?: number };
    err.status = 404;
    throw err;
  }
  if (!res.ok) {
    throw new Error("Something went wrong. Check your connection and try again.");
  }
  return (await res.json()) as Page;
}

// createPage creates a page from a title in the selected folder (D-12). The
// backend slugs the filename; we get back the new page's path to navigate to.
export async function createPage(
  folder: string,
  title: string,
): Promise<{ path: string }> {
  return mutate<{ path: string }>("/api/v1/pages", { folder, title });
}

// savePage writes a new version of a page. base_revision carries the optimistic-
// concurrency token; a stale value surfaces as a 409 (err.status === 409).
export async function savePage(
  path: string,
  payload: { body: string; frontmatter: string; base_revision: string },
): Promise<void> {
  await mutate<void>(`/api/v1/pages/${path}`, payload, "PUT");
}

// createFolder creates a folder (seeded with a blank index.md, NAV-03).
export async function createFolder(parent: string, name: string): Promise<void> {
  await mutate<void>("/api/v1/folders", { parent, name });
}

// renamePage renames a page to a new title within its folder (PAGE-04). The
// backend slugs a new filename, rewrites every inbound link, and returns the new
// page path to navigate to. Links to this page keep working.
export async function renamePage(
  path: string,
  newTitle: string,
): Promise<{ path: string }> {
  return mutate<{ path: string }>(`/api/v1/pages/${path}/rename`, {
    new_title: newTitle,
  });
}

// movePage moves a page into another folder (PAGE-05). The same /rename endpoint
// dispatches on the new_parent field. Inbound links are rewritten and history
// stays continuous; the new path is returned.
export async function movePage(
  path: string,
  newParent: string,
): Promise<{ path: string }> {
  return mutate<{ path: string }>(`/api/v1/pages/${path}/rename`, {
    new_parent: newParent,
  });
}

// --- Trash: delete-to-trash & restore (PAGE-06/07) ---

// TrashEntry mirrors a deleted-page record (GET /trash). It carries provenance
// only — title, where the page came from, who deleted it, and when — never page
// content or any Git vocabulary.
export interface TrashEntry {
  id: number;
  title: string;
  original_path: string;
  deleted_by: string;
  deleted_at: string;
}

// deletePage moves a page to trash (PAGE-06). Delete is recoverable — the page
// moves to Trash and can be restored anytime; nothing is permanently removed.
export async function deletePage(path: string): Promise<void> {
  await mutate<void>(`/api/v1/pages/${path}`, undefined, "DELETE");
}

// listTrash fetches the deleted pages (a GET). The user sees title, who deleted
// it, and when — rendered as a relative time in the UI.
export async function listTrash(): Promise<TrashEntry[]> {
  const res = await fetch("/api/v1/trash", { credentials: "same-origin" });
  if (!res.ok) {
    throw new Error("Couldn't load Trash — try again.");
  }
  return (await res.json()) as TrashEntry[];
}

// restoreFromTrash restores a trashed page to its original folder (PAGE-07). If
// a page with the same name already exists, the backend restores this one as
// "{title} (restored)" so nothing is clobbered (D-10); the returned path is the
// page's actual restored location.
export async function restoreFromTrash(id: number): Promise<{ path: string }> {
  return mutate<{ path: string }>(`/api/v1/trash/${id}/restore`, undefined);
}

// --- Version history & restore (VER-02/03) ---

// HistoryEntry mirrors one row of a page's version history. It carries ONLY
// human-readable fields plus an OPAQUE version token (used to view/restore a
// version) — there is NO sha/hash/commit field, so no Git internals reach the UI
// (VER-02). The frontend renders action+who+when as "Edited by Sam · 2 hours
// ago"; the version token is never displayed.
export interface HistoryEntry {
  version: string;
  action: string;
  who: string;
  when: string;
}

// getHistory fetches a page's version history (a GET, newest-first). Available
// to any authenticated user.
export async function getHistory(path: string): Promise<HistoryEntry[]> {
  const res = await fetch(`/api/v1/pages/${path}/history`, {
    credentials: "same-origin",
  });
  if (!res.ok) {
    throw new Error("Couldn't load this page's history — try again.");
  }
  return (await res.json()) as HistoryEntry[];
}

// viewVersion fetches a page as it existed at the given opaque version token (a
// GET). Returns the same Page shape as getPage — read-only.
export async function viewVersion(
  path: string,
  version: string,
): Promise<Page> {
  const res = await fetch(`/api/v1/pages/${path}/version/${version}`, {
    credentials: "same-origin",
  });
  if (!res.ok) {
    throw new Error("Couldn't load that version — try again.");
  }
  return (await res.json()) as Page;
}

// restoreVersion restores a page to an old version as a NEW forward version
// (VER-03). The current version is kept in history — nothing is lost. The
// version token is the opaque handle the client received from getHistory.
export async function restoreVersion(
  path: string,
  version: string,
): Promise<{ path: string }> {
  return mutate<{ path: string }>(`/api/v1/pages/${path}/restore`, { version });
}

// --- Attachments (ATT-01/02/03/09, SEC-02) ---

// AttachmentMeta mirrors the backend upload/list response. The original filename
// lives here (never in the on-disk path — SEC-02). extraction_status drives the
// card chip (slice 02-03); in slice 02-01 every fresh upload is "pending".
export interface AttachmentMeta {
  id: string;
  original_name: string;
  mime_type: string;
  size_bytes: number;
  uploader_name: string;
  uploaded_at: string;
  page_path: string;
  extraction_status?: "pending" | "done" | "empty" | "failed";
}

// downloadAttachmentUrl is the canonical byte-exact download endpoint for an
// attachment id. Used as an <a href> / <img src> — a GET, no CSRF needed.
export function downloadAttachmentUrl(id: string): string {
  return `/api/v1/attachments/${id}/download`;
}

// ExtractionStatusValue is the live text-extraction state streamed over SSE. It
// drives the ExtractionStatus chip. "extracting" is the in-flight state (the
// server maps its pending row to this); done/empty/failed are terminal.
export type ExtractionStatusValue =
  | "extracting"
  | "done"
  | "empty"
  | "failed";

// subscribeExtractionStatus opens an SSE subscription to an attachment's live
// extraction status and invokes onStatus for each event. It returns an unsubscribe
// function that closes the EventSource. On a dropped stream the caller keeps its
// last-known state (no error flash), so onError simply closes — the chip degrades
// gracefully (UI-SPEC). A GET stream needs no CSRF.
export function subscribeExtractionStatus(
  id: string,
  onStatus: (status: ExtractionStatusValue) => void,
): () => void {
  const es = new EventSource(`/api/v1/attachments/${id}/status`);
  es.onmessage = (e: MessageEvent) => {
    try {
      const parsed = JSON.parse(e.data) as { status?: ExtractionStatusValue };
      if (parsed.status) onStatus(parsed.status);
    } catch {
      // Ignore a malformed event; keep the last-known state.
    }
  };
  es.onerror = () => {
    // The stream closed (server finished + closed, or the connection dropped).
    // Close our side so the browser does not auto-reconnect to a finished stream;
    // the chip keeps its last-known state.
    es.close();
  };
  return () => es.close();
}

// humanFileSize formats a byte count as a short, human-friendly string using the
// DECIMAL (SI, 1000-based) convention so "1.4 MB" reads the way the UI-SPEC shows
// it (matches what most users and OS file managers display). Sub-KB values are
// shown as whole bytes; KB and up carry one decimal place (trailing ".0" trimmed).
// This is presentation only — the opaque on-disk id is never derived from it.
export function humanFileSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return "0 B";
  if (bytes < 1000) return `${bytes} B`;
  const units = ["KB", "MB", "GB", "TB", "PB", "EB"];
  let value = bytes / 1000;
  let i = 0;
  while (value >= 1000 && i < units.length - 1) {
    value /= 1000;
    i++;
  }
  const rounded = value.toFixed(1).replace(/\.0$/, "");
  return `${rounded} ${units[i]}`;
}

// humanDate formats an RFC 3339 timestamp as a human-friendly day ("21 Jun 2026")
// via toLocaleDateString. It NEVER surfaces a raw timestamp, opaque id, or any
// Git/SHA vocabulary (hidden-Git rule). An unparseable input falls back to the
// raw string so the card still renders something rather than "Invalid Date".
export function humanDate(rfc3339: string): string {
  const d = new Date(rfc3339);
  if (Number.isNaN(d.getTime())) return rfc3339;
  return d.toLocaleDateString(undefined, {
    day: "numeric",
    month: "short",
    year: "numeric",
  });
}

// isPreviewableImage reports whether an attachment's stored MIME type is one of
// the inline-previewable image types (png/jpg/svg). The stored mime_type may
// carry parameters (e.g. "image/svg+xml; charset=utf-8"), so only the media type
// before the first ";" is compared. Mirrors the server's isInlineImage allow-list
// (SEC-02): an <img>-loaded SVG cannot execute script.
export function isPreviewableImage(mimeType: string): boolean {
  const media = mimeType.split(";", 1)[0].trim().toLowerCase();
  return (
    media === "image/png" ||
    media === "image/jpeg" ||
    media === "image/svg+xml"
  );
}

// listAttachments fetches a page's attachments (a GET, newest-first).
export async function listAttachments(
  pagePath: string,
): Promise<AttachmentMeta[]> {
  const res = await fetch(`/api/v1/attachments/${pagePath}`, {
    credentials: "same-origin",
  });
  if (!res.ok) {
    throw new Error("Couldn't load attachments — try again.");
  }
  return (await res.json()) as AttachmentMeta[];
}

// uploadAttachment uploads a file to a page via multipart. It uses a raw fetch
// (NOT mutate(), which forces a JSON Content-Type) so the browser sets the
// multipart boundary itself; the CSRF token is echoed in the header (SEC-04).
export async function uploadAttachment(
  pagePath: string,
  file: File,
): Promise<AttachmentMeta> {
  const token = await ensureCSRF();
  const form = new FormData();
  form.append("page_path", pagePath);
  form.append("file", file);
  const res = await fetch("/api/v1/attachments", {
    method: "POST",
    credentials: "same-origin",
    headers: { [CSRF_HEADER]: token },
    body: form,
  });
  if (!res.ok) {
    let message = "Upload didn't finish. Check your connection and try again.";
    try {
      const data = (await res.json()) as { error?: string };
      if (data.error) message = data.error;
    } catch {
      // keep the generic message
    }
    const err = new Error(message) as Error & { status?: number };
    err.status = res.status;
    throw err;
  }
  return (await res.json()) as AttachmentMeta;
}

// replaceAttachment swaps an attachment's bytes in place, reusing the SAME id
// (ATT-05). Like uploadAttachment it uses a raw multipart fetch (NOT mutate(),
// which forces a JSON Content-Type) so the browser sets the boundary itself; the
// CSRF token is echoed in the header (SEC-04). The server re-validates size/type
// and returns the updated meta.
export async function replaceAttachment(
  id: string,
  file: File,
): Promise<AttachmentMeta> {
  const token = await ensureCSRF();
  const form = new FormData();
  form.append("file", file);
  const res = await fetch(`/api/v1/attachments/${id}`, {
    method: "PUT",
    credentials: "same-origin",
    headers: { [CSRF_HEADER]: token },
    body: form,
  });
  if (!res.ok) {
    let message = "Replacing didn't finish. Check your connection and try again.";
    try {
      const data = (await res.json()) as { error?: string };
      if (data.error) message = data.error;
    } catch {
      // keep the generic message
    }
    const err = new Error(message) as Error & { status?: number };
    err.status = res.status;
    throw err;
  }
  return (await res.json()) as AttachmentMeta;
}

// removeAttachment drops an attachment's link from a page and, when that was the
// last reference, deletes the file (ATT-06/07). The page to unlink is passed via
// the page_path query parameter. Returns whether the underlying file was deleted.
export async function removeAttachment(
  id: string,
  pagePath: string,
): Promise<{ deleted_orphan: boolean }> {
  return mutate<{ deleted_orphan: boolean }>(
    `/api/v1/attachments/${id}?page_path=${encodeURIComponent(pagePath)}`,
    undefined,
    "DELETE",
  );
}

// --- Search (SRCH-01/02/03/06) ---

// SearchResultKind is the typed kind of a search hit. Page results are live now;
// heading/attachment kinds render the moment 03-03 starts returning them (the
// SPA needs no further change).
export type SearchResultKind = "page" | "heading" | "attachment";

// SearchResult mirrors one item of the GET /api/v1/search response. snippet is a
// server fragment carrying ONLY weight-only highlight markers (<strong>) — it is
// sanitized client-side in highlight.ts and never injected as raw HTML (the XSS
// chokepoint, Phase 1 stored-XSS guard). path is the page to navigate to (the
// owning page for heading/attachment); anchor deep-links a heading section.
export interface SearchResult {
  kind: SearchResultKind;
  title: string;
  path: string;
  snippet: string;
  anchor?: string;
  page_title?: string;
}

// search runs a full-text query against GET /api/v1/search (a GET — no CSRF).
// A blank query short-circuits to [] with no network call (matches the server's
// empty-query fast path and keeps the palette quiet until the user types). A
// non-ok response throws a generic, internals-free message (T-03-09).
export async function search(q: string): Promise<SearchResult[]> {
  if (!q.trim()) return [];
  const res = await fetch(`/api/v1/search?q=${encodeURIComponent(q)}`, {
    credentials: "same-origin",
  });
  if (!res.ok) {
    throw new Error("Search is unavailable. Try again in a moment.");
  }
  return (await res.json()) as SearchResult[];
}

// reindexSearch triggers an admin rebuild of the search index (POST, CSRF-
// protected, admin-only server-side). The endpoint returns 202 (async — the
// rebuild runs in the background), so this resolves once the request is accepted.
// Uses no Git/index/Bleve vocabulary in the thrown error (hidden-Git rule).
export async function reindexSearch(): Promise<void> {
  await mutate<void>("/api/v1/admin/search/reindex", undefined);
}

// relativeMdLink computes a relative `.md` link destination from the page at
// fromPath to the page at toPath (both repo-relative slash paths), matching the
// canonical on-disk link format (D-05). Used by the LinkPicker so an inserted
// link is a portable relative path (e.g. ../runbooks/deploy.md), not a wiki or
// ID link. In read mode such links navigate within the app (D-06).
export function relativeMdLink(fromPath: string, toPath: string): string {
  const fromDir = fromPath.split("/").slice(0, -1);
  const toParts = toPath.split("/");
  let i = 0;
  while (
    i < fromDir.length &&
    i < toParts.length - 1 &&
    fromDir[i] === toParts[i]
  ) {
    i++;
  }
  const ups = fromDir.slice(i).map(() => "..");
  const downs = toParts.slice(i);
  const rel = [...ups, ...downs].join("/");
  return rel === "" ? toParts[toParts.length - 1] : rel;
}
