package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// fakeChatModel is a key-free, no-network stand-in for the eino
// model.ToolCallingChatModel. It records the messages and the ctx of the LAST
// Generate call so a test can assert the per-mode system prompt that was sent and
// the ~60s deadline on the call's context. reply is the canned content it
// returns. It is the single seam that lets TestDispatch drive the single-shot
// dispatch with NO provider (DEEPSEEK_API_KEY unset, no HTTP).
type fakeChatModel struct {
	reply   string
	err     error
	gotMsgs []*schema.Message
	gotCtx  context.Context
	calls   int
}

func (f *fakeChatModel) Generate(ctx context.Context, in []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	f.calls++
	f.gotMsgs = in
	f.gotCtx = ctx
	if f.err != nil {
		return nil, f.err
	}
	return schema.AssistantMessage(f.reply, nil), nil
}

// Stream is unused by the single-shot modes but is required to satisfy the
// interface; it is never called in TestDispatch.
func (f *fakeChatModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("fakeChatModel: Stream not implemented")
}

// WithTools returns the receiver unchanged — the single-shot modes never bind
// tools, and TestDispatch does not exercise the ReAct path.
func (f *fakeChatModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return f, nil
}

var _ model.ToolCallingChatModel = (*fakeChatModel)(nil)

// newFakeService builds an agent.Service with a fake ChatModel injected (so
// Enabled() is true and the single-shot path runs key-free) plus narrow fake
// page/attachment readers Summarize needs.
func newFakeService(cm *fakeChatModel) *Service {
	return &Service{
		cm: cm,
		deps: Deps{
			Pages:       fakePageReader{body: "# Page\n\nSome page content to summarize.", path: "notes/x.md"},
			Attachments: fakeAttachmentReader{text: "extracted attachment text"},
		},
		now: time.Now,
	}
}

// fakeAttachmentReader serves canned extracted text key-free.
type fakeAttachmentReader struct{ text string }

func (f fakeAttachmentReader) ExtractedText(context.Context, string) (string, error) {
	return f.text, nil
}

// assertDeadlineAbout60s asserts the ctx the fake's Generate received carries a
// deadline within a sane window of the ~60s single-shot timeout — proving the
// dispatch wrapped the call in context.WithTimeout (never unbounded).
func assertDeadlineAbout60s(t *testing.T, ctx context.Context) {
	t.Helper()
	if ctx == nil {
		t.Fatal("Generate received a nil context (no timeout was set)")
	}
	dl, ok := ctx.Deadline()
	if !ok {
		t.Fatal("Generate context carries no deadline — the single-shot timeout was not set")
	}
	remaining := time.Until(dl)
	// Allow a generous window below 60s for test-execution slack, and never above.
	if remaining <= 50*time.Second || remaining > singleShotTimeout+time.Second {
		t.Fatalf("Generate deadline %v out of expected ~%v window", remaining, singleShotTimeout)
	}
}

