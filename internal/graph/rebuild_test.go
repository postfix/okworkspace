package graph

import (
	"context"
	"reflect"
	"testing"
)

// seedGraphPages writes a small known graph of pages with links + tags to the
// harness repo. Edges (resolved): a->b, a->c, b->c, c->a (cycle), d isolated.
// Tags: a{ops,infra}, b{ops}, c{}, d{infra}.
func seedGraphPages(t *testing.T, h *testHarness) {
	t.Helper()
	h.writePage(t, "a.md", "A", []string{"ops", "infra"}, "see [b](b.md) and [c](c.md) and [ext](https://x.io)")
	h.writePage(t, "b.md", "B", []string{"ops"}, "ref [c](c.md) and [dangling](nope.md)")
	h.writePage(t, "c.md", "C", nil, "back to [a](a.md)")
	h.writePage(t, "d.md", "D", []string{"infra"}, "isolated, no links")
}

// TestDerivedOnly_RebuildReproducesAdjacency is the HARD success criterion: after
// a rebuild, deleting both tables and rebuilding again from the on-disk files
// reproduces byte-identical page_links + page_tags rows. SQLite is never truth.
func TestDerivedOnly_RebuildReproducesAdjacency(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	seedGraphPages(t, h)

	if err := h.st.RebuildGraph(ctx); err != nil {
		t.Fatalf("RebuildGraph (first): %v", err)
	}
	linksBefore := h.snapshotLinks(t)
	tagsBefore := h.snapshotTags(t)

	// Sanity: the expected adjacency the seed implies.
	wantLinks := []string{"a.md|b.md", "a.md|c.md", "b.md|c.md", "c.md|a.md"}
	if !reflect.DeepEqual(linksBefore, wantLinks) {
		t.Fatalf("links = %v, want %v", linksBefore, wantLinks)
	}
	wantTags := []string{"a.md|infra", "a.md|ops", "b.md|ops", "d.md|infra"}
	if !reflect.DeepEqual(tagsBefore, wantTags) {
		t.Fatalf("tags = %v, want %v", tagsBefore, wantTags)
	}

	// Nuke both tables and rebuild from files — must reproduce identical rows.
	if _, err := h.db.DB().Exec(`DELETE FROM page_links`); err != nil {
		t.Fatalf("delete page_links: %v", err)
	}
	if _, err := h.db.DB().Exec(`DELETE FROM page_tags`); err != nil {
		t.Fatalf("delete page_tags: %v", err)
	}
	if err := h.st.RebuildGraph(ctx); err != nil {
		t.Fatalf("RebuildGraph (second): %v", err)
	}
	linksAfter := h.snapshotLinks(t)
	tagsAfter := h.snapshotTags(t)

	if !reflect.DeepEqual(linksAfter, linksBefore) {
		t.Fatalf("page_links not reproduced: after=%v before=%v", linksAfter, linksBefore)
	}
	if !reflect.DeepEqual(tagsAfter, tagsBefore) {
		t.Fatalf("page_tags not reproduced: after=%v before=%v", tagsAfter, tagsBefore)
	}
}

// TestIncrementalEqualsRebuild applies a sequence of per-page upsert/delete jobs,
// then asserts the incremental table state equals a from-scratch RebuildGraph over
// the same files.
func TestIncrementalEqualsRebuild(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	seedGraphPages(t, h)

	// Incremental: upsert each page individually (what mutations will enqueue).
	for _, p := range []string{"a.md", "b.md", "c.md", "d.md"} {
		if err := h.st.upsertPage(ctx, p); err != nil {
			t.Fatalf("upsertPage %q: %v", p, err)
		}
	}
	incLinks := h.snapshotLinks(t)
	incTags := h.snapshotTags(t)

	// From-scratch rebuild over the same files.
	if err := h.st.RebuildGraph(ctx); err != nil {
		t.Fatalf("RebuildGraph: %v", err)
	}
	rbLinks := h.snapshotLinks(t)
	rbTags := h.snapshotTags(t)

	if !reflect.DeepEqual(incLinks, rbLinks) {
		t.Fatalf("incremental links %v != rebuild links %v", incLinks, rbLinks)
	}
	if !reflect.DeepEqual(incTags, rbTags) {
		t.Fatalf("incremental tags %v != rebuild tags %v", incTags, rbTags)
	}
}

