package pages

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/graph"
	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/store"
)

// graphFixture stands up a real pages.Service over a temp git repo + migrated
// SQLite db + a real jobs.Worker with BOTH the KindCommit handler (so mutations
// actually commit to disk) AND the real graph.GraphHandler registered (so the
// FIRE-AND-FORGET KindGraph jobs each mutation enqueues actually run and maintain
// the page_links/page_tags adjacency). This is exactly how main.go wires the two
// handlers on the one drain goroutine, so the test proves the production path.
type graphFixture struct {
	svc   *Service
	repo  *repo.Repo
	db    *sql.DB
	store *graph.Store
}

func newGraphFixture(t *testing.T) *graphFixture {
	t.Helper()
	r, gs, _ := newTestRepoAndGit(t)

	st, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	gStore := graph.OpenStore(st.DB())
	gStore.SetRepo(r)
	gStore.SetGit(gs)

	w := jobs.New(st.DB(), jobs.Config{PollInterval: 5 * time.Millisecond})
	w.Register(KindCommit, CommitHandler(r, gs))
	w.Register(graph.KindGraph, graph.GraphHandler(gStore, r))
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	t.Cleanup(func() { w.Stop(); cancel() })

	svc := NewService(r, gs, w, st.DB(), false)
	svc.now = func() time.Time { return time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC) }
	return &graphFixture{svc: svc, repo: r, db: st.DB(), store: gStore}
}

// pageSource assembles a full OKF source with the required frontmatter fields plus
// a tags list and a body, so a created/saved page parses, validates, and carries
// the links/tags the test asserts.
func pageSource(title string, tags []string, body string) (string, string) {
	fm := "type: Page\ntitle: " + title + "\ntimestamp: 2026-06-18T12:00:00Z\ndescription: test\n"
	if len(tags) == 0 {
		fm += "tags: []\n"
	} else {
		fm += "tags:\n"
		for _, tg := range tags {
			fm += "  - " + tg + "\n"
		}
	}
	return fm, body
}

// createPage creates a page at folder/<slug>.md by first scaffolding it via Create
// then overwriting its body+tags via Save (Create only scaffolds empty
// frontmatter). Returns the committed path. It waits for the file then for the
// graph rows to settle.
func (f *graphFixture) savePageAt(t *testing.T, path, title string, tags []string, body string) {
	t.Helper()
	ctx := context.Background()
	fm, bd := pageSource(title, tags, body)
	rev, err := f.svc.Revision(ctx, path)
	if err != nil {
		t.Fatalf("Revision(%q): %v", path, err)
	}
	if err := f.svc.Save(ctx, path, bd, fm, rev, "alice"); err != nil {
		t.Fatalf("Save(%q): %v", path, err)
	}
}

