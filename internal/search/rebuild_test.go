package search

import (
	"context"
	"testing"
)

// TestRebuild_Idempotent: rebuilding twice yields the same doc count and the
// same hits for a query.
func TestRebuild_Idempotent(t *testing.T) {
	h := newHarness(t)
	h.writePage(t, "one.md", "First Page", []string{"x"}, "alpha body")
	h.writePage(t, "two.md", "Second Page", []string{"y"}, "beta body")
	h.rebuild(t)

	count1, err := h.idx.DocCount()
	if err != nil {
		t.Fatalf("DocCount(1): %v", err)
	}
	res1, err := h.idx.Query(context.Background(), "page")
	if err != nil {
		t.Fatalf("Query(1): %v", err)
	}

	h.rebuild(t)
	count2, err := h.idx.DocCount()
	if err != nil {
		t.Fatalf("DocCount(2): %v", err)
	}
	res2, err := h.idx.Query(context.Background(), "page")
	if err != nil {
		t.Fatalf("Query(2): %v", err)
	}

	if count1 != count2 {
		t.Fatalf("rebuild not idempotent: doc count %d then %d", count1, count2)
	}
	if len(res1) != len(res2) {
		t.Fatalf("rebuild not idempotent: hit count %d then %d", len(res1), len(res2))
	}
}

// TestRebuild_ExcludesTrash: a page under .okf-workspace/trash is NOT indexed.
func TestRebuild_ExcludesTrash(t *testing.T) {
	h := newHarness(t)
	h.writePage(t, "live.md", "Live Trashword Page", nil, "trashword content")
	h.writePage(t, ".okf-workspace/trash/20260101T000000-gone.md", "Trashed Trashword Page", nil, "trashword content too")
	h.rebuild(t)

	results, err := h.idx.Query(context.Background(), "trashword")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !containsPath(results, "live.md") {
		t.Fatalf("live page missing from results: %+v", results)
	}
	for _, r := range results {
		if r.Path == ".okf-workspace/trash/20260101T000000-gone.md" {
			t.Fatalf("trashed page leaked into results: %+v", r)
		}
	}
}

// TestDrift_HeadMismatchRebuilds: a stored last_indexed_head different from the
// current gitstore HEAD makes DriftCheck return true.
func TestDrift_HeadMismatchRebuilds(t *testing.T) {
	h := newHarness(t)
	h.writePage(t, "p.md", "Drift Page", nil, "body")
	h.commit(t, "add p", "p.md")

	// Rebuild stores the current HEAD as last_indexed_head, so DriftCheck is false.
	h.rebuild(t)
	if err := h.idx.StoreHead(context.Background(), h.gs); err != nil {
		t.Fatalf("StoreHead: %v", err)
	}
	drifted, err := h.idx.DriftCheck(context.Background(), h.gs)
	if err != nil {
		t.Fatalf("DriftCheck (in sync): %v", err)
	}
	if drifted {
		t.Fatal("DriftCheck returned true while index is in sync with HEAD")
	}

	// Advance HEAD out-of-band; now the stored head is stale → drift.
	h.writePage(t, "q.md", "Another Page", nil, "more body")
	h.commit(t, "add q", "q.md")
	drifted, err = h.idx.DriftCheck(context.Background(), h.gs)
	if err != nil {
		t.Fatalf("DriftCheck (after drift): %v", err)
	}
	if !drifted {
		t.Fatal("DriftCheck returned false after HEAD advanced out-of-band")
	}
}
