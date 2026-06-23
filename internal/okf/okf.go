// Package okf is the byte-stable OKF document model: it splits an OKF Markdown
// file into an opaque body and a surgically-editable YAML frontmatter region,
// and re-emits it byte-identically when nothing changed. Round-trip rot — a
// parser/serializer silently reformatting a user's Markdown — is THE phase risk
// (RESEARCH Pitfall 1); the structural defense is that the body is NEVER routed
// through a Markdown AST and the frontmatter is re-marshaled ONLY when a field
// actually changed (FrontDirty). The golden-corpus round-trip test is the
// phase exit gate.
//
// Invariant: Parse(src) followed by Emit() with no edit returns bytes that are
// byte-identical to src for every supported file shape, including CRLF files
// and files whose body contains a literal "---" line inside a fenced code block.
package okf

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

// Required frontmatter fields (SPEC §10). Repair adds only the members of this
// set that are absent; it never reorders or rewrites existing keys.
const (
	FieldType        = "type"
	FieldTitle       = "title"
	FieldDescription = "description"
	FieldTags        = "tags"
	FieldTimestamp   = "timestamp"
)

// DefaultType is the frontmatter `type` value written for new/repaired pages
// when none is present (D-13).
const DefaultType = "Page"

// requiredFields is the canonical ordered set of required frontmatter keys.
// Order here determines the append order when Repair adds missing fields.
var requiredFields = []string{
	FieldType,
	FieldTitle,
	FieldDescription,
	FieldTags,
	FieldTimestamp,
}

// EOLStyle records the line-ending convention of the original file so Emit can
// preserve it verbatim (Assumption A4: per-file, never normalized).
type EOLStyle int

const (
	// EOLLF is Unix line endings ("\n"). Also the default for a body with no
	// detectable line ending.
	EOLLF EOLStyle = iota
	// EOLCRLF is Windows line endings ("\r\n").
	EOLCRLF
)

// Doc is a parsed OKF document. The body is opaque text held as raw bytes and
// is never re-serialized through any AST. The frontmatter region is kept both
// as its exact original bytes (RawFront) and as a yaml.Node (Front) for
// surgical inspection/edit. Emit re-attaches RawFront verbatim unless
// FrontDirty is set, in which case it re-marshals Front (preserving key order
// and unknown fields).
type Doc struct {
	// HasFrontmatter reports whether the source began with a frontmatter fence
	// at byte 0. When false, the whole file is Body.
	HasFrontmatter bool

	// RawFront is the exact original bytes of the YAML between the opening and
	// closing fence lines (excluding the fence lines themselves). Re-attached
	// verbatim by Emit when !FrontDirty.
	RawFront []byte

	// Front is RawFront parsed into a yaml.Node for inspection and surgical
	// edits. It is only re-marshaled into bytes when FrontDirty is set.
	Front yaml.Node

	// Body is the opaque remainder of the file after the closing fence (or the
	// whole file when HasFrontmatter is false). Never parsed as Markdown here.
	Body []byte

	// FrontDirty is set by Repair (or future surgical editors) when the
	// frontmatter was mutated and must be re-marshaled on Emit.
	FrontDirty bool

	// EOLStyle is the detected line-ending style of the source, preserved by
	// Emit when re-marshaling frontmatter.
	EOLStyle EOLStyle

	// fence is the exact opening fence line as it appeared in the source,
	// including its trailing EOL (e.g. "---\n" or "---\r\n"). The closing fence
	// uses the same EOL. Captured so Emit reproduces the original fence bytes.
	openFence  []byte
	closeFence []byte
}

// fenceMarker is the three-dash frontmatter delimiter.
var fenceMarker = []byte("---")

