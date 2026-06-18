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
  // logout returns 204 No Content.
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
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
