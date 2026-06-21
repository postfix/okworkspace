package search

import (
	"context"
	"strings"
	"testing"
)

// TestHighlight_WeightOnlySafe: a result snippet for a body match contains the
// weight-only <strong> markup and NEVER a raw <mark> tag (Bleve's default
// highlighter) or a <script> tag (XSS). The surrounding page text is escaped, so
// an injected tag from page content is neutralized in the fragment.
func TestHighlight_WeightOnlySafe(t *testing.T) {
	h := newHarness(t)
	// Body carries the match term plus an injected <script> tag and a literal
	// <mark> to prove neither survives unescaped in the snippet.
	h.writePage(t, "hl.md", "Highlight Page", nil,
		"the highlightterm appears here <script>alert(1)</script> and a <mark>x</mark> literal")
	h.rebuild(t)

	results, err := h.idx.Query(context.Background(), "highlightterm")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no results for highlightterm")
	}
	var sawStrong bool
	for _, r := range results {
		if r.Path != "hl.md" {
			continue
		}
		if strings.Contains(r.Snippet, "<strong>") {
			sawStrong = true
		}
		if strings.Contains(r.Snippet, "<mark>") {
			t.Fatalf("snippet contains a raw <mark> tag (default highlighter leaked): %q", r.Snippet)
		}
		if strings.Contains(r.Snippet, "<script>") {
			t.Fatalf("snippet contains an unescaped <script> tag (XSS): %q", r.Snippet)
		}
	}
	if !sawStrong {
		t.Fatalf("no <strong> weight-only markup found in any hl.md snippet: %+v", results)
	}
}