// Parse splits src into an opaque body and (optionally) a frontmatter region.
// A frontmatter fence is recognized ONLY at byte offset 0: a leading "---"
// followed by an end-of-line (LF or CRLF). A "---" anywhere else — including
// inside a fenced code block — is body text, never a fence. An opening fence
// with no matching closing fence is treated as a body-only file
// (HasFrontmatter=false), so malformed input is never silently restructured.
func Parse(src []byte) (*Doc, error) {
	d := &Doc{
		Body:     src,
		EOLStyle: detectEOL(src),
	}

	open, openLen, ok := leadingFence(src)
	if !ok {
		// No frontmatter: the entire file is opaque body. Nothing to parse.
		return d, nil
	}

	rest := src[openLen:]
	rawFront, closeFence, bodyStart, found := splitClosingFence(rest)
	if !found {
		// Unterminated opening fence: treat the whole file as body (A4 — never
		// restructure malformed input).
		return d, nil
	}

	d.HasFrontmatter = true
	d.openFence = open
	d.closeFence = closeFence
	d.RawFront = rawFront
	d.Body = rest[bodyStart:]

	// Parse the frontmatter region into a yaml.Node for surgical inspection.
	// An empty frontmatter region yields a zero Node, which is valid.
	if len(bytes.TrimSpace(rawFront)) > 0 {
		if err := yaml.Unmarshal(rawFront, &d.Front); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// detectEOL reports the dominant line-ending style of src. CRLF is reported
// only when a "\r\n" appears; otherwise LF (the default for single-line or
// newline-free content).
func detectEOL(src []byte) EOLStyle {
	if bytes.Contains(src, []byte("\r\n")) {
		return EOLCRLF
	}
	return EOLLF
}

// leadingFence reports whether src begins with a frontmatter fence at byte 0.
// It returns the exact fence line bytes (including the trailing EOL) and the
// number of bytes consumed. A fence is "---" immediately followed by "\n" or
// "\r\n" (and nothing else on the line).
func leadingFence(src []byte) (fence []byte, n int, ok bool) {
	if !bytes.HasPrefix(src, fenceMarker) {
		return nil, 0, false
	}
	after := src[len(fenceMarker):]
	switch {
	case bytes.HasPrefix(after, []byte("\r\n")):
		n = len(fenceMarker) + 2
		return src[:n], n, true
	case bytes.HasPrefix(after, []byte("\n")):
		n = len(fenceMarker) + 1
		return src[:n], n, true
	default:
		// "---" with trailing content (e.g. "----" or "--- x") is not a fence.
		return nil, 0, false
	}
}

// splitClosingFence scans rest (the bytes after the opening fence) for a
// closing "---" fence that begins a line. It returns the raw frontmatter bytes
// before the closing fence, the exact closing fence line bytes (including EOL,
// or without if the fence is the final line with no trailing newline), and the
// offset in rest at which the body begins. found is false when no closing fence
// exists.
func splitClosingFence(rest []byte) (rawFront, closeFence []byte, bodyStart int, found bool) {
	offset := 0
	for offset <= len(rest) {
		// A closing fence must start at a line boundary: offset 0 of rest, or
		// immediately after a newline.
		line := rest[offset:]
		if bytes.HasPrefix(line, fenceMarker) {
			afterMarker := line[len(fenceMarker):]
			switch {
			case bytes.HasPrefix(afterMarker, []byte("\r\n")):
				fenceLen := len(fenceMarker) + 2
				return rest[:offset], line[:fenceLen], offset + fenceLen, true
			case bytes.HasPrefix(afterMarker, []byte("\n")):
				fenceLen := len(fenceMarker) + 1
				return rest[:offset], line[:fenceLen], offset + fenceLen, true
			case len(afterMarker) == 0:
				// Closing fence is the final line with no trailing newline.
				return rest[:offset], line[:len(fenceMarker)], offset + len(fenceMarker), true
			}
		}
		// Advance to the start of the next line.
		nl := bytes.IndexByte(rest[offset:], '\n')
		if nl < 0 {
			return nil, nil, 0, false
		}
		offset += nl + 1
	}
	return nil, nil, 0, false
}
