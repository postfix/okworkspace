package tagmeta

import (
	"fmt"
	"strings"
)

// paletteTokens is the fixed safe set of named color tokens accepted alongside
// hex. A map gives O(1) lookup. Extending the subset is a deliberate decision —
// never open it to arbitrary CSS (the safe-color-subset lens).
var paletteTokens = map[string]bool{
	"red":    true,
	"blue":   true,
	"green":  true,
	"amber":  true,
	"purple": true,
	"gray":   true,
}

// ValidateColor rejects any color value outside the safe subset: a hex color
// (#RRGGBB or #RGB) or one of the fixed palette tokens. An empty string is valid
// (it means "clear / use default"). Anything else — url(), var(), expressions,
// arbitrary CSS — is rejected with a 400 at the server gate. This is a
// trust-boundary input: the color flows into a style attribute on badge DOM.
func ValidateColor(color string) error {
	if color == "" {
		return nil // clear = no color
	}
	if paletteTokens[color] {
		return nil
	}
	// Hex: #RGB or #RRGGBB (case-insensitive).
	if len(color) == 4 && color[0] == '#' && isHex(color[1:]) {
		return nil
	}
	if len(color) == 7 && color[0] == '#' && isHex(color[1:]) {
		return nil
	}
	return fmt.Errorf("invalid color %q: must be a hex color (#RGB or #RRGGBB) or a palette token", color)
}

// isHex reports whether s consists entirely of hex digits (case-insensitive).
func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return len(s) > 0
}

// ValidateDescription rejects multi-line text, HTML metacharacters (<, >, &, ", '),
// and descriptions longer than 200 characters. An empty string is valid (it means
// "clear / no description"). The description is rendered as textContent in the admin
// UI (never as HTML), but rejecting metacharacters at the gate is defense-in-depth
// against any future rendering change. Single-line is enforced (no newlines).
func ValidateDescription(desc string) error {
	if desc == "" {
		return nil
	}
	if strings.ContainsAny(desc, "\r\n") {
		return fmt.Errorf("description must be a single line")
	}
	if len(desc) > 200 {
		return fmt.Errorf("description must be at most 200 characters")
	}
	if strings.ContainsAny(desc, `<>&"'`) {
		return fmt.Errorf("description must not contain HTML metacharacters (<, >, &, \", ')")
	}
	return nil
}