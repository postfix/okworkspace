package okf

import (
	"bytes"
	"strings"
)

// hashtag.go — the SERVER-SIDE inline-`#tag` scanner + rewriter (TR-01), the
// byte-stable counterpart to links.go's FindLinks/RewriteLinks. It is a
// conservative, code-region-aware BYTE scanner — NOT a general Markdown parser —
// pinned offset-for-offset to the canonical TS Lezer grammar
// (web/src/lib/cm/hashtag.ts) by the whole-document parity corpus in
// hashtag_test.go / hashtag_corpus.json (TR-02). A single Hashtag recognizer
// (FindHashtags) is the ONE source of truth reused by RewriteHashtags AND the
// pages-service body enumeration (tagscan.go), so there is never a "second #tag
// parser".
//
// It recognizes a `#tag` exactly where the Lezer parser emits a Hashtag inline
// node, which means it must SKIP every region the Lezer parse excludes:
//   - fenced code blocks (``` / ~~~) — reuses fenceAt/skipFencedBlock (links.go)
//   - inline code spans (backtick runs) — reuses skipInlineCode (links.go)
//   - `\#` escapes — the backslash consumes the next byte
//   - indented (4-space / tab) code blocks — ADDED here (FindLinks does not skip)
//   - HTML blocks — ADDED here
//   - the ATX-heading leading `#` run — a HeaderMark, not inline content
//
// CONSERVATIVE / skip-when-uncertain (CONTEXT D-03 discipline): when a region
// cannot be confidently classified the `#` is left UNCHANGED. A missed rewrite
// leaves a stale tag that self-heals on the next correct op; an over-eager
// rewrite corrupts a code region. The bias is hard toward under-rewriting.

// Hashtag is one recognized inline `#tag` occurrence in a body. Start is the
// byte offset of the leading `#`; End is the exclusive byte offset just past the
// tag body (after trailing-`/` trim) — so [Start,End) is exactly the `#`+token
// span the Lezer Hashtag node covers. Name is the lower-cased tag name WITHOUT
// the leading `#` (matching inlineTagsFromBody's normalization).
type Hashtag struct {
	Start int
	End   int
	Name  string
}

// htmlBlockTags is the CommonMark type-6 block-tag set: a line beginning with
// `<tag`/`</tag` (tag in this set, followed by whitespace, `>`, `/>`, or EOL)
// starts an HTML block that runs to the next blank line. Conservative subset —
// enough to exclude the corpus HTML-block fixtures without misclassifying prose.
var htmlBlockTags = map[string]bool{
	"address": true, "article": true, "aside": true, "base": true,
	"basefont": true, "blockquote": true, "body": true, "caption": true,
	"center": true, "col": true, "colgroup": true, "dd": true, "details": true,
	"dialog": true, "dir": true, "div": true, "dl": true, "dt": true,
	"fieldset": true, "figcaption": true, "figure": true, "footer": true,
	"form": true, "frame": true, "frameset": true, "h1": true, "h2": true,
	"h3": true, "h4": true, "h5": true, "h6": true, "head": true,
	"header": true, "hr": true, "html": true, "iframe": true, "legend": true,
	"li": true, "link": true, "main": true, "menu": true, "menuitem": true,
	"nav": true, "noframes": true, "ol": true, "optgroup": true, "option": true,
	"p": true, "param": true, "section": true, "summary": true, "table": true,
	"tbody": true, "td": true, "tfoot": true, "th": true, "thead": true,
	"title": true, "tr": true, "track": true, "ul": true,
}

// htmlRawTags is the CommonMark type-1 raw-text set: an HTML block that runs to
// a line matching its close tag (or, more simply, the next blank line — used as
// a conservative terminator here alongside the close-tag check).
var htmlRawTags = map[string]bool{
	"pre": true, "script": true, "style": true, "textarea": true,
}

