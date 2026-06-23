package okf

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

// Emit reassembles the document into bytes. When the frontmatter is unchanged
// (!FrontDirty), RawFront is re-attached verbatim — guaranteeing byte-identical
// output for the round-trip exit gate. When FrontDirty is set, only the
// frontmatter is re-marshaled (yaml.Marshal preserves key order and unknown
// fields via the yaml.Node), while the opaque Body is always passed through
// untouched. The original fence bytes and EOL style are preserved.
func (d *Doc) Emit() ([]byte, error) {
	if !d.HasFrontmatter {
		// No frontmatter region: the body is the whole file.
		return d.Body, nil
	}

	front := d.RawFront
	if d.FrontDirty {
		marshaled, err := yaml.Marshal(&d.Front)
		if err != nil {
			return nil, err
		}
		// yaml.Marshal always emits LF and a trailing newline. Convert to the
		// document's EOL style so a CRLF file stays CRLF after a repair.
		if d.EOLStyle == EOLCRLF {
			marshaled = lfToCRLF(marshaled)
		}
		front = marshaled
	}

	var buf bytes.Buffer
	buf.Grow(len(d.openFence) + len(front) + len(d.closeFence) + len(d.Body))
	buf.Write(d.openFence)
	buf.Write(front)
	buf.Write(d.closeFence)
	buf.Write(d.Body)
	return buf.Bytes(), nil
}

// lfToCRLF converts bare LF line endings to CRLF without doubling an existing
// "\r\n". Used only when re-marshaling frontmatter for a CRLF document.
func lfToCRLF(b []byte) []byte {
	// Normalize any pre-existing CRLF down to LF first, then promote uniformly,
	// so we never emit "\r\r\n".
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n"))
}
