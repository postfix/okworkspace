package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/pages"
	"github.com/postfix/okworkspace/internal/search"
)

// ErrAgentDisabled is returned by agent operations when cfg.Agent.Enabled is
// false. Handlers map it to a structured "agent is turned off" UI state rather
// than a 500 — the service still constructs so the off-state can be rendered.
var ErrAgentDisabled = errors.New("agent: disabled in configuration")

// ErrNoExtractedText is returned by SummarizeAttachment when the attachment has
// no extracted text yet (extraction pending) or is a binary with no text layer.
// Handlers map it to a structured "nothing to summarize" message rather than
// letting the model hallucinate a summary of empty content.
var ErrNoExtractedText = errors.New("agent: attachment has no extracted text")

// maxReActStep bounds the ReAct loop (tool-call → exec → feed-back) so a flaky
// provider or a tool-loop can't run unbounded (T-04-06 DoS). DeepSeek tool-
// calling is less consistent than GPT-4-class, so a tight cap plus a structured
// step-exhaustion error (never an infinite retry) is required (RESEARCH §Item 5).
const maxReActStep = 12

// ScopeKind identifies which slice of the workspace an Ask is bounded to. The
// kind decides the system prompt and how the question's context is assembled
// (AGNT-02/03/04) — the SAME read-only ReAct agent serves all four.
type ScopeKind string

const (
	// ScopePage answers grounded in the page the user is viewing (slice 2).
	ScopePage ScopeKind = "page"
	// ScopeSelection answers about a span of selected text passed in the user
	// turn (AGNT-02) — no tool fetch needed for the selection itself.
	ScopeSelection ScopeKind = "selection"
	// ScopeAttachment answers from an attachment's extracted text, which the
	// agent reads via read_attachment_text (AGNT-03).
	ScopeAttachment ScopeKind = "attachment"
	// ScopeWorkspace answers over the whole (role-readable) workspace via
	// search-backed RAG — top-K via search_pages/search_attachments, never a
	// workspace dump (AGNT-04). Carries tool-trace-derived citations (D3).
	ScopeWorkspace ScopeKind = "workspace"
)

// Scope is the resolved, server-side Ask scope. It is built from the request by
// the handler: the KIND and any path/id/selection come from the body, but the
// ROLE that bounds retrieval is taken from the server session (never the client)
// and is applied when the handler constructs the role-scoped Deps.Search. The
// agent code here only varies prompting + which tools the model is steered to —
// it never widens the content the role-scoped services already permit.
type Scope struct {
	Kind ScopeKind
	// Path is the page path (page scope) or the attachment's owning page used as
	// a provenance hint; empty for selection/workspace.
	Path string
	// AttachmentID is the attachment whose extracted text answers an attachment
	// Ask (read_attachment_text); empty otherwise.
	AttachmentID string
	// Selection is the user-selected span answered about in selection scope. It
	// is UNTRUSTED and is delimited into the USER turn, never the system prompt.
	Selection string
}

// normalize fills in a safe default kind so an empty/unknown scope falls back to
// the read-only page Ask (the slice-2 behaviour) rather than misrouting.
func (sc Scope) normalize() Scope {
	switch sc.Kind {
	case ScopePage, ScopeSelection, ScopeAttachment, ScopeWorkspace:
		return sc
	default:
		sc.Kind = ScopePage
		return sc
	}
}

// scopeTrace collects the workspace-relative page paths the agent actually
// retrieved during a single Ask turn (from read_page / search_pages /
// search_attachments). It backs the workspace "Reasoned over:" citation (D3):
// citations come from this REAL tool-call trace, not from trusting the model to
// name its sources. It is per-request and goroutine-safe because the ReAct
// tools node may run tool calls concurrently. A nil *scopeTrace is a no-op, so
// non-workspace scopes (and the allow-list test) pay nothing.
type scopeTrace struct {
	mu    sync.Mutex
	paths []string
	seen  map[string]bool
}

func newScopeTrace() *scopeTrace { return &scopeTrace{seen: map[string]bool{}} }

// add records a retrieved page path once (dedup, insertion-ordered). Nil-safe
// and empty-path-safe.
func (t *scopeTrace) add(path string) {
	if t == nil || path == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.seen[path] {
		return
	}
	t.seen[path] = true
	t.paths = append(t.paths, path)
}

