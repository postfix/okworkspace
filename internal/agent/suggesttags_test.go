// suggesttags_test.go proves, key-free (no DEEPSEEK/OKF_LLM_API_KEY, no network),
// that the SuggestTags single-shot MODE + validateTags:
//   - normalize/dedupe/cap/reject-garbage the model's tag list (validateTags),
//   - flag each tag existing-vs-new against the workspace vocabulary,
//   - route to the suggest-tags system prompt with a ~60s deadline,
//   - delimit the page body as untrusted DATA and inject the vocabulary as a hint,
//   - validate-and-retry on garbage (3 calls, wrapping ErrTagsInvalid),
//   - fail closed when disabled and tolerate a nil vocabulary dep.
//
// It mirrors dispatch_test.go's fakeChatModel harness exactly.
package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeVocabReader serves a canned vocabulary key-free (mirrors fakeAttachmentReader).
type fakeVocabReader struct {
	vocab []string
	err   error
}

func (f fakeVocabReader) Vocabulary(context.Context) ([]string, error) {
	return f.vocab, f.err
}

var _ vocabularyReader = fakeVocabReader{}

// newFakeTagService builds an agent.Service with a fake ChatModel + fake page
// reader + fake vocabulary reader, all key-free.
func newFakeTagService(cm *fakeChatModel, vocab vocabularyReader) *Service {
	return &Service{
		cm: cm,
		deps: Deps{
			Pages:      fakePageReader{body: "# Release notes\n\nThe 2.0 release ships the new editor.", path: "notes/x.md", revision: "rev-N"},
			Vocabulary: vocab,
		},
		now: time.Now,
	}
}

func TestValidateTags(t *testing.T) {
	t.Run("clean list of 3 returns 3 normalized tags, no error", func(t *testing.T) {
		got, existing, err := validateTags([]string{"release", "notes", "draft"}, nil)
		if err != nil {
			t.Fatalf("validateTags: %v", err)
		}
		if len(got) != 3 || got[0] != "release" || got[1] != "notes" || got[2] != "draft" {
			t.Fatalf("got %v, want [release notes draft]", got)
		}
		if len(existing) != 3 {
			t.Fatalf("existing flags len = %d, want 3", len(existing))
		}
	})

	t.Run("lowercases and trims each tag", func(t *testing.T) {
		got, _, err := validateTags([]string{"  Release ", "NOTES"}, nil)
		if err != nil {
			t.Fatalf("validateTags: %v", err)
		}
		if len(got) != 2 || got[0] != "release" || got[1] != "notes" {
			t.Fatalf("got %v, want [release notes]", got)
		}
	})

	t.Run("dedupes after normalization, first wins, order preserved", func(t *testing.T) {
		got, _, err := validateTags([]string{"Release", "release", "RELEASE", "notes"}, nil)
		if err != nil {
			t.Fatalf("validateTags: %v", err)
		}
		if len(got) != 2 || got[0] != "release" || got[1] != "notes" {
			t.Fatalf("got %v, want [release notes]", got)
		}
	})

	t.Run("caps to exactly MaxSuggestedTags surviving tokens", func(t *testing.T) {
		raw := []string{"a", "b", "c", "d", "e", "f", "g"}
		got, _, err := validateTags(raw, nil)
		if err != nil {
			t.Fatalf("validateTags: %v", err)
		}
		if len(got) != MaxSuggestedTags {
			t.Fatalf("got %d tags, want capped to %d", len(got), MaxSuggestedTags)
		}
		if got[0] != "a" || got[MaxSuggestedTags-1] != "e" {
			t.Fatalf("cap kept the wrong first %d: %v", MaxSuggestedTags, got)
		}
	})

	t.Run("rejects garbage: empty/whitespace/over-length/NUL/interior-whitespace/control", func(t *testing.T) {
		long := strings.Repeat("x", maxTagLen+1)
		raw := []string{
			"",                 // empty
			"   ",              // whitespace only
			long,               // over-length
			"bad\x00tag",       // NUL
			"two words",        // interior whitespace (a sentence, not a token)
			"tab\ttag",         // interior control/whitespace
			"good",             // the one survivor
		}
		got, _, err := validateTags(raw, nil)
		if err != nil {
			t.Fatalf("validateTags: %v", err)
		}
		if len(got) != 1 || got[0] != "good" {
			t.Fatalf("got %v, want only [good] (all garbage dropped)", got)
		}
	})

	t.Run("all-garbage / empty result returns ErrTagsInvalid", func(t *testing.T) {
		_, _, err := validateTags([]string{"", "   ", "two words"}, nil)
		if !errors.Is(err, ErrTagsInvalid) {
			t.Fatalf("err = %v, want ErrTagsInvalid", err)
		}
	})

	t.Run("existing-vs-new flag is correct against normalized vocab", func(t *testing.T) {
		// Vocab uses mixed case to prove comparison is on the normalized form.
		got, existing, err := validateTags([]string{"Release", "brandnew"}, []string{"release", "Onboarding"})
		if err != nil {
			t.Fatalf("validateTags: %v", err)
		}
		if len(got) != 2 || !existing[0] || existing[1] {
			t.Fatalf("tags=%v existing=%v, want release existing=true, brandnew existing=false", got, existing)
		}
	})
}

