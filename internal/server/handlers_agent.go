package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/postfix/okworkspace/internal/agent"
	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/okf"
	"github.com/postfix/okworkspace/internal/pages"
)

// agentChatRequest is the POST /agent/chat body. The actor and the ROLE that
// bounds retrieval are NEVER read from the body — they come from the session.
// Only the SCOPE (which slice of the workspace) and the prompt are client-
// supplied; the server enforces what that scope is allowed to read.
//
//   - Scope selects page | selection | attachment | workspace (default page).
//   - PagePath is the page the user is viewing (page scope) or the selection's
//     owning page (provenance hint).
//   - Selection is the user-highlighted span (selection scope) — UNTRUSTED,
//     delimited into the USER turn by the agent, never the system prompt.
//   - AttachmentID is the attachment to answer from (attachment scope).
type agentChatRequest struct {
	Prompt       string `json:"prompt"`
	Scope        string `json:"scope"`
	PagePath     string `json:"page_path"`
	Selection    string `json:"selection"`
	AttachmentID string `json:"attachment_id"`
}

// maxPromptLen caps the untrusted prompt length (input-validation / DoS guard,
// mirrors search's query cap). Far above any real question.
const maxPromptLen = 4000

// maxSelectionLen caps the untrusted selection span. A selection is a paragraph
// or two, not a whole document; the cap bounds the prompt the model sees.
const maxSelectionLen = 16000

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
	if strings.ContainsAny(req.PagePath, "\x00") ||
		strings.ContainsAny(req.AttachmentID, "\x00") ||
		strings.Contains(req.Selection, "\x00") {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	if len(req.Selection) > maxSelectionLen {
		writeError(w, http.StatusBadRequest, "That selection is too long. Select less text and try again.")
		return
	}

	// Resolve the scope KIND from the body but bound it server-side. The ROLE
	// that scopes retrieval is taken from the SESSION (never the client) so a
	// workspace Ask can only retrieve pages the session role may read; the role
	// is derived here and recorded in the audit, not trusted from req.
	kind := scopeKindFromRequest(req.Scope)
	role := h.actorRole(r.Context())
	sc := agent.Scope{
		Kind:         kind,
		Path:         req.PagePath,
		AttachmentID: req.AttachmentID,
		Selection:    req.Selection,
	}

	actor := h.actorUsername(r.Context())

	// Audit the prompt event BEFORE streaming (non-fatal). Target is the scope
	// page path (provenance only); the prompt text never enters Detail. Detail
	// records the non-secret scope + the session-derived role for traceability.
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionAgentPrompt,
		Actor:  actor,
		Target: req.PagePath,
		Detail: "scope=" + string(kind) + " role=" + role,
		Source: auditSourceWeb,
	})

	// Stream the answer. AskStream writes SSE headers itself once it commits to
	// streaming; the errors below are all returned BEFORE the first byte, so we
	// can still emit a clean structured HTTP error. For workspace scope it also
	// emits the retrieved page paths as an SSE citation frame (the "Reasoned
	// over:" line); the returned slice is the same set (unused here beyond the
	// stream — kept for a future audit of cited paths).
	_, err := h.agent.AskStream(r.Context(), w, prompt, sc)
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

// scopeKindFromRequest maps the client-supplied scope string to a known
// agent.ScopeKind. An empty or unrecognized value falls back to the page Ask —
// the safe read-only default — so a malformed scope can never widen access.
func scopeKindFromRequest(s string) agent.ScopeKind {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "selection":
		return agent.ScopeSelection
	case "attachment":
		return agent.ScopeAttachment
	case "workspace":
		return agent.ScopeWorkspace
	default:
		return agent.ScopePage
	}
}