// FindHashtags returns every recognized inline `#tag` in body, in source order,
// with byte offsets. It is the SINGLE Hashtag recognizer — RewriteHashtags and
// the pages-service enumeration both consume it, so the recognition can never
// fork. It is a two-pass structural scan:
//
//	pass 1 — classify block structure: mark the byte ranges the Lezer parse
//	         excludes (fenced code, indented code, HTML blocks) and the content-
//	         start boundary of each line (the offset after any blockquote `>`
//	         markers), so a `#` abutting a block marker (`>#tag`) is a valid
//	         start boundary exactly as the Lezer `cx.offset` rule makes it.
//	pass 2 — a FindLinks-style byte walk that skips the excluded ranges, inline
//	         code, and `\#` escapes, and at each candidate `#` applies the start-
//	         boundary rule + scanBody (the exact hashtag.ts predicates).
func FindHashtags(body []byte) []Hashtag {
	excluded, contentStart := classifyBlocks(body)
	var out []Hashtag
	i := 0
	n := len(body)
	exIdx := 0
	for i < n {
		// Skip any excluded block range that starts at or before i.
		if exIdx < len(excluded) && i >= excluded[exIdx].start {
			if i < excluded[exIdx].end {
				i = excluded[exIdx].end
				exIdx++
				continue
			}
			exIdx++
			continue
		}
		c := body[i]
		switch c {
		case '`':
			i = skipInlineCode(body, i)
			continue
		case '\\':
			// Escaped char: skip the backslash and the next byte so `\#` never
			// begins a tag.
			i += 2
			continue
		case '#':
			if hashStartOK(body, i, contentStart) {
				if end, ok := scanTagBody(body, i+1, n); ok {
					out = append(out, Hashtag{
						Start: i,
						End:   end,
						Name:  strings.ToLower(string(body[i+1 : end])),
					})
					i = end
					continue
				}
			}
		}
		i++
	}
	return out
}

// blockRange is an excluded byte range [start,end) (fenced/indented code, HTML).
type blockRange struct {
	start int
	end   int
}

// classifyBlocks does a line-by-line block classification pass, returning the
// sorted excluded byte ranges and a set of line content-start offsets (the byte
// where inline content begins after any blockquote `>` markers on that line).
func classifyBlocks(body []byte) (excluded []blockRange, contentStart map[int]bool) {
	contentStart = map[int]bool{}
	n := len(body)
	i := 0
	inParagraph := false // a paragraph "interrupts" indented-code recognition
	for i < n {
		lineStart := i
		lineEnd := i
		for lineEnd < n && body[lineEnd] != '\n' {
			lineEnd++
		}
		// lineEnd points at '\n' or n; nextLine is the start of the following line.
		nextLine := lineEnd
		if nextLine < n {
			nextLine++
		}
		line := body[lineStart:lineEnd]

		// Blank line: ends a paragraph, opens indented-code recognition.
		if isBlankLine(line) {
			inParagraph = false
			i = nextLine
			continue
		}

		// Fenced code block: a line beginning with ``` or ~~~ (≤3 leading spaces).
		if fence, ok := fenceAt(body, lineStart); ok {
			end := skipFencedBlock(body, lineStart, fence)
			excluded = append(excluded, blockRange{start: lineStart, end: end})
			inParagraph = false
			i = end
			continue
		}

		// Indented code block: ≥4 leading spaces or a leading tab, but ONLY when
		// not continuing a paragraph (a lazy continuation line is prose, not code).
		if !inParagraph && indentedCodeLine(line) {
			// Consume the run of indented / blank lines as one code block.
			end := nextLine
			for end < n {
				ls := end
				le := end
				for le < n && body[le] != '\n' {
					le++
				}
				nl := le
				if nl < n {
					nl++
				}
				cur := body[ls:le]
				if isBlankLine(cur) || indentedCodeLine(cur) {
					end = nl
					continue
				}
				break
			}
			excluded = append(excluded, blockRange{start: lineStart, end: end})
			i = end
			continue
		}

		// HTML block (type 6 / type 1): a line beginning with a recognized block
		// tag, when not continuing a paragraph. Runs to the next blank line.
		if !inParagraph {
			if hEnd, ok := htmlBlockAt(body, lineStart); ok {
				excluded = append(excluded, blockRange{start: lineStart, end: hEnd})
				i = hEnd
				continue
			}
		}

		// A content line. Record the content-start boundary after any blockquote
		// `>` markers so a `#` directly after `>` is a valid start (matches the
		// Lezer cx.offset boundary for blockquote content). ATX-heading marker
		// runs need no special boundary — the required trailing space already
		// bounds any in-heading `#tag`, and the marker `#`s fail scanBody.
		cs := blockquoteContentStart(body, lineStart, lineEnd)
		contentStart[cs] = true
		// An ATX heading is a single-line block: it does NOT open a paragraph, so a
		// following indented line is still code, not a lazy continuation. Its inline
		// content is parsed (an in-heading `#tag` badges), but the leading marker
		// run is bounded by its required space and its own `#`s fail scanBody.
		inParagraph = !isATXHeadingLine(body, cs, lineEnd)
		i = nextLine
	}
	return excluded, contentStart
}

