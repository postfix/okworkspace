// suggesttags.go implements the SuggestTags single-shot agent MODE (TAG-01): a
// vocab-biased, capped, normalized per-page tag suggester that mirrors the
// Rewrite/Draft single-shot shape (agent.go) and the validate-and-retry harness
// (propose.go) — provider-agnostic (a JSON array of strings parsed + validated,
// NOT response_format) and fully testable WITHOUT an API key (a fake model).
//
// SuggestTags is a MODE, never a tool: the read-only 5-tool allow-list and its
// set-equality build gate (tools.go / tools_test.go) stay UNCHANGED. Apply is a
// separate non-tool HTTP endpoint (handlers_agent.go). SuggestTags itself NEVER
// writes — it captures the page's base revision (exactly like ProposePatch) so the
// later apply can 409 on a moved revision.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode"
)

// MaxSuggestedTags is the locked Phase-11 CONTEXT cap on how many tags a single
// suggestion returns. It is a NAMED constant — there is no bare literal 5 standing
// in for the cap in the suggest logic or the prompt.
const MaxSuggestedTags = 5

// maxTagLen caps a single normalized tag token. A tag is a single word (or a
// hyphenated word), never a sentence — a token longer than this is rejected as
// garbage by validateTags (it indicates the model returned prose, not a tag).
const maxTagLen = 40

// suggestTagsMaxTokens caps the suggest-tags Generate output. A JSON array of a
// handful of short tags is tiny — a few hundred tokens is ample. Always set
// (never nil/unbounded — T-04-14), mirroring the other single-shot modes.
const suggestTagsMaxTokens = 256

// suggestTagsTemperature keeps suggestion deterministic/cool (like summarize and
// rewrite) so the model reuses existing vocabulary rather than wandering.
const suggestTagsTemperature = 0.2

// ErrTagsInvalid is the sibling sentinel to ErrProposalInvalid: the model's tag
// output failed validation across every attempt (garbage/prose/over-cap with
// nothing surviving normalization). Wrapped by the retry loop so the handler maps
// it to a clean structured "couldn't produce a clean set" state (422) — a
// hallucinated set is NEVER returned.
var ErrTagsInvalid = errors.New("agent: suggested tags failed validation")

// tagsMaxRetries bounds the retry budget: 1 initial attempt + 2 retries = 3 total
// (mirrors proposeMaxRetries). After exhaustion a structured error wrapping
// ErrTagsInvalid is returned — never an infinite loop, never an invalid set.
const tagsMaxRetries = 2

// validateTags parses+validates a model-produced raw tag list into the normalized,
// deduped, capped result PLUS a parallel existing-vs-new flag slice (a tag present
// in vocab → existing=true). It is the provider-agnostic output contract for the
// suggest mode and the server-side re-validation gate the apply endpoint reuses.
//
// Normalization (per the locked CONTEXT decision): lowercase + trim; dedupe against
// each other (first occurrence wins, input order preserved); cap to
// MaxSuggestedTags. A surviving tag must be a single token — empty/whitespace-only,
// over-length, NUL-bearing, or interior-whitespace/control-char entries are
// rejected/dropped (a tag is never a sentence). An empty post-validation result
// returns ErrTagsInvalid so the retry loop treats it as a failed attempt.
//
// The existing flag is computed against the SAME normalized form of the vocab, so
// "Release" in vocab matches the normalized "release" suggestion.
func validateTags(raw []string, vocab []string) ([]string, []bool, error) {
	vocabSet := make(map[string]bool, len(vocab))
	for _, v := range vocab {
		if n := strings.ToLower(strings.TrimSpace(v)); n != "" {
			vocabSet[n] = true
		}
	}

	out := make([]string, 0, MaxSuggestedTags)
	existing := make([]bool, 0, MaxSuggestedTags)
	seen := make(map[string]bool, MaxSuggestedTags)

	for _, r := range raw {
		tag := strings.ToLower(strings.TrimSpace(r))
		if !isValidTagToken(tag) {
			continue // drop garbage: empty/over-length/NUL/interior-whitespace/control.
		}
		if seen[tag] {
			continue // dedupe after normalization; first occurrence wins, order preserved.
		}
		seen[tag] = true
		out = append(out, tag)
		existing = append(existing, vocabSet[tag])
		if len(out) >= MaxSuggestedTags {
			break // cap to exactly MaxSuggestedTags surviving tokens.
		}
	}

	if len(out) == 0 {
		return nil, nil, fmt.Errorf("%w: no valid tags survived normalization", ErrTagsInvalid)
	}
	return out, existing, nil
}

