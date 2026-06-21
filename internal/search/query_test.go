package search

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
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

// resultByKind returns the first result whose kind matches, or false.
func resultByKind(results []Result, kind string) (Result, bool) {
	for _, r := range results {
		if r.Kind == kind {
			return r, true
		}
	}
	return Result{}, false
}

// TestQuery_Filename (SRCH-04): a query matching an attachment's original
// filename returns a kind:"attachment" result.
func TestQuery_Filename(t *testing.T) {
	h := newHarness(t)
	h.writePage(t, "host.md", "Host Page", nil, "owning page body")
	h.writeAttachment(t, "att1", "quarterly-budget.pdf", "host.md", "irrelevant extracted prose")
	h.rebuild(t)

	results, err := h.idx.Query(context.Background(), "quarterly-budget")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	r, ok := resultByKind(results, TypeAttachment)
	if !ok {
		t.Fatalf("no attachment result for filename query; results=%+v", results)
	}
	if r.Title != "quarterly-budget.pdf" {
		t.Fatalf("attachment title = %q, want original filename", r.Title)
	}
	if r.Path != "host.md" {
		t.Fatalf("attachment path = %q, want owning page host.md", r.Path)
	}
}

// TestQuery_AttachmentOwningPage (SRCH-05): a word only in an attachment's
// extracted text returns a kind:"attachment" result whose path is the OWNING
// page (from AttachmentMeta.PagePath, no tree scan).
func TestQuery_AttachmentOwningPage(t *testing.T) {
	h := newHarness(t)
	h.writePage(t, "owner.md", "Owner Page", nil, "page body has none of it")
	h.writeAttachment(t, "att2", "report.pdf", "owner.md", "the hidden codeword is zorblax inside the document")
	h.rebuild(t)

	results, err := h.idx.Query(context.Background(), "zorblax")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	r, ok := resultByKind(results, TypeAttachment)
	if !ok {
		t.Fatalf("no attachment result for extracted-text query; results=%+v", results)
	}
	if r.Path != "owner.md" {
		t.Fatalf("attachment path = %q, want owning page owner.md", r.Path)
	}
	if r.PageTitle != "Owner Page" {
		t.Fatalf("attachment page_title = %q, want %q", r.PageTitle, "Owner Page")
	}
}

// TestQuery_HeadingDeepLink (SRCH-06 heading): a query matching a page heading
// returns a kind:"heading" result with the right anchor and page_title.
func TestQuery_HeadingDeepLink(t *testing.T) {
	h := newHarness(t)
	h.writePage(t, "guide.md", "Deploy Guide", nil, "intro\n\n## Rollback Procedure\n\nsteps here\n")
	h.rebuild(t)

	results, err := h.idx.Query(context.Background(), "rollback")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	r, ok := resultByKind(results, TypeHeading)
	if !ok {
		t.Fatalf("no heading result for 'rollback'; results=%+v", results)
	}
	if r.Path != "guide.md" {
		t.Fatalf("heading path = %q, want guide.md", r.Path)
	}
	if r.Anchor != "#rollback-procedure" {
		t.Fatalf("heading anchor = %q, want #rollback-procedure", r.Anchor)
	}
	if r.PageTitle != "Deploy Guide" {
		t.Fatalf("heading page_title = %q, want %q", r.PageTitle, "Deploy Guide")
	}
}

// TestQuery_TypedResultsAndFacet_AllKinds (SRCH-06): the type facet reports
// page/heading/attachment counts together.
func TestQuery_TypedResultsAndFacet_AllKinds(t *testing.T) {
	h := newHarness(t)
	h.writePage(t, "facet.md", "Facetword Page", nil, "facetword body\n\n## Facetword Section\n\ndetail\n")
	h.writeAttachment(t, "fa", "facetword-file.pdf", "facet.md", "facetword in the attachment text")
	h.rebuild(t)

	for _, typ := range []string{TypePage, TypeHeading, TypeAttachment} {
		count, err := h.idx.typeFacetCount(context.Background(), "facetword", typ)
		if err != nil {
			t.Fatalf("typeFacetCount(%s): %v", typ, err)
		}
		if count < 1 {
			t.Fatalf("type facet %s count = %d, want >=1", typ, count)
		}
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

// TestNormalizeQuery_RuneBoundary is the WR-05 regression: a query of multibyte
// runes longer than maxQueryLen must be truncated on a RUNE boundary, never
// mid-rune, so the result is always valid UTF-8 (a byte slice would split a
// 3-byte CJK rune at the 256-byte cap and produce invalid UTF-8).
func TestNormalizeQuery_RuneBoundary(t *testing.T) {
	// "世" is 3 bytes in UTF-8; maxQueryLen+10 of them is well over the byte cap.
	q := strings.Repeat("世", maxQueryLen+10)
	got, ok := normalizeQuery(q)
	if !ok {
		t.Fatal("normalizeQuery reported nothing to search for a non-empty query")
	}
	if !utf8.ValidString(got) {
		t.Fatalf("normalizeQuery produced invalid UTF-8 (split a rune): %q", got)
	}
	if n := utf8.RuneCountInString(got); n != maxQueryLen {
		t.Fatalf("normalizeQuery rune count = %d, want %d", n, maxQueryLen)
	}
	// Every rune must be the intact multibyte rune, none truncated.
	for _, r := range got {
		if r != '世' {
			t.Fatalf("unexpected rune %q in truncated query (truncation split a rune)", r)
		}
	}
}

// TestNormalizeQuery_ShortUnchanged: a sub-cap query is returned trimmed and
// unchanged.
func TestNormalizeQuery_ShortUnchanged(t *testing.T) {
	got, ok := normalizeQuery("  hello world  ")
	if !ok || got != "hello world" {
		t.Fatalf("normalizeQuery(short) = %q,%v; want \"hello world\",true", got, ok)
	}
}
