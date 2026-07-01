package okf

import (
	"fmt"
	"strings"
)

// ValidateInlineTag validates a BARE tag name (no leading '#') against the
// inline-`#tag` grammar SUBSET that the canonical Lezer body grammar
// (web/src/lib/cm/hashtag.ts) can represent, so a rename/merge target can be
// safely spliced into a page body as `#`+name and round-trip as the SAME tag
// the editor would badge.
//
// It is STRICTER than and DISTINCT from ValidateTagCharset (repair.go:159,
// `^[a-zA-Z0-9_ -]+$`, which permits a space and an all-digit token but forbids
// `/` — the exact inverse of what body injection needs). ValidateTagCharset
// keeps its frontmatter charset / SEC-02 HTML-metachar role unchanged; this is
// an ADDITIONAL gate applied to a token that will enter PROSE.
//
// The rules mirror hashtag.ts exactly:
//   - non-empty (a bare `#` is not a tag);
//   - body alphabet only: [A-Za-z0-9_/-] (isBody) — no space/whitespace/control
//     char, no HTML metachar, no '.', '@', etc.;
//   - ≥1 non-digit [A-Za-z_-] (isNonDigit) — so `1984` and `12/34` are rejected
//     (a `/` is a separator, NOT a qualifying non-digit);
//   - slash policy: `/` separates nested segments (`a/b` OK) but no leading,
//     trailing, or doubled slash, and a token that is only digits-and-slashes is
//     rejected (subsumed by the ≥1-non-digit rule).
//
// A leading `#` fails the alphabet check (it is not a body char), so a caller
// that forgot to strip the `#` is rejected rather than silently double-hashed.
func ValidateInlineTag(tag string) error {
	if tag == "" {
		return fmt.Errorf("invalid inline tag: empty")
	}
	hasNonDigit := false
	for i := range len(tag) {
		c := tag[i]
		switch {
		case c >= '0' && c <= '9':
			// digit — a body char but not a non-digit
		case (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_' || c == '-':
			hasNonDigit = true // isNonDigit
		case c == '/':
			// separator — a body char but not a qualifying non-digit
		default:
			return fmt.Errorf("invalid inline tag %q: only letters, digits, '_', '-', and '/' are allowed", tag)
		}
	}
	if !hasNonDigit {
		return fmt.Errorf("invalid inline tag %q: must contain at least one letter, '_', or '-'", tag)
	}
	// Slash policy: no leading, trailing, or doubled slash. A trailing '/' in the
	// grammar is trimmed (a dangling separator) — but a rename/merge TARGET must be
	// an already-canonical tag, so we reject rather than silently trim it.
	if strings.HasPrefix(tag, "/") {
		return fmt.Errorf("invalid inline tag %q: must not start with '/'", tag)
	}
	if strings.HasSuffix(tag, "/") {
		return fmt.Errorf("invalid inline tag %q: must not end with '/'", tag)
	}
	if strings.Contains(tag, "//") {
		return fmt.Errorf("invalid inline tag %q: must not contain a doubled '/'", tag)
	}
	return nil
}