// isValidTagToken reports whether a NORMALIZED (already lowercased+trimmed) tag is
// a single acceptable token: non-empty, within maxTagLen, and containing no NUL,
// whitespace, or control characters (a tag is one token, never a phrase). The
// caller normalizes first; this is the strict per-token content check.
func isValidTagToken(tag string) bool {
	if tag == "" || len(tag) > maxTagLen {
		return false
	}
	for _, r := range tag {
		if r == '\x00' || unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// parseTagArray leniently EXTRACTS a candidate JSON array of strings from the
// model's raw reply, tolerant of the wrapping real chat models (DeepSeek etc.)
// commonly add despite the bare-array contract: leading/trailing prose, a code
// fence with or without surrounding prose, a JSON object like {"tags":[...]}, or a
// prose-wrapped array. It only EXTRACTS the candidate array — the contents are
// still gated downstream by validateTags (which is NOT relaxed). The extraction
// order is:
//  1. trim + strip a ```...``` fence found ANYWHERE (use its inner content),
//  2. parse the (de-fenced) text as a JSON array of strings,
//  3. else parse as a JSON object and accept a string-array under "tags" /
//     "suggestions" / "labels",
//  4. else extract the first '[' .. matching ']' substring and parse that,
//  5. else return ErrTagsInvalid (so genuinely garbage replies still retry).
func parseTagArray(reply string) ([]string, error) {
	s := strings.TrimSpace(reply)
	s = stripCodeFence(s)

	// 2. Direct JSON array of strings (the contract / regression path).
	if tags, ok := tryUnmarshalStringArray(s); ok {
		return tags, nil
	}

	// 3. JSON object carrying the array under a known key.
	if tags, ok := tryUnmarshalTagObject(s); ok {
		return tags, nil
	}

	// 4. Prose-wrapped array: extract the first bracketed JSON array substring.
	if sub, ok := firstJSONArray(s); ok {
		if tags, ok := tryUnmarshalStringArray(sub); ok {
			return tags, nil
		}
	}

	return nil, fmt.Errorf("%w: reply was not a JSON array of strings", ErrTagsInvalid)
}

// tryUnmarshalStringArray attempts to parse s as a JSON array of strings. It
// returns (tags, true) only when s is a valid JSON string array; otherwise
// (nil, false). Contents are validated later by validateTags.
func tryUnmarshalStringArray(s string) ([]string, bool) {
	var tags []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &tags); err != nil {
		return nil, false
	}
	return tags, true
}

// tryUnmarshalTagObject attempts to parse s as a JSON object and pull a string
// array out from under a recognized key ("tags", "suggestions", "labels").
// Returns (tags, true) on the first key that holds a string array.
func tryUnmarshalTagObject(s string) ([]string, bool) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &obj); err != nil {
		return nil, false
	}
	for _, key := range []string{"tags", "suggestions", "labels"} {
		raw, present := obj[key]
		if !present {
			continue
		}
		if tags, ok := tryUnmarshalStringArray(string(raw)); ok {
			return tags, true
		}
	}
	return nil, false
}

// firstJSONArray returns the substring from the first '[' to its matching ']'
// (bracket-balanced, skipping brackets inside JSON string literals), so a
// prose-wrapped array like `Here are the tags:\n["a","b"]` yields `["a","b"]`.
// Returns (sub, true) when a balanced array is found.
func firstJSONArray(s string) (string, bool) {
	start := strings.IndexByte(s, '[')
	if start < 0 {
		return "", false
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}
	return "", false
}

// stripCodeFence removes a ```...``` (optionally ```json) code fence the model may
// wrap its JSON in, returning the inner content. It tolerates LEADING/TRAILING
// prose around the fence: it finds the first opening ``` anywhere, then the next
// closing ```, and returns what is between them. A reply with no fence is returned
// unchanged (after trimming).
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	open := strings.Index(s, "```")
	if open < 0 {
		return s
	}
	// Skip past the opening fence line (which may carry a language tag like ```json).
	rest := s[open+3:]
	nl := strings.IndexByte(rest, '\n')
	if nl < 0 {
		return s // a lone "```" with no newline — not a real fence; leave as-is.
	}
	inner := rest[nl+1:]
	// The closing fence is the next ``` in the remaining content.
	if close := strings.Index(inner, "```"); close >= 0 {
		inner = inner[:close]
	}
	return strings.TrimSpace(inner)
}

