// Package locks is the server-authoritative, file-backed soft-lock store for
// OKF Workspace collaboration (COLL-02). A lock marks a page as being edited by
// one live session so a second editor sees held-by-other instead of silently
// racing the write. Lock truth lives as a plain JSON file under
// `.okf-workspace/locks/` (mirror-the-tree), never inside the page body and
// never in SQLite — every lock-file touch routes through the repo.* SEC-01
// chokepoint (repo.Resolve), so a traversal-shaped page path can never let a
// lock escape the repo root.
//
// The store is pure and network-free: the clock is injected (now func()
// time.Time) so the load-bearing TTL/expiry/GC tests run deterministically with
// zero sleeps. No HTTP, SSE, or UI lives here — those land in later slices,
// which fill the Owner identity FROM THE SESSION (the service never trusts a
// client-named username/user_id; only the opaque SessionID is client-supplied).
package locks

import "time"

// Lock is the on-disk lock record (CONTEXT-locked JSON shape). One lock file
// per page at `.okf-workspace/locks/{pagePath}.lock`. Username/UserID identify
// the holder for the held-by-other / presence UI; LockedAt/ExpiresAt drive the
// TTL takeover and GC reaping. SessionID is the opaque client-supplied
// connection id that distinguishes two tabs of one logged-in user.
type Lock struct {
	Username  string    `json:"username"`
	UserID    int64     `json:"user_id"`
	SessionID string    `json:"session_id"`
	LockedAt  time.Time `json:"locked_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Owner is the identity the caller hands to Acquire/Refresh/Force. The HTTP
// layer (a later slice) fills Username/UserID FROM THE SESSION (server-trusted,
// never a free-form client field) and SessionID from the client-supplied
// connection id. Keeping this a distinct type from Lock fixes the SHAPE so a
// future handler cannot pass a client-named username.
type Owner struct {
	Username  string
	UserID    int64
	SessionID string
}

// Editor is one live holder in an EditorsFor presence snapshot for a page. You
// is true when the holder's SessionID matches the requesting connection id, so
// the snapshot never shows the caller their own lock (two-tabs-one-browser).
type Editor struct {
	Username string `json:"username"`
	You      bool   `json:"you"`
}
