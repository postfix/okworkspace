package pages

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/repo"
)

// seedBatchPage creates a page with a known body and returns its repo-relative
// path plus its current committed revision (the staged base_revision an approve
// would carry). It drives the real single-writer commit path so the page exists
// on disk + in git history, exactly as production would.
func seedBatchPage(t *testing.T, svc *Service, r *repo.Repo, title string) (path, rev string) {
	t.Helper()
	ctx := context.Background()
	p, err := svc.Create(ctx, "", title, "alice")
	if err != nil {
		t.Fatalf("seed Create %q: %v", title, err)
	}
	waitForFile(t, r, p)
	pg, err := svc.Get(ctx, p)
	if err != nil {
		t.Fatalf("seed Get %q: %v", p, err)
	}
	return p, pg.Revision
}

// TestApplyTagsBatchOneCommit is the BATCHED-COMMIT GATE (Pitfall 6): approving N
// pages produces EXACTLY ONE commit (history delta == 1, not N), every page is
// status=applied, and each page's body is byte-identical (only the tags lines
// changed).
func TestApplyTagsBatchOneCommit(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	p1, rev1 := seedBatchPage(t, svc, r, "Alpha")
	p2, rev2 := seedBatchPage(t, svc, r, "Bravo")
	p3, rev3 := seedBatchPage(t, svc, r, "Charlie")

	// Capture each body BEFORE the apply so we can prove byte-stability after.
	body1 := readBody(t, r, p1)
	body2 := readBody(t, r, p2)
	body3 := readBody(t, r, p3)

	before := commitCount(t, r.Root())

	items := []TagApplyItem{
		{PagePath: p1, Tags: []string{"ops"}, BaseRevision: rev1},
		{PagePath: p2, Tags: []string{"docs", "ops"}, BaseRevision: rev2},
		{PagePath: p3, Tags: []string{"runbook"}, BaseRevision: rev3},
	}
	results, err := svc.ApplyTagsBatch(ctx, items, "alice")
	if err != nil {
		t.Fatalf("ApplyTagsBatch: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("results len = %d, want 3", len(results))
	}
	for _, res := range results {
		if res.Status != TagApplyApplied {
			t.Fatalf("page %q status = %q, want applied", res.PagePath, res.Status)
		}
	}

	// THE GATE: N=3 applied pages → exactly ONE new commit, not three.
	after := commitCount(t, r.Root())
	if after-before != 1 {
		t.Fatalf("commit delta = %d, want exactly 1 (batched-commit invariant, Pitfall 6)", after-before)
	}

	// Byte-stability: the body of each page is unchanged; only the tags appear.
	for _, tc := range []struct {
		path, body, tag string
	}{{p1, body1, "ops"}, {p2, body2, "docs"}, {p3, body3, "runbook"}} {
		nb := readBody(t, r, tc.path)
		if nb != tc.body {
			t.Fatalf("page %q body changed by tag apply:\nbefore=%q\nafter =%q", tc.path, tc.body, nb)
		}
		fm := readFrontmatter(t, r, tc.path)
		if !strings.Contains(fm, tc.tag) {
			t.Fatalf("page %q frontmatter missing applied tag %q; got:\n%s", tc.path, tc.tag, fm)
		}
	}
}

// TestApplyTagsBatchStaleDoesNotSink proves a per-page stale base_revision 409s
// THAT page individually (not written, not clobbered) WITHOUT sinking the batch:
// the other pages still apply in ONE commit.
func TestApplyTagsBatchStaleDoesNotSink(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	p1, rev1 := seedBatchPage(t, svc, r, "Alpha")
	p2, rev2 := seedBatchPage(t, svc, r, "Bravo")
	p3, rev3 := seedBatchPage(t, svc, r, "Charlie")

	// Mutate page 2 OUT-OF-BAND so its base_revision (rev2) is now stale: a normal
	// save against the CURRENT revision lands a new commit and moves the blob.
	pg2, err := svc.Get(ctx, p2)
	if err != nil {
		t.Fatalf("Get p2: %v", err)
	}
	if err := svc.Save(ctx, p2, "moved body\n", pg2.Frontmatter, pg2.Revision, "mallory"); err != nil {
		t.Fatalf("out-of-band Save p2: %v", err)
	}
	// Wait until the out-of-band commit has moved p2's revision.
	waitForRevisionChanged(t, svc, p2, rev2)

	beforeBody2 := readBody(t, r, p2)
	before := commitCount(t, r.Root())

	items := []TagApplyItem{
		{PagePath: p1, Tags: []string{"ops"}, BaseRevision: rev1},
		{PagePath: p2, Tags: []string{"stale-should-not-write"}, BaseRevision: rev2}, // STALE
		{PagePath: p3, Tags: []string{"runbook"}, BaseRevision: rev3},
	}
	results, err := svc.ApplyTagsBatch(ctx, items, "alice")
	if err != nil {
		t.Fatalf("ApplyTagsBatch: %v", err)
	}

	got := map[string]string{}
	for _, res := range results {
		got[res.PagePath] = res.Status
	}
	if got[p1] != TagApplyApplied || got[p3] != TagApplyApplied {
		t.Fatalf("p1=%q p3=%q, want both applied", got[p1], got[p3])
	}
	if got[p2] != TagApplyStale {
		t.Fatalf("p2 status = %q, want stale", got[p2])
	}

	// Exactly ONE commit for the two applied pages (the stale page added nothing).
	after := commitCount(t, r.Root())
	if after-before != 1 {
		t.Fatalf("commit delta = %d, want exactly 1 (two applied pages in one commit)", after-before)
	}

	// The stale page was NOT clobbered: its body is the out-of-band body, and the
	// stale tag was NEVER written.
	if nb := readBody(t, r, p2); nb != beforeBody2 {
		t.Fatalf("stale page p2 body changed (clobbered): before=%q after=%q", beforeBody2, nb)
	}
	if fm := readFrontmatter(t, r, p2); strings.Contains(fm, "stale-should-not-write") {
		t.Fatalf("stale page p2 had the rejected tag written; got:\n%s", fm)
	}
}

// TestApplyTagsBatchNotFound proves a missing page yields status=notfound and is
// skipped while the rest still apply.
func TestApplyTagsBatchNotFound(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	p1, rev1 := seedBatchPage(t, svc, r, "Alpha")

	before := commitCount(t, r.Root())
	items := []TagApplyItem{
		{PagePath: p1, Tags: []string{"ops"}, BaseRevision: rev1},
		{PagePath: "does-not-exist.md", Tags: []string{"ghost"}, BaseRevision: "deadbeef"},
	}
	results, err := svc.ApplyTagsBatch(ctx, items, "alice")
	if err != nil {
		t.Fatalf("ApplyTagsBatch: %v", err)
	}
	got := map[string]string{}
	for _, res := range results {
		got[res.PagePath] = res.Status
	}
	if got[p1] != TagApplyApplied {
		t.Fatalf("p1 status = %q, want applied", got[p1])
	}
	if got["does-not-exist.md"] != TagApplyNotFound {
		t.Fatalf("missing page status = %q, want notfound", got["does-not-exist.md"])
	}
	if after := commitCount(t, r.Root()); after-before != 1 {
		t.Fatalf("commit delta = %d, want 1 (the one real page)", after-before)
	}
}

// TestApplyTagsBatchIdempotent proves re-running the batch for an already-applied
// page is safe: the second run re-applies the SAME tags byte-stably (against the
// new revision) with no corruption.
func TestApplyTagsBatchIdempotent(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	p1, rev1 := seedBatchPage(t, svc, r, "Alpha")

	if _, err := svc.ApplyTagsBatch(ctx, []TagApplyItem{{PagePath: p1, Tags: []string{"ops"}, BaseRevision: rev1}}, "alice"); err != nil {
		t.Fatalf("ApplyTagsBatch run 1: %v", err)
	}
	pg, err := svc.Get(ctx, p1)
	if err != nil {
		t.Fatalf("Get after run 1: %v", err)
	}
	bodyAfter1 := readBody(t, r, p1)

	// Re-run against the NEW revision with the SAME tags.
	results, err := svc.ApplyTagsBatch(ctx, []TagApplyItem{{PagePath: p1, Tags: []string{"ops"}, BaseRevision: pg.Revision}}, "alice")
	if err != nil {
		t.Fatalf("ApplyTagsBatch run 2: %v", err)
	}
	if results[0].Status != TagApplyApplied {
		t.Fatalf("run 2 status = %q, want applied", results[0].Status)
	}
	if nb := readBody(t, r, p1); nb != bodyAfter1 {
		t.Fatalf("idempotent re-apply changed body: %q -> %q", bodyAfter1, nb)
	}
	fm := readFrontmatter(t, r, p1)
	if strings.Count(fm, "ops") != 1 {
		t.Fatalf("idempotent re-apply duplicated tag; frontmatter:\n%s", fm)
	}
}

// --- helpers ---

func readBody(t *testing.T, r interface{ Read(string) ([]byte, error) }, path string) string {
	t.Helper()
	raw, err := r.Read(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	idx := strings.Index(string(raw), "\n---\n")
	if idx < 0 {
		return string(raw)
	}
	return string(raw)[idx+len("\n---\n"):]
}

func readFrontmatter(t *testing.T, r interface{ Read(string) ([]byte, error) }, path string) string {
	t.Helper()
	raw, err := r.Read(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return string(raw)
}

func waitForRevisionChanged(t *testing.T, svc *Service, path, old string) {
	t.Helper()
	for i := 0; i < 600; i++ {
		cur, err := svc.Revision(context.Background(), path)
		if err == nil && cur != old {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("revision for %q never moved off %q", path, old)
}
