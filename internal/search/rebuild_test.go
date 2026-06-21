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

// TestIndex_DeleteRemovesHeadings: re-indexing a page after a heading is renamed
// leaves no stale heading doc, and deleting a page removes its heading docs and
// page_headings rows. Exercises the REAL SQLite page_headings table (migration
// 0007 applied by the harness), not a stub.
func TestIndex_DeleteRemovesHeadings(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	// Initial body with a heading whose anchor encodes "old name".
	h.writePage(t, "doc.md", "Doc", nil, "intro\n\n## Old Name\n\nbody\n")
	if err := h.idx.indexPage(ctx, "doc.md"); err != nil {
		t.Fatalf("indexPage(initial): %v", err)
	}

	res, err := h.idx.Query(ctx, "old")
	if err != nil {
		t.Fatalf("Query(old): %v", err)
	}
	if _, ok := resultByKind(res, TypeHeading); !ok {
		t.Fatalf("expected a heading result before rename; got %+v", res)
	}

	// Rename the heading and re-index. The stale "#old-name" heading doc must go.
	h.writePage(t, "doc.md", "Doc", nil, "intro\n\n## New Name\n\nbody\n")
	if err := h.idx.indexPage(ctx, "doc.md"); err != nil {
		t.Fatalf("indexPage(renamed): %v", err)
	}

	res, err = h.idx.Query(ctx, "old")
	if err != nil {
		t.Fatalf("Query(old after rename): %v", err)
	}
	if r, ok := resultByKind(res, TypeHeading); ok {
		t.Fatalf("stale heading doc survived rename: %+v", r)
	}
	res, err = h.idx.Query(ctx, "new")
	if err != nil {
		t.Fatalf("Query(new): %v", err)
	}
	if _, ok := resultByKind(res, TypeHeading); !ok {
		t.Fatalf("renamed heading doc missing after re-index; got %+v", res)
	}

	// Deleting the page must remove its heading docs and clear page_headings.
	if err := h.idx.deletePage(ctx, "doc.md"); err != nil {
		t.Fatalf("deletePage: %v", err)
	}
	res, err = h.idx.Query(ctx, "new")
	if err != nil {
		t.Fatalf("Query(new after delete): %v", err)
	}
	if r, ok := resultByKind(res, TypeHeading); ok {
		t.Fatalf("heading doc survived page delete: %+v", r)
	}
	var rows int
	if err := h.idx.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM page_headings WHERE page_path=?`, "doc.md").Scan(&rows); err != nil {
		t.Fatalf("count page_headings: %v", err)
	}
	if rows != 0 {
		t.Fatalf("page_headings rows for deleted page = %d, want 0", rows)
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

// TestRebuild_PersistsHead is the CR-01 regression: RebuildIndex itself (via the
// attached gitstore handle, SetGit) must persist last_indexed_head, so a
// subsequent startup with an UNCHANGED HEAD reports DriftCheck == false. Before
// the fix StoreHead was never called from the rebuild path, last_indexed_head
// stayed empty, and every startup mis-fired a full rebuild.
func TestRebuild_PersistsHead(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.writePage(t, "p.md", "Head Page", nil, "body")
	h.commit(t, "add p", "p.md")

	// The harness wires SetGit, so RebuildIndex alone must record the HEAD —
	// no manual StoreHead call here (that is exactly the regression).
	h.rebuild(t)

	stored, err := h.idx.readMeta(ctx, metaKeyLastIndexedHead)
	if err != nil {
		t.Fatalf("readMeta: %v", err)
	}
	if stored == "" {
		t.Fatal("last_indexed_head is empty after rebuild — RebuildIndex did not persist HEAD (CR-01)")
	}
	head, err := h.gs.HeadSHA(ctx)
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}
	if stored != head {
		t.Fatalf("last_indexed_head %q != current HEAD %q after rebuild", stored, head)
	}

	// A second startup with an unchanged HEAD must see NO drift.
	drifted, err := h.idx.DriftCheck(ctx, h.gs)
	if err != nil {
		t.Fatalf("DriftCheck (unchanged HEAD): %v", err)
	}
	if drifted {
		t.Fatal("DriftCheck reported drift after a rebuild that persisted the current HEAD (CR-01)")
	}

	// After HEAD advances out-of-band, drift is detected again.
	h.writePage(t, "q.md", "Another", nil, "more")
	h.commit(t, "add q", "q.md")
	drifted, err = h.idx.DriftCheck(ctx, h.gs)
	if err != nil {
		t.Fatalf("DriftCheck (advanced HEAD): %v", err)
	}
	if !drifted {
		t.Fatal("DriftCheck did not detect drift after HEAD advanced out-of-band")
	}
}
