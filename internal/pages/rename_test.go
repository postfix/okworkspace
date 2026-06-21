package pages

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/repo"
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

// seedFolderPage creates a page at folder/<slug(title)>.md through the real
// fixture path (so frontmatter is authentic) and waits for the commit to drain.
// It returns the resulting repo-relative path.
func seedFolderPage(t *testing.T, svc *Service, r *repo.Repo, folder, title string) string {
	t.Helper()
	p, err := svc.Create(context.Background(), folder, title, "alice")
	if err != nil {
		t.Fatalf("Create %s/%s: %v", folder, title, err)
	}
	waitForFile(t, r, p)
	return p
}

// TestRelocateFolder renames a folder docs/ -> guides/ and asserts every
// descendant (index.md + a.md + sub/b.md) relocates in EXACTLY ONE commit and the
// old paths are gone.
func TestRelocateFolder(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	// docs/index.md via CreateFolder, plus docs/a.md and docs/sub/b.md.
	if err := svc.CreateFolder(ctx, "", "Docs", "alice"); err != nil {
		t.Fatalf("CreateFolder docs: %v", err)
	}
	waitForFile(t, r, "docs/index.md")
	seedFolderPage(t, svc, r, "docs", "A")       // docs/a.md
	seedFolderPage(t, svc, r, "docs/sub", "B")   // docs/sub/b.md
	waitForFile(t, r, "docs/sub/b.md")

	commitsBefore := commitCount(t, r.Root())

	newDir, err := svc.RenameFolder(ctx, "docs", "Guides", "alice")
	if err != nil {
		t.Fatalf("RenameFolder: %v", err)
	}
	if newDir != "guides" {
		t.Fatalf("newDir = %q, want guides", newDir)
	}

	for _, p := range []string{"guides/index.md", "guides/a.md", "guides/sub/b.md"} {
		waitForFile(t, svc.repo, p)
	}
	for _, p := range []string{"docs/index.md", "docs/a.md", "docs/sub/b.md"} {
		waitForGone(t, svc, p)
	}

	// EXACTLY one new commit for the whole folder relocate (TREE-02 atomic).
	commitsAfter := waitForCommitCount(t, r.Root(), commitsBefore+1)
	if commitsAfter != commitsBefore+1 {
		t.Fatalf("expected exactly 1 new commit for folder rename, got %d (before=%d after=%d)",
			commitsAfter-commitsBefore, commitsBefore, commitsAfter)
	}
}

// TestMoveFolder moves docs/ under manuals/ in one commit; old paths gone.
func TestMoveFolder(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	if err := svc.CreateFolder(ctx, "", "Docs", "alice"); err != nil {
		t.Fatalf("CreateFolder docs: %v", err)
	}
	waitForFile(t, r, "docs/index.md")
	seedFolderPage(t, svc, r, "docs", "A") // docs/a.md
	if err := svc.CreateFolder(ctx, "", "Manuals", "alice"); err != nil {
		t.Fatalf("CreateFolder manuals: %v", err)
	}
	waitForFile(t, r, "manuals/index.md")

	commitsBefore := commitCount(t, r.Root())

	newDir, err := svc.MoveFolder(ctx, "docs", "manuals", "alice")
	if err != nil {
		t.Fatalf("MoveFolder: %v", err)
	}
	if newDir != "manuals/docs" {
		t.Fatalf("newDir = %q, want manuals/docs", newDir)
	}

	for _, p := range []string{"manuals/docs/index.md", "manuals/docs/a.md"} {
		waitForFile(t, svc.repo, p)
	}
	for _, p := range []string{"docs/index.md", "docs/a.md"} {
		waitForGone(t, svc, p)
	}

	commitsAfter := waitForCommitCount(t, r.Root(), commitsBefore+1)
	if commitsAfter != commitsBefore+1 {
		t.Fatalf("expected exactly 1 new commit for folder move, got %d", commitsAfter-commitsBefore)
	}
}

