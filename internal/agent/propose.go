// propose.go is the body-output contract every free-form mode that produces a
// candidate page/selection body depends on (Rewrite + Draft in this slice;
// Propose-patch in slice 5). The model returns free text; we validate it into a
// usable body and retry on failure rather than trusting the provider's output
// (AI-SPEC §4b "Structured Outputs" — validate-and-retry, not ResponseFormat).
//
// The two structural guarantees here:
//   - validateProposedBody rejects an empty body, a whole-body ```-fenced body,
//     and a body whose YAML frontmatter key-set/order was changed relative to the
//     source (round-trip rot / over-eager churn — RESEARCH Pitfall 1 & 6).
//   - proposeWithRetry runs ≤2 retries (3 attempts), logs each failed attempt
//     with the attempt index + validation error (NEVER the API key), and returns
//     a structured error after exhaustion — a malformed body is NEVER returned.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/postfix/okworkspace/internal/okf"
)

// yaml.Node kinds used when walking parsed frontmatter. okf.Doc.Front is a
// yaml.Node; a top-level frontmatter region parses to a document node wrapping a
// mapping node.
const (
	yamlDocumentNode = yaml.DocumentNode
	yamlMappingNode  = yaml.MappingNode
)

// ErrProposalInvalid is the sentinel a caller can errors.Is against when a
// proposed body failed validation across every attempt. Wrapped (with the last
// validation error) by proposeWithRetry so the UI renders a clean structured
// "couldn't produce a valid result" state, never a malformed body.
var ErrProposalInvalid = errors.New("agent: proposed body failed validation")

// proposeMaxRetries bounds the retry budget: 1 initial attempt + 2 retries = 3
// total. After exhaustion a structured error is returned (never an infinite loop,
// never a malformed body).
const proposeMaxRetries = 2

// validateProposedBody checks a model-produced candidate body against the source
// document it is meant to replace. It returns nil only for a body that is safe to
// hand to the diff/editor; otherwise a descriptive (key-free) error.
//
// Rejections (all per AI-SPEC §4b / RESEARCH Pitfall 6):
//   - empty or whitespace-only body.
//   - a body whose entire content is wrapped in a ``` code fence (the model
//     returned the body INSIDE a fence instead of as raw Markdown).
//   - a body whose YAML frontmatter key-set or key ORDER differs from the
//     source's frontmatter (dropped/added/reordered keys = round-trip rot). The
//     comparison is done by parsing BOTH with okf.Parse and comparing the ordered
//     top-level frontmatter keys — never a regex over raw bytes.
//
// When the source has no frontmatter, only the empty/fenced checks apply (there
// is no key-set to preserve).
func validateProposedBody(source, body string) error {
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("%w: body is empty", ErrProposalInvalid)
	}
	if isWholeBodyFenced(body) {
		return fmt.Errorf("%w: body is wrapped in a code fence", ErrProposalInvalid)
	}

	srcDoc, err := okf.Parse([]byte(source))
	if err != nil {
		// The source did not parse — we cannot assert frontmatter preservation, so
		// the empty/fenced checks above are all we can enforce. Treat as acceptable
		// (the caller already validated the source elsewhere).
		return nil
	}
	if !srcDoc.HasFrontmatter {
		return nil // no frontmatter to preserve.
	}

	bodyDoc, err := okf.Parse([]byte(body))
	if err != nil {
		return fmt.Errorf("%w: proposed body did not parse", ErrProposalInvalid)
	}
	if !bodyDoc.HasFrontmatter {
		return fmt.Errorf("%w: proposed body dropped the frontmatter", ErrProposalInvalid)
	}

	srcKeys := frontmatterKeys(srcDoc)
	bodyKeys := frontmatterKeys(bodyDoc)
	if !sameOrderedKeys(srcKeys, bodyKeys) {
		return fmt.Errorf("%w: frontmatter keys were changed or reordered", ErrProposalInvalid)
	}
	return nil
}

// isWholeBodyFenced reports whether the trimmed body begins and ends with a ```
// code fence (i.e. the model returned the whole body inside a fence). A body that
// merely CONTAINS a fenced code block in the middle is fine — only a wrapping
// fence is rejected.
func isWholeBodyFenced(body string) bool {
	t := strings.TrimSpace(body)
	if !strings.HasPrefix(t, "```") {
		return false
	}
	// First line is the opening fence (``` optionally followed by a lang tag).
	nl := strings.IndexByte(t, '\n')
	if nl < 0 {
		return false // a single ``` line with no content/close is degenerate but not a wrap.
	}
	rest := t[nl+1:]
	return strings.HasSuffix(strings.TrimRight(rest, "\n"), "```")
}

// frontmatterKeys returns the top-level frontmatter keys of a parsed doc in
// document order. A mapping yaml.Node stores keys at even indices of Content.
func frontmatterKeys(d *okf.Doc) []string {
	root := d.Front
	// The Front node is a document node wrapping the mapping; descend to the map.
	if root.Kind == yamlDocumentNode && len(root.Content) == 1 {
		root = *root.Content[0]
	}
	if root.Kind != yamlMappingNode {
		return nil
	}
	keys := make([]string, 0, len(root.Content)/2)
	for i := 0; i+1 < len(root.Content); i += 2 {
		keys = append(keys, root.Content[i].Value)
	}
	return keys
}

// sameOrderedKeys reports whether a and b are identical in length, content, and
// order.
func sameOrderedKeys(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// bodyGenerator produces a candidate body for one attempt. attempt is the
// zero-based attempt index (0 = initial) so the caller can append a corrective
// hint on retries. It returns the raw model body or a provider/transport error.
type bodyGenerator func(ctx context.Context, attempt int) (string, error)

// proposeWithRetry runs gen up to proposeMaxRetries+1 times, validating each
// candidate against source via validateProposedBody. It returns the FIRST valid
// body. On a provider error or a validation failure it retries (logging the
// attempt index + error, NEVER the API key). After exhausting the budget it
// returns a structured error wrapping the last failure — a malformed body is
// NEVER returned to the caller.
func proposeWithRetry(ctx context.Context, source string, gen bodyGenerator) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= proposeMaxRetries; attempt++ {
		body, err := gen(ctx, attempt)
		if err != nil {
			lastErr = err
			slog.Warn("agent proposal provider error", "attempt", attempt, "err", err)
			continue
		}
		if v := validateProposedBody(source, body); v != nil {
			lastErr = v
			slog.Warn("agent proposal failed validation", "attempt", attempt, "err", v)
			continue
		}
		return body, nil
	}
	if lastErr == nil {
		lastErr = ErrProposalInvalid
	}
	return "", fmt.Errorf("agent could not produce a valid body: %w", lastErr)
}
