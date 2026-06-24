package okf_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/postfix/okworkspace/internal/okf"
	"gopkg.in/yaml.v3"
)

// bodyRegion returns the bytes AFTER the closing frontmatter fence — i.e. the
// opaque body that okf.SetTags must NEVER perturb. It locates the SECOND "---"
// line at a line boundary (the closing fence), so a literal "---" inside a
// fenced code block in the body is not mistaken for the fence.
func bodyRegion(t *testing.T, src []byte) []byte {
	t.Helper()
	// Opening fence is at byte 0.
	if !bytes.HasPrefix(src, []byte("---\n")) && !bytes.HasPrefix(src, []byte("---\r\n")) {
		t.Fatalf("fixture does not start with a frontmatter fence:\n%s", src)
	}
	// Skip the opening fence line, then find the next line that is exactly "---".
	rest := src
	// advance past first line
	if i := bytes.IndexByte(rest, '\n'); i >= 0 {
		rest = rest[i+1:]
	}
	offset := len(src) - len(rest)
	for {
		line := rest
		if nl := bytes.IndexByte(rest, '\n'); nl >= 0 {
			line = rest[:nl]
		}
		trimmed := bytes.TrimRight(line, "\r")
		if bytes.Equal(trimmed, []byte("---")) {
			// Body begins after this closing fence line (and its newline).
			nl := bytes.IndexByte(rest, '\n')
			if nl < 0 {
				return nil
			}
			_ = offset
			return rest[nl+1:]
		}
		nl := bytes.IndexByte(rest, '\n')
		if nl < 0 {
			t.Fatalf("no closing fence found in fixture:\n%s", src)
		}
		rest = rest[nl+1:]
	}
}

// nonTagsKeyValues re-parses a full OKF document's frontmatter and returns the
// ordered list of top-level "key=value" strings, EXCLUDING the tags key. Used to
// assert that a SetTags edit preserves every other frontmatter key in content
// AND order (a structural comparison, not a substring grep).
func nonTagsKeyValues(t *testing.T, src []byte) []string {
	t.Helper()
	doc, err := okf.Parse(src)
	if err != nil {
		t.Fatalf("Parse for key extraction: %v", err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(doc.RawFront, &node); err != nil {
		t.Fatalf("unmarshal frontmatter: %v", err)
	}
	m := &node
	if m.Kind == yaml.DocumentNode {
		if len(m.Content) == 0 {
			return nil
		}
		m = m.Content[0]
	}
	if m.Kind != yaml.MappingNode {
		return nil
	}
	var out []string
	for i := 0; i+1 < len(m.Content); i += 2 {
		key := m.Content[i].Value
		if key == okf.FieldTags {
			continue
		}
		val := m.Content[i+1]
		out = append(out, key+"="+val.Value)
	}
	return out
}

// emittedTagLines returns the rendered `tags` block lines from emitted output:
// the `tags:` key line plus the following `- <tag>` sequence item lines.
func emittedTagLines(t *testing.T, emitted []byte) []string {
	t.Helper()
	lines := strings.Split(string(emitted), "\n")
	var out []string
	inTags := false
	for _, ln := range lines {
		trimmed := strings.TrimRight(ln, "\r")
		if strings.HasPrefix(trimmed, "tags:") {
			inTags = true
			out = append(out, trimmed)
			continue
		}
		if inTags {
			st := strings.TrimSpace(trimmed)
			// A closing frontmatter fence ends the tags block (not a sequence item).
			if st == "---" {
				break
			}
			// Block-style sequence items are indented and start with "- ".
			if strings.HasPrefix(st, "- ") {
				out = append(out, trimmed)
				continue
			}
			// first non-sequence line ends the tags block
			break
		}
	}
	return out
}

// TestSetTags is the Phase-11 exit gate: across three fixtures (no tags key,
// existing block tags, other frontmatter), okf.SetTags must change ONLY the tags
// lines — the body bytes are byte-identical and every non-tags frontmatter key is
// preserved in content and order. The assertions are structural (re-parse and
// compare regions), not loose substring checks.
func TestSetTags(t *testing.T) {
	cases := []struct {
		fixture string
		newTags []string
	}{
		{"no-tags-key.md", []string{"alpha", "beta"}},
		{"existing-block-tags.md", []string{"replaced-one", "replaced-two", "replaced-three"}},
		{"other-frontmatter.md", []string{"x-tag", "y-tag"}},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("testdata", "settags", tc.fixture))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			// Capture original invariants BEFORE the edit.
			origBody := bodyRegion(t, src)
			origNonTags := nonTagsKeyValues(t, src)

			doc, err := okf.Parse(src)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			okf.SetTags(doc, tc.newTags)
			if !doc.FrontDirty {
				t.Fatal("SetTags did not set FrontDirty")
			}

			out, err := doc.Emit()
			if err != nil {
				t.Fatalf("Emit: %v", err)
			}

			// GATE 1: body bytes are byte-identical (only the tags lines changed).
			gotBody := bodyRegion(t, out)
			if !bytes.Equal(origBody, gotBody) {
				t.Fatalf("only the tags lines should change, but the BODY differs\n--- want body (%d) ---\n%q\n--- got body (%d) ---\n%q",
					len(origBody), origBody, len(gotBody), gotBody)
			}

			// GATE 2: every non-tags frontmatter key preserved in content AND order
			// (structural re-parse comparison, not a substring grep).
			gotNonTags := nonTagsKeyValues(t, out)
			if len(gotNonTags) != len(origNonTags) {
				t.Fatalf("non-tags frontmatter key count changed: want %v, got %v", origNonTags, gotNonTags)
			}
			for i := range origNonTags {
				if gotNonTags[i] != origNonTags[i] {
					t.Fatalf("only the tags lines should change, but non-tags frontmatter differs at %d: want %q, got %q\nwant all: %v\ngot all:  %v",
						i, origNonTags[i], gotNonTags[i], origNonTags, gotNonTags)
				}
			}

			// GATE 3: the new tags render as a block-style sequence in order.
			tagLines := emittedTagLines(t, out)
			if len(tagLines) != len(tc.newTags)+1 { // "tags:" + one line per tag
				t.Fatalf("expected block-style tags (tags: + %d items), got lines: %v", len(tc.newTags), tagLines)
			}
			if strings.TrimSpace(tagLines[0]) != "tags:" {
				t.Fatalf("tags key line = %q, want \"tags:\"", tagLines[0])
			}
			for i, want := range tc.newTags {
				item := strings.TrimSpace(tagLines[i+1])
				if item != "- "+want {
					t.Fatalf("tag item %d = %q, want %q (block style, in order)", i, item, "- "+want)
				}
			}

			// Re-parse the emitted output and confirm the tags value is a sequence
			// with exactly the new tags, in order.
			redoc, err := okf.Parse(out)
			if err != nil {
				t.Fatalf("re-parse emitted: %v", err)
			}
			var rn yaml.Node
			if err := yaml.Unmarshal(redoc.RawFront, &rn); err != nil {
				t.Fatalf("re-unmarshal frontmatter: %v", err)
			}
			m := &rn
			if m.Kind == yaml.DocumentNode {
				m = m.Content[0]
			}
			var seq *yaml.Node
			for i := 0; i+1 < len(m.Content); i += 2 {
				if m.Content[i].Value == okf.FieldTags {
					seq = m.Content[i+1]
				}
			}
			if seq == nil || seq.Kind != yaml.SequenceNode {
				t.Fatalf("emitted tags value is not a sequence node: %+v", seq)
			}
			if seq.Style == yaml.FlowStyle {
				t.Fatalf("emitted tags sequence must be block style, got flow style")
			}
			if len(seq.Content) != len(tc.newTags) {
				t.Fatalf("emitted tags count = %d, want %d", len(seq.Content), len(tc.newTags))
			}
			for i, want := range tc.newTags {
				if seq.Content[i].Value != want {
					t.Fatalf("emitted tag %d = %q, want %q (order preserved)", i, seq.Content[i].Value, want)
				}
			}
		})
	}
}