// ─── Single-shot modes: Summarize / Rewrite / Draft (AGNT-05/06/07/08) ───────
//
// Unlike Ask, these are AWAITED (full JSON response, not streamed): a Summarize
// answer is whole, and a Rewrite/Draft body must be validated as a whole before
// it can be diffed/opened (a half-streamed body cannot — AI-SPEC §4b). All four
// are read/proposal operations: Summarize is read-only; Rewrite returns a
// proposal for the diff dialog and Draft a body that opens in the editor —
// NEITHER auto-writes (the apply path is a separate gated endpoint, slice 5).
//
// Each fails closed when the agent is off/unreachable and audits via
// ActionAgentPrompt with the non-secret mode in Detail (never the prompt text).

// summarizePageRequest is the POST /agent/summarize-page body. page_path is the
// page to summarize (read via the role-scoped pages service).
type summarizePageRequest struct {
	PagePath string `json:"page_path"`
}

// summarizeAttachmentRequest is the POST /agent/summarize-attachment body.
type summarizeAttachmentRequest struct {
	AttachmentID string `json:"attachment_id"`
}

// rewriteRequest is the POST /agent/rewrite body. selection is the UNTRUSTED span
// to rewrite; instruction is how to change it. The server diffs the returned text
// against selection in slice 5/6 — this endpoint returns a proposal only.
type rewriteRequest struct {
	Selection   string `json:"selection"`
	Instruction string `json:"instruction"`
}

// draftRequest is the POST /agent/draft body. instruction describes the page to
// draft; the returned body opens in the editor pending an explicit save.
type draftRequest struct {
	Instruction string `json:"instruction"`
}

// handleSummarizePage summarizes the requested page (AGNT-05). Read-only,
// any-authed; the page is fetched via the role-scoped pages service, never a
// client path read. Returns the summary as JSON (awaited, not streamed).
func (h *authHandlers) handleSummarizePage(w http.ResponseWriter, r *http.Request) {
	if h.agent == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	var req summarizePageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	path := strings.TrimSpace(req.PagePath)
	if path == "" || strings.ContainsRune(path, '\x00') {
		writeError(w, http.StatusBadRequest, "Choose a page to summarize.")
		return
	}

	h.auditAgentMode(r, "summarize-page", path)

	summary, err := h.agent.SummarizePage(r.Context(), path)
	if err != nil {
		writeAgentModeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"summary": summary})
}

// handleSummarizeAttachment summarizes an attachment's extracted text (AGNT-06).
// An attachment with no extracted text yet (extraction pending / no text layer)
// returns a structured 422 rather than a hallucinated summary.
func (h *authHandlers) handleSummarizeAttachment(w http.ResponseWriter, r *http.Request) {
	if h.agent == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	var req summarizeAttachmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	id := strings.TrimSpace(req.AttachmentID)
	if id == "" || strings.ContainsRune(id, '\x00') {
		writeError(w, http.StatusBadRequest, "Choose an attachment to summarize.")
		return
	}

	h.auditAgentMode(r, "summarize-attachment", id)

	summary, err := h.agent.SummarizeAttachment(r.Context(), id)
	if errors.Is(err, agent.ErrNoExtractedText) {
		writeError(w, http.StatusUnprocessableEntity, "This attachment has no readable text to summarize yet.")
		return
	}
	if err != nil {
		writeAgentModeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"summary": summary})
}

