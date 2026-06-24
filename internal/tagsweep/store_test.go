package tagsweep

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/store"
)

// newTestStore wires a real tagsweep Store over a real content repo + migrated
// SQLite store (mirrors the graph package's t.TempDir() harness convention). It
// returns the tagsweep Store and the underlying *store.Store so a test can seed
// page_tags / re-run Migrate directly.
func newTestStore(t *testing.T) (*Store, *store.Store, *repo.Repo) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	base := t.TempDir()

	r, err := repo.New(filepath.Join(base, "repo"))
	if err != nil {
		t.Fatalf("repo.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	st, err := store.Open(filepath.Join(base, "app.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	ts := OpenStore(st.DB())
	ts.SetRepo(r)
	return ts, st, r
}

// writePage writes a minimal .md page to the content repo (path enumeration is
// all Targets needs — bodies are irrelevant to the store).
func writePage(t *testing.T, r *repo.Repo, path string) {
	t.Helper()
	content := fmt.Sprintf("---\ntype: page\ntitle: %s\n---\n\nbody\n", path)
	if err := r.Write(path, []byte(content)); err != nil {
		t.Fatalf("write page %q: %v", path, err)
	}
}

// seedTag inserts a page_tags row so a page counts as tagged.
func seedTag(t *testing.T, st *store.Store, pagePath, tag string) {
	t.Helper()
	if _, err := st.DB().Exec(`INSERT INTO page_tags (page_path, tag) VALUES (?, ?)`, pagePath, tag); err != nil {
		t.Fatalf("seed page_tags %q: %v", pagePath, err)
	}
}

// TestMigrationIdempotent asserts migration 0010 applies idempotently: a second
// Migrate is a no-op and version 10 is recorded exactly once.
func TestMigrationIdempotent(t *testing.T) {
	_, st, _ := newTestStore(t)
	ctx := context.Background()

	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	var n int
	if err := st.DB().QueryRow(
		`SELECT COUNT(1) FROM schema_migrations WHERE version=?`, 10).Scan(&n); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if n != 1 {
		t.Fatalf("schema_migrations rows for version 10 = %d, want 1", n)
	}
}

// TestStagePendingRoundTrip asserts StagePending then ListPending round-trips the
// suggestions (tag + existing flag) and base_revision.
func TestStagePendingRoundTrip(t *testing.T) {
	ts, _, _ := newTestStore(t)
	ctx := context.Background()

	sugg := []Suggestion{{Tag: "ops", Existing: true}, {Tag: "runbook", Existing: false}}
	if err := ts.StagePending(ctx, "a.md", sugg, "rev-1"); err != nil {
		t.Fatalf("StagePending: %v", err)
	}

	got, err := ts.ListPending(ctx)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListPending len = %d, want 1", len(got))
	}
	want := PendingEntry{PagePath: "a.md", Suggestions: sugg, BaseRevision: "rev-1"}
	if !reflect.DeepEqual(got[0], want) {
		t.Fatalf("ListPending[0] = %+v, want %+v", got[0], want)
	}
}

// TestStagePendingSupersedes asserts re-staging the same page yields exactly one
// pending row (the last suggestion wins).
func TestStagePendingSupersedes(t *testing.T) {
	ts, st, _ := newTestStore(t)
	ctx := context.Background()

	if err := ts.StagePending(ctx, "a.md", []Suggestion{{Tag: "old"}}, "rev-1"); err != nil {
		t.Fatalf("StagePending #1: %v", err)
	}
	if err := ts.StagePending(ctx, "a.md", []Suggestion{{Tag: "new", Existing: true}}, "rev-2"); err != nil {
		t.Fatalf("StagePending #2: %v", err)
	}

	var n int
	if err := st.DB().QueryRow(
		`SELECT COUNT(1) FROM tag_suggestions WHERE page_path=? AND status='pending'`, "a.md").Scan(&n); err != nil {
		t.Fatalf("count pending: %v", err)
	}
	if n != 1 {
		t.Fatalf("pending rows for a.md = %d, want 1 (supersede)", n)
	}

	got, err := ts.ListPending(ctx)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(got) != 1 || len(got[0].Suggestions) != 1 || got[0].Suggestions[0].Tag != "new" || got[0].BaseRevision != "rev-2" {
		t.Fatalf("after supersede got %+v, want the second suggestion (new/rev-2)", got)
	}
}

