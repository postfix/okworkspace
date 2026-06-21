package agent

import (
	"context"
	"errors"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/search"
)

// ErrAgentDisabled is returned by agent operations when cfg.Agent.Enabled is
// false. Handlers map it to a structured "agent is turned off" UI state rather
// than a 500 — the service still constructs so the off-state can be rendered.
var ErrAgentDisabled = errors.New("agent: disabled in configuration")

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

// searcher backs the workspace-RAG tools (slice 3). Role-scoped at the call
// site; never the client.
type searcher interface {
	Query(ctx context.Context, q string) ([]search.Result, error)
}

// attachmentReader backs read_attachment_text (slice 3).
type attachmentReader interface {
	GetPlainText(ctx context.Context, id string) (string, error)
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