// handleRewrite rewrites a selected span (AGNT-07) and returns the rewritten text
// as a PROPOSAL — it never auto-applies. The frontend routes the result to the
// diff dialog (old selection ↔ new). The selection is UNTRUSTED and length-/NUL-
// capped like the Ask selection.
func (h *authHandlers) handleRewrite(w http.ResponseWriter, r *http.Request) {
	if h.agent == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	var req rewriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	selection := req.Selection
	instruction := strings.TrimSpace(req.Instruction)
	if strings.TrimSpace(selection) == "" {
		writeError(w, http.StatusBadRequest, "Select some text to rewrite.")
		return
	}
	if instruction == "" {
		writeError(w, http.StatusBadRequest, "Describe how you'd like the selection rewritten.")
		return
	}
	if strings.ContainsRune(selection, '\x00') || strings.ContainsRune(instruction, '\x00') {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	if len(selection) > maxSelectionLen {
		writeError(w, http.StatusBadRequest, "That selection is too long. Select less text and try again.")
		return
	}
	if len(instruction) > maxPromptLen {
		writeError(w, http.StatusBadRequest, "That instruction is too long. Please shorten it and try again.")
		return
	}

	h.auditAgentMode(r, "rewrite", "")

	rewritten, err := h.agent.Rewrite(r.Context(), selection, instruction)
	if err != nil {
		writeAgentModeError(w, err)
		return
	}
	// A proposal — the frontend diffs old↔new and routes to the review dialog.
	writeJSON(w, http.StatusOK, map[string]string{"rewritten": rewritten})
}

// handleDraft drafts a full new-page body (AGNT-08) and returns it for the editor
// to open pending an explicit save — it never auto-writes a page.
func (h *authHandlers) handleDraft(w http.ResponseWriter, r *http.Request) {
	if h.agent == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	var req draftRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	instruction := strings.TrimSpace(req.Instruction)
	if instruction == "" {
		writeError(w, http.StatusBadRequest, "Describe the page you'd like to draft.")
		return
	}
	if strings.ContainsRune(instruction, '\x00') {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	if len(instruction) > maxPromptLen {
		writeError(w, http.StatusBadRequest, "That instruction is too long. Please shorten it and try again.")
		return
	}

	h.auditAgentMode(r, "draft", "")

	body, err := h.agent.Draft(r.Context(), instruction)
	if err != nil {
		writeAgentModeError(w, err)
		return
	}
	// The draft opens in the editor pending an explicit save — never auto-written.
	writeJSON(w, http.StatusOK, map[string]string{"body": body})
}

// auditAgentMode records a single-shot mode invocation (non-fatal). The Detail
// carries only the non-secret mode; the prompt/selection/draft text NEVER enters
// the audit. Target is the page/attachment id where one exists (provenance only).
func (h *authHandlers) auditAgentMode(r *http.Request, mode, target string) {
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionAgentPrompt,
		Actor:  h.actorUsername(r.Context()),
		Target: target,
		Detail: "mode=" + mode + " role=" + h.actorRole(r.Context()),
		Source: auditSourceWeb,
	})
}

// writeAgentModeError maps a single-shot mode error to a structured JSON status,
// failing CLOSED on a disabled/unreachable agent and on a validation-exhaustion
// (the model could not produce a usable body) — never a malformed body or a hang.
func writeAgentModeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, agent.ErrAgentDisabled):
		writeError(w, http.StatusServiceUnavailable, "The assistant is turned off. Ask an administrator to enable it.")
	case errors.Is(err, agent.ErrProposalInvalid):
		writeError(w, http.StatusUnprocessableEntity, "The assistant couldn't produce a clean result. Try rephrasing your request.")
	default:
		writeError(w, http.StatusBadGateway, "The assistant is unavailable right now. Try again in a moment.")
	}
}

// ─── Propose / Apply patch: the SAFETY CORE gate (AGNT-09/10/11) ─────────────
//
// /propose-patch (editor) calls the agent's ProposePatch (the ONLY whole-body
// proposal path), returning the OLD body + the proposed NEW body + the base
// revision captured at proposal time, so the FRONTEND renders a real diff — never
// a prose "I updated it for you" summary. /apply-patch (editor + CSRF) is a
// SEPARATE gated HTTP endpoint — NOT an Eino tool (the read-only 5-tool allow-list
// is unchanged, AGNT-11) — that reuses pages.Save(baseRevision) → the single-writer
// gitstore.Commit(Action="approved_agent_patch", Source="agent"). A revision that
// moved since the proposal blocks the apply with a 409 (ErrStaleRevision) and writes
// NOTHING — never a silent overwrite of a concurrent edit (T-04-17). Both events are
// audited (ActionAgentPatchProposal / ActionAgentPatchApproval, non-fatal).

// proposePatchRequest is the POST /agent/propose-patch body. page_path is the page
// to patch; instruction describes the change. The actor/role are session-derived,
// never read from the body.
type proposePatchRequest struct {
	PagePath    string `json:"page_path"`
	Instruction string `json:"instruction"`
}