// TestUpsertReplacesOnlyPageRows asserts upsert(A) atomically replaces ONLY A's
// link + tag rows, leaving other pages' rows untouched.
func TestUpsertReplacesOnlyPageRows(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	seedGraphPages(t, h)
	if err := h.st.RebuildGraph(ctx); err != nil {
		t.Fatalf("RebuildGraph: %v", err)
	}

	// Rewrite A's body to link only B (drop C) and tag only ops (drop infra).
	h.writePage(t, "a.md", "A", []string{"ops"}, "now only [b](b.md)")
	if err := h.st.upsertPage(ctx, "a.md"); err != nil {
		t.Fatalf("upsertPage a.md: %v", err)
	}

	links := h.snapshotLinks(t)
	wantLinks := []string{"a.md|b.md", "b.md|c.md", "c.md|a.md"}
	if !reflect.DeepEqual(links, wantLinks) {
		t.Fatalf("links after upsert = %v, want %v", links, wantLinks)
	}
	tags := h.snapshotTags(t)
	wantTags := []string{"a.md|ops", "b.md|ops", "d.md|infra"}
	if !reflect.DeepEqual(tags, wantTags) {
		t.Fatalf("tags after upsert = %v, want %v", tags, wantTags)
	}
}

// TestDeletePageRemovesInboundAndOutbound asserts delete(A) removes A's outbound
// edges (src_path=A), inbound edges (dst_path=A), and A's tag rows.
func TestDeletePageRemovesInboundAndOutbound(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	seedGraphPages(t, h)
	if err := h.st.RebuildGraph(ctx); err != nil {
		t.Fatalf("RebuildGraph: %v", err)
	}

	if err := h.st.deletePage(ctx, "a.md"); err != nil {
		t.Fatalf("deletePage a.md: %v", err)
	}

	links := h.snapshotLinks(t)
	// a->b, a->c removed (outbound); c->a removed (inbound). b->c remains.
	wantLinks := []string{"b.md|c.md"}
	if !reflect.DeepEqual(links, wantLinks) {
		t.Fatalf("links after delete = %v, want %v", links, wantLinks)
	}
	tags := h.snapshotTags(t)
	wantTags := []string{"b.md|ops", "d.md|infra"}
	if !reflect.DeepEqual(tags, wantTags) {
		t.Fatalf("tags after delete = %v, want %v", tags, wantTags)
	}
}

// TestUpsertMissingFileIsDelete asserts a stale upsert for a removed page acts as
// a no-op delete (its rows are cleared, not an error).
func TestUpsertMissingFileIsDelete(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	seedGraphPages(t, h)
	if err := h.st.RebuildGraph(ctx); err != nil {
		t.Fatalf("RebuildGraph: %v", err)
	}

	// Remove d.md from disk, then upsert it: should clear d's rows.
	if err := h.repo.Remove("d.md"); err != nil {
		t.Fatalf("remove d.md: %v", err)
	}
	if err := h.st.upsertPage(ctx, "d.md"); err != nil {
		t.Fatalf("upsertPage missing d.md: %v", err)
	}
	tags := h.snapshotTags(t)
	for _, row := range tags {
		if row == "d.md|infra" {
			t.Fatalf("expected d.md tag rows cleared, got %v", tags)
		}
	}
}

// TestGraphHandler_RecoversPanic asserts the handler converts a panic into a
// returned error (so the single drain goroutine survives a corrupt scenario).
func TestGraphHandler_RecoversPanic(t *testing.T) {
	h := newHarness(t)
	handler := GraphHandler(h.st, h.repo)
	// Malformed JSON payload triggers an unmarshal error path (a returned error,
	// not a panic). An unknown op also returns an error.
	if err := handler(context.Background(), "not json"); err == nil {
		t.Fatal("expected error for malformed payload")
	}
	if err := handler(context.Background(), `{"op":"bogus"}`); err == nil {
		t.Fatal("expected error for unknown op")
	}
}

// TestGraphHandler_Dispatch exercises the upsert/delete/rebuild op dispatch via
// the JSON payload builders.
func TestGraphHandler_Dispatch(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	seedGraphPages(t, h)
	handler := GraphHandler(h.st, h.repo)

	if err := handler(ctx, RebuildPayload()); err != nil {
		t.Fatalf("rebuild payload: %v", err)
	}
	if got := h.snapshotLinks(t); len(got) != 4 {
		t.Fatalf("after rebuild payload, links = %v", got)
	}
	if err := handler(ctx, DeletePagePayload("a.md")); err != nil {
		t.Fatalf("delete payload: %v", err)
	}
	if got := h.snapshotLinks(t); !reflect.DeepEqual(got, []string{"b.md|c.md"}) {
		t.Fatalf("after delete payload, links = %v", got)
	}
	if err := handler(ctx, UpsertPagePayload("a.md")); err != nil {
		t.Fatalf("upsert payload: %v", err)
	}
	if got := h.snapshotLinks(t); len(got) != 4 {
		t.Fatalf("after re-upsert payload, links = %v", got)
	}
}