// hashStartOK reports whether body[i]=='#' is a valid tag START boundary: it is
// at offset 0, OR the preceding byte is whitespace, OR it sits exactly at a
// recorded line content-start (after a blockquote marker) — the byte-level
// analog of the hashtag.ts start-boundary rule (start-of-section OR whitespace).
func hashStartOK(body []byte, i int, contentStart map[int]bool) bool {
	if i == 0 {
		return true
	}
	if contentStart[i] {
		return true
	}
	return isWsByte(body[i-1])
}

// scanTagBody scans the tag body starting at start (the byte after `#`) up to
// limit, returning the exclusive end (after trailing-`/` trim) and ok. It ports
// hashtag.ts scanBody EXACTLY: body alphabet [A-Za-z0-9_/-], ≥1 non-digit
// [A-Za-z_-] (a `/` is a separator, not a qualifying non-digit), trailing-`/`
// trimmed; a bare `#` or an all-digit/all-separator run is rejected.
func scanTagBody(body []byte, start, limit int) (int, bool) {
	end := start
	hasNonDigit := false
	for end < limit {
		c := body[end]
		if !isBodyByte(c) {
			break
		}
		if isNonDigitByte(c) {
			hasNonDigit = true
		}
		end++
	}
	for end > start && body[end-1] == '/' {
		end--
	}
	if end <= start || !hasNonDigit {
		return 0, false
	}
	return end, true
}

// isBodyByte is hashtag.ts isBody: [A-Za-z0-9_/-].
func isBodyByte(c byte) bool {
	return (c >= '0' && c <= '9') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= 'a' && c <= 'z') ||
		c == '_' || c == '-' || c == '/'
}

// isNonDigitByte is hashtag.ts isNonDigit: [A-Za-z_-] (slash excluded).
func isNonDigitByte(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_' || c == '-'
}

// isWsByte is hashtag.ts isWs: space, tab, LF, CR, form feed.
func isWsByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// isBlankLine reports whether a line (without its trailing '\n') is empty or
// whitespace-only.
func isBlankLine(line []byte) bool {
	for _, c := range line {
		if c != ' ' && c != '\t' && c != '\r' {
			return false
		}
	}
	return true
}

// indentedCodeLine reports whether a line begins an indented code block: ≥4
// leading spaces, or a leading tab.
func indentedCodeLine(line []byte) bool {
	if len(line) > 0 && line[0] == '\t' {
		return true
	}
	spaces := 0
	for spaces < len(line) && line[spaces] == ' ' {
		spaces++
	}
	return spaces >= 4
}

// isATXHeadingLine reports whether the content beginning at cs (after any
// blockquote marker strip) is an ATX heading: 1–6 leading `#` followed by a
// space/tab or end-of-line. Matches the CommonMark ATX rule the Lezer parser
// uses to emit a HeaderMark rather than inline content.
func isATXHeadingLine(body []byte, cs, lineEnd int) bool {
	i := cs
	hashes := 0
	for i < lineEnd && body[i] == '#' && hashes < 7 {
		i++
		hashes++
	}
	if hashes < 1 || hashes > 6 {
		return false
	}
	return i >= lineEnd || body[i] == ' ' || body[i] == '\t'
}

// blockquoteContentStart returns the byte offset where inline content begins on
// a line after stripping any leading blockquote `>` markers (each optionally
// followed by one space), matching the CommonMark blockquote-marker strip.
func blockquoteContentStart(body []byte, lineStart, lineEnd int) int {
	i := lineStart
	// Optional up-to-3-space indent before the first marker.
	sp := 0
	for i < lineEnd && body[i] == ' ' && sp < 3 {
		i++
		sp++
	}
	for i < lineEnd && body[i] == '>' {
		i++
		if i < lineEnd && body[i] == ' ' {
			i++
		}
		// Allow a nested marker after this one (>> or > >).
		sp = 0
		for i < lineEnd && body[i] == ' ' && sp < 3 {
			i++
			sp++
		}
	}
	return i
}