// TestDispatch proves, key-free (no DEEPSEEK_API_KEY, no network), that the
// single-shot dispatch:
//   - routes each mode to its OWN path (the system prompt the fake recorded
//     matches Summarize vs Rewrite vs Draft, not another mode),
//   - runs Rewrite and Draft outputs through validateProposedBody before return
//     (a ```-fenced fake body is rejected/retried, never surfaced),
//   - sets a ~60s timeout context on every Generate call.
func TestDispatch(t *testing.T) {
	ctx := context.Background()

	t.Run("summarize page routes to the summarize path with a 60s deadline", func(t *testing.T) {
		cm := &fakeChatModel{reply: "A concise grounded summary."}
		svc := newFakeService(cm)

		out, err := svc.SummarizePage(ctx, "notes/x.md")
		if err != nil {
			t.Fatalf("SummarizePage: %v", err)
		}
		if out != "A concise grounded summary." {
			t.Fatalf("SummarizePage returned %q", out)
		}
		sysPrompt := cm.gotMsgs[0].Content
		if sysPrompt != summarizeSystemPrompt {
			t.Fatalf("SummarizePage used the wrong system prompt; got:\n%s", sysPrompt)
		}
		// The page body must be delimited as untrusted DATA in the user turn.
		if !strings.Contains(cm.gotMsgs[1].Content, "page content to summarize") {
			t.Fatalf("page body did not reach the user turn: %s", cm.gotMsgs[1].Content)
		}
		assertDeadlineAbout60s(t, cm.gotCtx)
	})

	t.Run("summarize attachment routes to the summarize path", func(t *testing.T) {
		cm := &fakeChatModel{reply: "Attachment summary."}
		svc := newFakeService(cm)

		out, err := svc.SummarizeAttachment(ctx, "att-1")
		if err != nil {
			t.Fatalf("SummarizeAttachment: %v", err)
		}
		if out != "Attachment summary." {
			t.Fatalf("SummarizeAttachment returned %q", out)
		}
		if cm.gotMsgs[0].Content != summarizeSystemPrompt {
			t.Fatalf("SummarizeAttachment used the wrong system prompt")
		}
		if !strings.Contains(cm.gotMsgs[1].Content, "extracted attachment text") {
			t.Fatalf("attachment text did not reach the user turn")
		}
		assertDeadlineAbout60s(t, cm.gotCtx)
	})

	t.Run("summarize attachment with empty extraction returns ErrNoExtractedText", func(t *testing.T) {
		cm := &fakeChatModel{reply: "should not be called"}
		svc := &Service{cm: cm, deps: Deps{Attachments: fakeAttachmentReader{text: "   "}}, now: time.Now}
		_, err := svc.SummarizeAttachment(ctx, "att-empty")
		if !errors.Is(err, ErrNoExtractedText) {
			t.Fatalf("empty extraction err = %v, want ErrNoExtractedText", err)
		}
		if cm.calls != 0 {
			t.Fatalf("model was called %d times for an empty attachment; want 0", cm.calls)
		}
	})

	t.Run("rewrite routes to the rewrite path and passes validateProposedBody", func(t *testing.T) {
		cm := &fakeChatModel{reply: "the cleanly rewritten span"}
		svc := newFakeService(cm)

		out, err := svc.Rewrite(ctx, "old span", "make it concise")
		if err != nil {
			t.Fatalf("Rewrite: %v", err)
		}
		if out != "the cleanly rewritten span" {
			t.Fatalf("Rewrite returned %q", out)
		}
		if cm.gotMsgs[0].Content != rewriteSystemPrompt {
			t.Fatalf("Rewrite used the wrong system prompt; got:\n%s", cm.gotMsgs[0].Content)
		}
		assertDeadlineAbout60s(t, cm.gotCtx)
	})

	t.Run("rewrite REJECTS a fenced body (validateProposedBody), never returns it", func(t *testing.T) {
		fenced := "```\nthe rewritten span\n```"
		cm := &fakeChatModel{reply: fenced}
		svc := newFakeService(cm)

		out, err := svc.Rewrite(ctx, "old span", "make it concise")
		if err == nil {
			t.Fatalf("Rewrite returned a fenced body %q instead of erroring", out)
		}
		if out != "" {
			t.Fatalf("Rewrite returned non-empty body %q on a rejected (fenced) output", out)
		}
		if !errors.Is(err, ErrProposalInvalid) {
			t.Fatalf("Rewrite err = %v, want wraps ErrProposalInvalid", err)
		}
		// The fenced body should have been retried (1 initial + 2 retries = 3).
		if cm.calls != 3 {
			t.Fatalf("Rewrite called the model %d times; want 3 (validate-and-retry)", cm.calls)
		}
	})

	t.Run("draft routes to the draft path and passes validateProposedBody", func(t *testing.T) {
		cm := &fakeChatModel{reply: "# New Page\n\nDrafted body."}
		svc := newFakeService(cm)

		out, err := svc.Draft(ctx, "draft a page about onboarding")
		if err != nil {
			t.Fatalf("Draft: %v", err)
		}
		if out != "# New Page\n\nDrafted body." {
			t.Fatalf("Draft returned %q", out)
		}
		if cm.gotMsgs[0].Content != draftSystemPrompt {
			t.Fatalf("Draft used the wrong system prompt; got:\n%s", cm.gotMsgs[0].Content)
		}
		assertDeadlineAbout60s(t, cm.gotCtx)
	})

	t.Run("draft REJECTS a fenced body, never returns it", func(t *testing.T) {
		cm := &fakeChatModel{reply: "```markdown\n# New Page\n\nbody\n```"}
		svc := newFakeService(cm)

		out, err := svc.Draft(ctx, "draft a page")
		if err == nil || out != "" {
			t.Fatalf("Draft = (%q, %v), want empty body + error on a fenced output", out, err)
		}
		if !errors.Is(err, ErrProposalInvalid) {
			t.Fatalf("Draft err = %v, want wraps ErrProposalInvalid", err)
		}
		if cm.calls != 3 {
			t.Fatalf("Draft called the model %d times; want 3 (validate-and-retry)", cm.calls)
		}
	})

	t.Run("disabled service fails closed on every mode", func(t *testing.T) {
		svc := &Service{cm: nil, now: time.Now} // disabled (no model).
		if _, err := svc.SummarizePage(ctx, "x"); !errors.Is(err, ErrAgentDisabled) {
			t.Fatalf("SummarizePage disabled err = %v", err)
		}
		if _, err := svc.SummarizeAttachment(ctx, "x"); !errors.Is(err, ErrAgentDisabled) {
			t.Fatalf("SummarizeAttachment disabled err = %v", err)
		}
		if _, err := svc.Rewrite(ctx, "s", "i"); !errors.Is(err, ErrAgentDisabled) {
			t.Fatalf("Rewrite disabled err = %v", err)
		}
		if _, err := svc.Draft(ctx, "i"); !errors.Is(err, ErrAgentDisabled) {
			t.Fatalf("Draft disabled err = %v", err)
		}
	})
}
