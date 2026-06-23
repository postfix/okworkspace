package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fencedBody builds a whole-body ```-fenced string at test time. Built here (not
// as a package-level prose constant) so the validator's no-fence rule is not
// accidentally tripped by a fixture living in non-test source.
func fencedBody(inner string) string {
	return "```markdown\n" + inner + "\n```"
}

// sourceDoc is a minimal frontmatter+body OKF document used as the "original"
// that a proposed body must preserve the frontmatter key-set/order of.
const sourceDoc = "---\n" +
	"type: Page\n" +
	"title: Runbook\n" +
	"description: deploy steps\n" +
	"tags: [ops]\n" +
	"timestamp: 2026-06-21T00:00:00Z\n" +
	"---\n" +
	"# Deploy\n\nStep one.\n"

func TestValidateProposedBody(t *testing.T) {
	// A clean candidate that preserves the exact frontmatter (keys + order) and
	// only changes the body text.
	cleanBody := "---\n" +
		"type: Page\n" +
		"title: Runbook\n" +
		"description: deploy steps\n" +
		"tags: [ops]\n" +
		"timestamp: 2026-06-21T00:00:00Z\n" +
		"---\n" +
		"# Deploy\n\nStep one, revised.\n"

	// Frontmatter keys reordered relative to the source (title before type) —
	// must be rejected as mangled even though the same keys are present.
	reorderedFront := "---\n" +
		"title: Runbook\n" +
		"type: Page\n" +
		"description: deploy steps\n" +
		"tags: [ops]\n" +
		"timestamp: 2026-06-21T00:00:00Z\n" +
		"---\n" +
		"# Deploy\n\nStep one.\n"

	// A key dropped relative to the source (description removed).
	droppedKey := "---\n" +
		"type: Page\n" +
		"title: Runbook\n" +
		"tags: [ops]\n" +
		"timestamp: 2026-06-21T00:00:00Z\n" +
		"---\n" +
		"# Deploy\n\nStep one.\n"

	tests := []struct {
		name    string
		source  string
		body    string
		wantErr bool
	}{
		{name: "empty body", source: sourceDoc, body: "", wantErr: true},
		{name: "whitespace-only body", source: sourceDoc, body: "   \n\t\n", wantErr: true},
		{name: "whole-body fenced", source: sourceDoc, body: fencedBody(cleanBody), wantErr: true},
		{name: "frontmatter reordered", source: sourceDoc, body: reorderedFront, wantErr: true},
		{name: "frontmatter key dropped", source: sourceDoc, body: droppedKey, wantErr: true},
		{name: "clean preserving body", source: sourceDoc, body: cleanBody, wantErr: false},
		// When the source has no frontmatter, a clean bodyonly candidate is fine.
		{name: "bodyonly source clean candidate", source: "# Title\n\ntext\n", body: "# Title\n\nnew text\n", wantErr: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateProposedBody(tc.source, tc.body)
			if tc.wantErr && err == nil {
				t.Fatalf("validateProposedBody(%q) = nil, want error", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateProposedBody(%q) = %v, want nil", tc.name, err)
			}
		})
	}
}

// TestProposeWithRetryExhausts proves the retry wrapper returns the structured
// "could not produce a valid patch" error after the attempt budget is exhausted
// and NEVER returns a malformed body. The generator always returns a fenced
// (invalid) body, so every attempt fails validation.
func TestProposeWithRetryExhausts(t *testing.T) {
	attempts := 0
	gen := func(_ context.Context, _ int) (string, error) {
		attempts++
		return fencedBody("# bad\n"), nil // always invalid
	}

	body, err := proposeWithRetry(context.Background(), sourceDoc, gen)
	if err == nil {
		t.Fatalf("proposeWithRetry = nil error, want structured exhaustion error")
	}
	if body != "" {
		t.Fatalf("proposeWithRetry returned a non-empty body %q on exhaustion; a malformed body must never be returned", body)
	}
	if !errors.Is(err, ErrProposalInvalid) {
		t.Fatalf("proposeWithRetry err = %v, want wraps ErrProposalInvalid", err)
	}
	// 1 initial + 2 retries = 3 attempts total.
	if attempts != 3 {
		t.Fatalf("generator called %d times, want 3 (1 initial + 2 retries)", attempts)
	}
}

// TestProposeWithRetrySucceedsOnSecond proves the wrapper retries a transient
// invalid output and returns the first valid body without surfacing the bad one.
func TestProposeWithRetrySucceedsOnSecond(t *testing.T) {
	good := strings.Replace(sourceDoc, "Step one.", "Step one, revised.", 1)
	attempts := 0
	gen := func(_ context.Context, _ int) (string, error) {
		attempts++
		if attempts == 1 {
			return fencedBody("# bad\n"), nil // first attempt invalid
		}
		return good, nil
	}

	body, err := proposeWithRetry(context.Background(), sourceDoc, gen)
	if err != nil {
		t.Fatalf("proposeWithRetry = %v, want nil", err)
	}
	if body != good {
		t.Fatalf("proposeWithRetry body = %q, want %q", body, good)
	}
	if attempts != 2 {
		t.Fatalf("generator called %d times, want 2", attempts)
	}
}

// TestProposeWithRetryProviderError proves a provider/transport error is retried
// and, if it persists, surfaced as the structured error (never a panic / empty
// success).
func TestProposeWithRetryProviderError(t *testing.T) {
	provErr := errors.New("provider unreachable")
	gen := func(_ context.Context, _ int) (string, error) {
		return "", provErr
	}
	body, err := proposeWithRetry(context.Background(), sourceDoc, gen)
	if err == nil || body != "" {
		t.Fatalf("proposeWithRetry = (%q, %v), want empty body + error on persistent provider failure", body, err)
	}
	if !errors.Is(err, provErr) {
		t.Fatalf("proposeWithRetry err = %v, want wraps the provider error", err)
	}
}
