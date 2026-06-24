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
  type: "folder" | "page" | "attachment";
  path: string;
  title: string;
  children?: TreeNode[];
  // Attachments hang off a page node as leaf children (type "attachment", where
  // `path` is the attachment id and `title` is the original filename).
  attachments?: TreeNode[];
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

// --- Soft locks (COLL-02) ---
//
// Three CSRF-bearing POSTs cloning the createPage/savePage mutate() pattern. The
// page path is interpolated into the URL exactly like savePage (slashes
// preserved); the ONLY body field is the opaque client connection id (conn) — the
// lock's username/user_id are filled server-side FROM THE SESSION, never sent from
// the client. Force is take-over-only (it never calls savePage and never touches
// base_revision — the save-time revision check stays the sole write authority).

// AcquireLockResult mirrors POST /pages/{path}/lock. "acquired" means you now hold
// (or refreshed) the lock; "held-by-other" means a different live session holds it
// and holder.username names them for the SoftLockBanner ("{name} is editing this
// page."). holder is present ONLY on held-by-other and carries only the username.
export interface AcquireLockResult {
  result: "acquired" | "held-by-other";
  holder?: { username: string };
}

// acquireLock acquires (or, for the same connection, refreshes) the soft lock on a
// page. Acquire doubles as the heartbeat refresh: re-calling it with the same conn
// re-stamps the TTL (the store treats a same-session Acquire as a refresh).
export async function acquireLock(
  path: string,
  conn: string,
): Promise<AcquireLockResult> {
  return mutate<AcquireLockResult>(`/api/v1/pages/${path}/lock`, { conn });
}

// forceLock unconditionally takes over the soft lock for this connection (the
// "Force edit" affordance). It is take-over-ONLY — it never saves and never alters
// base_revision; a subsequent save still revision-checks (the load-bearing rule).
export async function forceLock(path: string, conn: string): Promise<void> {
  await mutate<void>(`/api/v1/pages/${path}/lock/force`, { conn });
}

