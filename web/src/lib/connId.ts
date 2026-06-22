// getConnId returns this browser tab's stable, opaque connection id — the soft-lock
// SessionID (COLL-02) and (in Plan 03) the presence stream identity. It is
// client-generated per RESEARCH A3: the server NEVER trusts it for identity (the
// lock username/user_id come from the session), only for self/dedup matching, so a
// random UUID is sufficient and never a path component.
//
// It is persisted in sessionStorage so a reload within the same tab keeps the same
// id (the lock the tab already holds is then refreshed, not seen as held-by-other),
// while a second tab gets its own id (two tabs of one user are distinct sessions).
// A read-or-create with an in-memory fallback keeps it working when sessionStorage
// is unavailable (private mode / disabled storage) — a fresh id per call is the
// safe degradation (worst case: a reload looks like a new session, GC reaps the old
// lock within the TTL).

const STORAGE_KEY = "okf.connId";

let memoryFallback: string | null = null;

function newId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  // Fallback for environments without crypto.randomUUID (older/test runtimes):
  // an opaque-enough token; identity safety does not depend on it being a v4 UUID.
  return `c-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

export function getConnId(): string {
  try {
    const existing = sessionStorage.getItem(STORAGE_KEY);
    if (existing) return existing;
    const id = newId();
    sessionStorage.setItem(STORAGE_KEY, id);
    return id;
  } catch {
    // sessionStorage unavailable — keep a per-page-load id in memory so all lock
    // calls within this load share one SessionID.
    if (!memoryFallback) memoryFallback = newId();
    return memoryFallback;
  }
}