// proposePatchResponse carries the two bodies the frontend diffs (old↔new) plus the
// base revision the eventual apply must echo back for the stale-revision floor. The
// diff is computed by the client from old_body + new_body — the server never returns
// a prose summary in place of the change.
type proposePatchResponse struct {
	PagePath     string `json:"page_path"`
	OldBody      string `json:"old_body"`
	NewBody      string `json:"new_body"`
	BaseRevision string `json:"base_revision"`
}

// applyPatchRequest is the POST /agent/apply-patch body. It is the one consequential
// write: base_revision is the token captured at proposal time, re-checked by
// pages.Save so a moved revision 409s instead of overwriting.
type applyPatchRequest struct {
	PagePath     string `json:"page_path"`
	NewBody      string `json:"new_body"`
	Frontmatter  string `json:"frontmatter"`
	BaseRevision string `json:"base_revision"`
}

// handleProposePatch produces a whole-body patch proposal (AGNT-09). Editor-gated.
// It returns the old body (for the diff's left side), the proposed new body, and the
// base revision captured at proposal time — never auto-applying. The proposal is
// validated + retried inside ProposePatch; an exhausted validation fails closed.
func (h *authHandlers) handleProposePatch(w http.ResponseWriter, r *http.Request) {
	if h.agent == nil || h.pages == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	var req proposePatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	path := strings.TrimSpace(req.PagePath)
	instruction := strings.TrimSpace(req.Instruction)
	if path == "" || strings.ContainsRune(path, '\x00') {
		writeError(w, http.StatusBadRequest, "Choose a page to patch.")
		return
	}
	if instruction == "" {
		writeError(w, http.StatusBadRequest, "Describe the change you'd like the assistant to propose.")
		return
	}
	if strings.ContainsRune(instruction, '\x00') {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	if len(instruction) > maxPromptLen {
		writeError(w, http.StatusBadRequest, "That instruction is too long. Please shorten it and try again.")
		return
	}

	// The proposal fetches the current body + captures the base revision; we also
	// fetch the page here for the OLD side of the diff (and to 404 cleanly).
	pg, err := h.pages.Get(r.Context(), path)
	if err != nil {
		if errors.Is(err, pages.ErrPageNotFound) {
			writeError(w, http.StatusNotFound, "This page no longer exists. It may have been moved or deleted.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}

	newBody, baseRev, err := h.agent.ProposePatch(r.Context(), path, instruction)
	if err != nil {
		if errors.Is(err, pages.ErrPageNotFound) {
			writeError(w, http.StatusNotFound, "This page no longer exists. It may have been moved or deleted.")
			return
		}
		writeAgentModeError(w, err)
		return
	}

	// Audit the proposal (non-fatal). Detail carries only the non-secret churn
	// metric + role — never the instruction text or the proposed body. The proposal
	// is body-only, so churn is the body↔body changed-line fraction (frontmatter is
	// never part of the diff under the body-only contract — CR-01).
	churn := agent.ChurnRatio(pg.Body, newBody)
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionAgentPatchProposal,
		Actor:  h.actorUsername(r.Context()),
		Target: path,
		Detail: fmt.Sprintf("churn=%.3f role=%s", churn, h.actorRole(r.Context())),
		Source: auditSourceWeb,
	})

	writeJSON(w, http.StatusOK, proposePatchResponse{
		PagePath:     path,
		OldBody:      pg.Body,
		NewBody:      newBody,
		BaseRevision: baseRev,
	})
}

// handleApplyPatch applies an approved patch (AGNT-10). Editor-gated + CSRF (the
// global nosurf middleware covers the mutating method). It reuses pages.Save with
// the proposal's base_revision: a moved revision returns ErrStaleRevision → 409 and
// writes nothing (never a silent overwrite). pages.Save reaching
// gitstore.Commit(Action="approved_agent_patch", Source="agent") is the ONLY write
// path — there is no bespoke commit here and apply is NOT an Eino tool (AGNT-11).
func (h *authHandlers) handleApplyPatch(w http.ResponseWriter, r *http.Request) {
	if h.pages == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	var req applyPatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	path := strings.TrimSpace(req.PagePath)
	if path == "" || strings.ContainsRune(path, '\x00') {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	if strings.TrimSpace(req.NewBody) == "" {
		writeError(w, http.StatusBadRequest, "There is no proposed change to apply.")
		return
	}
	if strings.ContainsRune(req.NewBody, '\x00') || strings.ContainsRune(req.Frontmatter, '\x00') {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}

	// Re-validate the body BEFORE it reaches the write path (defense in depth — the
	// same gate ProposePatch ran, in case the client tampered the body before
	// approving). The proposal is BODY-ONLY (the frontmatter is server-owned and
	// re-attached by pages.Save), so validate against an empty source: there is no
	// frontmatter key-set to preserve here, only the empty/fenced rules. A fenced/
	// empty body never reaches pages.Save.
	if err := agent.ValidateProposedBody("", req.NewBody); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "The proposed change is not in a clean state to apply. Re-run the assistant.")
		return
	}

	// pages.Save assembles ---frontmatter--- + body exactly once. req.NewBody is the
	// body only and req.Frontmatter is the page's original frontmatter region (the
	// single source of frontmatter) — so the frontmatter is written exactly once, no
	// stray second fence (the CR-01 double-write). A body that itself carries a
	// leading "---" fence would be double-fenced, so reject one defensively.
	if hasLeadingFrontmatterFence(req.NewBody) {
		writeError(w, http.StatusUnprocessableEntity, "The proposed change is not in a clean state to apply. Re-run the assistant.")
		return
	}

	actor := h.actorUsername(r.Context())
	err := h.pages.Save(r.Context(), path, req.NewBody, req.Frontmatter, req.BaseRevision, actor)
	if err != nil {
		if errors.Is(err, pages.ErrPageNotFound) {
			writeError(w, http.StatusNotFound, "This page no longer exists. It may have been moved or deleted.")
			return
		}
		if errors.Is(err, pages.ErrStaleRevision) {
			// The page changed since the assistant read it — block, never overwrite.
			writeError(w, http.StatusConflict, "This page changed since the assistant read it. Re-run the assistant to propose against the latest version.")
			return
		}
		writeError(w, http.StatusInternalServerError, "We couldn't apply that change just now. Check your connection and try again.")
		return
	}

	// Audit the approval (non-fatal). Every Source=agent commit must reconcile to one
	// of these approval rows (AI-SPEC §7 metric #1).
	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionAgentPatchApproval,
		Actor:  actor,
		Target: path,
		Detail: "role=" + h.actorRole(r.Context()),
		Source: auditSourceWeb,
	})
	w.WriteHeader(http.StatusNoContent)
}

// hasLeadingFrontmatterFence reports whether body begins with its own YAML
// frontmatter fence at byte 0. The propose→apply contract is body-only (CR-01): the
// frontmatter is server-owned and re-attached by pages.Save exactly once. A body
// that itself opens with a "---" fence block would be double-fenced on write, so
// apply rejects it. okf.Parse is the single source of "is this a frontmatter fence"
// (the same recognizer pages.Save uses), so this can never drift from the writer.
func hasLeadingFrontmatterFence(body string) bool {
	doc, err := okf.Parse([]byte(body))
	if err != nil {
		// A parse error here means okf could not split it — treat as not-a-clean-body
		// (the caller rejects). Returning true funnels it to the same 422.
		return true
	}
	return doc.HasFrontmatter
}

// actorRole resolves the session-bound user's role for the audit Detail. It
// reads ONLY from the request context (the session-loaded user), never from the
// request body — the role that bounds retrieval is server-derived (T-04-08).
// Falls back to "unknown" when the user cannot be resolved.
func (h *authHandlers) actorRole(ctx context.Context) string {
	u, ok := auth.CurrentUser(ctx)
	if !ok {
		return "unknown"
	}
	return u.UserRole()
}
