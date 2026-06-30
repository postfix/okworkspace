// describe_test.go proves, key-free (no DEEPSEEK/OKF_LLM_API_KEY, no network),
// that the GenerateDescription single-shot MODE + ValidateDescription:
//   - return ONE trimmed single-line description + a non-empty base revision,
//   - route to the describe system prompt with a ~60s deadline and delimit the
//     page body as untrusted DATA,
//   - validate-and-retry on a markup/multi-line/over-cap/empty reply (3 calls,
//     wrapping ErrDescriptionInvalid) — fail closed, never a malformed value,
//   - fail closed when disabled (ErrAgentDisabled).
//
// It mirrors suggesttags_test.go's fake harness exactly (fakeChatModel +
// fakePageReader, both key-free).
package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateDescription(t *testing.T) {
	t.Run("clean sentence passes unchanged", func(t *testing.T) {
		got, err := ValidateDescription("A concise note about the 2.0 release process.")
		if err != nil {
			t.Fatalf("ValidateDescription: %v", err)
		}
		if got != "A concise note about the 2.0 release process." {
			t.Fatalf("got %q, want the clean sentence unchanged", got)
		}
	})

	t.Run("trims leading/trailing whitespace", func(t *testing.T) {
		got, err := ValidateDescription("   Trimmed me.   ")
		if err != nil {
			t.Fatalf("ValidateDescription: %v", err)
		}
		if got != "Trimmed me." {
			t.Fatalf("got %q, want %q", got, "Trimmed me.")
		}
	})

	t.Run("collapses internal whitespace runs to a single space", func(t *testing.T) {
		got, err := ValidateDescription("Too    many     spaces here.")
		if err != nil {
			t.Fatalf("ValidateDescription: %v", err)
		}
		if got != "Too many spaces here." {
			t.Fatalf("got %q, want collapsed single spaces", got)
		}
	})

	t.Run("rejects a newline (multi-line is not a description)", func(t *testing.T) {
		if _, err := ValidateDescription("Line one.\nLine two."); !errors.Is(err, ErrDescriptionInvalid) {
			t.Fatalf("newline err = %v, want ErrDescriptionInvalid", err)
		}
	})

	t.Run("rejects a carriage return / tab / control / NUL", func(t *testing.T) {
		for _, bad := range []string{"has\rcr", "has\ttab", "has\x07bell", "has\x00nul"} {
			if _, err := ValidateDescription(bad); !errors.Is(err, ErrDescriptionInvalid) {
				t.Fatalf("ValidateDescription(%q) err = %v, want ErrDescriptionInvalid", bad, err)
			}
		}
	})

	t.Run("rejects empty / whitespace-only", func(t *testing.T) {
		for _, bad := range []string{"", "    ", "\t\t"} {
			if _, err := ValidateDescription(bad); !errors.Is(err, ErrDescriptionInvalid) {
				t.Fatalf("ValidateDescription(%q) err = %v, want ErrDescriptionInvalid", bad, err)
			}
		}
	})

	t.Run("rejects an over-cap description", func(t *testing.T) {
		over := strings.Repeat("word ", 60) // ~300 chars single-line, > maxDescriptionLen
		if _, err := ValidateDescription(over); !errors.Is(err, ErrDescriptionInvalid) {
			t.Fatalf("over-cap err = %v, want ErrDescriptionInvalid", err)
		}
	})

	t.Run("accepts exactly the cap", func(t *testing.T) {
		atCap := strings.Repeat("x", maxDescriptionLen)
		got, err := ValidateDescription(atCap)
		if err != nil {
			t.Fatalf("at-cap ValidateDescription: %v", err)
		}
		if got != atCap {
			t.Fatalf("at-cap got %q", got)
		}
	})
}