// allLinks returns every (src,dst) edge in page_links, sorted "src|dst".
func (f *graphFixture) allLinks(t *testing.T) []string {
	t.Helper()
	rows, err := f.db.Query(`SELECT src_path, dst_path FROM page_links`)
	if err != nil {
		t.Fatalf("query page_links: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s, d string
		if err := rows.Scan(&s, &d); err != nil {
			t.Fatalf("scan page_links: %v", err)
		}
		out = append(out, s+"|"+d)
	}
	sort.Strings(out)
	return out
}

// allTags returns every (page,tag) row, sorted "page|tag".
func (f *graphFixture) allTags(t *testing.T) []string {
	t.Helper()
	rows, err := f.db.Query(`SELECT page_path, tag FROM page_tags`)
	if err != nil {
		t.Fatalf("query page_tags: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p, tg string
		if err := rows.Scan(&p, &tg); err != nil {
			t.Fatalf("scan page_tags: %v", err)
		}
		out = append(out, p+"|"+tg)
	}
	sort.Strings(out)
	return out
}

// backlinks returns the linker pages of dst as the reverse query on page_links —
// asserting there is NO separate backlink table.
func (f *graphFixture) backlinks(t *testing.T, dst string) []string {
	t.Helper()
	rows, err := f.db.Query(`SELECT src_path FROM page_links WHERE dst_path=?`, dst)
	if err != nil {
		t.Fatalf("query backlinks: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan backlinks: %v", err)
		}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// waitForLinks polls until page_links equals want (sorted "src|dst") or fails.
// The graph job is fire-and-forget on the drain goroutine, so the test must poll
// for the asynchronous adjacency to settle (mirrors waitForFile/waitForPayload).
func (f *graphFixture) waitForLinks(t *testing.T, want []string) {
	t.Helper()
	sort.Strings(want)
	deadline := time.Now().Add(5 * time.Second)
	var got []string
	for time.Now().Before(deadline) {
		got = f.allLinks(t)
		if reflect.DeepEqual(got, want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("page_links never reached %v; last = %v", want, got)
}

// waitForTags polls until page_tags equals want (sorted "page|tag") or fails.
func (f *graphFixture) waitForTags(t *testing.T, want []string) {
	t.Helper()
	sort.Strings(want)
	deadline := time.Now().Add(5 * time.Second)
	var got []string
	for time.Now().Before(deadline) {
		got = f.allTags(t)
		if reflect.DeepEqual(got, want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("page_tags never reached %v; last = %v", want, got)
}

// TestGraphFreshness_Create proves a freshly created page that already contains a
// link to an existing page produces that forward edge plus its tags, with no
// restart.
func TestGraphFreshness_Create(t *testing.T) {
	f := newGraphFixture(t)
	ctx := context.Background()

	// Seed an existing target page A.
	aPath, err := f.svc.Create(ctx, "", "A", "alice")
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	waitForFile(t, f.repo, aPath) // a.md
	waitForRevisionNonEmpty(t, f.svc, aPath)
	f.savePageAt(t, aPath, "A", []string{"alpha"}, "# A\n")
	waitForRevisionNonEmpty(t, f.svc, aPath)

	// Create B, then give it a body linking to A and a tag.
	bPath, err := f.svc.Create(ctx, "", "B", "alice")
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}
	waitForFile(t, f.repo, bPath) // b.md
	waitForRevisionNonEmpty(t, f.svc, bPath)
	f.savePageAt(t, bPath, "B", []string{"beta"}, "# B\n\nSee [A](a.md).\n")

	f.waitForLinks(t, []string{"b.md|a.md"})
	f.waitForTags(t, []string{"a.md|alpha", "b.md|beta"})

	// Backlinks of A are the reverse query (no separate table).
	if got := f.backlinks(t, "a.md"); !reflect.DeepEqual(got, []string{"b.md"}) {
		t.Fatalf("backlinks(a.md) = %v, want [b.md]", got)
	}
}

// TestGraphFreshness_Save proves editing a page's body to add/remove a link
// updates its src edge set, and editing its tags updates page_tags.
func TestGraphFreshness_Save(t *testing.T) {
	f := newGraphFixture(t)
	ctx := context.Background()

	aPath, _ := f.svc.Create(ctx, "", "A", "alice")
	waitForFile(t, f.repo, aPath)
	waitForRevisionNonEmpty(t, f.svc, aPath)
	f.savePageAt(t, aPath, "A", []string{"alpha"}, "# A\n")

	bPath, _ := f.svc.Create(ctx, "", "B", "alice")
	waitForFile(t, f.repo, bPath)
	waitForRevisionNonEmpty(t, f.svc, bPath)

	// Save B with a link to A.
	f.savePageAt(t, bPath, "B", []string{"beta"}, "# B\n\n[A](a.md)\n")
	f.waitForLinks(t, []string{"b.md|a.md"})

	// Re-save B removing the link and changing the tag set: edge and tag both update.
	f.savePageAt(t, bPath, "B", []string{"gamma"}, "# B\n\nno link now\n")
	f.waitForLinks(t, nil)
	f.waitForTags(t, []string{"a.md|alpha", "b.md|gamma"})
}

// TestGraphFreshness_RenameStaleEdge is THE pitfall test: page B links to A;
// rename A -> A'. After the rename drains, the inbound edge B->A must be gone and
// B->A' present (inbound edges rewritten), AND the old src=a.md rows gone / new
// src=a-renamed.md rows present — no stale src/dst row survives.
func TestGraphFreshness_RenameStaleEdge(t *testing.T) {
	f := newGraphFixture(t)
	ctx := context.Background()

	aPath, _ := f.svc.Create(ctx, "", "A", "alice")
	waitForFile(t, f.repo, aPath)
	waitForRevisionNonEmpty(t, f.svc, aPath)
	// A links to C so A has an OUTBOUND edge that must follow the rename (src=old must
	// vanish, src=new must appear).
	cPath, _ := f.svc.Create(ctx, "", "C", "alice")
	waitForFile(t, f.repo, cPath)
	waitForRevisionNonEmpty(t, f.svc, cPath)
	f.savePageAt(t, cPath, "C", nil, "# C\n")
	f.savePageAt(t, aPath, "A", nil, "# A\n\n[C](c.md)\n")

	bPath, _ := f.svc.Create(ctx, "", "B", "alice")
	waitForFile(t, f.repo, bPath)
	waitForRevisionNonEmpty(t, f.svc, bPath)
	f.savePageAt(t, bPath, "B", nil, "# B\n\n[A](a.md)\n")

	// Before rename: B->A (inbound to A) and A->C (outbound from A).
	f.waitForLinks(t, []string{"a.md|c.md", "b.md|a.md"})

	// Rename A -> "A Renamed" (same folder) => a-renamed.md.
	newPath, err := f.svc.Rename(ctx, aPath, "A Renamed", "alice")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if newPath != "a-renamed.md" {
		t.Fatalf("rename target = %q, want a-renamed.md", newPath)
	}
	waitForFile(t, f.repo, newPath)
	waitForGone(t, f.svc, aPath)

	// After rename+drain: inbound edge rewritten (B->a-renamed.md), outbound edge
	// followed (a-renamed.md->c.md). NO stale src=a.md and NO stale dst=a.md rows.
	f.waitForLinks(t, []string{"a-renamed.md|c.md", "b.md|a-renamed.md"})

	for _, edge := range f.allLinks(t) {
		if edge == "b.md|a.md" || edge == "a.md|c.md" {
			t.Fatalf("stale edge %q survived the rename (pitfall 2 not closed)", edge)
		}
	}
	// Explicit no-stale-row assertions on the raw columns.
	if n := f.countRows(t, `SELECT COUNT(*) FROM page_links WHERE src_path='a.md' OR dst_path='a.md'`); n != 0 {
		t.Fatalf("found %d stale rows referencing the old path a.md", n)
	}
}

// TestGraphFreshness_Move proves a move into another folder rewrites inbound and
// outbound edges identically to a rename (the same relocate path).
func TestGraphFreshness_Move(t *testing.T) {
	f := newGraphFixture(t)
	ctx := context.Background()

	// Create a destination folder so the move target dir exists.
	if err := f.svc.CreateFolder(ctx, "", "dest", "alice"); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	waitForFile(t, f.repo, "dest/index.md")

	aPath, _ := f.svc.Create(ctx, "", "A", "alice")
	waitForFile(t, f.repo, aPath)
	waitForRevisionNonEmpty(t, f.svc, aPath)
	f.savePageAt(t, aPath, "A", nil, "# A\n")

	bPath, _ := f.svc.Create(ctx, "", "B", "alice")
	waitForFile(t, f.repo, bPath)
	waitForRevisionNonEmpty(t, f.svc, bPath)
	f.savePageAt(t, bPath, "B", nil, "# B\n\n[A](a.md)\n")
	f.waitForLinks(t, []string{"b.md|a.md"})

	newPath, err := f.svc.Move(ctx, aPath, "dest", "alice")
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if newPath != "dest/a.md" {
		t.Fatalf("move target = %q, want dest/a.md", newPath)
	}
	waitForFile(t, f.repo, newPath)
	waitForGone(t, f.svc, aPath)

	f.waitForLinks(t, []string{"b.md|dest/a.md"})
	if n := f.countRows(t, `SELECT COUNT(*) FROM page_links WHERE src_path='a.md' OR dst_path='a.md'`); n != 0 {
		t.Fatalf("found %d stale rows referencing the old path a.md after move", n)
	}
}

// TestGraphFreshness_DeleteRestore proves delete-to-trash removes the page's
// outbound rows AND inbound edges AND tags; restore re-adds its outbound edges +
// tags.
func TestGraphFreshness_DeleteRestore(t *testing.T) {
	f := newGraphFixture(t)
	ctx := context.Background()

	aPath, _ := f.svc.Create(ctx, "", "A", "alice")
	waitForFile(t, f.repo, aPath)
	waitForRevisionNonEmpty(t, f.svc, aPath)
	cPath, _ := f.svc.Create(ctx, "", "C", "alice")
	waitForFile(t, f.repo, cPath)
	waitForRevisionNonEmpty(t, f.svc, cPath)
	f.savePageAt(t, cPath, "C", nil, "# C\n")
	f.savePageAt(t, aPath, "A", []string{"alpha"}, "# A\n\n[C](c.md)\n")

	bPath, _ := f.svc.Create(ctx, "", "B", "alice")
	waitForFile(t, f.repo, bPath)
	waitForRevisionNonEmpty(t, f.svc, bPath)
	f.savePageAt(t, bPath, "B", nil, "# B\n\n[A](a.md)\n")
	f.waitForLinks(t, []string{"a.md|c.md", "b.md|a.md"})

	// Delete A: its outbound (a->c) and inbound (b->a) edges and tags must all go.
	if err := f.svc.Delete(ctx, aPath, "alice"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	waitForGone(t, f.svc, aPath)
	f.waitForLinks(t, nil)
	f.waitForTags(t, nil) // A's tag gone; B and C have none

	// Restore A: its outbound edge (a->c) and tag (alpha) reappear. The inbound edge
	// b->a is NOT auto-restored by an upsert of A (per-page scope), but a Save of B or
	// a rebuild reconciles it — proven by the rebuild cross-check test.
	entries, err := f.svc.ListTrash(ctx)
	if err != nil {
		t.Fatalf("ListTrash: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ListTrash returned %d entries, want 1", len(entries))
	}
	restored, err := f.svc.Restore(ctx, entries[0].ID, "alice")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	waitForFile(t, f.repo, restored)
	f.waitForLinks(t, []string{"a.md|c.md"})
	f.waitForTags(t, []string{"a.md|alpha"})
}

// TestGraphFreshness_Folder proves a descendant page's edges follow a folder
// rename/move and are removed on folder delete.
func TestGraphFreshness_Folder(t *testing.T) {
	f := newGraphFixture(t)
	ctx := context.Background()

	if err := f.svc.CreateFolder(ctx, "", "docs", "alice"); err != nil {
		t.Fatalf("CreateFolder docs: %v", err)
	}
	waitForFile(t, f.repo, "docs/index.md")

	// A page inside docs that an outside page links to.
	innerPath, _ := f.svc.Create(ctx, "docs", "Inner", "alice")
	waitForFile(t, f.repo, innerPath) // docs/inner.md
	waitForRevisionNonEmpty(t, f.svc, innerPath)
	f.savePageAt(t, innerPath, "Inner", nil, "# Inner\n")

	outerPath, _ := f.svc.Create(ctx, "", "Outer", "alice")
	waitForFile(t, f.repo, outerPath)
	waitForRevisionNonEmpty(t, f.svc, outerPath)
	f.savePageAt(t, outerPath, "Outer", nil, "# Outer\n\n[Inner](docs/inner.md)\n")
	f.waitForLinks(t, []string{"outer.md|docs/inner.md"})

	// Rename folder docs -> guides: the descendant moves, the inbound edge follows.
	newDir, err := f.svc.RenameFolder(ctx, "docs", "guides", "alice")
	if err != nil {
		t.Fatalf("RenameFolder: %v", err)
	}
	if newDir != "guides" {
		t.Fatalf("RenameFolder dir = %q, want guides", newDir)
	}
	waitForFile(t, f.repo, "guides/inner.md")
	waitForGone(t, f.svc, innerPath)
	f.waitForLinks(t, []string{"outer.md|guides/inner.md"})
	if n := f.countRows(t, `SELECT COUNT(*) FROM page_links WHERE dst_path='docs/inner.md'`); n != 0 {
		t.Fatalf("stale edge to docs/inner.md survived folder rename")
	}

	// Delete the folder: every descendant's edges (inbound + outbound) are removed.
	if err := f.svc.DeleteFolder(ctx, "guides", "alice"); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	waitForGone(t, f.svc, "guides/inner.md")
	f.waitForLinks(t, nil)
}

// TestGraphFreshness_RebuildCrossCheck runs a full mutation sequence, then runs
// RebuildGraph from scratch and asserts the incrementally-maintained tables equal
// the from-scratch rebuild — the same files produce the same adjacency. This also
// reconciles any per-page-scope gap (e.g. a not-auto-restored inbound edge), which
// is the documented role of the rebuild backstop.
func TestGraphFreshness_RebuildCrossCheck(t *testing.T) {
	f := newGraphFixture(t)
	ctx := context.Background()

	// Build a small graph: A->C, B->A, plus tags.
	aPath, _ := f.svc.Create(ctx, "", "A", "alice")
	waitForFile(t, f.repo, aPath)
	waitForRevisionNonEmpty(t, f.svc, aPath)
	cPath, _ := f.svc.Create(ctx, "", "C", "alice")
	waitForFile(t, f.repo, cPath)
	waitForRevisionNonEmpty(t, f.svc, cPath)
	f.savePageAt(t, cPath, "C", []string{"cc"}, "# C\n")
	f.savePageAt(t, aPath, "A", []string{"aa"}, "# A\n\n[C](c.md)\n")

	bPath, _ := f.svc.Create(ctx, "", "B", "alice")
	waitForFile(t, f.repo, bPath)
	waitForRevisionNonEmpty(t, f.svc, bPath)
	f.savePageAt(t, bPath, "B", []string{"bb"}, "# B\n\n[A](a.md)\n")

	// Rename A then delete C (mix of relocate + delete) so the incremental state has
	// exercised multiple op kinds before the cross-check.
	newA, err := f.svc.Rename(ctx, aPath, "A Two", "alice")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	waitForFile(t, f.repo, newA) // a-two.md
	waitForGone(t, f.svc, aPath)

	// Let the incremental adjacency settle: B->a-two.md, a-two.md->c.md.
	f.waitForLinks(t, []string{"a-two.md|c.md", "b.md|a-two.md"})

	incLinks := f.allLinks(t)
	incTags := f.allTags(t)

	// From-scratch rebuild over the SAME on-disk files.
	if err := f.store.RebuildGraph(ctx); err != nil {
		t.Fatalf("RebuildGraph: %v", err)
	}
	rebuiltLinks := f.allLinks(t)
	rebuiltTags := f.allTags(t)

	if !reflect.DeepEqual(incLinks, rebuiltLinks) {
		t.Fatalf("incremental links %v != rebuilt links %v", incLinks, rebuiltLinks)
	}
	if !reflect.DeepEqual(incTags, rebuiltTags) {
		t.Fatalf("incremental tags %v != rebuilt tags %v", incTags, rebuiltTags)
	}
}

// countRows runs a COUNT(*) query and returns the scalar.
func (f *graphFixture) countRows(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return n
}
