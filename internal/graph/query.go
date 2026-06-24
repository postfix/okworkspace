package graph

import (
	"context"
	"path"
	"sort"
	"strings"

	"github.com/postfix/okworkspace/internal/okf"
)

// Popular-tag cap (hairball prevention). A tag that sits on a large share of all
// pages becomes a hub node every page connects to — it dominates the layout and
// tells the reader nothing. We exclude such ubiquitous tags from BOTH the tag
// nodes and the tag membership edges so the graph stays legible.
//
//   - popularTagShare: a tag appearing on MORE than this fraction of all pages is
//     dropped (0.25 = a tag on >25% of pages).
//   - popularTagMinPages: the cap only engages once the workspace has at least
//     this many pages, so a tiny (e.g. 3-page) workspace where every page shares a
//     tag is never over-pruned into an edge-less graph.
const (
	popularTagShare    = 0.25
	popularTagMinPages = 8
)

// depthMin / depthMax bound the local-graph traversal so a local payload can
// never explode: depth<1 clamps up to 1, depth>3 clamps down to 3.
const (
	depthMin = 1
	depthMax = 3
)

// GraphNode is one node in a lean graph payload: a page or a tag. It carries an
// id, a human label, and a type ONLY — never a body, frontmatter, or excerpt
// (the lean-payload invariant; T-09-01).
type GraphNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Type  string `json:"type"` // "page" | "tag"
}

// GraphEdge is a directed typed edge. A "link" edge is page(source)->page(target)
// (direction carries backlink derivability — no separate backlink edge type). A
// "tag" edge is page(source)->tag(target) membership.
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // "link" | "tag"
}

// GraphData is the lean bipartite typed-edge payload returned by GraphData /
// Neighborhood. Nodes/Edges are always non-nil so an empty workspace marshals as
// [] (not null), matching the search empty-result convention.
type GraphData struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// BacklinkEntry is one page that links TO a queried page, with its resolved title.
type BacklinkEntry struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

// tagNodeID namespaces a tag id so it can never collide with a page path.
func tagNodeID(tag string) string { return "tag:" + tag }

// GraphData returns the whole derived graph: every live page node (label = its
// title) plus every surviving tag node, the page->page link edges, and the
// page->tag membership edges. The page-node set is the UNION of every live page
// on disk (enumerated from the repo Tree with the SAME skip rules as
// RebuildGraph — dirs, non-.md, trashed) and every page referenced by the
// page_links/page_tags cache tables (belt-and-suspenders). Including the live
// set guarantees ORPHAN pages (no links, no tags) still appear as nodes —
// without it a page that is never a link src/dst and carries no tag is invisible
// (the Phase-9 "returns ALL pages as nodes" criterion / Phase-10 GRAPH-02
// orphan-visibility dependency). Edges and tag nodes are unchanged: still built
// ONLY from the cache tables (no .md body reads in the request path; titles are
// the sole file touch, via the SEC-01 resolver), and popular tags are excluded
// per the cap above. The live walk reads no bodies — only path enumeration.
func (s *Store) GraphData(ctx context.Context) (GraphData, error) {
	g := GraphData{Nodes: []GraphNode{}, Edges: []GraphEdge{}}
	if s.db == nil {
		return g, nil
	}

	// Cache-referenced pages (any link src/dst, any tagged page).
	pageSet, err := s.allPages(ctx)
	if err != nil {
		return g, err
	}
	// Union in every live page on disk so orphans (no links, no tags) are nodes.
	// If the repo is not wired (nil) we fall back to the cache-derived set rather
	// than erroring — the server path wires the repo (main.go SetRepo), so
	// production always returns all live pages.
	for p := range s.livePages() {
		pageSet[p] = struct{}{}
	}
	for _, p := range sortedKeys(pageSet) {
		g.Nodes = append(g.Nodes, GraphNode{ID: p, Label: s.titleFor(p), Type: "page"})
	}

	linkEdges, err := s.linkEdges(ctx, nil)
	if err != nil {
		return g, err
	}
	g.Edges = append(g.Edges, linkEdges...)

	tagNodes, tagEdges, err := s.tagNodesAndEdges(ctx, nil)
	if err != nil {
		return g, err
	}
	g.Nodes = append(g.Nodes, tagNodes...)
	g.Edges = append(g.Edges, tagEdges...)

	return g, nil
}

