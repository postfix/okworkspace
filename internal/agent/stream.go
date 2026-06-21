// stream.go bridges an Eino ReAct agent's streaming output to an http.Flusher
// Server-Sent Events response, mirroring the in-repo SSE template
// (internal/server/handlers_sse.go): the same four headers (incl.
// X-Accel-Buffering:no), `data: %s\n\n` framing, and request-context cancel on
// client disconnect.
//
// The one addition over the extraction-status stream is `defer sr.Close()`: that
// handler owns no producer goroutine, but the Eino StreamReader DOES — failing
// to Close it leaks that goroutine (T-04-06 / RESEARCH §Item 6, NON-NEGOTIABLE).
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ErrStreamingUnsupported is returned when the ResponseWriter is not an
// http.Flusher (no SSE possible). Handlers map it to a structured error before
// any stream bytes are written.
var ErrStreamingUnsupported = errors.New("agent: streaming not supported by the response writer")

// AskStream runs a scope-aware Ask turn for question and streams the answer
// token-by-token as SSE into w. The scope (page | selection | attachment |
// workspace) selects the system prompt and how context is assembled (AGNT-02/
// 03/04) over the SAME read-only ReAct agent — no new tool is added.
//
// It returns ErrAgentDisabled when the agent is off, ErrStreamingUnsupported
// when w cannot flush, and any provider/build error BEFORE the first byte is
// written so the caller can emit a clean structured error (never a silent hang).
//
// For workspace scope the answer is search-backed RAG (top-K, never a dump): the
// page paths the agent actually retrieved are captured from the tool-call trace
// and emitted as a terminal `event: citation` SSE frame BEFORE `event: done` so
// the frontend can render the "Reasoned over:" line (D3 / RESEARCH Q2). Only
// role-readable pages can appear there because Deps.Search is role-scoped by the
// caller. The returned []string is the same retrieved path set (empty for non-
// workspace scopes / when nothing was retrieved) for the caller's audit/use.
//
// Once streaming has begun, a mid-stream provider error or a client disconnect
// ends the loop cleanly: the request context (passed straight to ag.Stream)
// cancels the model call and unblocks Recv(); defer sr.Close() reaps the
// producer goroutine on every exit path.
func (s *Service) AskStream(ctx context.Context, w http.ResponseWriter, question string, sc Scope) ([]string, error) {
	if s.cm == nil {
		return nil, ErrAgentDisabled
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, ErrStreamingUnsupported
	}

	sc = sc.normalize()
	// A per-request trace records which pages the agent retrieves so the
	// workspace citation comes from the REAL tool calls, not the model's prose.
	trace := newScopeTrace()

	ag, err := s.buildReActAgent(ctx, trace)
	if err != nil {
		return nil, err
	}

	msgs := buildScopedMessages(question, sc)

	// Start the stream BEFORE writing SSE headers so a build/connect error
	// surfaces as a structured non-SSE error (the handler can still writeError).
	sr, err := ag.Stream(ctx, msgs)
	if err != nil {
		return nil, err
	}
	defer sr.Close() // NON-NEGOTIABLE — else the producer goroutine leaks.

	// Commit to SSE: mirror handlers_sse.go's header set exactly.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // never let an intermediary buffer SSE.

	for {
		chunk, rerr := sr.Recv()
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			// Mid-stream provider error: the headers are already sent, so emit a
			// terminal SSE error frame rather than a (now-impossible) HTTP status.
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", escapeSSE("the assistant could not finish this answer"))
			flusher.Flush()
			return trace.retrieved(), rerr
		}
		if chunk == nil || chunk.Content == "" {
			continue
		}
		if _, werr := fmt.Fprintf(w, "data: %s\n\n", escapeSSE(chunk.Content)); werr != nil {
			// Client disconnect: the request context cancels Recv on the next
			// loop; returning here lets defer sr.Close() reap the goroutine.
			return trace.retrieved(), werr
		}
		flusher.Flush()
	}

	// Workspace answers carry their citations: emit the retrieved page paths the
	// frontend renders as "Reasoned over:" (D3). Only emitted for workspace scope
	// and only when RAG actually retrieved something (empty → no frame).
	cited := trace.retrieved()
	if sc.Kind == ScopeWorkspace && len(cited) > 0 {
		writeCitationFrame(w, cited)
	}

	// Signal a clean end-of-stream so the client closes its reader deterministically.
	fmt.Fprint(w, "event: done\ndata: {}\n\n")
	flusher.Flush()
	return cited, nil
}

// writeCitationFrame emits the retrieved page paths as a single SSE `event:
// citation` frame whose data is a JSON array of paths. It is written after the
// answer text and before `event: done` so the client can attach the "Reasoned
// over:" line to the completed answer. Marshal errors are swallowed (the answer
// already streamed; a missing citation must never break the response).
func writeCitationFrame(w http.ResponseWriter, paths []string) {
	b, err := json.Marshal(paths)
	if err != nil {
		return
	}
	// The JSON array has no raw newlines, so it is SSE-safe as a single data line.
	fmt.Fprintf(w, "event: citation\ndata: %s\n\n", b)
}

// escapeSSE collapses newlines in a delta into SSE-safe framing. An SSE `data:`
// field cannot contain a raw newline (a bare `\n` would split the event), so each
// embedded line becomes its own `data:` continuation line within the same event;
// carriage returns are dropped.
func escapeSSE(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	return strings.ReplaceAll(s, "\n", "\ndata: ")
}
