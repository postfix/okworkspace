// API client for the OKF Workspace backend. Sends cookies (credentials) and
// echoes the nosurf CSRF token in the X-CSRF-Token header on every mutating
// request (SEC-04). The token is fetched from GET /api/v1/csrf and cached.

export interface Me {
  username: string;
  display_name: string;
  role: string;
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

async function mutate<T>(path: string, body: unknown): Promise<T> {
  const token = await ensureCSRF();
  const res = await fetch(path, {
    method: "POST",
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
