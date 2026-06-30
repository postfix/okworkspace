// describe.go implements GenerateDescription — the single-shot agent MODE behind
// ED-01b (the LLM-generated page description). It mirrors the SuggestTags shape
// exactly: Get the page → capture the base revision AT generate time (echoed to
// /apply-description so a moved page 409s instead of clobbering) → truncate the
// body to the input budget → build a describe prompt → generateOnce → validate-
// and-retry (fail-closed). It NEVER writes — apply is a separate editor+CSRF HTTP
// endpoint (handlers_agent.go), and GenerateDescription is a MODE, never a tool
// (the read-only 5-tool allow-list is unchanged).
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxDescriptionLen is the HARD ceiling on a generated/approved description's
// length, in runes. The prompt asks the model for ~160 chars (the soft target);
// this slightly higher hard cap gives the model a little slack while still
// bounding the value byte-stably written into the `description:` frontmatter
// scalar. A reply over this cap is rejected → retried → fails closed.
const maxDescriptionLen = 200

// describeMaxTokens caps the generate-description output. ONE short sentence is
// tiny — a few hundred tokens is ample. Always set (never nil/unbounded), like the
// other single-shot modes.
const describeMaxTokens = 256

// describeTemperature keeps the description deterministic/cool (like summarize and
// suggest-tags) so the model produces a faithful one-line meta sentence rather
// than wandering into prose.
const describeTemperature = 0.2

// describeMaxRetries bounds the retry budget: 1 initial attempt + 2 retries = 3
// total (mirrors tagsMaxRetries / proposeMaxRetries). After exhaustion a structured
// error wrapping ErrDescriptionInvalid is returned — never an infinite loop, never
// an invalid (multi-line / over-cap / markup) description.
const describeMaxRetries = 2

// ErrDescriptionInvalid is the sibling sentinel to ErrTagsInvalid: the model's
// description output failed validation across every attempt (empty / control-bearing
// / multi-line / over-cap). Wrapped by the retry loop so the handler maps it to a
// clean structured 422 — a malformed description is NEVER returned.
var ErrDescriptionInvalid = errors.New("agent: generated description failed validation")

// ValidateDescription is the shared, exported validator for the generated text:
// the generate loop uses it as its output gate AND the /apply-description handler
// reuses it as the defense-in-depth re-validation of the client-approved text (the
// client value is NEVER trusted verbatim — ED-01b "re-validated server-side before
// write"). It enforces the description contract and returns the CLEANED text:
//
//   - trim leading/trailing whitespace;
//   - reject empty (nothing to write);
//   - reject NUL and ANY control character — this includes newline/CR/tab, which
//     is exactly what makes the result a single line (a description is one line);
//   - collapse internal runs of whitespace to a single space (cosmetic — the model
//     occasionally double-spaces; after the control reject the only whitespace left
//     is the regular space);
//   - reject a result longer than maxDescriptionLen runes.
//
// A failing input returns ErrDescriptionInvalid (wrapped) so the retry loop treats
// it as a failed attempt and the handler maps it to a 422.
func ValidateDescription(text string) (string, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", fmt.Errorf("%w: empty after trim", ErrDescriptionInvalid)
	}
	// Reject NUL + any control char (newline/CR/tab included) → enforces a single
	// plain-text line. This is the load-bearing gate the server apply re-runs.
	for _, r := range trimmed {
		if r == '\x00' || unicode.IsControl(r) {
			return "", fmt.Errorf("%w: control or NUL character", ErrDescriptionInvalid)
		}
	}
	// Collapse internal whitespace runs to a single space. strings.Fields splits on
	// Unicode whitespace and the join trims — after the control-char reject above the
	// only whitespace is the regular space (and the rare NBSP, normalized here too).
	cleaned := strings.Join(strings.Fields(trimmed), " ")
	if utf8.RuneCountInString(cleaned) > maxDescriptionLen {
		return "", fmt.Errorf("%w: longer than %d characters", ErrDescriptionInvalid, maxDescriptionLen)
	}
	return cleaned, nil
}

// GenerateDescription generates ONE plain-text meta sentence describing the page at
// path and returns it WITH the page's base revision captured at generate time (the
// optimistic-concurrency token the later /apply-description re-checks — exactly like
// SuggestTags / ProposePatch). It is a single-shot ChatModel.Generate MODE (NOT a
// tool): the page body is supplied inline as untrusted DATA, and the output runs
// through validate-and-retry (3 attempts) — a markup/multi-line/over-cap/empty reply
// never reaches the caller (fail-closed, mirroring the other single-shot modes).
//
// Flow (mirrors SuggestTags): fetch the body via the role-scoped pages reader (never
// os.ReadFile) → capture baseRev via pages.Revision AT generate time → build the
// messages → run the validate-and-retry loop → return (cleanedText, baseRev).
// generateOnce wraps each call in the single-shot ~60s timeout + an explicit
// MaxTokens. It NEVER writes — apply is the separate gated endpoint.
func (s *Service) GenerateDescription(ctx context.Context, path string) (text string, baseRev string, err error) {
	if s.cm == nil {
		return "", "", ErrAgentDisabled
	}
	if s.deps.Pages == nil {
		return "", "", errors.New("agent: page reader not configured")
	}

	pg, err := s.deps.Pages.Get(ctx, path)
	if err != nil {
		return "", "", err
	}

	// Capture the optimistic-concurrency token AT generate time. A concurrent edit
	// between now and /apply-description moves this, and apply will 409 (never clobber).
	baseRev, err = s.deps.Pages.Revision(ctx, path)
	if err != nil {
		return "", "", err
	}

	body := truncateForBudget(pg.Body, maxSingleShotInput)

	var lastErr error
	for attempt := 0; attempt <= describeMaxRetries; attempt++ {
		msgs := buildDescribeMessages(body, attempt)
		reply, gerr := s.generateOnce(ctx, msgs, describeMaxTokens, describeTemperature)
		if gerr != nil {
			lastErr = gerr
			slog.Warn("agent generate-description provider error", "attempt", attempt, "err", gerr)
			continue
		}
		cleaned, verr := ValidateDescription(reply)
		if verr != nil {
			lastErr = verr
			slog.Warn("agent generate-description failed validation", "attempt", attempt, "err", verr)
			continue
		}
		return cleaned, baseRev, nil
	}

	if lastErr == nil {
		lastErr = ErrDescriptionInvalid
	}
	return "", "", fmt.Errorf("agent could not produce a clean description: %w", lastErr)
}
