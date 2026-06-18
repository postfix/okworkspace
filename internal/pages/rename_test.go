package pages

import (
	"context"
	"strings"
	"testing"
	"time"
)

// waitForRevisionChange polls until the page at path no longer exists OR the
// target path appears, then returns. Used to wait for a rename/move commit to
// drain.
func waitForGone(t *testing.T, svc *Service, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ok, _ := svc.repo.Exists(path); !ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("path %q never disappeared (rename commit did not drain)", path)
}

// waitForRevisionChange polls until the committed revision of path differs from
// prev, i.e. the save COMMITTED (not merely wrote the working tree). This is the
// reliable drain signal — the working-tree file is written before the commit
// finishes, so asserting on file contents alone races the commit.
func waitForRevisionChange(t *testing.T, svc *Service, path, prev string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rev, _ := svc.Revision(context.Background(), path)
		if rev != "" && rev != prev {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("revision of %q never changed from %q (commit did not drain)", path, prev)
}

// waitForCommitCount polls `git rev-list --count HEAD` until it reaches want (or
// the deadline). The commit object is created at the end of the single-writer
// commit, slightly after the working-tree files appear, so a count sampled right
// after waitForFile/waitForGone can lag by one; polling removes that race.
func waitForCommitCount(t *testing.T, root string, want int) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	last := 0
	for time.Now().Before(deadline) {
		last = commitCount(t, root)
		if last >= want {
			return last
		}
		time.Sleep(5 * time.Millisecond)
	}
	return last
}

func TestRename(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	// A page to rename, and a second page that links to it.
	target, err := svc.Create(ctx, "runbooks", "Deploy", "alice")
	if err != nil {
		t.Fatalf("Create target: %v", err)
	}
	waitForFile(t, r, target) // runbooks/deploy.md

	linker, err := svc.Create(ctx, "architecture", "Overview", "alice")
	if err != nil {
		t.Fatalf("Create linker: %v", err)
	}
	waitForFile(t, r, linker) // architecture/overview.md

	// Put an inbound link in the linker page (architecture/overview.md ->
	// ../runbooks/deploy.md).
	page, _ := svc.Get(ctx, linker)
	linkBody := "See [Deploy](../runbooks/deploy.md) for steps.\n"
	if err := svc.Save(ctx, linker, linkBody, page.Frontmatter, page.Revision, "alice"); err != nil {
		t.Fatalf("Save linker body: %v", err)
	}
	// Wait for the linker save to COMMIT (revision advances), not merely for the
	// working-tree file to change (which happens before the commit lands).
	waitForRevisionChange(t, svc, linker, page.Revision)

	commitsBefore := commitCount(t, r.Root())

	newPath, err := svc.Rename(ctx, target, "Deploy Prod", "alice")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if newPath != "runbooks/deploy-prod.md" {
		t.Fatalf("new path = %q, want runbooks/deploy-prod.md", newPath)
	}
	waitForFile(t, svc.repo, newPath)
	waitForGone(t, svc, target)

	// Exactly ONE new commit for the move + the inbound rewrite (D-07 atomic).
	// Poll: the commit object lands shortly after the working-tree write, so the
	// rev-list count can momentarily lag the file appearing.
	commitsAfter := waitForCommitCount(t, r.Root(), commitsBefore+1)
	if commitsAfter != commitsBefore+1 {
		t.Fatalf("expected exactly 1 new commit for rename, got %d (before=%d after=%d)",
			commitsAfter-commitsBefore, commitsBefore, commitsAfter)
	}

	// The inbound link was rewritten to the new path.
	linked, _ := svc.Get(ctx, linker)
	if !strings.Contains(linked.Body, "../runbooks/deploy-prod.md") {
		t.Fatalf("inbound link not rewritten:\n%s", linked.Body)
	}

	// git log --follow traces history across the rename (continuous history).
	follow := gitOut(t, r.Root(), "log", "--follow", "--format=%H", "--", newPath)
	if len(strings.Fields(follow)) < 2 {
		t.Fatalf("history not continuous across rename; --follow returned:\n%s", follow)
	}
}