// TestRelocateFolder_Collision renames docs/ -> guides when guides/ already
// exists: it must return ErrFolderExists, add NO commit, and touch NO file.
func TestRelocateFolder_Collision(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	if err := svc.CreateFolder(ctx, "", "Docs", "alice"); err != nil {
		t.Fatalf("CreateFolder docs: %v", err)
	}
	waitForFile(t, r, "docs/index.md")
	if err := svc.CreateFolder(ctx, "", "Guides", "alice"); err != nil {
		t.Fatalf("CreateFolder guides: %v", err)
	}
	waitForFile(t, r, "guides/index.md")

	commitsBefore := commitCount(t, r.Root())

	_, err := svc.RenameFolder(ctx, "docs", "Guides", "alice")
	if !errors.Is(err, ErrFolderExists) {
		t.Fatalf("RenameFolder collision err = %v, want ErrFolderExists", err)
	}

	// No disk write: docs/index.md is still present and no new commit landed.
	if ok, _ := svc.repo.Exists("docs/index.md"); !ok {
		t.Fatalf("docs/index.md was removed despite a rejected collision")
	}
	commitsAfter := commitCount(t, r.Root())
	if commitsAfter != commitsBefore {
		t.Fatalf("collision added %d commits, want 0", commitsAfter-commitsBefore)
	}
}

// TestRelocateFolder_NoCorruption proves a folder move with cross-linked siblings
// (a links to b AND b links to a) rewrites BOTH cross-links to the new paths in
// the same commit without losing either rewrite (Pitfall 1 double-staging), and a
// page whose fenced code block contains link-shaped text is byte-identical after.
func TestRelocateFolder_NoCorruption(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	if err := svc.CreateFolder(ctx, "", "Docs", "alice"); err != nil {
		t.Fatalf("CreateFolder docs: %v", err)
	}
	waitForFile(t, r, "docs/index.md")
	aPath := seedFolderPage(t, svc, r, "docs", "A") // docs/a.md
	bPath := seedFolderPage(t, svc, r, "docs", "B") // docs/b.md

	// a links to b, b links to a (sibling links).
	aPage, _ := svc.Get(ctx, aPath)
	if err := svc.Save(ctx, aPath, "See [B](b.md) for more.\n", aPage.Frontmatter, aPage.Revision, "alice"); err != nil {
		t.Fatalf("Save a: %v", err)
	}
	waitForRevisionChange(t, svc, aPath, aPage.Revision)
	bPage, _ := svc.Get(ctx, bPath)
	codeBody := "Link back: [A](a.md).\n\n```bash\ncat a.md b.md\n```\n"
	if err := svc.Save(ctx, bPath, codeBody, bPage.Frontmatter, bPage.Revision, "alice"); err != nil {
		t.Fatalf("Save b: %v", err)
	}
	waitForRevisionChange(t, svc, bPath, bPage.Revision)

	newDir, err := svc.RenameFolder(ctx, "docs", "Guides", "alice")
	if err != nil {
		t.Fatalf("RenameFolder: %v", err)
	}
	if newDir != "guides" {
		t.Fatalf("newDir = %q, want guides", newDir)
	}
	waitForFile(t, svc.repo, "guides/a.md")
	waitForFile(t, svc.repo, "guides/b.md")
	waitForGone(t, svc, "docs/a.md")

	// Both cross-links survive and now resolve to the moved siblings. The links
	// are computed relative to each page's own (new) directory; siblings stay bare
	// filenames (a.md / b.md), so the rewrite is a no-op-shaped sibling reference —
	// the critical property is the link is NOT lost or pointing at the old folder.
	aMoved, _ := svc.Get(ctx, "guides/a.md")
	if !strings.Contains(aMoved.Body, "[B](b.md)") {
		t.Fatalf("a -> b cross-link lost after folder move:\n%s", aMoved.Body)
	}
	bMoved, _ := svc.Get(ctx, "guides/b.md")
	if !strings.Contains(bMoved.Body, "[A](a.md)") {
		t.Fatalf("b -> a cross-link lost after folder move:\n%s", bMoved.Body)
	}
	// The fenced code block text is byte-identical (okf.RewriteLinks skips code).
	if !strings.Contains(bMoved.Body, "```bash\ncat a.md b.md\n```") {
		t.Fatalf("code block corrupted by folder move:\n%s", bMoved.Body)
	}
}
