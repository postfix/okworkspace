package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// rebuild populates the cache tables from the on-disk pages via the REAL rebuild
// path (never hand-inserted rows), so the query tests exercise the actual schema.
func (h *testHarness) rebuild(t *testing.T) {
	t.Helper()
	if err := h.st.RebuildGraph(context.Background()); err != nil {
		t.Fatalf("RebuildGraph: %v", err)
	}
}

// nodeIDsByType returns the sorted node ids of the given type.
func nodeIDsByType(g GraphData, typ string) []string {
	var out []string
	for _, n := range g.Nodes {
		if n.Type == typ {
			out = append(out, n.ID)
		}
	}
	sort.Strings(out)
	return out
}

// edgeStrsByType returns sorted "src->dst" for edges of the given type.
func edgeStrsByType(g GraphData, typ string) []string {
	var out []string
	for _, e := range g.Edges {
		if e.Type == typ {
			out = append(out, e.Source+"->"+e.Target)
		}
	}
	sort.Strings(out)
	return out
}

func nodeLabel(g GraphData, id string) (string, bool) {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n.Label, true
		}
	}
	return "", false
}

// TestGraphData_BipartiteLeanShape: a 3-page workspace (a->b, b->c, all tagged
// design) yields page nodes a/b/c (label = title), a tag node tag:design, link
// edges a->b and b->c, and tag membership edges page->tag:design. No body field.
func TestGraphData_BipartiteLeanShape(t *testing.T) {
	h := newHarness(t)
	h.writePage(t, "a.md", "Alpha", []string{"design"}, "see [b](b.md)")
	h.writePage(t, "b.md", "Bravo", []string{"design"}, "see [c](c.md)")
	h.writePage(t, "c.md", "Charlie", []string{"design"}, "no links")
	h.rebuild(t)

	g, err := h.st.GraphData(context.Background())
	if err != nil {
		t.Fatalf("GraphData: %v", err)
	}

	if got, want := nodeIDsByType(g, "page"), []string{"a.md", "b.md", "c.md"}; !equalSlices(got, want) {
		t.Fatalf("page nodes = %v, want %v", got, want)
	}
	if lbl, _ := nodeLabel(g, "a.md"); lbl != "Alpha" {
		t.Fatalf("a.md label = %q, want Alpha (title)", lbl)
	}
	if got, want := nodeIDsByType(g, "tag"), []string{"tag:design"}; !equalSlices(got, want) {
		t.Fatalf("tag nodes = %v, want %v", got, want)
	}
	if lbl, _ := nodeLabel(g, "tag:design"); lbl != "design" {
		t.Fatalf("tag:design label = %q, want design", lbl)
	}
	if got, want := edgeStrsByType(g, "link"), []string{"a.md->b.md", "b.md->c.md"}; !equalSlices(got, want) {
		t.Fatalf("link edges = %v, want %v", got, want)
	}
	if got, want := edgeStrsByType(g, "tag"), []string{"a.md->tag:design", "b.md->tag:design", "c.md->tag:design"}; !equalSlices(got, want) {
		t.Fatalf("tag edges = %v, want %v", got, want)
	}

	// Lean-shape JSON guard: typed nodes/edges present, NO body/frontmatter.
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	js := string(raw)
	for _, want := range []string{`"type":"page"`, `"type":"tag"`, `"type":"link"`} {
		if !strings.Contains(js, want) {
			t.Fatalf("payload missing %q: %s", want, js)
		}
	}
	for _, leak := range []string{"body", "frontmatter"} {
		if strings.Contains(js, leak) {
			t.Fatalf("payload leaks %q: %s", leak, js)
		}
	}
}

// TestGraphData_PopularTagCap: 10 pages, "common" on 9 (> 25%), "rare" on 2.
// The common tag node and its edges are excluded; rare survives.
func TestGraphData_PopularTagCap(t *testing.T) {
	h := newHarness(t)
	for i := 0; i < 10; i++ {
		tags := []string{"common"}
		if i == 9 { // page 9 only "rare" (so common is on 9 of 10)
			tags = []string{"rare"}
		}
		if i == 0 {
			tags = []string{"common", "rare"}
		}
		h.writePage(t, fmt.Sprintf("p%d.md", i), fmt.Sprintf("Page %d", i), tags, "x")
	}
	h.rebuild(t)

	g, err := h.st.GraphData(context.Background())
	if err != nil {
		t.Fatalf("GraphData: %v", err)
	}

	if _, ok := nodeLabel(g, "tag:common"); ok {
		t.Fatalf("tag:common should be capped (on >25%% of pages) but is present")
	}
	for _, e := range g.Edges {
		if e.Type == "tag" && e.Target == "tag:common" {
			t.Fatalf("tag:common edges should be excluded, found %v", e)
		}
	}
	if _, ok := nodeLabel(g, "tag:rare"); !ok {
		t.Fatalf("tag:rare (under threshold) should be present")
	}
}