func TestMove(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	target, err := svc.Create(ctx, "runbooks", "Deploy", "alice")
	if err != nil {
		t.Fatalf("Create target: %v", err)
	}
	waitForFile(t, r, target) // runbooks/deploy.md

	linker, err := svc.Create(ctx, "architecture", "Overview", "alice")
	if err != nil {
		t.Fatalf("Create linker: %v", err)
	}
	waitForFile(t, r, linker) // architecture/overview.md

	page, _ := svc.Get(ctx, linker)
	if err := svc.Save(ctx, linker, "See [Deploy](../runbooks/deploy.md).\n", page.Frontmatter, page.Revision, "alice"); err != nil {
		t.Fatalf("Save linker body: %v", err)
	}
	waitForRevisionChange(t, svc, linker, page.Revision)

	commitsBefore := commitCount(t, r.Root())

	newPath, err := svc.Move(ctx, target, "architecture", "alice")
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if newPath != "architecture/deploy.md" {
		t.Fatalf("moved path = %q, want architecture/deploy.md", newPath)
	}
	waitForFile(t, svc.repo, newPath)
	waitForGone(t, svc, target)

	commitsAfter := waitForCommitCount(t, r.Root(), commitsBefore+1)
	if commitsAfter != commitsBefore+1 {
		t.Fatalf("expected exactly 1 new commit for move, got %d", commitsAfter-commitsBefore)
	}

	// The linker now lives in the same folder as the moved target, so the link
	// recomputes to a bare sibling reference (deploy.md), not ../runbooks/deploy.md.
	linked, _ := svc.Get(ctx, linker)
	if !strings.Contains(linked.Body, "[Deploy](deploy.md)") {
		t.Fatalf("inbound link not recomputed relative to linker location:\n%s", linked.Body)
	}

	// History continuous across the move.
	follow := gitOut(t, r.Root(), "log", "--follow", "--format=%H", "--", newPath)
	if len(strings.Fields(follow)) < 2 {
		t.Fatalf("history not continuous across move; --follow returned:\n%s", follow)
	}
}

// TestRename_NoCorruption proves a page whose fenced code block contains the old
// page's filename is byte-unchanged across a rename (only the genuine inbound
// link, if any, is rewritten).
func TestRename_NoCorruption(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	target, err := svc.Create(ctx, "runbooks", "Deploy", "alice")
	if err != nil {
		t.Fatalf("Create target: %v", err)
	}
	waitForFile(t, r, target)

	codePage, err := svc.Create(ctx, "guides", "Snippet", "alice")
	if err != nil {
		t.Fatalf("Create code page: %v", err)
	}
	waitForFile(t, r, codePage)

	page, _ := svc.Get(ctx, codePage)
	codeBody := "Here is a command block:\n\n```bash\ncat ../runbooks/deploy.md\n```\n\nDone.\n"
	if err := svc.Save(ctx, codePage, codeBody, page.Frontmatter, page.Revision, "alice"); err != nil {
		t.Fatalf("Save code page: %v", err)
	}
	waitForRevisionChange(t, svc, codePage, page.Revision)
	beforeRaw, _ := r.Read(codePage)
	beforeRev, _ := svc.Revision(ctx, codePage)

	if _, err := svc.Rename(ctx, target, "Deploy Prod", "alice"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	waitForFile(t, svc.repo, "runbooks/deploy-prod.md")

	// The code page's bytes must be unchanged: the old filename inside the fenced
	// code block is NOT a link and must never be rewritten.
	afterRaw, _ := r.Read(codePage)
	if string(beforeRaw) != string(afterRaw) {
		t.Fatalf("code page corrupted by rename.\nbefore:\n%s\nafter:\n%s", beforeRaw, afterRaw)
	}
	// And its committed revision is untouched (it was never re-committed).
	afterRev, _ := svc.Revision(ctx, codePage)
	if afterRev != beforeRev {
		t.Fatalf("code page was re-committed by rename (rev %q -> %q)", beforeRev, afterRev)
	}
}

func TestRename_NotFound(t *testing.T) {
	svc, _, _ := newServiceFixture(t, false)
	if _, err := svc.Rename(context.Background(), "nope/missing.md", "X", "alice"); err != ErrPageNotFound {
		t.Fatalf("Rename missing err = %v, want ErrPageNotFound", err)
	}
}