// retrieved returns a copy of the unique retrieved page paths in the order they
// were first seen. Always non-nil. Used to render the citation line.
func (t *scopeTrace) retrieved() []string {
	if t == nil {
		return []string{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, len(t.paths))
	copy(out, t.paths)
	return out
}

// Narrow consumer interfaces over the existing services. They are declared here
// (and left unwired in slice 1) so later slices inject fakes in tests without
// standing up git/db/bleve, mirroring the pages.Service `reviser`/`enqueuer`
// pattern. Only the ChatModel is wired this slice.

// pageWriter is the gated apply path (slice 5). It is intentionally NOT exposed
// as an Eino tool — apply is a separate approval-gated HTTP endpoint. The exact
// signature mirrors pages.Service.Save (ctx, path, body, frontmatter,
// baseRevision, user).
type pageWriter interface {
	Save(ctx context.Context, path, body string, frontmatter map[string]any, baseRevision, user string) error
}

// searcher backs the workspace-RAG tools (slice 2). Role-scoped at the call
// site; never the client. *search.Index satisfies it (Query is repo.Resolve-
// backed via the Bleve index built from on-disk files).
type searcher interface {
	Query(ctx context.Context, q string) ([]search.Result, error)
}

// pageReader backs read_page + list_tree (slice 2). Both methods read through
// the repo.Resolve-backed pages.Service (never os.ReadFile). *pages.Service
// satisfies it (Get returns pages.Page; Tree returns the nested page/folder
// tree). Declared as an interface so tools_test can inject a fake without
// standing up git/db.
type pageReader interface {
	Get(ctx context.Context, path string) (pages.Page, error)
	Tree(ctx context.Context) ([]pages.Node, error)
	// Revision returns the current committed blob revision of path — the optimistic-
	// concurrency token ProposePatch captures at proposal time for the stale-during-
	// review check (slice 5). *pages.Service satisfies it.
	Revision(ctx context.Context, path string) (string, error)
}

// attachmentReader backs read_attachment_text (slice 2). ExtractedText returns
// the repo.Resolve-backed `.txt` extraction sidecar for an attachment id (empty
// when extraction is pending/absent — never an error, never raw bytes off a
// client-supplied path). Routed through repo.Read (SEC-01 resolver), NOT
// os.ReadFile.
type attachmentReader interface {
	ExtractedText(ctx context.Context, id string) (string, error)
}

// auditRecorder records prompt/proposal/approval events (non-fatal). The API
// key and secret-shaped prompt content are NEVER passed to it.
type auditRecorder interface {
	Record(ctx context.Context, ev audit.Event) error
}

// Deps holds the injected service collaborators the agent will need in later
// slices. All fields are optional in slice 1 (the smoke test passes nil); each
// later slice wires the ones it needs (and the slice-2 read tools will adapt
// pages.Service.Get/Revision behind a small reader interface defined there) and
// asserts non-nil at its call site.
type Deps struct {
	PageWriter  pageWriter
	Pages       pageReader
	Search      searcher
	Attachments attachmentReader
	Audit       auditRecorder
}

// Service is the agent orchestration service. In slice 1 it holds only the
// built ChatModel; later slices add the ReAct agent builder and wire the read
// tools through Deps. The resolved API key lives ONLY inside cm (constructed
// from cfg.APIKey()) and is never stored, logged, or returned by this struct.
type Service struct {
	// cm is the eino ChatModel interface (model.ToolCallingChatModel), NOT the
	// concrete *openai.ChatModel — so a fake can be injected in unit tests
	// (TestDispatch) without standing up a provider. openai.NewChatModel already
	// returns a value satisfying this interface, so production wiring is unchanged.
	// nil when the agent is disabled.
	cm   model.ToolCallingChatModel
	cfg  config.AgentConfig
	deps Deps
	now  func() time.Time
}

// NewService constructs the agent service from the agent config and its
// (optional) service deps. When cfg.Enabled is false it constructs a DISABLED
// service (cm == nil) rather than panicking, so handlers can render the
// "agent off" state and operations return ErrAgentDisabled.
//
// Building the ChatModel reads the secret exactly once, via cfg.APIKey(), inside
// newChatModel; the key is never logged or placed in any returned value. A
// build error from the eino-ext constructor is returned wrapped WITHOUT the
// key (the eino-ext error does not echo the APIKey field).
func NewService(cfg config.AgentConfig, deps *Deps) *Service {
	s := &Service{cfg: cfg, now: time.Now}
	if deps != nil {
		s.deps = *deps
	}
	if !cfg.Enabled {
		return s // disabled: cm stays nil, ErrAgentDisabled at call sites.
	}
	// Best-effort build at construction time. If the provider config is
	// malformed the service stays in a degraded (cm == nil) state and callers
	// surface ErrAgentDisabled; we deliberately do not panic at wiring time.
	cm, err := newChatModel(context.Background(), cfg)
	if err != nil {
		// Do not log or wrap the key; the eino-ext error carries no secret.
		return s
	}
	s.cm = cm
	return s
}

// Enabled reports whether the agent is configured on AND its ChatModel built.
func (s *Service) Enabled() bool { return s.cm != nil }

// buildReActAgent constructs a fresh ReAct agent wired to the read-only tool set
// and the built ChatModel. It is built per request (cheap; the heavy ChatModel
// is reused) so a future slice can scope the tools to the caller's session role
// without mutating shared state.
//
// SECURITY (T-04-03 / Pitfall 2): only AgentConfig.ToolCallingModel is set —
// the deprecated AgentConfig.Model field (whose in-place BindTools races when a
// ChatModel is shared across goroutines) is left nil. The tool list is exactly
// readTools' read-only allow-list; no write/apply tool is reachable from here.
// MaxStep is capped at maxReActStep and UnknownToolsHandler absorbs DeepSeek's
// hallucinated tool names instead of erroring the whole turn (RESEARCH §Item 5).
func (s *Service) buildReActAgent(ctx context.Context, trace *scopeTrace) (*react.Agent, error) {
	if s.cm == nil {
		return nil, ErrAgentDisabled
	}
	tools, _, err := readTools(s.deps, trace)
	if err != nil {
		return nil, err
	}
	return react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: s.cm, // model.ToolCallingChatModel (openai.NewChatModel satisfies it).
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: tools,
			// Gracefully absorb a hallucinated tool name rather than failing the
			// whole turn — the model gets a benign "unknown tool" result and can
			// recover within the MaxStep budget.
			UnknownToolsHandler: func(_ context.Context, name, _ string) (string, error) {
				return "unknown tool: " + name, nil
			},
		},
		MaxStep: maxReActStep,
	})
}

