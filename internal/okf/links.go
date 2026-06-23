package okf

import (
	"bytes"
	"path"
)

// Link is one Markdown link found in an opaque body. Text is the link label;
// Dest is the raw destination as it appears in the source (before any title or
// fragment is stripped). start/end are byte offsets into the body of the
// destination substring (the bytes between the parentheses, excluding an
// optional title), so a rewrite can splice exactly the destination and leave
// every surrounding byte untouched.
type Link struct {
	Text  string
	Dest  string
	start int // byte offset of Dest within the body
	end   int // byte offset just past Dest within the body
}

// FindLinks scans body for inline Markdown links of the form [text](dest) and
// returns them in source order. It is STRUCTURAL, not a substring search: link
// tokens inside fenced code blocks (``` / ~~~) and inline code spans (backtick
// runs) are skipped, so a code block containing text that looks like a link is
// never matched. Reference-style link definitions ([id]: dest) are also matched
// so a rewrite can keep them honest. The body is never routed through a Markdown
// AST — this is a byte scanner that preserves the round-trip invariant.
func FindLinks(body []byte) []Link {
	var links []Link
	i := 0
	n := len(body)
	atLineStart := true
	for i < n {
		c := body[i]

		// Fenced code block: a line beginning with ``` or ~~~ opens a fence that
		// runs until the matching closing fence. Everything inside is opaque.
		if atLineStart {
			if fence, ok := fenceAt(body, i); ok {
				i = skipFencedBlock(body, i, fence)
				atLineStart = true
				continue
			}
			// Reference-style link definition: `[label]: destination` at the start
			// of a line (optionally indented). Capture and rewrite its destination.
			if ref, ok := refDefAt(body, i); ok {
				links = append(links, ref)
				i = ref.end
				// Advance to end of line; refDefAt set end to just past Dest.
				continue
			}
		}

		switch c {
		case '`':
			// Inline code span: skip a backtick run and everything up to the
			// matching run of the same length.
			i = skipInlineCode(body, i)
			atLineStart = false
			continue
		case '\\':
			// Escaped character: skip the backslash and the next byte so an
			// escaped '[' never opens a link.
			i += 2
			atLineStart = false
			continue
		case '[':
			if link, next, ok := inlineLinkAt(body, i); ok {
				links = append(links, link)
				i = next
				atLineStart = false
				continue
			}
		case '\n':
			atLineStart = true
			i++
			continue
		}
		atLineStart = false
		i++
	}
	return links
}

// fenceAt reports whether body at offset i (a line start) begins a fenced code
// block, returning the fence marker run (e.g. "```" or "~~~~").
func fenceAt(body []byte, i int) (fence []byte, ok bool) {
	// Allow up to three leading spaces (CommonMark), then a run of >=3 of the
	// same fence char.
	j := i
	for j < len(body) && body[j] == ' ' && j-i < 3 {
		j++
	}
	if j >= len(body) {
		return nil, false
	}
	ch := body[j]
	if ch != '`' && ch != '~' {
		return nil, false
	}
	k := j
	for k < len(body) && body[k] == ch {
		k++
	}
	if k-j < 3 {
		return nil, false
	}
	return body[j:k], true
}

// skipFencedBlock returns the offset just past the closing fence (or end of
// body) of the fenced block opened at offset i with the given fence marker.
func skipFencedBlock(body []byte, i int, fence []byte) int {
	// Advance to the next line (past the opening fence line).
	nl := bytes.IndexByte(body[i:], '\n')
	if nl < 0 {
		return len(body)
	}
	pos := i + nl + 1
	for pos < len(body) {
		lineEnd := bytes.IndexByte(body[pos:], '\n')
		var line []byte
		if lineEnd < 0 {
			line = body[pos:]
		} else {
			line = body[pos : pos+lineEnd]
		}
		// A closing fence is a line of >=len(fence) of the same fence char,
		// optionally indented up to three spaces, with only trailing whitespace.
		if isClosingFence(line, fence) {
			if lineEnd < 0 {
				return len(body)
			}
			return pos + lineEnd + 1
		}
		if lineEnd < 0 {
			return len(body)
		}
		pos += lineEnd + 1
	}
	return len(body)
}

// isClosingFence reports whether line closes a fence opened with marker.
func isClosingFence(line, marker []byte) bool {
	j := 0
	for j < len(line) && line[j] == ' ' && j < 3 {
		j++
	}
	ch := marker[0]
	count := 0
	for j < len(line) && line[j] == ch {
		j++
		count++
	}
	if count < len(marker) {
		return false
	}
	for ; j < len(line); j++ {
		if line[j] != ' ' && line[j] != '\t' && line[j] != '\r' {
			return false
		}
	}
	return true
}