// TestListPendingOrdered asserts ListPending returns entries ordered by page_path.
func TestListPendingOrdered(t *testing.T) {
	ts, _, _ := newTestStore(t)
	ctx := context.Background()

	if err := ts.StagePending(ctx, "z.md", []Suggestion{{Tag: "z"}}, "r"); err != nil {
		t.Fatalf("StagePending z: %v", err)
	}
	if err := ts.StagePending(ctx, "a.md", []Suggestion{{Tag: "a"}}, "r"); err != nil {
		t.Fatalf("StagePending a: %v", err)
	}

	got, err := ts.ListPending(ctx)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(got) != 2 || got[0].PagePath != "a.md" || got[1].PagePath != "z.md" {
		t.Fatalf("ListPending order = %v, want [a.md z.md]", []string{got[0].PagePath, got[1].PagePath})
	}
}

// TestTargetsUntagged asserts Targets(false) returns live pages absent from
// page_tags, and that a tagged-but-deleted page is never targeted.
func TestTargetsUntagged(t *testing.T) {
	ts, st, r := newTestStore(t)
	ctx := context.Background()

	writePage(t, r, "a.md") // untagged → target
	writePage(t, r, "b.md") // tagged → not a target
	writePage(t, r, "c.md") // untagged → target
	seedTag(t, st, "b.md", "ops")
	// A tag row for a page that does NOT exist on disk: must never be targeted.
	seedTag(t, st, "ghost.md", "ops")

	got, err := ts.Targets(ctx, false)
	if err != nil {
		t.Fatalf("Targets(false): %v", err)
	}
	want := []string{"a.md", "c.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Targets(false) = %v, want %v", got, want)
	}
}

// TestTargetsAll asserts Targets(true) returns every live page (tagged + untagged)
// and still excludes a tagged-but-deleted page.
func TestTargetsAll(t *testing.T) {
	ts, st, r := newTestStore(t)
	ctx := context.Background()

	writePage(t, r, "a.md")
	writePage(t, r, "b.md")
	seedTag(t, st, "b.md", "ops")
	seedTag(t, st, "ghost.md", "ops") // not on disk → excluded

	got, err := ts.Targets(ctx, true)
	if err != nil {
		t.Fatalf("Targets(true): %v", err)
	}
	want := []string{"a.md", "b.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Targets(true) = %v, want %v", got, want)
	}
}

// TestTargetsZeroWhenAllTagged asserts Targets(false) returns an empty slice (not
// an error) when every live page is already tagged.
func TestTargetsZeroWhenAllTagged(t *testing.T) {
	ts, st, r := newTestStore(t)
	ctx := context.Background()

	writePage(t, r, "a.md")
	writePage(t, r, "b.md")
	seedTag(t, st, "a.md", "ops")
	seedTag(t, st, "b.md", "ops")

	got, err := ts.Targets(ctx, false)
	if err != nil {
		t.Fatalf("Targets(false): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Targets(false) with all tagged = %v, want empty", got)
	}
}

// TestTargetsNilRepoEmpty asserts a nil repo yields an empty slice (no panic).
func TestTargetsNilRepoEmpty(t *testing.T) {
	ts, _, _ := newTestStore(t)
	ts.SetRepo(nil)
	got, err := ts.Targets(context.Background(), true)
	if err != nil {
		t.Fatalf("Targets(true) nil repo: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Targets with nil repo = %v, want empty", got)
	}
}
