package okf

import (
	"strconv"
	"strings"
	"unicode"
)

// Heading is one ATX heading found in an opaque body. Level is the heading
// level (1-6, the number of leading '#'). Text is the heading text with the
// leading '#' run, surrounding whitespace, and any trailing '#' closer run
// removed. Anchor is the GitHub-style slug WITH a leading '#', de-duplicated
// within the document exactly as github-slugger (and therefore rehype-slug on
// the read view) does, so a heading search result deep-links to the rendered
// section.
type Heading struct {
	Level  int
	Text   string
	Anchor string
}

// ScanHeadings scans body line by line for ATX headings (1-6 leading '#'
// followed by a space) and returns them in source order. It is byte-scanning
// only: the body is NEVER routed through a Markdown AST and is never re-emitted
// (the round-trip invariant — search reads the body, it never writes it back).
//
// A '#' line inside a fenced code block (``` or ~~~) is opaque and never
// treated as a heading, reusing the same fence-skipping discipline FindLinks
// uses (fenceAt/skipFencedBlock). Anchors are computed with the same algorithm
// github-slugger implements (lowercase, strip everything that is not a Unicode
// letter/number/'-'/'_'/space, then map each space to '-' — NO whitespace
// collapsing and NO trimming of the resulting hyphens), with the same -1/-2
// de-dup suffix, so the Go anchors equal the ids rehype-slug assigns.
func ScanHeadings(body []byte) []Heading {
	var headings []Heading
	occurrences := map[string]int{}

	i := 0
	n := len(body)
	for i < n {
		// At a line start: a fence opens an opaque block we skip entirely.
		if fence, ok := fenceAt(body, i); ok {
			i = skipFencedBlock(body, i, fence)
			continue
		}

		// Extract the current line [i, lineEnd) and advance i past it.
		lineEnd := i
		for lineEnd < n && body[lineEnd] != '\n' {
			lineEnd++
		}
		line := body[i:lineEnd]
		if lineEnd < n {
			i = lineEnd + 1
		} else {
			i = lineEnd
		}

		level, text, ok := atxHeading(line)
		if !ok {
			continue
		}
		anchor := dedupSlug(occurrences, slug(text))
		headings = append(headings, Heading{Level: level, Text: text, Anchor: "#" + anchor})
	}
	return headings
}

// atxHeading reports whether line is an ATX heading. CommonMark allows up to
// three leading spaces, then 1-6 '#', then at least one space/tab before the
// text. The returned text has the trailing '#' closer run and surrounding
// whitespace stripped.
func atxHeading(line []byte) (level int, text string, ok bool) {
	j := 0
	// Up to three leading spaces.
	for j < len(line) && line[j] == ' ' && j < 3 {
		j++
	}
	// Count the '#' run.
	hashes := 0
	for j < len(line) && line[j] == '#' {
		hashes++
		j++
	}
	if hashes < 1 || hashes > 6 {
		return 0, "", false
	}
	// A space or tab MUST follow the '#' run (otherwise "#NoSpace" is not a
	// heading), unless the line is just the '#' run (empty heading).
	if j < len(line) && line[j] != ' ' && line[j] != '\t' {
		return 0, "", false
	}
	rest := strings.TrimRight(string(line[j:]), " \t\r")
	rest = strings.TrimLeft(rest, " \t")
	// Strip a trailing closing '#' run (preceded by whitespace, per CommonMark)
	// and any whitespace before it.
	rest = trimATXClosing(rest)
	return hashes, rest, true
}

// trimATXClosing removes a trailing run of '#' (the optional ATX closing
// sequence) and the whitespace that precedes it.
func trimATXClosing(s string) string {
	t := strings.TrimRight(s, "#")
	if t == s {
		return strings.TrimRight(s, " \t")
	}
	// There was a trailing '#' run; CommonMark requires it be preceded by a
	// space (or be the whole line). Trim the trailing whitespace before it.
	trimmed := strings.TrimRight(t, " \t")
	if trimmed != t {
		return trimmed
	}
	// No space before the '#' run (e.g. "foo#") — not a closer; keep original
	// minus trailing whitespace.
	return strings.TrimRight(s, " \t")
}

// slug computes a GitHub-style anchor slug for text, mirroring github-slugger's
// non-unique `slug()`: lowercase, drop every rune that is not a Unicode
// letter/number or one of '-'/'_'/space, then replace each space with '-'.
// Whitespace is NOT collapsed and hyphens are NOT trimmed — github-slugger does
// neither, and the read view's rehype-slug must agree byte-for-byte.
func slug(text string) string {
	lower := strings.ToLower(text)
	var b strings.Builder
	b.Grow(len(lower))
	for _, r := range lower {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || r == '_':
			b.WriteRune(r)
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
		default:
			// dropped (punctuation, symbols, control, etc.)
		}
	}
	return b.String()
}

// dedupSlug returns base unchanged the first time it is seen and base+"-N"
// (N=1,2,...) on repeats, tracking occurrences exactly as github-slugger's
// BananaSlug.slug does so the Nth duplicate heading gets the same suffix on the
// rendered page.
func dedupSlug(occurrences map[string]int, base string) string {
	result := base
	if _, seen := occurrences[result]; seen {
		for {
			occurrences[base]++
			result = base + "-" + strconv.Itoa(occurrences[base])
			if _, taken := occurrences[result]; !taken {
				break
			}
		}
	}
	occurrences[result] = 0
	return result
}