// htmlBlockAt reports whether the line at lineStart begins an HTML block, and if
// so returns the exclusive end offset of the block (up to and including the
// terminating blank line, or end of body). Recognizes CommonMark type-1
// (pre/script/style/textarea) and type-6 (the block-tag set) starts.
func htmlBlockAt(body []byte, lineStart int) (int, bool) {
	n := len(body)
	i := lineStart
	// ≤3 leading spaces allowed.
	sp := 0
	for i < n && body[i] == ' ' && sp < 3 {
		i++
		sp++
	}
	if i >= n || body[i] != '<' {
		return 0, false
	}
	j := i + 1
	if j < n && body[j] == '/' {
		j++
	}
	tagStart := j
	for j < n && ((body[j] >= 'a' && body[j] <= 'z') || (body[j] >= 'A' && body[j] <= 'Z') || (body[j] >= '0' && body[j] <= '9')) {
		j++
	}
	if j == tagStart {
		return 0, false
	}
	name := strings.ToLower(string(body[tagStart:j]))
	// The char after the tag name must be whitespace, `>`, `/`, or EOL for a
	// type-6 start (conservative — matches the corpus fixtures).
	var after byte = '\n'
	if j < n {
		after = body[j]
	}
	okStart := after == ' ' || after == '\t' || after == '\n' || after == '\r' || after == '>' || after == '/'
	if !okStart {
		return 0, false
	}
	if !htmlBlockTags[name] && !htmlRawTags[name] {
		return 0, false
	}
	// Both types: run to the next blank line (conservative terminator that covers
	// the corpus fixtures for type-1 and type-6 alike).
	pos := lineStart
	for pos < n {
		le := pos
		for le < n && body[le] != '\n' {
			le++
		}
		nl := le
		if nl < n {
			nl++
		}
		if pos != lineStart && isBlankLine(body[pos:le]) {
			return pos, true
		}
		pos = nl
	}
	return n, true
}

// RewriteHashtags rewrites body `#tag` occurrences whose (case-insensitive) name
// equals old: for a rename/merge it splices `#`+new; for a delete (new=="") it
// removes the `#`+token with the D-03a whitespace rule. It returns the edited
// body and whether anything changed; on no match the ORIGINAL slice is returned
// unchanged (byte-identical). Only matched `#tag` spans are touched — every other
// byte (prose, code regions, other tags) is preserved exactly.
//
// D-03a (delete whitespace rule): removing a token collapses exactly ONE adjacent
// space, and ONLY when the token was space-bounded on both sides (` #ops ` → ` `);
// otherwise whitespace is untouched and a newline is NEVER merged across.
// D-03b (merge adjacent-duplicate rule): a merge yielding `#b #b` leaves both
// tokens as-is (no dedupe — the frontmatter reconcile dedupes the set).
func RewriteHashtags(body []byte, old, new string) ([]byte, bool) {
	oldNorm := strings.ToLower(strings.TrimSpace(old))
	if oldNorm == "" {
		return body, false
	}
	tags := FindHashtags(body)
	// Collect matches in source order.
	type edit struct {
		start, end int // the span to replace/remove
		repl       []byte
	}
	var edits []edit
	del := new == ""
	newBytes := []byte("#" + new)
	for _, t := range tags {
		if t.Name != oldNorm {
			continue
		}
		if del {
			s, e := t.Start, t.End
			// D-03a: collapse exactly one space only when space-bounded BOTH sides.
			leftSpace := s > 0 && body[s-1] == ' '
			rightSpace := e < len(body) && body[e] == ' '
			if leftSpace && rightSpace {
				e++ // absorb the single trailing space, keep the leading one
			}
			edits = append(edits, edit{start: s, end: e, repl: nil})
		} else {
			edits = append(edits, edit{start: t.Start, end: t.End, repl: newBytes})
		}
	}
	if len(edits) == 0 {
		return body, false
	}
	var buf bytes.Buffer
	buf.Grow(len(body))
	prev := 0
	for _, e := range edits {
		buf.Write(body[prev:e.start])
		buf.Write(e.repl)
		prev = e.end
	}
	buf.Write(body[prev:])
	return buf.Bytes(), true
}