// TestSetTags_NoTagsKey_OnlyTagsLinesAdded asserts that after SetTags on a page
// with NO tags key, the ONLY new lines are the `tags:` key and its `- ` items —
// every pre-existing frontmatter line and the entire body are unchanged.
func TestSetTags_NoTagsKey_OnlyTagsLinesAdded(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "settags", "no-tags-key.md"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	origBody := bodyRegion(t, src)
	origNonTags := nonTagsKeyValues(t, src)

	doc, err := okf.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	okf.SetTags(doc, []string{"alpha", "beta"})
	out, err := doc.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}

	// Body unchanged.
	if !bytes.Equal(origBody, bodyRegion(t, out)) {
		t.Fatalf("body changed when adding a tags key to a page that had none")
	}
	// Every prior frontmatter key preserved, in order; the only addition is tags.
	gotNonTags := nonTagsKeyValues(t, out)
	if len(gotNonTags) != len(origNonTags) {
		t.Fatalf("non-tags frontmatter keys changed: want %v, got %v", origNonTags, gotNonTags)
	}
	for i := range origNonTags {
		if gotNonTags[i] != origNonTags[i] {
			t.Fatalf("non-tags key changed at %d: want %q, got %q", i, origNonTags[i], gotNonTags[i])
		}
	}
	// The tags block is now present.
	tagLines := emittedTagLines(t, out)
	if len(tagLines) != 3 || strings.TrimSpace(tagLines[0]) != "tags:" {
		t.Fatalf("expected a fresh block-style tags: with 2 items, got %v", tagLines)
	}
}

// TestSetTags_NoOpRoundTripIsByteIdentical is the control: Parse→Emit with NO
// SetTags call returns bytes byte-identical to each fixture (the FrontDirty=false
// verbatim path), proving the test harness itself does not perturb bytes.
func TestSetTags_NoOpRoundTripIsByteIdentical(t *testing.T) {
	for _, fixture := range []string{"no-tags-key.md", "existing-block-tags.md", "other-frontmatter.md"} {
		t.Run(fixture, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("testdata", "settags", fixture))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			doc, err := okf.Parse(src)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			out, err := doc.Emit()
			if err != nil {
				t.Fatalf("Emit: %v", err)
			}
			if !bytes.Equal(src, out) {
				t.Fatalf("no-op round-trip not byte-identical for %s\nin:  %q\nout: %q", fixture, src, out)
			}
		})
	}
}
