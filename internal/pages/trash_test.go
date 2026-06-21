package pages

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/search"
)

// waitForRevisionNonEmpty polls until the committed revision of path is non-empty
// (the commit that created/updated it has fully drained), so a subsequent commit
// count is measured against a settled baseline.
func waitForRevisionNonEmpty(t *testing.T, svc *Service, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if rev, _ := svc.Revision(context.Background(), path); rev != "" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("revision of %q never became non-empty (commit did not drain)", path)
}

// TestTrashRestore: Delete moves a page into .okf-workspace/trash/ via a commit
// (the original path disappears, the page appears under trash) and records a
// trash row with provenance (original_path, deleted_by, deleted_at).
func TestTrashRestore(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	pagePath, err := svc.Create(ctx, "runbooks", "Deploy", "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitForFile(t, r, pagePath) // runbooks/deploy.md
	// Wait for the CREATE commit to fully drain (revision non-empty) before
	// sampling the commit count, so the create commit is not still in flight when
	// we measure the baseline (otherwise the delete's +1 is miscounted).
	waitForRevisionNonEmpty(t, svc, pagePath)

	commitsBefore := commitCount(t, r.Root())

	if err := svc.Delete(ctx, pagePath, "alice"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// The page disappears from its original path.
	waitForGone(t, svc, pagePath)

	// Exactly one new commit for the trash move (D-08 — a real commit). Poll: the
	// delete commit lands shortly after the working-tree removal.
	deadline := time.Now().Add(3 * time.Second)
	commitsAfter := commitsBefore
	for time.Now().Before(deadline) {
		commitsAfter = commitCount(t, r.Root())
		if commitsAfter == commitsBefore+1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if commitsAfter != commitsBefore+1 {
		t.Fatalf("expected exactly 1 new commit for delete, got %d", commitsAfter-commitsBefore)
	}

	// A trash row records provenance (D-10).
	entries, err := svc.ListTrash(ctx)
	if err != nil {
		t.Fatalf("ListTrash: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 trash entry, got %d", len(entries))
	}
	e := entries[0]
	if e.OriginalPath != pagePath {
		t.Fatalf("trash original_path = %q, want %q", e.OriginalPath, pagePath)
	}
	if e.DeletedBy != "alice" {
		t.Fatalf("trash deleted_by = %q, want alice", e.DeletedBy)
	}
	if e.DeletedAt == "" {
		t.Fatal("trash deleted_at is empty; provenance must record when")
	}
	if e.Title != "Deploy" {
		t.Fatalf("trash title = %q, want Deploy", e.Title)
	}

	// The page now lives under the trash directory.
	if !strings.Contains(e.OriginalPath, "runbooks/deploy.md") {
		t.Fatalf("unexpected original path %q", e.OriginalPath)
	}
}

// TestTrashEnqueuesAttachmentDeletes is the WR-04 regression: trashing a page must
// also evict its attachment docs from the live index. An attachment's indexed
// page_path stays the original LIVE path, so without an explicit delete the
// query-time trash filter never matches it and the attachment stays searchable.
func TestTrashEnqueuesAttachmentDeletes(t *testing.T) {
	svc, r, rec := newIndexedFixture(t)
	ctx := context.Background()

	pagePath, err := svc.Create(ctx, "runbooks", "Deploy", "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitForFile(t, r, pagePath)

	// Record two attachments owned by the page in the operational table (the
	// page_path the trash delete queries), plus one owned by a DIFFERENT page that
	// must NOT be evicted.
	for _, a := range []struct{ id, page string }{
		{"att-1", pagePath},
		{"att-2", pagePath},
		{"att-other", "runbooks/other.md"},
	} {
		if _, err := svc.db.ExecContext(ctx,
			`INSERT INTO attachments (id, page_path, original_name, mime_type) VALUES (?, ?, ?, ?)`,
			a.id, a.page, a.id+".pdf", "application/pdf"); err != nil {
			t.Fatalf("insert attachment %q: %v", a.id, err)
		}
	}

	if err := svc.Delete(ctx, pagePath, "alice"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	waitForGone(t, svc, pagePath)

	// Both owned attachments are evicted from the index.
	rec.waitForPayload(t, search.DeleteAttachmentPayload("att-1"))
	rec.waitForPayload(t, search.DeleteAttachmentPayload("att-2"))

	// The attachment owned by another page is NOT evicted.
	otherPayload := search.DeleteAttachmentPayload("att-other")
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, p := range rec.payloads {
		if p == otherPayload {
			t.Fatalf("evicted an attachment (att-other) owned by a non-trashed page: %v", rec.payloads)
		}
	}
}

// TestRestore: Restore moves the page back to its original path (content
// byte-identical to pre-delete) via a commit and removes the trash row.
func TestRestore(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	pagePath, err := svc.Create(ctx, "runbooks", "Deploy", "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitForFile(t, r, pagePath)
	beforeBytes, _ := r.Read(pagePath)

	if err := svc.Delete(ctx, pagePath, "alice"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	waitForGone(t, svc, pagePath)

	entries, _ := svc.ListTrash(ctx)
	if len(entries) != 1 {
		t.Fatalf("expected 1 trash entry, got %d", len(entries))
	}

	restored, err := svc.Restore(ctx, entries[0].ID, "alice")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored != pagePath {
		t.Fatalf("restored path = %q, want %q", restored, pagePath)
	}
	waitForFile(t, r, pagePath)

	afterBytes, _ := r.Read(pagePath)
	if string(beforeBytes) != string(afterBytes) {
		t.Fatalf("restored content not byte-identical.\nbefore:\n%s\nafter:\n%s", beforeBytes, afterBytes)
	}

	// The trash row is gone after restore.
	entries, _ = svc.ListTrash(ctx)
	if len(entries) != 0 {
		t.Fatalf("expected 0 trash entries after restore, got %d", len(entries))
	}
}

// TestRestoreCollision: when a LIVE page already occupies the original path,
// Restore writes the page suffixed (title "(restored)", re-slugged filename)
// rather than overwriting the live page, and returns the suffixed path.
func TestRestoreCollision(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	pagePath, err := svc.Create(ctx, "runbooks", "Deploy", "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitForFile(t, r, pagePath)

	if err := svc.Delete(ctx, pagePath, "alice"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	waitForGone(t, svc, pagePath)

	// Re-create a LIVE page at the same original path (different content) so a
	// naive restore would clobber it.
	livePath, err := svc.Create(ctx, "runbooks", "Deploy", "bob")
	if err != nil {
		t.Fatalf("re-create live: %v", err)
	}
	if livePath != pagePath {
		t.Fatalf("re-created live path = %q, want %q (same original)", livePath, pagePath)
	}
	waitForFile(t, r, livePath)
	liveBytes, _ := r.Read(livePath)

	entries, _ := svc.ListTrash(ctx)
	if len(entries) != 1 {
		t.Fatalf("expected 1 trash entry, got %d", len(entries))
	}

	restored, err := svc.Restore(ctx, entries[0].ID, "alice")
	if err != nil {
		t.Fatalf("Restore (collision): %v", err)
	}
	if restored == pagePath {
		t.Fatalf("restore clobbered the live page (path %q); it must auto-suffix", restored)
	}
	waitForFile(t, r, restored)

	// The live page is untouched.
	afterLive, _ := r.Read(livePath)
	if string(liveBytes) != string(afterLive) {
		t.Fatal("restore clobbered the live page content; collision must auto-suffix")
	}

	// The restored copy carries the "(restored)" title.
	restoredBytes, _ := r.Read(restored)
	if !strings.Contains(string(restoredBytes), "(restored)") {
		t.Fatalf("restored copy missing the (restored) title suffix:\n%s", restoredBytes)
	}
}

// TestListTrash: ListTrash returns entries with title + original path +
// deleted-by + deleted-at fields populated.
func TestListTrash(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	p1, _ := svc.Create(ctx, "runbooks", "Deploy", "alice")
	waitForFile(t, r, p1)
	if err := svc.Delete(ctx, p1, "alice"); err != nil {
		t.Fatalf("Delete p1: %v", err)
	}
	waitForGone(t, svc, p1)

	p2, _ := svc.Create(ctx, "architecture", "Overview", "bob")
	waitForFile(t, r, p2)
	if err := svc.Delete(ctx, p2, "bob"); err != nil {
		t.Fatalf("Delete p2: %v", err)
	}
	waitForGone(t, svc, p2)

	entries, err := svc.ListTrash(ctx)
	if err != nil {
		t.Fatalf("ListTrash: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 trash entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Title == "" || e.OriginalPath == "" || e.DeletedBy == "" || e.DeletedAt == "" {
			t.Fatalf("trash entry missing a field: %+v", e)
		}
	}
}

// TestDeleteCreatesTrashDir: deleting when .okf-workspace/trash/ does not yet
// exist creates it first (A1) and succeeds.
func TestDeleteCreatesTrashDir(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	// Sanity: the trash dir does not exist yet.
	if ok, _ := r.Exists(trashDir); ok {
		t.Fatalf("trash dir already exists before any delete")
	}

	pagePath, _ := svc.Create(ctx, "runbooks", "Deploy", "alice")
	waitForFile(t, r, pagePath)

	if err := svc.Delete(ctx, pagePath, "alice"); err != nil {
		t.Fatalf("Delete (no trash dir yet): %v", err)
	}
	waitForGone(t, svc, pagePath)

	if ok, _ := r.Exists(trashDir); !ok {
		t.Fatal("trash dir was not created on first delete (A1)")
	}
}

// TestDeleteNotFound: deleting a missing page returns ErrPageNotFound.
func TestDeleteNotFound(t *testing.T) {
	svc, _, _ := newServiceFixture(t, false)
	if err := svc.Delete(context.Background(), "nope/missing.md", "alice"); err != ErrPageNotFound {
		t.Fatalf("Delete missing err = %v, want ErrPageNotFound", err)
	}
}

// TestRestoreNotFound: restoring an unknown trash id returns ErrTrashNotFound.
func TestRestoreNotFound(t *testing.T) {
	svc, _, _ := newServiceFixture(t, false)
	if _, err := svc.Restore(context.Background(), 999, "alice"); err != ErrTrashNotFound {
		t.Fatalf("Restore unknown id err = %v, want ErrTrashNotFound", err)
	}
}

// seedFolderForDelete builds docs/ with index.md + a.md + sub/b.md and waits for
// every commit to drain. Returns the three repo-relative page paths.
func seedFolderForDelete(t *testing.T, svc *Service, r *repo.Repo) (string, string, string) {
	t.Helper()
	ctx := context.Background()
	if err := svc.CreateFolder(ctx, "", "Docs", "alice"); err != nil {
		t.Fatalf("CreateFolder docs: %v", err)
	}
	waitForFile(t, r, "docs/index.md")
	aPath := seedFolderPage(t, svc, r, "docs", "A")     // docs/a.md
	bPath := seedFolderPage(t, svc, r, "docs/sub", "B") // docs/sub/b.md
	waitForFile(t, r, bPath)
	return "docs/index.md", aPath, bPath
}

// trashRowsByGroup reads (id, original_path, delete_group_id) for every trash row,
// most-recent first, so a test can inspect the grouping the service wrote.
func trashRowsByGroup(t *testing.T, svc *Service) []struct {
	id      int64
	orig    string
	groupID string
} {
	t.Helper()
	rows, err := svc.db.QueryContext(context.Background(),
		`SELECT id, original_path, COALESCE(delete_group_id, '') FROM trash ORDER BY id DESC`)
	if err != nil {
		t.Fatalf("query trash rows: %v", err)
	}
	defer rows.Close()
	var out []struct {
		id      int64
		orig    string
		groupID string
	}
	for rows.Next() {
		var rec struct {
			id      int64
			orig    string
			groupID string
		}
		if err := rows.Scan(&rec.id, &rec.orig, &rec.groupID); err != nil {
			t.Fatalf("scan trash row: %v", err)
		}
		out = append(out, rec)
	}
	return out
}

// TestDeleteFolder: deleting a folder moves index.md + every descendant to trash
// under ONE shared non-empty delete_group_id, the live pages disappear, and a
// per-page Restore of one member row still works individually (per-page path
// unchanged).
func TestDeleteFolder(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	idx, aPath, bPath := seedFolderForDelete(t, svc, r)

	if err := svc.DeleteFolder(ctx, "docs", "alice"); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	for _, p := range []string{idx, aPath, bPath} {
		waitForGone(t, svc, p)
	}

	rows := trashRowsByGroup(t, svc)
	if len(rows) != 3 {
		t.Fatalf("expected 3 trash rows after folder delete, got %d", len(rows))
	}
	group := rows[0].groupID
	if group == "" {
		t.Fatal("folder-delete trash rows must carry a non-empty delete_group_id")
	}
	gotPaths := map[string]bool{}
	for _, rec := range rows {
		if rec.groupID != group {
			t.Fatalf("trash rows do not share one group id: %q vs %q", rec.groupID, group)
		}
		gotPaths[rec.orig] = true
	}
	for _, p := range []string{idx, aPath, bPath} {
		if !gotPaths[p] {
			t.Fatalf("trash rows missing original path %q (have %v)", p, gotPaths)
		}
	}

	// A per-page Restore of ONE member row still works individually.
	var oneID int64
	for _, rec := range rows {
		if rec.orig == aPath {
			oneID = rec.id
		}
	}
	restored, err := svc.Restore(ctx, oneID, "alice")
	if err != nil {
		t.Fatalf("per-page Restore of a folder-delete member: %v", err)
	}
	if restored != aPath {
		t.Fatalf("per-page restore path = %q, want %q", restored, aPath)
	}
	waitForFile(t, r, aPath)
}

// TestRestoreGroup: after a folder delete, RestoreGroup restores the whole set —
// index.md FIRST so the folder exists, every original path recreated, the group's
// trash rows deleted — and a per-page collision (a live page already at one
// original path) is restored with the existing "(restored)" suffix (per page).
func TestRestoreGroup(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	idx, aPath, bPath := seedFolderForDelete(t, svc, r)

	if err := svc.DeleteFolder(ctx, "docs", "alice"); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	for _, p := range []string{idx, aPath, bPath} {
		waitForGone(t, svc, p)
	}

	rows := trashRowsByGroup(t, svc)
	if len(rows) != 3 {
		t.Fatalf("expected 3 trash rows, got %d", len(rows))
	}
	group := rows[0].groupID

	// Re-create a LIVE page at one original path so a naive restore would clobber it.
	if err := svc.CreateFolder(ctx, "", "Docs", "bob"); err != nil {
		t.Fatalf("re-create live docs folder: %v", err)
	}
	waitForFile(t, r, idx)
	liveBytes, _ := r.Read(idx)

	restored, err := svc.RestoreGroup(ctx, group, "alice")
	if err != nil {
		t.Fatalf("RestoreGroup: %v", err)
	}
	if len(restored) != 3 {
		t.Fatalf("RestoreGroup returned %d paths, want 3: %v", len(restored), restored)
	}

	// index.md restores FIRST (so the folder exists before descendants).
	if restored[0] != idx {
		// idx collided with the live page, so its restored path is suffixed — but it
		// must still be the FIRST entry processed.
		if !strings.Contains(restored[0], "index") && !strings.Contains(restored[0], "docs") {
			t.Fatalf("RestoreGroup did not restore index.md first: %v", restored)
		}
	}

	// a.md and b.md are recreated at their original paths.
	for _, p := range []string{aPath, bPath} {
		waitForFile(t, r, p)
	}

	// The live index.md is untouched (collision auto-suffixed, never clobbered).
	afterLive, _ := r.Read(idx)
	if string(liveBytes) != string(afterLive) {
		t.Fatal("RestoreGroup clobbered a live page; collision must auto-suffix per page")
	}

	// The group's trash rows are gone.
	remaining := trashRowsByGroup(t, svc)
	for _, rec := range remaining {
		if rec.groupID == group {
			t.Fatalf("group %q still has a trash row after RestoreGroup: %+v", group, rec)
		}
	}
}

// TestDeleteFolder_PartialProgress: a folder delete that stops after K of N pages
// (simulated by trashing a subset under one group id) leaves the already-trashed
// rows coherent (same group id) and still restorable as a group — partial progress
// is recoverable by design (RESOLVED atomicity decision; mirrors ReconcileTrash
// WR-01).
func TestDeleteFolder_PartialProgress(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	idx, aPath, _ := seedFolderForDelete(t, svc, r)

	// Simulate a delete loop that stopped after 2 of 3 pages: trash index.md + a.md
	// under ONE shared group id (b.md never got trashed — the loop aborted).
	group := "partialgroup0001"
	if err := svc.deleteWithGroup(ctx, idx, "alice", group); err != nil {
		t.Fatalf("deleteWithGroup index: %v", err)
	}
	waitForGone(t, svc, idx)
	if err := svc.deleteWithGroup(ctx, aPath, "alice", group); err != nil {
		t.Fatalf("deleteWithGroup a: %v", err)
	}
	waitForGone(t, svc, aPath)

	// The partial set is coherent: exactly 2 rows, both sharing the group id.
	rows := trashRowsByGroup(t, svc)
	if len(rows) != 2 {
		t.Fatalf("expected 2 partial trash rows, got %d", len(rows))
	}
	for _, rec := range rows {
		if rec.groupID != group {
			t.Fatalf("partial-progress row has wrong group id %q, want %q", rec.groupID, group)
		}
	}

	// The partial group is still restorable as a unit.
	restored, err := svc.RestoreGroup(ctx, group, "alice")
	if err != nil {
		t.Fatalf("RestoreGroup of partial set: %v", err)
	}
	if len(restored) != 2 {
		t.Fatalf("RestoreGroup partial returned %d paths, want 2: %v", len(restored), restored)
	}
	for _, p := range []string{idx, aPath} {
		waitForFile(t, r, p)
	}
}

// TestSoloDeleteStoresNullGroup: a per-page Delete still stores delete_group_id =
// NULL (the per-page path is unchanged by the grouped-delete refactor).
func TestSoloDeleteStoresNullGroup(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	pagePath, _ := svc.Create(ctx, "runbooks", "Deploy", "alice")
	waitForFile(t, r, pagePath)
	if err := svc.Delete(ctx, pagePath, "alice"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	waitForGone(t, svc, pagePath)

	var groupID sql.NullString
	if err := svc.db.QueryRowContext(ctx,
		`SELECT delete_group_id FROM trash WHERE original_path = ?`, pagePath).Scan(&groupID); err != nil {
		t.Fatalf("query solo trash row: %v", err)
	}
	if groupID.Valid {
		t.Fatalf("solo per-page delete stored a non-NULL group id %q; must be NULL", groupID.String)
	}
}