// SuggestTags suggests up to MaxSuggestedTags normalized tags for the page at path
// and returns them WITH their existing-vs-new flags (against the workspace
// vocabulary) AND the page's base revision captured at suggest time (the
// optimistic-concurrency token the later /apply-tags re-checks — exactly like
// ProposePatch). It is a single-shot ChatModel.Generate MODE (NOT a tool): the page
// body is supplied inline as untrusted DATA, the vocabulary biases the prompt
// (best-effort; a nil vocab dep is tolerated), and the output runs through
// validate-and-retry (3 attempts) — a garbage/prose reply never reaches the caller.
//
// Flow (mirrors ProposePatch): fetch the body via the role-scoped pages reader
// (never os.ReadFile) → capture baseRev via pages.Revision AT suggest time → read
// the vocabulary via the narrow dep (best-effort) → build the messages → run the
// validate-and-retry loop → return (tags, existing-flags, baseRev). generateOnce
// wraps each call in the single-shot ~60s timeout + an explicit MaxTokens.
func (s *Service) SuggestTags(ctx context.Context, path string) (tags []string, existing []bool, baseRev string, err error) {
	if s.cm == nil {
		return nil, nil, "", ErrAgentDisabled
	}
	if s.deps.Pages == nil {
		return nil, nil, "", errors.New("agent: page reader not configured")
	}

	pg, err := s.deps.Pages.Get(ctx, path)
	if err != nil {
		return nil, nil, "", err
	}

	// Capture the optimistic-concurrency token AT suggest time. A concurrent edit
	// between now and /apply-tags will move this, and apply will 409 (never clobber).
	baseRev, err = s.deps.Pages.Revision(ctx, path)
	if err != nil {
		return nil, nil, "", err
	}

	// Vocabulary biasing is best-effort: a nil dep or a read error simply means no
	// bias hint (never a hard error — the suggestion still runs).
	var vocab []string
	if s.deps.Vocabulary != nil {
		if v, verr := s.deps.Vocabulary.Vocabulary(ctx); verr == nil {
			vocab = v
		} else {
			slog.Warn("agent suggest-tags vocabulary read failed (continuing without bias)", "err", verr)
		}
	}

	body := truncateForBudget(pg.Body, maxSingleShotInput)

	var lastErr error
	for attempt := 0; attempt <= tagsMaxRetries; attempt++ {
		msgs := buildSuggestTagsMessages(body, vocab, attempt)
		reply, gerr := s.generateOnce(ctx, msgs, suggestTagsMaxTokens, suggestTagsTemperature)
		if gerr != nil {
			lastErr = gerr
			slog.Warn("agent suggest-tags provider error", "attempt", attempt, "err", gerr)
			continue
		}
		rawTags, perr := parseTagArray(reply)
		if perr != nil {
			lastErr = perr
			slog.Warn("agent suggest-tags parse failed", "attempt", attempt, "err", perr)
			continue
		}
		validated, flags, verr := validateTags(rawTags, vocab)
		if verr != nil {
			lastErr = verr
			slog.Warn("agent suggest-tags failed validation", "attempt", attempt, "err", verr)
			continue
		}
		return validated, flags, baseRev, nil
	}

	if lastErr == nil {
		lastErr = ErrTagsInvalid
	}
	return nil, nil, "", fmt.Errorf("agent could not produce a clean tag set: %w", lastErr)
}

// ValidateTags is the exported wrapper over validateTags for the server's
// /apply-tags defense-in-depth re-validation (a tampered/un-normalized/over-cap
// client tag list is cleaned BEFORE it reaches pages.Save — the client list is
// NEVER trusted). It returns the normalized/capped/deduped tags + their existing
// flags, or ErrTagsInvalid (wrapped) when nothing survives.
func ValidateTags(raw []string, vocab []string) ([]string, []bool, error) {
	return validateTags(raw, vocab)
}