// TestParseTagArray proves parseTagArray EXTRACTS the candidate array from the
// real-model wrappers DeepSeek-class models add (prose, fences-with-prose, JSON
// objects, prose-wrapped arrays) — and still rejects genuinely garbage replies so
// the retry loop fires. Extraction only; validateTags still gates contents.
func TestParseTagArray(t *testing.T) {
	cases := []struct {
		name  string
		reply string
		want  []string
	}{
		{"bare array (regression)", `["release","notes","draft"]`, []string{"release", "notes", "draft"}},
		{"whole-reply json fence (regression)", "```json\n[\"release\",\"notes\"]\n```", []string{"release", "notes"}},
		{"plain fence no lang (regression)", "```\n[\"release\"]\n```", []string{"release"}},
		{"fence WITH leading prose", "Here are the tags:\n```json\n[\"release\",\"notes\"]\n```\nHope that helps!", []string{"release", "notes"}},
		{"prose-then-array no fence", "Sure! Here are some good tags: [\"release\", \"notes\", \"editor\"] — let me know.", []string{"release", "notes", "editor"}},
		{"tags object form", `{"tags":["release","notes"]}`, []string{"release", "notes"}},
		{"suggestions object form", `{"suggestions":["release"]}`, []string{"release"}},
		{"object with prose and fence", "Here you go:\n```json\n{\"tags\": [\"alpha\", \"beta\"]}\n```", []string{"alpha", "beta"}},
		{"prose-wrapped object via array extraction", "The tags are: [\"x\",\"y\"]. Object was {\"other\":1}.", []string{"x", "y"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseTagArray(tc.reply)
			if err != nil {
				t.Fatalf("parseTagArray(%q): unexpected err %v", tc.reply, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}

	t.Run("genuinely garbage reply returns ErrTagsInvalid", func(t *testing.T) {
		for _, reply := range []string{
			"Here are some great tags for your page!",
			"I'm sorry, I cannot do that.",
			"",
			"{not even json",
		} {
			if _, err := parseTagArray(reply); !errors.Is(err, ErrTagsInvalid) {
				t.Fatalf("parseTagArray(%q) err = %v, want ErrTagsInvalid", reply, err)
			}
		}
	})
}

func TestSuggestTags(t *testing.T) {
	ctx := context.Background()

	t.Run("canned JSON array returns normalized tags + flags + base revision, one call", func(t *testing.T) {
		cm := &fakeChatModel{reply: `["release","notes","draft"]`}
		svc := newFakeTagService(cm, fakeVocabReader{vocab: []string{"release", "onboarding"}})

		tags, existing, baseRev, err := svc.SuggestTags(ctx, "notes/x.md")
		if err != nil {
			t.Fatalf("SuggestTags: %v", err)
		}
		if len(tags) != 3 || tags[0] != "release" || tags[1] != "notes" || tags[2] != "draft" {
			t.Fatalf("tags = %v, want [release notes draft]", tags)
		}
		if !existing[0] {
			t.Fatalf("release should flag existing=true (it is in vocab); existing=%v", existing)
		}
		if existing[1] || existing[2] {
			t.Fatalf("notes/draft should flag existing=false (not in vocab); existing=%v", existing)
		}
		if baseRev == "" {
			t.Fatal("baseRev must be non-empty (captured at suggest time)")
		}
		if cm.calls != 1 {
			t.Fatalf("model called %d times; want 1", cm.calls)
		}
		if cm.gotMsgs[0].Content != suggestTagsSystemPrompt {
			t.Fatalf("wrong system prompt; got:\n%s", cm.gotMsgs[0].Content)
		}
		assertDeadlineAbout60s(t, cm.gotCtx)
	})

	t.Run("real-model wrapped replies parse end-to-end through SuggestTags", func(t *testing.T) {
		wrapped := []struct {
			name  string
			reply string
		}{
			{"fence with leading prose", "Here are the tags:\n```json\n[\"release\",\"notes\"]\n```"},
			{"prose then bare array", "Sure! Good tags: [\"release\", \"notes\"]."},
			{"tags object", `{"tags":["release","notes"]}`},
		}
		for _, w := range wrapped {
			t.Run(w.name, func(t *testing.T) {
				cm := &fakeChatModel{reply: w.reply}
				svc := newFakeTagService(cm, fakeVocabReader{vocab: []string{"release"}})
				tags, existing, _, err := svc.SuggestTags(ctx, "notes/x.md")
				if err != nil {
					t.Fatalf("SuggestTags(%q): %v", w.reply, err)
				}
				if len(tags) != 2 || tags[0] != "release" || tags[1] != "notes" {
					t.Fatalf("got %v, want [release notes]", tags)
				}
				if !existing[0] || existing[1] {
					t.Fatalf("existing=%v, want [true false]", existing)
				}
				if cm.calls != 1 {
					t.Fatalf("model called %d times; want 1 (parsed on first attempt)", cm.calls)
				}
			})
		}
	})

	t.Run("page body is delimited as untrusted DATA and vocab appears as a bias hint", func(t *testing.T) {
		cm := &fakeChatModel{reply: `["release"]`}
		svc := newFakeTagService(cm, fakeVocabReader{vocab: []string{"onboarding", "security"}})

		if _, _, _, err := svc.SuggestTags(ctx, "notes/x.md"); err != nil {
			t.Fatalf("SuggestTags: %v", err)
		}
		user := cm.gotMsgs[1].Content
		if !strings.Contains(user, "BEGIN PAGE CONTENT (untrusted)") {
			t.Fatalf("page body was not delimited as untrusted DATA: %s", user)
		}
		if !strings.Contains(user, "The 2.0 release ships") {
			t.Fatalf("page body did not reach the user turn: %s", user)
		}
		if !strings.Contains(user, "onboarding") || !strings.Contains(user, "security") {
			t.Fatalf("vocabulary bias hint missing from the prompt: %s", user)
		}
	})

	t.Run("garbage reply is rejected and retried (3 calls), final error wraps ErrTagsInvalid", func(t *testing.T) {
		cm := &fakeChatModel{reply: "Here are some great tags for your page!"}
		svc := newFakeTagService(cm, fakeVocabReader{})

		tags, _, _, err := svc.SuggestTags(ctx, "notes/x.md")
		if err == nil {
			t.Fatalf("garbage reply returned tags %v instead of erroring", tags)
		}
		if tags != nil {
			t.Fatalf("garbage reply returned non-nil tags %v", tags)
		}
		if !errors.Is(err, ErrTagsInvalid) {
			t.Fatalf("err = %v, want wraps ErrTagsInvalid", err)
		}
		if cm.calls != 3 {
			t.Fatalf("model called %d times; want 3 (validate-and-retry)", cm.calls)
		}
	})

	t.Run("over-cap junk reply still validates+caps (not an error if some survive)", func(t *testing.T) {
		cm := &fakeChatModel{reply: `["a","b","c","d","e","f","g","h"]`}
		svc := newFakeTagService(cm, fakeVocabReader{})

		tags, _, _, err := svc.SuggestTags(ctx, "notes/x.md")
		if err != nil {
			t.Fatalf("SuggestTags: %v", err)
		}
		if len(tags) != MaxSuggestedTags {
			t.Fatalf("got %d tags, want capped to %d", len(tags), MaxSuggestedTags)
		}
	})

	t.Run("disabled service fails closed with ErrAgentDisabled", func(t *testing.T) {
		svc := &Service{cm: nil, now: time.Now}
		if _, _, _, err := svc.SuggestTags(ctx, "notes/x.md"); !errors.Is(err, ErrAgentDisabled) {
			t.Fatalf("disabled err = %v, want ErrAgentDisabled", err)
		}
	})

	t.Run("nil vocabulary dep is tolerated (best-effort bias)", func(t *testing.T) {
		cm := &fakeChatModel{reply: `["release","notes"]`}
		// nil Vocabulary dep.
		svc := newFakeTagService(cm, nil)

		tags, existing, _, err := svc.SuggestTags(ctx, "notes/x.md")
		if err != nil {
			t.Fatalf("SuggestTags with nil vocab: %v", err)
		}
		if len(tags) != 2 {
			t.Fatalf("got %v, want 2 tags even without a vocab dep", tags)
		}
		// With no vocab, nothing flags existing.
		if existing[0] || existing[1] {
			t.Fatalf("with nil vocab no tag should flag existing; existing=%v", existing)
		}
	})

	t.Run("vocabulary read error is tolerated (best-effort)", func(t *testing.T) {
		cm := &fakeChatModel{reply: `["release"]`}
		svc := newFakeTagService(cm, fakeVocabReader{err: errors.New("db down")})

		tags, _, _, err := svc.SuggestTags(ctx, "notes/x.md")
		if err != nil {
			t.Fatalf("a vocab read error must not fail SuggestTags: %v", err)
		}
		if len(tags) != 1 || tags[0] != "release" {
			t.Fatalf("got %v, want [release]", tags)
		}
	})
}