// skipInlineCode returns the offset just past a backtick code span opened at i.
// A code span opens with a run of N backticks and closes with the next run of
// exactly N backticks. If no closing run exists the backticks are literal and
// we advance past just the opening run.
func skipInlineCode(body []byte, i int) int {
	n := len(body)
	runStart := i
	for i < n && body[i] == '`' {
		i++
	}
	runLen := i - runStart
	// Scan for a closing run of exactly runLen backticks.
	for i < n {
		if body[i] == '`' {
			closeStart := i
			for i < n && body[i] == '`' {
				i++
			}
			if i-closeStart == runLen {
				return i
			}
			continue
		}
		i++
	}
	// No close: treat the opening run as literal text.
	return runStart + runLen
}

// inlineLinkAt parses an inline link starting at body[i] == '['. It returns the
// parsed Link (with Dest offsets into body), the offset just past the link, and
// ok=false when body[i] does not begin a well-formed [text](dest) link.
func inlineLinkAt(body []byte, i int) (Link, int, bool) {
	n := len(body)
	// Find the closing ']' of the label, honoring escapes. Nested brackets are
	// not supported (rare in link text); a '[' inside aborts the match.
	j := i + 1
	depth := 0
	for j < n {
		switch body[j] {
		case '\\':
			j += 2
			continue
		case '[':
			depth++
		case ']':
			if depth == 0 {
				goto labelEnd
			}
			depth--
		case '\n':
			// Labels may span lines but a blank context is unusual; keep simple.
		}
		j++
	}
	return Link{}, 0, false
labelEnd:
	text := string(body[i+1 : j])
	// Require an immediately-following '(' for an inline link.
	k := j + 1
	if k >= n || body[k] != '(' {
		return Link{}, 0, false
	}
	k++ // past '('
	destStart := k
	// The destination runs until whitespace (introducing a title) or the closing
	// ')'. Angle-bracket destinations <...> are supported.
	if k < n && body[k] == '<' {
		// <dest> form.
		k++
		destStart = k
		for k < n && body[k] != '>' && body[k] != '\n' {
			k++
		}
		if k >= n || body[k] != '>' {
			return Link{}, 0, false
		}
		destEnd := k
		// Skip to closing ')'.
		k++
		for k < n && body[k] != ')' && body[k] != '\n' {
			k++
		}
		if k >= n || body[k] != ')' {
			return Link{}, 0, false
		}
		return Link{Text: text, Dest: string(body[destStart:destEnd]), start: destStart, end: destEnd}, k + 1, true
	}
	depthParen := 0
	for k < n {
		ch := body[k]
		if ch == '\\' {
			k += 2
			continue
		}
		if ch == ' ' || ch == '\t' || ch == '\n' {
			break // start of an optional title
		}
		if ch == '(' {
			depthParen++
		}
		if ch == ')' {
			if depthParen == 0 {
				break
			}
			depthParen--
		}
		k++
	}
	destEnd := k
	if destEnd == destStart {
		return Link{}, 0, false
	}
	// Advance past an optional title to the closing ')'.
	for k < n && body[k] != ')' {
		if body[k] == '\\' {
			k += 2
			continue
		}
		k++
	}
	if k >= n || body[k] != ')' {
		return Link{}, 0, false
	}
	return Link{Text: text, Dest: string(body[destStart:destEnd]), start: destStart, end: destEnd}, k + 1, true
}

// refDefAt parses a reference-style link definition `[label]: destination` at a
// line start (offset i). It returns a Link whose Dest offsets cover only the
// destination token, and ok=false when the line is not a reference definition.
func refDefAt(body []byte, i int) (Link, bool) {
	n := len(body)
	j := i
	for j < n && body[j] == ' ' && j-i < 3 {
		j++
	}
	if j >= n || body[j] != '[' {
		return Link{}, false
	}
	labelStart := j + 1
	k := labelStart
	for k < n && body[k] != ']' && body[k] != '\n' {
		if body[k] == '\\' {
			k += 2
			continue
		}
		k++
	}
	if k >= n || body[k] != ']' {
		return Link{}, false
	}
	label := string(body[labelStart:k])
	k++ // past ']'
	if k >= n || body[k] != ':' {
		return Link{}, false
	}
	k++ // past ':'
	for k < n && (body[k] == ' ' || body[k] == '\t') {
		k++
	}
	destStart := k
	for k < n && body[k] != ' ' && body[k] != '\t' && body[k] != '\n' && body[k] != '\r' {
		k++
	}
	if k == destStart {
		return Link{}, false
	}
	return Link{Text: label, Dest: string(body[destStart:k]), start: destStart, end: k}, true
}

