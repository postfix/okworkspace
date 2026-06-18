package pages

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/repo"
)

func newTestRepoAndGit(t *testing.T) (*repo.Repo, *gitstore.GitStore, string) {
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
	gs := gitstore.New(r, config.GitConfig{Enabled: true, Branch: "main"})
	if err := gs.Init(context.Background()); err != nil {
		t.Fatalf("gitstore.Init: %v", err)
	}
	return r, gs, r.Root()
}

func gitOut(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func commitCount(t *testing.T, root string) int {
	t.Helper()
	cmd := exec.Command("git", "rev-list", "--count", "HEAD")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0 // no commits yet
	}
	n := 0
	for _, c := range strings.TrimSpace(string(out)) {
		n = n*10 + int(c-'0')
	}
	return n
}

func TestCommitHandler_WritesAndCommits(t *testing.T) {
	r, gs, root := newTestRepoAndGit(t)
	h := CommitHandler(r, gs)

	payload := commitPayload{
		Writes: []fileWrite{{Path: "notes/hello.md", Bytes: []byte("# Hello\n")}},
		Spec: gitstore.CommitSpec{
			Paths:   []string{"notes/hello.md"},
			Message: "create hello",
			User:    "alice",
			Action:  "edit",
			Source:  "web-ui",
		},
	}
	raw := mustMarshal(t, payload)

	if err := h(context.Background(), raw); err != nil {
		t.Fatalf("handler: %v", err)
	}

	if got := commitCount(t, root); got != 1 {
		t.Fatalf("commit count = %d, want 1", got)
	}
	// File was written through the resolver and is tracked.
	if files := gitOut(t, root, "ls-files"); !strings.Contains(files, "notes/hello.md") {
		t.Fatalf("notes/hello.md not committed; ls-files = %q", files)
	}
	// Author identity is the payload User.
	author := gitOut(t, root, "log", "-1", "--format=%an")
	if !strings.Contains(author, "alice") {
		t.Fatalf("author = %q, want to contain 'alice'", author)
	}
	// The Action/Source trailer is present (history view parses this back).
	body := gitOut(t, root, "log", "-1", "--format=%B")
	if !strings.Contains(body, "Action: edit") || !strings.Contains(body, "Source: web-ui") {
		t.Fatalf("commit body missing trailer: %q", body)
	}
}

func TestCommitHandler_BadPayload(t *testing.T) {
	r, gs, root := newTestRepoAndGit(t)
	h := CommitHandler(r, gs)

	if err := h(context.Background(), "{not valid json"); err == nil {
		t.Fatal("expected error for unmarshalable payload, got nil")
	}
	// Nothing was written/committed.
	if got := commitCount(t, root); got != 0 {
		t.Fatalf("commit count = %d, want 0 (bad payload must write nothing)", got)
	}

	// An empty-writes payload is also an error (nothing to commit).
	empty := mustMarshal(t, commitPayload{Spec: gitstore.CommitSpec{Paths: []string{"x.md"}, Message: "x"}})
	if err := h(context.Background(), empty); err == nil {
		t.Fatal("expected error for empty writes, got nil")
	}
}

func TestCommitHandler_MultipleWrites(t *testing.T) {
	r, gs, root := newTestRepoAndGit(t)
	h := CommitHandler(r, gs)

	payload := commitPayload{
		Writes: []fileWrite{
			{Path: "a.md", Bytes: []byte("# A\n")},
			{Path: "b.md", Bytes: []byte("# B\n")},
		},
		Spec: gitstore.CommitSpec{
			Paths:   []string{"a.md", "b.md"},
			Message: "batch two",
			User:    "bob",
			Action:  "rename",
			Source:  "web-ui",
		},
	}
	if err := h(context.Background(), mustMarshal(t, payload)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	// Both paths staged in ONE commit.
	if got := commitCount(t, root); got != 1 {
		t.Fatalf("commit count = %d, want 1 (single batched commit)", got)
	}
	files := gitOut(t, root, "ls-files")
	if !strings.Contains(files, "a.md") || !strings.Contains(files, "b.md") {
		t.Fatalf("both files not committed; ls-files = %q", files)
	}
}

func mustMarshal(t *testing.T, p commitPayload) string {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return string(raw)
}
