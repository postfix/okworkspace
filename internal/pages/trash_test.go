package pages

import (
	"context"
	"strings"
	"testing"
	"time"
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
