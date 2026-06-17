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
  method: "POST" | "PUT" = "POST",
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