// Neighborhood returns the seed page plus the pages reachable within depth hops
// over link edges (BOTH directions), as the same lean node/edge shape (link edges
// where both endpoints are in the set; tag nodes/edges for the in-set pages, still
// popular-tag capped). The seed node is always present even with no edges. depth
// is clamped into [depthMin, depthMax].
func (s *Store) Neighborhood(ctx context.Context, pagePath string, depth int) (GraphData, error) {
	g := GraphData{Nodes: []GraphNode{}, Edges: []GraphEdge{}}
	if s.db == nil {
		return g, nil
	}
	if depth < depthMin {
		depth = depthMin
	}
	if depth > depthMax {
		depth = depthMax
	}

	// BFS over undirected link adjacency up to depth hops.
	visited := map[string]struct{}{pagePath: {}}
	frontier := []string{pagePath}
	for hop := 0; hop < depth && len(frontier) > 0; hop++ {
		var next []string
		for _, cur := range frontier {
			neighbors, err := s.linkNeighbors(ctx, cur)
			if err != nil {
				return g, err
			}
			for _, nb := range neighbors {
				if _, seen := visited[nb]; !seen {
					visited[nb] = struct{}{}
					next = append(next, nb)
				}
			}
		}
		frontier = next
	}

	for _, p := range sortedKeys(visited) {
		g.Nodes = append(g.Nodes, GraphNode{ID: p, Label: s.titleFor(p), Type: "page"})
	}

	linkEdges, err := s.linkEdges(ctx, visited)
	if err != nil {
		return g, err
	}
	g.Edges = append(g.Edges, linkEdges...)

	tagNodes, tagEdges, err := s.tagNodesAndEdges(ctx, visited)
	if err != nil {
		return g, err
	}
	g.Nodes = append(g.Nodes, tagNodes...)
	g.Edges = append(g.Edges, tagEdges...)

	return g, nil
}