// ─── Single-shot modes (Summarize / Rewrite / Draft) ─────────────────────────
//
// These four modes (AGNT-05/06/07/08) use a DIRECT ChatModel.Generate call — NOT
// the ReAct loop (AI-SPEC §4 "Core Pattern"). The context is supplied up front
// (the one page/attachment body, the selection, or just the user instruction), so
// no tool round-trips are needed: cheaper, fewer failure modes. Every call is
// wrapped in context.WithTimeout(singleShotTimeout) with an explicit per-mode
// MaxTokens + Temperature (never unbounded — T-04-14). Rewrite and Draft outputs
// are candidate bodies and MUST pass validateProposedBody (+ retry) before they
// are returned — a fenced/empty/frontmatter-mangled body never reaches the user
// (T-04-12). Neither Rewrite nor Draft ever writes: Draft opens in the editor
// pending an explicit save; Rewrite returns a proposal the slice-5/6 diff dialog
// renders.

// singleShotTimeout is the hard per-request ceiling for a single-shot Generate
// call so a hung provider can't hang the request goroutine (AI-SPEC §4 #6 / §4
// Per-call guards). ~60s matches the ReAct path's intent.
const singleShotTimeout = 60 * time.Second

// Per-mode output token caps and sampling. Grounded modes (Summarize/Rewrite)
// run cool; Draft is allowed slightly more latitude. MaxTokens is ALWAYS set —
// never nil/unbounded in production (Pitfall 6).
const (
	summarizeMaxTokens   = 1024
	rewriteMaxTokens     = 2048 // a rewritten span can be longer than the answer to a question.
	draftMaxTokens       = 4096 // a full new page body.
	summarizeTemperature = 0.2
	rewriteTemperature   = 0.2
	draftTemperature     = 0.4
)

// maxSingleShotInput caps the page/attachment/selection text spliced into a
// single-shot prompt so an oversized body can't blow the context window
// (T-04-14). Bodies over the cap are truncated to a head+tail window
// (truncateForBudget) rather than overflowing. ~24k chars ≈ a generous page.
const maxSingleShotInput = 24000

