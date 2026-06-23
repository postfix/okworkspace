package search

import (
	"context"

	"github.com/blevesearch/bleve/v2"
)

// typeFacetCount returns the facet count for a given type value for a query. It is
// a TEST-ONLY helper (IN-02): kept in a _test.go file so it never ships in the
// binary, used by the facet tests to assert the type facet is present and
// populated (SRCH-06).
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