// Backlinks returns every page linking TO pagePath (reverse page_links query),
// each with its resolved title (best-effort, basename fallback). Always returns a
// non-nil slice.
func (s *Store) Backlinks(ctx context.Context, pagePath string) ([]BacklinkEntry, error) {
	entries := []BacklinkEntry{}
	if s.db == nil {
		return entries, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT src_path FROM page_links WHERE dst_path=? ORDER BY src_path`, pagePath)
	if err != nil {
		return entries, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var src string
		if err := rows.Scan(&src); err != nil {
			return entries, err
		}
		entries = append(entries, BacklinkEntry{Path: src, Title: s.titleFor(src)})
	}
	if err := rows.Err(); err != nil {
		return entries, err
	}
	return entries, nil
}

// allPages returns the set of every page mentioned by the cache tables (any
// src/dst of a link, or any tagged page).
func (s *Store) allPages(ctx context.Context) (map[string]struct{}, error) {
	set := map[string]struct{}{}
	queries := []string{
		`SELECT src_path FROM page_links`,
		`SELECT dst_path FROM page_links`,
		`SELECT page_path FROM page_tags`,
	}
	for _, q := range queries {
		rows, err := s.db.QueryContext(ctx, q)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				_ = rows.Close()
				return nil, err
			}
			set[p] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		_ = rows.Close()
	}
	return set, nil
}

// livePages enumerates every live page on disk from the repo Tree, applying the
// SAME skip rules as RebuildGraph (skip directories, non-.md files, and anything
// under the trash prefix). It reads NO bodies — only path enumeration — so it
// preserves the lean request-path invariant (titles remain the sole file touch,
// via titleFor). A nil repo (no-repo harness) or a Tree() error returns an empty
// set so GraphData degrades to the cache-derived node set rather than erroring;
// the server path wires the repo (main.go SetRepo) so production enumerates all
// live pages.
func (s *Store) livePages() map[string]struct{} {
	live := map[string]struct{}{}
	if s.repo == nil {
		return live
	}
	items, err := s.repo.Tree()
	if err != nil {
		return live
	}
	for _, it := range items {
		if it.IsDir || isTrashed(it.Path) || !strings.HasSuffix(it.Path, ".md") {
			continue
		}
		live[it.Path] = struct{}{}
	}
	return live
}

// linkEdges returns all page->page link edges, optionally restricted to a page
// set (both endpoints must be in the set when restrict is non-nil).
func (s *Store) linkEdges(ctx context.Context, restrict map[string]struct{}) ([]GraphEdge, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT src_path, dst_path FROM page_links ORDER BY src_path, dst_path`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var edges []GraphEdge
	for rows.Next() {
		var src, dst string
		if err := rows.Scan(&src, &dst); err != nil {
			return nil, err
		}
		if restrict != nil {
			if _, ok := restrict[src]; !ok {
				continue
			}
			if _, ok := restrict[dst]; !ok {
				continue
			}
		}
		edges = append(edges, GraphEdge{Source: src, Target: dst, Type: "link"})
	}
	return edges, rows.Err()
}

// linkNeighbors returns every page directly link-adjacent to pagePath (in OR out).
func (s *Store) linkNeighbors(ctx context.Context, pagePath string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT dst_path FROM page_links WHERE src_path=?
		 UNION
		 SELECT src_path FROM page_links WHERE dst_path=?`, pagePath, pagePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// tagNodesAndEdges computes the surviving (non-popular) tag nodes and the
// page->tag membership edges, optionally restricted to a page set. The popular-tag
// cap is evaluated against the WHOLE workspace's page/tag counts (not the
// restricted subset) so a tag's hub-ness is judged globally and a neighborhood
// view agrees with the global graph on which tags are capped.
func (s *Store) tagNodesAndEdges(ctx context.Context, restrict map[string]struct{}) ([]GraphNode, []GraphEdge, error) {
	totalPages, err := s.distinctPageCount(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Per-tag global page counts → which tags survive the cap.
	counts, err := s.tagPageCounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	surviving := map[string]bool{}
	for tag, c := range counts {
		if totalPages >= popularTagMinPages && float64(c) > popularTagShare*float64(totalPages) {
			continue // capped: ubiquitous hub tag
		}
		surviving[tag] = true
	}

	// Membership rows → edges (and the set of tags actually present after restrict).
	rows, err := s.db.QueryContext(ctx, `SELECT page_path, tag FROM page_tags ORDER BY page_path, tag`)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	var edges []GraphEdge
	emittedTags := map[string]struct{}{}
	for rows.Next() {
		var pagePath, tag string
		if err := rows.Scan(&pagePath, &tag); err != nil {
			return nil, nil, err
		}
		if !surviving[tag] {
			continue
		}
		if restrict != nil {
			if _, ok := restrict[pagePath]; !ok {
				continue
			}
		}
		edges = append(edges, GraphEdge{Source: pagePath, Target: tagNodeID(tag), Type: "tag"})
		emittedTags[tag] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var nodes []GraphNode
	for _, tag := range sortedKeys(emittedTags) {
		nodes = append(nodes, GraphNode{ID: tagNodeID(tag), Label: tag, Type: "tag"})
	}
	return nodes, edges, nil
}

// distinctPageCount returns the number of distinct pages in the workspace (the cap
// denominator) — the union of every page referenced by the cache tables.
func (s *Store) distinctPageCount(ctx context.Context) (int, error) {
	set, err := s.allPages(ctx)
	if err != nil {
		return 0, err
	}
	return len(set), nil
}

// Vocabulary returns the workspace's existing tag vocabulary — the DISTINCT set
// of tags currently present across all pages in the derived page_tags cache,
// sorted ascending for a deterministic, stable order (stable prompts and tests).
// It reads the DERIVED cache only (never the source-of-truth files) and exists
// solely to bias the Phase-11 tag-suggestion prompt toward reusing existing tags
// over inventing near-synonyms (TAG-04). It always returns a non-nil slice (an
// empty, non-nil slice when no tags exist) so callers never special-case nil.
func (s *Store) Vocabulary(ctx context.Context) ([]string, error) {
	out := []string{}
	if s.db == nil {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT tag FROM page_tags ORDER BY tag`)
	if err != nil {
		return out, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return out, err
		}
		out = append(out, tag)
	}
	return out, rows.Err()
}

// tagPageCounts returns, per tag, the number of distinct pages carrying it.
func (s *Store) tagPageCounts(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT tag, COUNT(DISTINCT page_path) FROM page_tags GROUP BY tag`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var tag string
		var c int
		if err := rows.Scan(&tag, &c); err != nil {
			return nil, err
		}
		out[tag] = c
	}
	return out, rows.Err()
}

// titleFor resolves a page's display title by reading ONLY its frontmatter title
// through the SEC-01 resolver (s.repo), with a basename fallback. This is the only
// place the query layer touches files, and only for titles, never bodies. A nil
// repo or any read/parse error => basename fallback (never an error return),
// mirroring internal/pages.Service.pageTitle.
func (s *Store) titleFor(pagePath string) string {
	fallback := strings.TrimSuffix(path.Base(pagePath), ".md")
	if s.repo == nil {
		return fallback
	}
	raw, err := s.repo.Read(pagePath)
	if err != nil {
		return fallback
	}
	doc, err := okf.Parse(raw)
	if err != nil || !doc.HasFrontmatter {
		return fallback
	}
	if title := okf.Field(doc, okf.FieldTitle); strings.TrimSpace(title) != "" {
		return title
	}
	return fallback
}

// sortedKeys returns the map keys sorted (stable, deterministic payloads).
func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