// releaseLock best-effort releases the soft lock on unmount / leaving Edit. The
// server only deletes a lock whose session matches conn (idempotent), and GC is
// the backstop — so callers SWALLOW errors at the call site (a failed release is
// reaped by the TTL).
export async function releaseLock(path: string, conn: string): Promise<void> {
  await mutate<void>(`/api/v1/pages/${path}/lock/release`, { conn });
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

// renameFolder renames a folder (its index.md + every descendant page) in ONE
// commit, rewriting all inbound links (TREE-02). The backend slugs the new dir name
// and returns the new folder path to navigate to. A target-dir collision rejects
// with HTTP 409 (TREE-06); the thrown error carries err.status === 409 so the dialog
// can surface the collision copy.
export async function renameFolder(
  dir: string,
  newName: string,
): Promise<{ path: string }> {
  return mutate<{ path: string }>(`/api/v1/pages/${dir}/rename-folder`, {
    new_name: newName,
  });
}

// moveFolder relocates a folder subtree into newParent ("" = root) in ONE commit,
// rewriting all inbound links (TREE-02). Returns the new folder path. A destination
// collision rejects with HTTP 409 (TREE-06, err.status === 409).
export async function moveFolder(
  dir: string,
  newParent: string,
): Promise<{ path: string }> {
  return mutate<{ path: string }>(`/api/v1/pages/${dir}/move-folder`, {
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
  // delete_group_id groups every row produced by ONE folder delete (TREE-04/05) so
  // the TrashView can render them as a single restorable unit. Empty for a solo
  // per-page delete.
  delete_group_id: string;
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

// deleteFolder moves a whole folder (its index.md + every descendant page) to Trash
// under one shared delete_group_id (TREE-04). Recoverable — the folder can be
// restored as a unit from Trash; nothing is permanently removed. Dispatched on the
// /pages/* POST catch-all by the "/delete-folder" suffix.
export async function deleteFolder(dir: string): Promise<void> {
  await mutate<void>(`/api/v1/pages/${dir}/delete-folder`, undefined);
}

// restoreFolderGroup restores a folder-delete as a unit (TREE-05), index.md first.
// If a page already exists at one of the original paths, that page is restored as
// "{title} (restored)" so nothing is clobbered; the returned paths are the pages'
// actual restored locations. groupId is the opaque delete_group_id from a TrashEntry.
export async function restoreFolderGroup(
  groupId: string,
): Promise<{ paths: string[] }> {
  return mutate<{ paths: string[] }>(
    `/api/v1/trash/group/${groupId}/restore`,
    undefined,
  );
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

// --- Presence (COLL-01) ---

// PresenceSnapshot is one full-state presence frame pushed by the per-page
// presence SSE stream. editors is the complete set of live lock holders (one, at
// most, given one-lock-per-page), each carrying only a username + a you bool —
// never a session id (the server filters that out, T-05-12). you_hold_lock is
// whether THIS connection holds the page's live lock, so a consumer can reconcile
// presence with its own lock state from the same stream.
export interface PresenceSnapshot {
  editors: { username: string; you: boolean }[];
  you_hold_lock: boolean;
}

// PresenceState is the connection lifecycle the PresenceIndicator renders around
// the snapshot: "connecting" before the first frame/open, "open" once streaming,
// "reconnecting" after a drop (the native EventSource auto-reconnects).
export type PresenceState = "connecting" | "open" | "reconnecting";

// subscribePresence opens an SSE subscription to a page's live editing presence
// (COLL-01) and invokes onSnapshot for each full-state frame. It mirrors
// subscribeExtractionStatus: a GET EventSource (no CSRF), JSON.parse per message
// (keep the last snapshot on a parse error), and an unsubscribe that closes the
// stream. Unlike the extraction stream, presence has no terminal state, so onError
// does NOT close — it reports "reconnecting" and lets the native EventSource
// auto-reconnect (the connection IS the heartbeat). onState surfaces the lifecycle
// for the indicator's connecting/reconnecting copy.
export function subscribePresence(
  path: string,
  conn: string,
  onSnapshot: (snapshot: PresenceSnapshot) => void,
  onState?: (state: PresenceState) => void,
): () => void {
  const es = new EventSource(
    `/api/v1/pages/${path}/presence?conn=${encodeURIComponent(conn)}`,
  );
  es.onopen = () => {
    onState?.("open");
  };
  es.onmessage = (e: MessageEvent) => {
    try {
      const parsed = JSON.parse(e.data) as PresenceSnapshot;
      onState?.("open");
      onSnapshot(parsed);
    } catch {
      // Ignore a malformed event; keep the last-known snapshot.
    }
  };
  es.onerror = () => {
    // The stream dropped. Presence has no terminal state, so do NOT close — the
    // native EventSource auto-reconnects; surface the degraded "reconnecting"
    // awareness state in the meantime.
    onState?.("reconnecting");
  };
  return () => es.close();
}

// --- Agent (AGNT-01..AGNT-10) ---

// AgentScope is the retrieval scope the PromptBar sends with each Ask/stream
// request. It mirrors the backend scopeKindFromRequest mapping: an unknown value
// falls back server-side to the safe read-only page Ask, never widening access.
export type AgentScope = "page" | "selection" | "attachment" | "workspace";

// AgentChatRequest is the POST body for the streamed Ask endpoint. The actor and
// the role that scopes retrieval are taken from the SESSION server-side — never
// from this body — so a workspace Ask can only reach pages the role may read.
export interface AgentChatRequest {
  prompt: string;
  scope: AgentScope;
  page_path?: string;
  selection?: string;
  attachment_id?: string;
}

// AgentStreamHandlers are the callbacks the fetch-stream consumer invokes as it
// decodes SSE frames. onToken fires per answer delta (append to the live answer);
// onCitation fires once on the workspace "Reasoned over:" frame (page paths);
// onDone fires on clean end-of-stream; onError fires on a mid-stream `event:
// error` frame OR a pre-stream structured HTTP error (e.g. agent off → 503,
// provider unreachable → 502). disabled distinguishes the agent-off 503 so the
// PromptBar can render its dedicated "turned off" copy vs. a transient failure.
export interface AgentStreamHandlers {
  onToken: (delta: string) => void;
  onCitation?: (paths: string[]) => void;
  onDone?: () => void;
  onError?: (message: string, opts: { disabled: boolean; unreachable: boolean }) => void;
}

// subscribeAgentChat opens the streamed Ask (AGNT-01). Unlike
// subscribeExtractionStatus (an EventSource GET), the agent chat is an
// authenticated POST-with-body token stream — EventSource cannot send a body or
// the CSRF header — so this uses a fetch + ReadableStream reader. It returns an
// unsubscribe function that aborts the in-flight fetch (which cancels the
// server's request context and tears the stream down cleanly). It is fail-closed:
// a 503 (agent off) or 502 (provider unreachable) JSON error is surfaced via
// onError BEFORE any token, never a silent hang.
//
// The SSE wire format (internal/agent/stream.go) is: `data: <delta>` answer
// frames (multi-line deltas continue across `data:` lines within one event),
// `event: citation\ndata: [paths]`, a mid-stream `event: error\ndata: <msg>`,
// and a terminal `event: done\ndata: {}`.
export function subscribeAgentChat(
  req: AgentChatRequest,
  handlers: AgentStreamHandlers,
): () => void {
  const controller = new AbortController();

  void (async () => {
    let token: string;
    try {
      token = await ensureCSRF();
    } catch {
      handlers.onError?.("Could not initialize the session.", {
        disabled: false,
        unreachable: false,
      });
      return;
    }

    let res: Response;
    try {
      res = await fetch("/api/v1/agent/chat", {
        method: "POST",
        credentials: "same-origin",
        headers: {
          "Content-Type": "application/json",
          [CSRF_HEADER]: token,
        },
        body: JSON.stringify(req),
        signal: controller.signal,
      });
    } catch {
      if (controller.signal.aborted) return; // caller cancelled — silent
      handlers.onError?.(
        "That didn't go through — check your connection and try again.",
        { disabled: false, unreachable: false },
      );
      return;
    }

    // A non-2xx response is a structured JSON error emitted BEFORE the first SSE
    // byte (fail-closed): 503 = agent off, 502 = provider unreachable. Surface
    // the dedicated copy so the PromptBar never hangs.
    if (!res.ok || !res.body) {
      const disabled = res.status === 503;
      const unreachable = res.status === 502;
      let message = unreachable
        ? "The assistant is unavailable right now. Try again in a moment."
        : disabled
          ? "The assistant is turned off."
          : "That didn't go through — check your connection and try again.";
      try {
        const data = (await res.json()) as { error?: string };
        if (data.error) message = data.error;
      } catch {
        // keep the status-derived message
      }
      handlers.onError?.(message, { disabled, unreachable });
      return;
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    // Parse one complete SSE event (frames are separated by a blank line). An
    // event may carry an `event:` name and one or more `data:` lines; multi-line
    // answer deltas arrive as consecutive `data:` continuation lines (joined with
    // newlines, mirroring the server's escapeSSE).
    function handleEvent(raw: string) {
      let name = "message";
      const dataLines: string[] = [];
      for (const line of raw.split("\n")) {
        if (line.startsWith("event:")) {
          name = line.slice(6).trim();
        } else if (line.startsWith("data:")) {
          // Preserve the single space after "data:" semantics: strip exactly one
          // leading space if present (SSE convention) without trimming content.
          dataLines.push(line.slice(5).replace(/^ /, ""));
        }
      }
      const data = dataLines.join("\n");
      if (name === "citation") {
        try {
          const paths = JSON.parse(data) as string[];
          if (Array.isArray(paths)) handlers.onCitation?.(paths);
        } catch {
          // ignore a malformed citation frame — the answer already streamed
        }
      } else if (name === "error") {
        handlers.onError?.(data || "The assistant could not finish this answer.", {
          disabled: false,
          unreachable: false,
        });
      } else if (name === "done") {
        handlers.onDone?.();
      } else if (data) {
        handlers.onToken(data);
      }
    }

    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        let sep: number;
        // SSE events are delimited by a blank line ("\n\n").
        while ((sep = buffer.indexOf("\n\n")) !== -1) {
          const rawEvent = buffer.slice(0, sep);
          buffer = buffer.slice(sep + 2);
          if (rawEvent.trim() !== "") handleEvent(rawEvent);
        }
      }
      // Flush any trailing event without a final blank line.
      if (buffer.trim() !== "") handleEvent(buffer);
      handlers.onDone?.();
    } catch {
      if (controller.signal.aborted) return; // caller cancelled — silent
      handlers.onError?.(
        "Something went wrong while answering. Your prompt is kept — try again.",
        { disabled: false, unreachable: false },
      );
    }
  })();

  return () => controller.abort();
}

// ProposePatchResult mirrors POST /agent/propose-patch (editor). The client
// renders a REAL diff from old_body ↔ new_body — the server never returns a prose
// summary. base_revision is the optimistic-concurrency token captured at proposal
// time, echoed back to applyPatch so a moved revision 409s instead of overwriting.
export interface ProposePatchResult {
  page_path: string;
  old_body: string;
  new_body: string;
  base_revision: string;
}

// proposePatch asks the assistant to propose a whole-body change to a page
// (AGNT-09, editor-gated server-side). It returns the old + new body for the
// DiffReviewDialog; it NEVER writes — apply is the separate gated endpoint below.
export async function proposePatch(
  pagePath: string,
  instruction: string,
): Promise<ProposePatchResult> {
  return mutate<ProposePatchResult>("/api/v1/agent/propose-patch", {
    page_path: pagePath,
    instruction,
  });
}

// --- Single-shot agent modes: Summarize / Rewrite / Draft (AGNT-05..08) ---
//
// Unlike the streamed Ask, these are AWAITED JSON calls: the whole result is
// returned at once (a summary string, a rewritten span, or a drafted body). None
// auto-writes — Rewrite/Draft return proposals the UI routes to the diff dialog or
// editor. The server is fail-closed (503 agent off, 502 unreachable, 422 the model
// could not produce a clean body); mutate() surfaces the server's error message.

// summarizePage summarizes the open page (AGNT-05). page_path is required; no
// selection. Returns the summary text.
export async function summarizePage(pagePath: string): Promise<string> {
  const res = await mutate<{ summary: string }>("/api/v1/agent/summarize-page", {
    page_path: pagePath,
  });
  return res.summary;
}

// summarizeAttachment summarizes an attachment's extracted text (AGNT-06). A 422
// (no readable text yet) surfaces as a thrown error with the server's message.
export async function summarizeAttachment(attachmentId: string): Promise<string> {
  const res = await mutate<{ summary: string }>(
    "/api/v1/agent/summarize-attachment",
    { attachment_id: attachmentId },
  );
  return res.summary;
}

// rewrite rewrites a selected span per an instruction (AGNT-07, editor-gated
// server-side). selection is the UNTRUSTED span; instruction is how to change it.
// Returns the rewritten text as a PROPOSAL — the caller diffs old↔new and routes
// to the review dialog; it never auto-applies.
export async function rewrite(
  selection: string,
  instruction: string,
): Promise<string> {
  const res = await mutate<{ rewritten: string }>("/api/v1/agent/rewrite", {
    selection,
    instruction,
  });
  return res.rewritten;
}

// draft drafts a full new-page body from an instruction (AGNT-08). The returned
// body opens in the editor pending an explicit save — never auto-written.
export async function draft(instruction: string): Promise<string> {
  const res = await mutate<{ body: string }>("/api/v1/agent/draft", {
    instruction,
  });
  return res.body;
}

// applyPatch applies an approved patch (AGNT-10, editor + CSRF). base_revision is
// the token proposePatch captured; a stale value (the page moved while the user
// reviewed) surfaces as a 409 (err.status === 409) so the DiffReviewDialog shows
// its "page changed — re-run" stale state and never overwrites a concurrent edit.
export async function applyPatch(payload: {
  page_path: string;
  new_body: string;
  frontmatter: string;
  base_revision: string;
}): Promise<void> {
  await mutate<void>("/api/v1/agent/apply-patch", payload);
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

// reindexGraph triggers an admin rebuild of the derived link/graph store from
// files (POST, CSRF-protected, admin-only server-side). The endpoint returns 202
// (async — the rebuild runs in the background), so this resolves once the request
// is accepted. Uses no Git/index/Bleve vocabulary in the thrown error (hidden-Git
// rule).
export async function reindexGraph(): Promise<void> {
  await mutate<void>("/api/v1/admin/graph/reindex", undefined);
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
