package okf_test

import (
	"testing"

	"github.com/postfix/okworkspace/internal/okf"
)

// TestValidateInlineTag exercises the inline-#tag grammar-subset validator over
// the SAME positive/negative corpus web/src/lib/cm/hashtag.test.ts pins the
// Lezer grammar against, so the Go gate agrees with what the editor materializes.
// The validator takes the BARE tag name (no leading '#').
func TestValidateInlineTag(t *testing.T) {
	cases := []struct {
		tag    string
		ok     bool
		reason string
	}{
		// Accept — the hashtag.test.ts positive corpus (bare names).
		{"deploy", true, "simple word"},
		{"a/b", true, "nested slash segment"},
		{"a_b-c", true, "underscore + dash body"},
		{"abc", true, "hex-like short word (Obsidian parity)"},
		{"fff", true, "hex-like word"},
		{"world123", true, "letters + digits"},
		{"a/b/c", true, "deep nested path"},
		{"a", true, "single letter"},
		{"_", true, "bare underscore is a non-digit"},
		{"-", true, "bare dash is a non-digit"},
		{"MixedCase", true, "uppercase allowed (handler lowercases first, but grammar is case-insensitive)"},

		// Reject — empty / whitespace / metachar.
		{"", false, "empty"},
		{"a b", false, "space (would break the token in a body)"},
		{"a\tb", false, "tab"},
		{"a\nb", false, "newline"},
		{"foo.bar", false, "dot is not a body char"},
		{"foo@bar", false, "at-sign is not a body char"},
		{"<script>", false, "HTML metachar"},
		{"café", false, "non-ASCII letter"},

		// Reject — all-digit / digits-and-slashes (the ≥1-non-digit rule).
		{"1984", false, "all-digit run"},
		{"12/34", false, "digits-and-slashes only"},
		{"0", false, "single digit"},

		// Reject — slash policy.
		{"/frag", false, "leading slash"},
		{"a/", false, "trailing slash"},
		{"a//b", false, "doubled slash"},
		{"///", false, "only slashes (also all-non-non-digit)"},

		// Reject — a leading '#' (caller must pass the bare name).
		{"#deploy", false, "leading '#' is not a body char"},
	}
	for _, tc := range cases {
		err := okf.ValidateInlineTag(tc.tag)
		if tc.ok && err != nil {
			t.Errorf("ValidateInlineTag(%q) = %v, want nil (%s)", tc.tag, err, tc.reason)
		}
		if !tc.ok && err == nil {
			t.Errorf("ValidateInlineTag(%q) = nil, want error (%s)", tc.tag, tc.reason)
		}
	}
}

// TestValidateInlineTag_DoesNotLoosenCharset is a guard: ValidateTagCharset keeps
// its EXISTING behavior (permits space + all-digit, forbids '/') — the two gates
// are complementary, not the same, and this phase must not repurpose the weaker
// one. (The stricter ValidateInlineTag is the body-injection gate.)
func TestValidateInlineTag_DoesNotLoosenCharset(t *testing.T) {
	// ValidateTagCharset still ACCEPTS a space and an all-digit token (its role).
	if err := okf.ValidateTagCharset("tag with spaces"); err != nil {
		t.Errorf("ValidateTagCharset(space) unexpectedly changed: %v", err)
	}
	if err := okf.ValidateTagCharset("1984"); err != nil {
		t.Errorf("ValidateTagCharset(all-digit) unexpectedly changed: %v", err)
	}
	// ValidateTagCharset still FORBIDS a slash (its role).
	if err := okf.ValidateTagCharset("a/b"); err == nil {
		t.Error("ValidateTagCharset(slash) unexpectedly accepted; charset gate was loosened")
	}
	// ValidateInlineTag is the INVERSE where it matters: rejects space/all-digit, accepts slash.
	if err := okf.ValidateInlineTag("tag with spaces"); err == nil {
		t.Error("ValidateInlineTag(space) accepted; must reject")
	}
	if err := okf.ValidateInlineTag("1984"); err == nil {
		t.Error("ValidateInlineTag(all-digit) accepted; must reject")
	}
	if err := okf.ValidateInlineTag("a/b"); err != nil {
		t.Errorf("ValidateInlineTag(slash) = %v, want nil", err)
	}
}
