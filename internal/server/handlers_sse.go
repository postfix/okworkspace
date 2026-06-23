package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/postfix/okworkspace/internal/attachments"
)

// sseTick is how often the extraction-status stream re-reads the row. Short enough
// that the chip feels live, long enough to stay cheap at this scale (5 users).
const sseTick = 500 * time.Millisecond

// sseMaxDuration is a generous absolute cap on how long a single extraction-status
// stream may stay open (WR-03). A wedged extraction (status stuck at
// pending/extracting because the worker stalled or the row never advances) would
// otherwise pin a goroutine + a 500ms DB query forever for every tab left open. On
// the deadline we emit one terminal "failed" event and close, so a stuck stream
// cannot leak a goroutine indefinitely. It is far longer than any real extraction
// so it never truncates a healthy in-flight stream.
const sseMaxDuration = 10 * time.Minute

// handleExtractionStatus streams an attachment's text-extraction status as
// Server-Sent Events so the card chip transitions live (extracting → done/empty/
// failed) without polling from the client. It is dispatched on the GET
// /attachments/* catch-all (id derived from the "{id}/status" suffix) and so runs
// under the authed group — any authenticated user may read status, read-only, no
// CSRF (a GET) — same authority as reading a page (T-02-13).
//
// The stream emits the CURRENT status immediately, then re-reads on a ticker and
// emits on each tick, closing when a TERMINAL status (done/empty/failed) is
// reached or the client disconnects (r.Context().Done()). On disconnect the UI
// falls back to its last-known state (no error flash), so a dropped stream is
// graceful by design (Pitfall 7). The server sets NO global WriteTimeout (see
// main.go), so this long-lived response is not killed mid-stream; if one is ever
// added, this route must be exempted or use http.ResponseController to extend the
// per-connection deadline.
func (h *authHandlers) handleExtractionStatus(w http.ResponseWriter, r *http.Request, id string) {
	if h.attachments == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	if id == "" || strings.ContainsAny(id, "/\x00") {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming is not supported.")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Never let an intermediary buffer an SSE stream (e.g. nginx) — defense in depth.
	w.Header().Set("X-Accel-Buffering", "no")

	ctx := r.Context()

	// emit reads the current status and writes one SSE event. It returns whether the
	// status is terminal (so the caller can close the stream) and any read error.
	emit := func() (terminal bool, err error) {
		status, serr := h.attachments.ExtractionStatus(ctx, id)
		if serr != nil {
			if errors.Is(serr, attachments.ErrAttachmentNotFound) {
				// The row is gone (or never existed): nothing to stream. Emit a
				// terminal "failed" so the client stops waiting, then close.
				_, _ = fmt.Fprintf(w, "data: {\"status\":%q}\n\n", "failed")
				flusher.Flush()
				return true, nil
			}
			return false, serr
		}
		// Map the in-flight DB value (pending) to the client-facing "extracting"
		// term so the SSE shape is {extracting|done|empty|failed} per the contract.
		wire := string(status)
		if status == attachments.ExtractionPending {
			wire = "extracting"
		}
		_, _ = fmt.Fprintf(w, "data: {\"status\":%q}\n\n", wire)
		flusher.Flush()
		return attachments.IsTerminalStatus(status), nil
	}

	// Emit the current state immediately so a stream that connects after extraction
	// already finished still gets exactly one terminal event and closes.
	if terminal, err := emit(); err != nil || terminal {
		return
	}

	ticker := time.NewTicker(sseTick)
	defer ticker.Stop()
	// Absolute max-duration cap (WR-03): a wedged extraction must not pin this
	// goroutine forever. On the deadline, emit a terminal "failed" event and close.
	deadline := time.NewTimer(sseMaxDuration)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			// Stream has been open too long without reaching a terminal status —
			// tell the client to stop waiting and close (WR-03).
			_, _ = fmt.Fprintf(w, "data: {\"status\":%q}\n\n", "failed")
			flusher.Flush()
			return
		case <-ticker.C:
			if terminal, err := emit(); err != nil || terminal {
				return
			}
		}
	}
}
