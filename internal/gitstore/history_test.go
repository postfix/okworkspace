package gitstore

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/repo"
)

func newHistoryTestStore(t *testing.T) (*GitStore, *repo.Repo) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	root := t.TempDir()
	r, err := repo.New(root)
	if err != nil {
		t.Fatalf("repo.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	gs := New(r, config.GitConfig{Enabled: true, Branch: "main"})
	if err := gs.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return gs, r
}

// commitFile writes path and commits it through the real Commit with the given
// action/user so the trailer is present (History parses it back).
func commitFile(t *testing.T, gs *GitStore, r *repo.Repo, path, content, action, user string) {
	t.Helper()
	if err := r.Write(path, []byte(content)); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
	err := gs.Commit(context.Background(), CommitSpec{
		Paths:   []string{path},
		Message: action + " " + path,
		User:    user,
		Action:  action,
		Source:  "web-ui",
	})
	if err != nil {
		t.Fatalf("Commit %q: %v", path, err)
	}
}

func TestHistory(t *testing.T) {
	gs, r := newHistoryTestStore(t)
	ctx := context.Background()

	commitFile(t, gs, r, "notes/page.md", "# v1\n", "create", "Sam")
	commitFile(t, gs, r, "notes/page.md", "# v2\n", "edit", "Sam")
	commitFile(t, gs, r, "notes/page.md", "# v3\n", "edit", "Riley")

	hist, err := gs.History(ctx, "notes/page.md")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 3 {
		t.Fatalf("history len = %d, want 3", len(hist))
	}
	// Newest-first.
	if hist[0].Action != "edit" || hist[0].Who != "Riley" {
		t.Fatalf("newest entry = %+v, want edit by Riley", hist[0])
	}
	if hist[2].Action != "create" || hist[2].Who != "Sam" {
		t.Fatalf("oldest entry = %+v, want create by Sam", hist[2])
	}
	// Every entry carries a When and an opaque Token (the SHA lives ONLY here).
	for i, c := range hist {
		if c.When.IsZero() {
			t.Fatalf("entry %d has zero When", i)
		}
		if strings.TrimSpace(c.Token) == "" {
			t.Fatalf("entry %d has empty Token", i)
		}
	}
}

func TestHistoryAcrossRename(t *testing.T) {
	gs, r := newHistoryTestStore(t)
	ctx := context.Background()

	// Create at the old path, then "rename" by writing the new path + removing
	// the old in one commit (git records a rename).
	commitFile(t, gs, r, "old/name.md", "# original\n", "create", "Sam")

	// Perform the rename move in one commit: write new, remove old.
	if err := r.Write("new/renamed.md", []byte("# original\n")); err != nil {
		t.Fatalf("write new: %v", err)
	}
	if err := r.Remove("old/name.md"); err != nil {
		t.Fatalf("remove old: %v", err)
	}
	err := gs.Commit(ctx, CommitSpec{
		Paths:   []string{"new/renamed.md", "old/name.md"},
		Message: "rename old/name.md to new/renamed.md",
		User:    "Sam",
		Action:  "rename",
		Source:  "web-ui",
	})
	if err != nil {
		t.Fatalf("Commit rename: %v", err)
	}

	hist, err := gs.History(ctx, "new/renamed.md")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	// --follow: the pre-rename "create" must still be returned.
	if len(hist) < 2 {
		t.Fatalf("history len = %d, want >=2 (--follow across rename)", len(hist))
	}
	foundCreate := false
	for _, c := range hist {
		if c.Action == "create" {
			foundCreate = true
		}
	}
	if !foundCreate {
		t.Fatalf("pre-rename 'create' version missing from history: %+v", hist)
	}
}

func TestShowAt(t *testing.T) {
	gs, r := newHistoryTestStore(t)
	ctx := context.Background()

	commitFile(t, gs, r, "notes/page.md", "# version one\n", "create", "Sam")
	commitFile(t, gs, r, "notes/page.md", "# version two\n", "edit", "Sam")

	hist, err := gs.History(ctx, "notes/page.md")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("history len = %d, want 2", len(hist))
	}
	// Read the OLDEST version (the create) by its opaque token.
	oldest := hist[len(hist)-1]
	bytesOut, err := gs.ShowAt(ctx, oldest.Token, "notes/page.md")
	if err != nil {
		t.Fatalf("ShowAt: %v", err)
	}
	if !strings.Contains(string(bytesOut), "version one") {
		t.Fatalf("ShowAt returned %q, want the v1 content", bytesOut)
	}
}

func TestHistoryEmptyForUncommittedPath(t *testing.T) {
	gs, _ := newHistoryTestStore(t)
	hist, err := gs.History(context.Background(), "never/committed.md")
	if err != nil {
		t.Fatalf("History on uncommitted path: %v", err)
	}
	if len(hist) != 0 {
		t.Fatalf("history len = %d, want 0 for an uncommitted path", len(hist))
	}
}
