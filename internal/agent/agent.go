package agent

import (
	"context"
	"errors"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/pages"
	"github.com/postfix/okworkspace/internal/search"
)

// ErrAgentDisabled is returned by agent operations when cfg.Agent.Enabled is
// false. Handlers map it to a structured "agent is turned off" UI state rather
// than a 500 — the service still constructs so the off-state can be rendered.
var ErrAgentDisabled = errors.New("agent: disabled in configuration")

// maxReActStep bounds the ReAct loop (tool-call → exec → feed-back) so a flaky
// provider or a tool-loop can't run unbounded (T-04-06 DoS). DeepSeek tool-
// calling is less consistent than GPT-4-class, so a tight cap plus a structured
// step-exhaustion error (never an infinite retry) is required (RESEARCH §Item 5).
const maxReActStep = 12

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
	cm   *openai.ChatModel // nil when the agent is disabled.
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
func (s *Service) buildReActAgent(ctx context.Context) (*react.Agent, error) {
	if s.cm == nil {
		return nil, ErrAgentDisabled
	}
	tools, _, err := readTools(s.deps)
	if err != nil {
		return nil, err
	}
	return react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: s.cm, // *openai.ChatModel satisfies model.ToolCallingChatModel.
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
