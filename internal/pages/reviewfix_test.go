package pages

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestCreateFolderEmptySlug proves a punctuation/CJK-only folder name (which
// slugs to "") returns ErrTitleRequired (a clean 400) rather than building an
// absolute "/index.md" path that the resolver rejects as a 500 (WR-07).
func TestCreateFolderEmptySlug(t *testing.T) {
	svc, _, _ := newServiceFixture(t, false)
	ctx := context.Background()

	for _, name := range []string{"!!!", "***", "##", "...", "/ / /"} {
		err := svc.CreateFolder(ctx, "", name, "alice")
		if !errors.Is(err, ErrTitleRequired) {
			t.Fatalf("CreateFolder(%q) err = %v, want ErrTitleRequired", name, err)
		}
	}

	// A normal name still works (regression guard for the happy path).
	if err := svc.CreateFolder(ctx, "", "Architecture", "alice"); err != nil {
		t.Fatalf("CreateFolder(normal): %v", err)
	}
}

// TestVersionTokenMustBelongToPageHistory proves a well-formed hex token that
// belongs to ANOTHER page's history is rejected for an unrelated path with
// ErrInvalidVersion in both ViewVersion and RestoreVersion, and never reaches
// ShowAt — closing the cross-page version disclosure/restore gap (WR-04). The
// page's OWN token still flows through.
func TestVersionTokenMustBelongToPageHistory(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	pathA, err := svc.Create(ctx, "notes", "Page A", "Sam")
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	waitForFile(t, r, pathA)
	// Give page-A a distinct second version so its body diverges from any other
	// page (keeps git's --follow rename heuristic from conflating the two pages).
	revA, _ := svc.Revision(ctx, pathA)
	if err := svc.Save(ctx, pathA, "# page A unique content alpha\n", "", revA, "Sam"); err != nil {
		t.Fatalf("Save A: %v", err)
	}
	waitForRevisionChange(t, svc, pathA, revA)

	pathB, err := svc.Create(ctx, "notes", "Page B", "Sam")
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}
	waitForFile(t, r, pathB)
	revB, _ := svc.Revision(ctx, pathB)
	if err := svc.Save(ctx, pathB, "# page B unique content beta\n", "", revB, "Sam"); err != nil {
		t.Fatalf("Save B: %v", err)
	}
	waitForRevisionChange(t, svc, pathB, revB)

	histA, err := svc.History(ctx, pathA)
	if err != nil || len(histA) == 0 {
		t.Fatalf("History A: %v (len=%d)", err, len(histA))
	}
	tokenA := histA[0].Version // page-A's latest version token

	// tokenA belongs to pathA, not pathB. Using it against pathB must NOT disclose
	// or restore page-A's bytes: it is either rejected by the history-membership
	// check (ErrInvalidVersion) or fails because the path did not exist at that
	// commit — never a successful read of another page's content (WR-04).
	if pg, err := svc.ViewVersion(ctx, pathB, tokenA); err == nil {
		t.Fatalf("ViewVersion(pathB, tokenA) unexpectedly succeeded, disclosing %q", pg.Body)
	}
	if err := svc.RestoreVersion(ctx, pathB, tokenA, "Sam"); err == nil {
		t.Fatal("RestoreVersion(pathB, tokenA) unexpectedly succeeded — cross-page restore")
	}

	// The page's OWN token still works (regression guard).
	if _, err := svc.ViewVersion(ctx, pathA, tokenA); err != nil {
		t.Fatalf("ViewVersion(pathA, tokenA) failed: %v", err)
	}
}

// TestReconcileTrashPrunesPhantomRows proves ReconcileTrash deletes trash rows
// whose trash_path is absent on disk (the DB/Git divergence left by a failed
// async commit) while leaving rows whose backing file still exists (WR-01).
func TestReconcileTrashPrunesPhantomRows(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	// A real delete: row + on-disk trash file both present.
	pagePath, err := svc.Create(ctx, "notes", "Real Page", "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitForFile(t, r, pagePath)
	if err := svc.Delete(ctx, pagePath, "alice"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// Wait for the trash file to materialize so the real row is NOT pruned.
	entries, err := svc.ListTrash(ctx)
	if err != nil || len(entries) != 1 {
		t.Fatalf("ListTrash after delete: %v (len=%d)", err, len(entries))
	}
	var realTrashPath string
	if err := svc.db.QueryRowContext(ctx, `SELECT trash_path FROM trash WHERE id = ?`, entries[0].ID).
		Scan(&realTrashPath); err != nil {
		t.Fatalf("read real trash_path: %v", err)
	}
	waitForFile(t, r, realTrashPath)

	// A phantom row: INSERT a trash row pointing at a trash_path never written to
	// disk (simulating a Delete whose async commit failed).
	if _, err := svc.db.ExecContext(ctx,
		`INSERT INTO trash (original_path, trash_path, title, deleted_by, deleted_at)
		 VALUES (?, ?, ?, ?, ?)`,
		"notes/ghost.md", ".okf-workspace/trash/never-written.md", "Ghost", "alice",
		time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert phantom row: %v", err)
	}

	pruned, err := svc.ReconcileTrash(ctx)
	if err != nil {
		t.Fatalf("ReconcileTrash: %v", err)
	}
	if pruned != 1 {
		t.Fatalf("ReconcileTrash pruned = %d, want 1 (only the phantom row)", pruned)
	}

	after, err := svc.ListTrash(ctx)
	if err != nil {
		t.Fatalf("ListTrash after reconcile: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("trash entries after reconcile = %d, want 1 (the real row survives)", len(after))
	}
	if after[0].OriginalPath != pagePath {
		t.Fatalf("surviving row original_path = %q, want %q", after[0].OriginalPath, pagePath)
	}
}
