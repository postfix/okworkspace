package search

import (
	"context"
	"testing"
)

// containsPath reports whether any result navigates to the given page path.
func containsPath(results []Result, path string) bool {
	for _, r := range results {
		if r.Path == path {
			return true
		}
	}
	return false
}

// TestQuery_TitleBoost (SRCH-01): a page whose query term is ONLY in its title
// ranks above a page whose term is ONLY in its body (title boost).
func TestQuery_TitleBoost(t *testing.T) {
	h := newHarness(t)
	// "alpha" appears only in the title of titlepage and only in the body of
	// bodypage.
	h.writePage(t, "titlepage.md", "Alpha Guide", nil, "general overview content here")
	h.writePage(t, "bodypage.md", "Random Notes", nil, "this mentions alpha somewhere in the body")
	h.rebuild(t)

	results, err := h.idx.Query(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results for 'alpha', got %d: %+v", len(results), results)
	}
	if results[0].Path != "titlepage.md" {
		t.Fatalf("title-match page should rank first, got order: %s then %s", results[0].Path, results[1].Path)
	}
}

// TestQuery_Body (SRCH-02): a term present only in the page body is found.
func TestQuery_Body(t *testing.T) {
	h := newHarness(t)
	h.writePage(t, "doc.md", "Plain Title", nil, "the secret keyword is xyzzy in the prose")
	h.rebuild(t)

	results, err := h.idx.Query(context.Background(), "xyzzy")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !containsPath(results, "doc.md") {
		t.Fatalf("body term 'xyzzy' not found; results=%+v", results)
	}
}

// TestQuery_Tag (SRCH-03): a page carrying tag "spec" is returned for a tag
// query "spec" (tags read sequence-aware from frontmatter — Pitfall 7).
func TestQuery_Tag(t *testing.T) {
	h := newHarness(t)
	h.writePage(t, "tagged.md", "Untagged Words Here", []string{"spec", "reference"}, "body without the query term")
	h.rebuild(t)

	results, err := h.idx.Query(context.Background(), "spec")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !containsPath(results, "tagged.md") {
		t.Fatalf("tag 'spec' not found; results=%+v", results)
	}
}

// TestQuery_TypedResultsAndFacet (SRCH-06, page type): every page hit carries
// type "page", and the response (via the Index Query path) is built from a
// search that includes a type facet with a page count.
func TestQuery_TypedResultsAndFacet(t *testing.T) {
	h := newHarness(t)
	h.writePage(t, "a.md", "Faceted Apple", nil, "apple body")
	h.writePage(t, "b.md", "Faceted Banana", nil, "banana body")
	h.rebuild(t)

	results, err := h.idx.Query(context.Background(), "faceted")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected >=2 page results, got %d", len(results))
	}
	for _, r := range results {
		if r.Kind != TypePage {
			t.Fatalf("result kind = %q, want %q: %+v", r.Kind, TypePage, r)
		}
	}

	// Facet presence is asserted at the raw-search level via the package-internal
	// helper used by Query, exercised through QueryWithFacet.
	count, err := h.idx.typeFacetCount(context.Background(), "faceted", TypePage)
	if err != nil {
		t.Fatalf("typeFacetCount: %v", err)
	}
	if count < 2 {
		t.Fatalf("type facet page count = %d, want >=2", count)
	}
}

// TestQuery_EmptyFastPath: an empty/whitespace query returns an empty slice with
// no error (no Bleve call).
func TestQuery_EmptyFastPath(t *testing.T) {
	h := newHarness(t)
	results, err := h.idx.Query(context.Background(), "   ")
	if err != nil {
		t.Fatalf("Query empty: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("empty query returned %d results, want 0", len(results))
	}
}
