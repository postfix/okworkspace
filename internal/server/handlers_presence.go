package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/postfix/okworkspace/internal/locks"
)

// presenceTick is how often the presence stream re-reads the lock store and
// re-emits the editors snapshot. It is deliberately coarser than the
// extraction-status sseTick (500ms): presence is ambient awareness, not a live
// progress chip, and the underlying lock TTL moves on the order of minutes, so a
// ~2s cadence keeps the "{name} is editing" line feeling current while staying
// cheap at this scale (5 users).
const presenceTick = 2 * time.Second

// presenceMaxDuration is an absolute cap on how long a single presence stream may
// stay open (T-05-11). The presence stream never reaches a terminal state on its
// own (a page may be edited indefinitely), so without this cap a forgotten editor
// tab would pin a goroutine + a lock-store read every tick forever. On the
// deadline we simply close; the client's native EventSource will reconnect if the
// tab is still genuinely editing, so a real editor is not cut off — only a leaked
// goroutine is reaped. It mirrors sseMaxDuration's role for handleExtractionStatus.
const presenceMaxDuration = 30 * time.Minute

// presenceSnapshot is one full-state presence frame pushed per tick. Editors is
// the complete set of live lock holders for the page (one, at most, given the
// one-lock-per-page model), each carrying only a username + a you bool — never a
// session id, user id, or another user's connection id (T-05-12). YouHoldLock is
// whether the page's live lock is held by THIS connection, so the client can
// reconcile presence with its own lock state from the same stream (one snapshot
// stream carries both, per RESEARCH A2/A7).
type presenceSnapshot struct {
	Editors     []locks.Editor `json:"editors"`
	YouHoldLock bool           `json:"you_hold_lock"`
}

// handlePresence streams a per-page editing-presence snapshot as Server-Sent
// Events so the toolbar PresenceIndicator can show "{name} is editing" before a
// collision (COLL-01). Presence is DERIVED from the soft-lock store (Slice 1): a
// "currently editing" user is a live lock holder, read via lockStore.EditorsFor.
//
// It is dispatched on the GET /pages/* catch-all by the ".md/presence" suffix and
// so runs under the authed group — any authenticated user may observe presence,
// read-only, no CSRF (a GET), same authority as reading the page itself. The
// client connection id arrives as the opaque ?conn= query param and is used ONLY
// to mark you:true / derive YouHoldLock (self-exclusion) — never as a path
// component (the page path is the validated wildcard).
//
// The handler clones handleExtractionStatus's skeleton verbatim: Flusher assert →
// 500 if unsupported; the full SSE header set; emit the current snapshot
// immediately; then a ticker loop that re-emits each tick and closes on
// r.Context().Done() (client disconnect) or an absolute max-duration cap so a
// forgotten tab cannot leak a goroutine (T-05-11). The server sets no global
// WriteTimeout (see main.go), so this long-lived response is not killed mid-stream.
func (h *authHandlers) handlePresence(w http.ResponseWriter, r *http.Request, path string) {
	if h.locks == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming is not supported.")
		return
	}

	// The opaque client connection id — used solely to mark you:true / derive
	// YouHoldLock (self/dedup), never as a path component.
	conn := r.URL.Query().Get("conn")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Never let an intermediary buffer an SSE stream (e.g. nginx) — defense in depth.
	w.Header().Set("X-Accel-Buffering", "no")

	ctx := r.Context()

	// emit reads the current live editors for the page and writes one SSE snapshot
	// frame. The snapshot surfaces only username + you (T-05-12); YouHoldLock is a
	// cheap derive — the page's live lock SessionID == conn.
	emit := func() error {
		eds, err := h.locks.EditorsFor(ctx, path, conn)
		if err != nil {
			return err
		}
		youHold := false
		for _, e := range eds {
			if e.You {
				youHold = true
				break
			}
		}
		b, err := json.Marshal(presenceSnapshot{Editors: eds, YouHoldLock: youHold})
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
		return nil
	}

	// Emit the current state immediately so a freshly opened editor sees who else
	// is editing without waiting a full tick.
	if err := emit(); err != nil {
		return
	}

	ticker := time.NewTicker(presenceTick)
	defer ticker.Stop()
	// Absolute max-duration cap (T-05-11): presence never terminates on its own, so
	// a forgotten tab must not pin this goroutine forever. On the deadline we close;
	// a genuinely active editor's EventSource reconnects.
	deadline := time.NewTimer(presenceMaxDuration)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			return
		case <-ticker.C:
			if err := emit(); err != nil {
				return
			}
		}
	}
}
