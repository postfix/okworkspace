package search

import (
	"context"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
)

// Result is the typed search-result DTO returned to the SPA (the interface
// contract consumed by 03-02). THIS plan emits only kind:"page"; heading/
// attachment kinds are added by 03-03 without changing the envelope.
type Result struct {
	Kind      string `json:"kind"`                 // "page" | "heading" | "attachment"
	Title     string `json:"title"`                // page/heading title or attachment filename
	Path      string `json:"path"`                 // workspace-relative page path to navigate to
	Snippet   string `json:"snippet"`              // weight-only-safe highlighted fragment (no <mark>)
	Anchor    string `json:"anchor,omitempty"`     // heading only
	PageTitle string `json:"page_title,omitempty"` // heading/attachment only
}

const (
	// maxQueryLen caps the untrusted query string length (V5 input validation /
	// DoS guard against pathological fuzzy/prefix expansion — T-03-05).
	maxQueryLen = 256
	// maxResults caps the result set size.
	maxResults = 50
	// facetName is the type-facet key (SRCH-06).
	facetName = "types"
	// facetSize bounds the number of facet terms returned (page/heading/attachment).
	facetSize = 3
	// fuzziness is the fixed edit distance for typo tolerance.
	fuzziness = 1
)

// buildQuery constructs the disjunction (OR) query over the typed fields: title
// (boosted, fuzzy) + body/extracted_text/filename (fuzzy) + tags (exact term) +
// title prefix (boosted, as-you-type) + body phrase. q is already trimmed and
// length-capped by the caller.
func buildQuery(q string) *bleve.SearchRequest {
	dis := bleve.NewDisjunctionQuery()

	mt := bleve.NewMatchQuery(q)
	mt.SetField("title")
	mt.SetBoost(3.0)
	mt.SetFuzziness(fuzziness)
	dis.AddQuery(mt)

	for _, f := range []string{"body", "extracted_text", "filename"} {
		m := bleve.NewMatchQuery(q)
		m.SetField(f)
		m.SetFuzziness(fuzziness)
		dis.AddQuery(m)
	}

	tag := bleve.NewTermQuery(q)
	tag.SetField("tags")
	dis.AddQuery(tag)

	pre := bleve.NewPrefixQuery(q)
	pre.SetField("title")
	pre.SetBoost(2.0)
	dis.AddQuery(pre)

	ph := bleve.NewMatchPhraseQuery(q)
	ph.SetField("body")
	dis.AddQuery(ph)

	req := bleve.NewSearchRequestOptions(dis, maxResults, 0, false)
	req.Fields = []string{"type", "title", "page_path", "page_title", "anchor", "filename"}
	req.Highlight = bleve.NewHighlightWithStyle(highlightStyle)
	req.Highlight.AddField("title")
	req.Highlight.AddField("body")
	req.Highlight.AddField("extracted_text")
	req.AddFacet(facetName, bleve.NewFacetRequest("type", facetSize))
	return req
}

// normalizeQuery trims and length-caps the untrusted query; the bool reports
// whether there is anything to search (empty/whitespace → no Bleve call).
func normalizeQuery(q string) (string, bool) {
	q = strings.TrimSpace(q)
	if q == "" {
		return "", false
	}
	if len(q) > maxQueryLen {
		q = q[:maxQueryLen]
	}
	return q, true
}

// Query runs a search and maps the hits to typed Result DTOs. An empty/whitespace
// query returns an empty (non-nil) slice with no Bleve call (the fast path). A hit
// whose page_path is under the trash prefix is dropped defensively even though the
// rebuild/upsert path already excludes trash (defense in depth — T-03-02).
func (s *Index) Query(ctx context.Context, q string) ([]Result, error) {
	q, ok := normalizeQuery(q)
	if !ok {
		return []Result{}, nil
	}

	var sr *bleve.SearchResult
	if err := s.withIndex(func(idx bleve.Index) error {
		req := buildQuery(q)
		res, err := idx.SearchInContext(ctx, req)
		if err != nil {
			return err
		}
		sr = res
		return nil
	}); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(sr.Hits))
	for _, hit := range sr.Hits {
		r := mapHit(hit)
		// Defense in depth (T-03-02/T-03-12): drop any hit (page, heading, or
		// attachment) whose owning page lives under the trash prefix.
		if isTrashed(r.Path) {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// mapHit converts a Bleve DocumentMatch into a typed Result.
func mapHit(hit *search.DocumentMatch) Result {
	r := Result{
		Kind:      fieldString(hit.Fields, "type"),
		Title:     fieldString(hit.Fields, "title"),
		Path:      fieldString(hit.Fields, "page_path"),
		Anchor:    fieldString(hit.Fields, "anchor"),
		PageTitle: fieldString(hit.Fields, "page_title"),
	}
	// An attachment carries its filename in `filename`, not `title`.
	if r.Title == "" {
		r.Title = fieldString(hit.Fields, "filename")
	}
	r.Snippet = bestFragment(hit)
	return r
}

// bestFragment picks the most useful highlighted fragment (body preferred, then
// title, then extracted_text). The fragments are already weight-only-safe (the
// registered highlighter escapes surrounding text and wraps matches in <strong>).
func bestFragment(hit *search.DocumentMatch) string {
	for _, f := range []string{"body", "title", "extracted_text"} {
		if frags := hit.Fragments[f]; len(frags) > 0 {
			return frags[0]
		}
	}
	return ""
}

func fieldString(fields map[string]interface{}, key string) string {
	if v, ok := fields[key].(string); ok {
		return v
	}
	return ""
}

// typeFacetCount returns the facet count for a given type value for a query. Used
// by tests to assert the type facet is present and populated (SRCH-06).
func (s *Index) typeFacetCount(ctx context.Context, q, typ string) (int, error) {
	q, ok := normalizeQuery(q)
	if !ok {
		return 0, nil
	}
	var count int
	err := s.withIndex(func(idx bleve.Index) error {
		req := buildQuery(q)
		res, err := idx.SearchInContext(ctx, req)
		if err != nil {
			return err
		}
		fr, ok := res.Facets[facetName]
		if !ok || fr.Terms == nil {
			return nil
		}
		for _, term := range fr.Terms.Terms() {
			if term.Term == typ {
				count = term.Count
			}
		}
		return nil
	})
	return count, err
}