// TestGraphData_CapDisabledBelowMinPages: a 3-page workspace where every page has
// "design" still emits tag:design (small workspace never over-pruned).
func TestGraphData_CapDisabledBelowMinPages(t *testing.T) {
	h := newHarness(t)
	h.writePage(t, "a.md", "A", []string{"design"}, "x")
	h.writePage(t, "b.md", "B", []string{"design"}, "x")
	h.writePage(t, "c.md", "C", []string{"design"}, "x")
	h.rebuild(t)

	g, err := h.st.GraphData(context.Background())
	if err != nil {
		t.Fatalf("GraphData: %v", err)
	}
	if _, ok := nodeLabel(g, "tag:design"); !ok {
		t.Fatalf("tag:design must survive in a sub-min-pages workspace")
	}
}

// TestNeighborhood_Depth1: over a->b->c, Neighborhood(b, 1) returns b + direct
// neighbors a (inbound) and c (outbound) + b's tag node, NOT a two-hop node.
func TestNeighborhood_Depth1(t *testing.T) {
	h := newHarness(t)
	h.writePage(t, "a.md", "A", []string{"design"}, "[b](b.md)")
	h.writePage(t, "b.md", "B", []string{"design"}, "[c](c.md)")
	h.writePage(t, "c.md", "C", []string{"design"}, "[d](d.md)")
	h.writePage(t, "d.md", "D", []string{"design"}, "x")
	h.rebuild(t)

	g, err := h.st.Neighborhood(context.Background(), "b.md", 1)
	if err != nil {
		t.Fatalf("Neighborhood: %v", err)
	}
	pages := nodeIDsByType(g, "page")
	if !equalSlices(pages, []string{"a.md", "b.md", "c.md"}) {
		t.Fatalf("depth-1 neighborhood pages = %v, want [a.md b.md c.md]", pages)
	}
	for _, p := range pages {
		if p == "d.md" {
			t.Fatalf("d.md is two hops away and must not appear")
		}
	}
}

// TestNeighborhood_DepthClamp: depth=0/-1 => 1; depth>3 => 3.
func TestNeighborhood_DepthClamp(t *testing.T) {
	h := newHarness(t)
	h.writePage(t, "a.md", "A", nil, "[b](b.md)")
	h.writePage(t, "b.md", "B", nil, "[c](c.md)")
	h.writePage(t, "c.md", "C", nil, "x")
	h.rebuild(t)

	// depth 0 clamps to 1: from a, only b is reachable (not c).
	g0, err := h.st.Neighborhood(context.Background(), "a.md", 0)
	if err != nil {
		t.Fatalf("Neighborhood depth0: %v", err)
	}
	if pages := nodeIDsByType(g0, "page"); !equalSlices(pages, []string{"a.md", "b.md"}) {
		t.Fatalf("depth0 clamp pages = %v, want [a.md b.md]", pages)
	}

	// large depth clamps to 3: from a, a->b->c reachable within 3.
	g9, err := h.st.Neighborhood(context.Background(), "a.md", 99)
	if err != nil {
		t.Fatalf("Neighborhood depth99: %v", err)
	}
	if pages := nodeIDsByType(g9, "page"); !equalSlices(pages, []string{"a.md", "b.md", "c.md"}) {
		t.Fatalf("depth99 clamp pages = %v, want [a.md b.md c.md]", pages)
	}
}

// TestNeighborhood_SeedAlwaysPresent: an isolated page still returns its own node.
func TestNeighborhood_SeedAlwaysPresent(t *testing.T) {
	h := newHarness(t)
	h.writePage(t, "lonely.md", "Lonely", nil, "no links")
	h.rebuild(t)

	g, err := h.st.Neighborhood(context.Background(), "lonely.md", 1)
	if err != nil {
		t.Fatalf("Neighborhood: %v", err)
	}
	if _, ok := nodeLabel(g, "lonely.md"); !ok {
		t.Fatalf("seed page must always be present")
	}
}

// TestBacklinks_ReverseQuery: over a->b and c->b, Backlinks(b) returns a + c with
// resolved titles; a page with no inbound links returns a non-nil empty slice.
func TestBacklinks_ReverseQuery(t *testing.T) {
	h := newHarness(t)
	h.writePage(t, "a.md", "Alpha", nil, "[b](b.md)")
	h.writePage(t, "b.md", "Bravo", nil, "x")
	h.writePage(t, "c.md", "Charlie", nil, "[b](b.md)")
	h.rebuild(t)

	entries, err := h.st.Backlinks(context.Background(), "b.md")
	if err != nil {
		t.Fatalf("Backlinks: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("backlinks count = %d, want 2 (%v)", len(entries), entries)
	}
	got := map[string]string{}
	for _, e := range entries {
		got[e.Path] = e.Title
	}
	if got["a.md"] != "Alpha" {
		t.Fatalf("a.md title = %q, want Alpha", got["a.md"])
	}
	if got["c.md"] != "Charlie" {
		t.Fatalf("c.md title = %q, want Charlie", got["c.md"])
	}

	// No inbound links => non-nil empty slice.
	none, err := h.st.Backlinks(context.Background(), "a.md")
	if err != nil {
		t.Fatalf("Backlinks(a): %v", err)
	}
	if none == nil {
		t.Fatalf("Backlinks must return a non-nil slice for no results")
	}
	if len(none) != 0 {
		t.Fatalf("a.md has no inbound links, got %v", none)
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
