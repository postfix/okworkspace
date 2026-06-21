package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/postfix/okworkspace/internal/agent"
	"github.com/postfix/okworkspace/internal/audit"
)

// agentChatRequest is the POST /agent/chat body: a free-text prompt and the
// optional workspace-relative path of the page the user is viewing (the scope
// the agent grounds its answer in via read_page). The actor is NEVER read from
// the body — it comes from the session (h.actorUsername).
type agentChatRequest struct {
	Prompt   string `json:"prompt"`
	PagePath string `json:"page_path"`
}

// maxPromptLen caps the untrusted prompt length (input-validation / DoS guard,
// mirrors search's query cap). Far above any real question.
const maxPromptLen = 4000

// handleAgentChat answers an Ask question grounded in the current page and
// streams the answer token-by-token as SSE (AGNT-01). It is any-authed (mounted
// in the authed group) and read-only — the agent reaches the workspace only
// through the five read-only tools, and no write/apply tool is reachable here.
//
// Fail-closed (AI-SPEC §6): when the agent is disabled or its provider is
// unreachable, it returns a structured JSON error BEFORE any stream byte — never
// a silent hang. A mid-stream provider failure is surfaced as a terminal SSE
// error frame by agent.AskStream.
//
// The prompt is audited via ActionAgentPrompt (non-fatal); the prompt text and
// any secret-shaped content are NEVER placed in the audit Detail.
func (h *authHandlers) handleAgentChat(w http.ResponseWriter, r *http.Request) {
	if h.agent == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}

	var req agentChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		writeError(w, http.StatusBadRequest, "Enter a question for the assistant.")
		return
	}
	if len(prompt) > maxPromptLen {
		writeError(w, http.StatusBadRequest, "That question is too long. Please shorten it and try again.")
		return
	}
	// page_path is a scope hint the agent reads via read_page; reject control
	// characters defensively (the read tools resolve it server-side, but keep
	// obvious garbage out of the prompt and the audit target).
	scope := req.PagePath
	if strings.ContainsAny(scope, "\x00") {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}

	actor := h.actorUsername(r.Context())

	// Audit the prompt event BEFORE streaming (non-fatal). Target is the scope
	// page path (provenance only); the prompt text never enters Detail.
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionAgentPrompt,
		Actor:  actor,
		Target: scope,
		Source: auditSourceWeb,
	})

	// Stream the answer. AskStream writes SSE headers itself once it commits to
	// streaming; the errors below are all returned BEFORE the first byte, so we
	// can still emit a clean structured HTTP error.
	err := h.agent.AskStream(r.Context(), w, prompt, scope)
	if err == nil {
		return
	}
	switch {
	case errors.Is(err, agent.ErrAgentDisabled):
		writeError(w, http.StatusServiceUnavailable, "The assistant is turned off. Ask an administrator to enable it.")
	case errors.Is(err, agent.ErrStreamingUnsupported):
		writeError(w, http.StatusInternalServerError, "Streaming is not supported.")
	default:
		// A build/connect error before streaming (provider unreachable) — fail
		// closed with a structured error, never a hang. A mid-stream error is
		// already surfaced as an SSE error frame inside AskStream, in which case
		// headers are sent and writeError is a no-op on the status (still safe).
		writeError(w, http.StatusBadGateway, "The assistant is unavailable right now. Try again in a moment.")
	}
}