func TestGenerateDescription(t *testing.T) {
	ctx := context.Background()

	t.Run("clean reply returns a trimmed single-line description + base revision, one call", func(t *testing.T) {
		cm := &fakeChatModel{reply: "  Release notes for the 2.0 editor.  "}
		svc := newFakeTagService(cm, nil)

		text, baseRev, err := svc.GenerateDescription(ctx, "notes/x.md")
		if err != nil {
			t.Fatalf("GenerateDescription: %v", err)
		}
		if text != "Release notes for the 2.0 editor." {
			t.Fatalf("text = %q, want the trimmed sentence", text)
		}
		if baseRev == "" {
			t.Fatal("baseRev must be non-empty (captured at generate time)")
		}
		if cm.calls != 1 {
			t.Fatalf("model called %d times; want 1", cm.calls)
		}
		if cm.gotMsgs[0].Content != describeSystemPrompt {
			t.Fatalf("wrong system prompt; got:\n%s", cm.gotMsgs[0].Content)
		}
		assertDeadlineAbout60s(t, cm.gotCtx)
	})

	t.Run("page body is delimited as untrusted DATA in the user turn", func(t *testing.T) {
		cm := &fakeChatModel{reply: "A short description."}
		svc := newFakeTagService(cm, nil)

		if _, _, err := svc.GenerateDescription(ctx, "notes/x.md"); err != nil {
			t.Fatalf("GenerateDescription: %v", err)
		}
		user := cm.gotMsgs[1].Content
		if !strings.Contains(user, "BEGIN PAGE CONTENT (untrusted)") {
			t.Fatalf("page body was not delimited as untrusted DATA: %s", user)
		}
		if !strings.Contains(user, "The 2.0 release ships") {
			t.Fatalf("page body did not reach the user turn: %s", user)
		}
	})

	t.Run("a multi-line (markup) reply is rejected and retried (3 calls), wraps ErrDescriptionInvalid", func(t *testing.T) {
		cm := &fakeChatModel{reply: "# Heading\n\nThis is a paragraph, not a one-liner."}
		svc := newFakeTagService(cm, nil)

		text, _, err := svc.GenerateDescription(ctx, "notes/x.md")
		if err == nil {
			t.Fatalf("multi-line reply returned %q instead of erroring", text)
		}
		if text != "" {
			t.Fatalf("rejected reply returned non-empty text %q", text)
		}
		if !errors.Is(err, ErrDescriptionInvalid) {
			t.Fatalf("err = %v, want wraps ErrDescriptionInvalid", err)
		}
		if cm.calls != 3 {
			t.Fatalf("model called %d times; want 3 (validate-and-retry)", cm.calls)
		}
	})

	t.Run("an over-cap reply is rejected and retried (3 calls), fails closed", func(t *testing.T) {
		cm := &fakeChatModel{reply: strings.Repeat("word ", 60)}
		svc := newFakeTagService(cm, nil)

		text, _, err := svc.GenerateDescription(ctx, "notes/x.md")
		if err == nil || text != "" {
			t.Fatalf("over-cap = (%q, %v), want empty + error", text, err)
		}
		if !errors.Is(err, ErrDescriptionInvalid) {
			t.Fatalf("err = %v, want wraps ErrDescriptionInvalid", err)
		}
		if cm.calls != 3 {
			t.Fatalf("model called %d times; want 3", cm.calls)
		}
	})

	t.Run("an empty reply is rejected and retried, fails closed", func(t *testing.T) {
		cm := &fakeChatModel{reply: "   "}
		svc := newFakeTagService(cm, nil)

		_, _, err := svc.GenerateDescription(ctx, "notes/x.md")
		if !errors.Is(err, ErrDescriptionInvalid) {
			t.Fatalf("empty reply err = %v, want ErrDescriptionInvalid", err)
		}
		if cm.calls != 3 {
			t.Fatalf("model called %d times; want 3", cm.calls)
		}
	})

	t.Run("disabled service fails closed with ErrAgentDisabled", func(t *testing.T) {
		svc := &Service{cm: nil, now: time.Now}
		if _, _, err := svc.GenerateDescription(ctx, "notes/x.md"); !errors.Is(err, ErrAgentDisabled) {
			t.Fatalf("disabled err = %v, want ErrAgentDisabled", err)
		}
	})
}