// RewriteLinks rewrites every relative Markdown link destination in body whose
// resolved target equals oldRel to point at newRel, returning the edited body
// and whether anything changed. fromDir is the repo-relative directory of the
// file that contains body (so a relative destination can be resolved to a
// repo-relative target, and the replacement can be recomputed relative to the
// same directory). oldRel and newRel are repo-relative slash paths.
//
// Only the matched destination bytes are spliced; every other byte — body text,
// other frontmatter, code blocks containing link-like text, the link label and
// surrounding punctuation — is preserved exactly. A destination that merely ends
// with the old filename by substring coincidence is NOT rewritten: matching is
// on the resolved structural target, not bare text.
func RewriteLinks(body []byte, fromDir, oldRel, newRel string) ([]byte, bool) {
	links := FindLinks(body)
	if len(links) == 0 {
		return body, false
	}
	oldRel = path.Clean(oldRel)
	newRel = path.Clean(newRel)

	var out bytes.Buffer
	out.Grow(len(body))
	last := 0
	changed := false
	for _, l := range links {
		dest, frag := splitFragment(l.Dest)
		if dest == "" {
			continue
		}
		if isAbsoluteOrExternal(dest) {
			continue
		}
		resolved := path.Clean(path.Join(fromDir, dest))
		if resolved != oldRel {
			continue
		}
		// Recompute the destination relative to fromDir so the replacement is a
		// correct relative path from this file's location.
		newDest := relPath(fromDir, newRel) + frag
		out.Write(body[last:l.start])
		out.WriteString(newDest)
		last = l.end
		changed = true
	}
	if !changed {
		return body, false
	}
	out.Write(body[last:])
	return out.Bytes(), true
}

// RewriteLinksMoved is RewriteLinks for the case where the LINKING page is itself
// being relocated in the same operation (a folder move). resolveDir is the page's
// CURRENT directory (used to resolve each link to its target, matching oldRel);
// emitDir is the page's NEW directory (used to recompute the replacement relative
// path so the rewritten link is correct from the page's new location). When
// resolveDir == emitDir this is identical to RewriteLinks. It preserves the same
// round-trip safety: only genuine repo-relative links that resolve to oldRel are
// touched, code/inline-code spans are skipped by FindLinks, and unrelated bytes are
// never altered.
func RewriteLinksMoved(body []byte, resolveDir, emitDir, oldRel, newRel string) ([]byte, bool) {
	links := FindLinks(body)
	if len(links) == 0 {
		return body, false
	}
	oldRel = path.Clean(oldRel)
	newRel = path.Clean(newRel)

	var out bytes.Buffer
	out.Grow(len(body))
	last := 0
	changed := false
	for _, l := range links {
		dest, frag := splitFragment(l.Dest)
		if dest == "" {
			continue
		}
		if isAbsoluteOrExternal(dest) {
			continue
		}
		resolved := path.Clean(path.Join(resolveDir, dest))
		if resolved != oldRel {
			continue
		}
		newDest := relPath(emitDir, newRel) + frag
		if newDest == l.Dest {
			// The recomputed destination is byte-identical to the original (e.g. a
			// moved sibling link that stays a bare filename). Leave it untouched so a
			// moving page that only carries such links is NOT needlessly re-emitted —
			// preserving byte-stability (the caller copies its bytes verbatim).
			continue
		}
		out.Write(body[last:l.start])
		out.WriteString(newDest)
		last = l.end
		changed = true
	}
	if !changed {
		return body, false
	}
	out.Write(body[last:])
	return out.Bytes(), true
}

// splitFragment splits a destination into its path part and an optional
// #fragment (preserved verbatim, including the leading '#').
func splitFragment(dest string) (pathPart, frag string) {
	if idx := indexByteStr(dest, '#'); idx >= 0 {
		return dest[:idx], dest[idx:]
	}
	return dest, ""
}

func indexByteStr(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// isAbsoluteOrExternal reports whether a destination is an absolute path, a
// scheme URL (http:, mailto:, etc.), or a protocol-relative URL — none of which
// are repo-relative links and must never be rewritten.
func isAbsoluteOrExternal(dest string) bool {
	if dest == "" {
		return true
	}
	// A leading '/' covers both an absolute repo path ("/x") and a
	// protocol-relative URL ("//host/x") — neither is a repo-relative link, so
	// both are treated as external and never rewritten.
	if dest[0] == '/' {
		return true
	}
	// scheme:... — a colon before any slash indicates a URL scheme.
	for i := 0; i < len(dest); i++ {
		switch dest[i] {
		case ':':
			return true
		case '/':
			return false
		}
	}
	return false
}

// relPath returns target expressed relative to fromDir (both repo-relative slash
// paths). It mirrors filepath.Rel but for forward-slash repo paths and always
// produces a "./"-free, "../"-using relative path suitable for a Markdown link.
func relPath(fromDir, target string) string {
	fromDir = path.Clean(fromDir)
	if fromDir == "." {
		fromDir = ""
	}
	target = path.Clean(target)

	fromParts := splitNonEmpty(fromDir)
	targetParts := splitNonEmpty(target)

	// Common prefix.
	i := 0
	for i < len(fromParts) && i < len(targetParts) && fromParts[i] == targetParts[i] {
		i++
	}
	var rel []string
	for range fromParts[i:] {
		rel = append(rel, "..")
	}
	rel = append(rel, targetParts[i:]...)
	if len(rel) == 0 {
		return "."
	}
	return path.Join(rel...)
}

func splitNonEmpty(p string) []string {
	if p == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i <= len(p); i++ {
		if i == len(p) || p[i] == '/' {
			if i > start {
				parts = append(parts, p[start:i])
			}
			start = i + 1
		}
	}
	return parts
}
