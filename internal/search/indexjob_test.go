package search

import (
	"context"
	"encoding/json"
	"testing"
)

// TestIndex_DeleteRemovesPage: an upsert op indexes a page; a delete op for the
// same path removes it so it is no longer found.
func TestIndex_DeleteRemovesPage(t *testing.T) {
	h := newHarness(t)
	h.writePage(t, "gone.md", "Disappearing Term", nil, "disappearterm body")

	handler := IndexHandler(h.idx, h.repo)
	ctx := context.Background()

	upsert, _ := json.Marshal(indexPayload{Op: "upsert", Kind: TypePage, Path: "gone.md"})
	if err := handler(ctx, string(upsert)); err != nil {
		t.Fatalf("upsert handler: %v", err)
	}
	results, err := h.idx.Query(ctx, "disappearterm")
	if err != nil {
		t.Fatalf("Query after upsert: %v", err)
	}
	if !containsPath(results, "gone.md") {
		t.Fatalf("page not found after upsert: %+v", results)
	}

	del, _ := json.Marshal(indexPayload{Op: "delete", Kind: TypePage, Path: "gone.md", ID: "gone.md"})
	if err := handler(ctx, string(del)); err != nil {
		t.Fatalf("delete handler: %v", err)
	}
	results, err = h.idx.Query(ctx, "disappearterm")
	if err != nil {
		t.Fatalf("Query after delete: %v", err)
	}
	if containsPath(results, "gone.md") {
		t.Fatalf("page still found after delete: %+v", results)
	}
}

// TestIndex_RebuildOp: the rebuild op rebuilds the whole index from files.
func TestIndex_RebuildOp(t *testing.T) {
	h := newHarness(t)
	h.writePage(t, "r.md", "Rebuildable Page", nil, "rebuildterm body")

	handler := IndexHandler(h.idx, h.repo)
	if err := handler(context.Background(), RebuildPayload()); err != nil {
		t.Fatalf("rebuild handler: %v", err)
	}
	results, err := h.idx.Query(context.Background(), "rebuildterm")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !containsPath(results, "r.md") {
		t.Fatalf("rebuild op did not index the page: %+v", results)
	}
}