// generateOnce runs one single-shot ChatModel.Generate with the per-call timeout
// and the given sampling, returning the model's text content. It is the single
// choke point that guarantees every single-shot call is bounded (timeout +
// MaxTokens). The ctx handed to Generate carries the ~60s deadline asserted by
// TestDispatch.
func (s *Service) generateOnce(ctx context.Context, msgs []*schema.Message, maxTokens int, temperature float32) (string, error) {
	if s.cm == nil {
		return "", ErrAgentDisabled
	}
	ctx, cancel := context.WithTimeout(ctx, singleShotTimeout)
	defer cancel()
	out, err := s.cm.Generate(ctx, msgs,
		model.WithMaxTokens(maxTokens),
		model.WithTemperature(temperature),
	)
	if err != nil {
		return "", err
	}
	if out == nil {
		return "", errors.New("agent: empty model response")
	}
	return out.Content, nil
}

// SummarizePage returns a grounded summary of the page at path. The body is
// fetched via the role-scoped pages reader (never os.ReadFile) and truncated to
// the input budget if oversized. Single-shot, read-only — it produces an answer,
// not a candidate body, so it does NOT go through validateProposedBody.
func (s *Service) SummarizePage(ctx context.Context, path string) (string, error) {
	if s.cm == nil {
		return "", ErrAgentDisabled
	}
	if s.deps.Pages == nil {
		return "", errors.New("agent: page reader not configured")
	}
	pg, err := s.deps.Pages.Get(ctx, path)
	if err != nil {
		return "", err
	}
	content := truncateForBudget(pg.Body, maxSingleShotInput)
	msgs := buildSummarizeMessages(summarizeKindPage, path, content)
	return s.generateOnce(ctx, msgs, summarizeMaxTokens, summarizeTemperature)
}

// SummarizeAttachment returns a grounded summary of an attachment's extracted
// text. Extraction may be pending/empty (never an error) — an empty extraction
// yields a structured "nothing to summarize" error rather than a hallucinated
// summary. Single-shot, read-only.
func (s *Service) SummarizeAttachment(ctx context.Context, id string) (string, error) {
	if s.cm == nil {
		return "", ErrAgentDisabled
	}
	if s.deps.Attachments == nil {
		return "", errors.New("agent: attachment reader not configured")
	}
	text, err := s.deps.Attachments.ExtractedText(ctx, id)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", ErrNoExtractedText
	}
	content := truncateForBudget(text, maxSingleShotInput)
	msgs := buildSummarizeMessages(summarizeKindAttachment, id, content)
	return s.generateOnce(ctx, msgs, summarizeMaxTokens, summarizeTemperature)
}

// Rewrite rewrites the selected span per the user's instruction and returns the
// rewritten text as a PROPOSAL (the server diffs old-selection↔new in slice 5/6
// — this never auto-applies). The output passes validateProposedBody + retry
// before return: a fenced/empty body is rejected and retried, never surfaced. The
// selection has no frontmatter, so validation enforces the empty/fenced rules.
func (s *Service) Rewrite(ctx context.Context, selection, instruction string) (string, error) {
	if s.cm == nil {
		return "", ErrAgentDisabled
	}
	gen := func(ctx context.Context, attempt int) (string, error) {
		msgs := buildRewriteMessages(selection, instruction, attempt)
		return s.generateOnce(ctx, msgs, rewriteMaxTokens, rewriteTemperature)
	}
	// The rewritten span is compared against the selection (no frontmatter to
	// preserve) — validateProposedBody enforces the empty/fenced rules.
	return proposeWithRetry(ctx, selection, gen)
}

// Draft drafts a full new-page Markdown body from the user's instruction and
// returns it to OPEN IN THE EDITOR pending an explicit save (never auto-written).
// The output passes validateProposedBody + retry before return. A draft has no
// source document to preserve frontmatter against, so the empty/fenced rules
// apply.
func (s *Service) Draft(ctx context.Context, instruction string) (string, error) {
	if s.cm == nil {
		return "", ErrAgentDisabled
	}
	gen := func(ctx context.Context, attempt int) (string, error) {
		msgs := buildDraftMessages(instruction, attempt)
		return s.generateOnce(ctx, msgs, draftMaxTokens, draftTemperature)
	}
	return proposeWithRetry(ctx, "", gen)
}

// truncateForBudget caps text to a head+tail window of about max chars so an
// oversized body fits the context budget without dropping the document's start
// AND end (the parts a summary most needs). A body within budget is returned
// unchanged.
func truncateForBudget(text string, max int) string {
	if len(text) <= max {
		return text
	}
	half := max / 2
	const marker = "\n\n…[content truncated to fit]…\n\n"
	return text[:half] + marker + text[len(text)-half:]
}
