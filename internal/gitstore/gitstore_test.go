package gitstore_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/repo"
)

func gitConfig() config.GitConfig {
	return config.GitConfig{
		Enabled:       true,
		RemoteEnabled: false,
		Branch:        "main",
		PullOnStartup: false,
	}
}

func newGitStore(t *testing.T) (*gitstore.GitStore, *repo.Repo, string) {
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
	gs := gitstore.New(r, gitConfig())
	return gs, r, r.Root()
}

func TestInitIdempotent(t *testing.T) {
	gs, _, root := newGitStore(t)
	ctx := context.Background()

	if err := gs.Init(ctx); err != nil {
		t.Fatalf("Init (first): %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Fatalf(".git not created: %v", err)
	}
	// Running Init again is a no-op (no error).
	if err := gs.Init(ctx); err != nil {
		t.Fatalf("Init (second): %v", err)
	}
}

func TestCommitWithIdentityMetadata(t *testing.T) {
	gs, r, root := newGitStore(t)
	ctx := context.Background()
	if err := gs.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := r.Write("index.md", []byte("# Home\n")); err != nil {
		t.Fatalf("write file: %v", err)
	}

	err := gs.Commit(ctx, gitstore.CommitSpec{
		Paths:   []string{"index.md"},
		Message: "seed starter repo",
		User:    "admin",
		Action:  "seed",
		Source:  "bootstrap",
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Exactly one commit.
	if got := gitLogCount(t, root); got != 1 {
		t.Fatalf("commit count = %d, want 1", got)
	}
	// Author identity reflects the user.
	author := gitShow(t, root, "%an")
	if !strings.Contains(author, "admin") {
		t.Fatalf("author %q does not reflect user 'admin'", author)
	}
	// Message body carries action + source metadata (SPEC §14.2).
	body := gitShow(t, root, "%B")
	if !strings.Contains(body, "seed") || !strings.Contains(body, "bootstrap") {
		t.Fatalf("commit message body %q missing action/source metadata", body)
	}
	// The file is actually tracked/committed.
	if files := gitShow(t, root, ""); !strings.Contains(gitLsFiles(t, root), "index.md") {
		t.Fatalf("index.md not committed (ls-files: %q, show: %q)", gitLsFiles(t, root), files)
	}
}

func TestSelfHealStaleLock(t *testing.T) {
	gs, r, root := newGitStore(t)
	ctx := context.Background()
	if err := gs.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Simulate a crash that left a stale index.lock with no live git process.
	lock := filepath.Join(root, ".git", "index.lock")
	if err := os.WriteFile(lock, []byte{}, 0o600); err != nil {
		t.Fatalf("create stale lock: %v", err)
	}

	healed, err := gs.SelfHealStaleLock(ctx)
	if err != nil {
		t.Fatalf("SelfHealStaleLock: %v", err)
	}
	if !healed {
		t.Fatal("SelfHealStaleLock reported healed=false, want true")
	}
	if _, statErr := os.Stat(lock); !os.IsNotExist(statErr) {
		t.Fatalf("index.lock still present after self-heal")
	}

	// A subsequent commit succeeds, proving the wedge is cleared.
	if err := r.Write("after.md", []byte("ok\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := gs.Commit(ctx, gitstore.CommitSpec{
		Paths: []string{"after.md"}, Message: "post-heal", User: "admin", Action: "edit", Source: "test",
	}); err != nil {
		t.Fatalf("Commit after self-heal: %v", err)
	}
}

func TestPullOnStartupNoNetworkWhenRemoteDisabled(t *testing.T) {
	gs, _, _ := newGitStore(t)
	ctx := context.Background()
	if err := gs.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// remote_enabled=false (default in gitConfig) => no-op, no error.
	if err := gs.PullOnStartup(ctx); err != nil {
		t.Fatalf("PullOnStartup with remote disabled should be a no-op, got %v", err)
	}
}

func TestHealthReportsOK(t *testing.T) {
	gs, _, _ := newGitStore(t)
	ctx := context.Background()
	if err := gs.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	h, err := gs.Health(ctx)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !h.OK {
		t.Fatalf("Health.OK = false (detail: %q), want true", h.Detail)
	}
	if h.Diverged {
		t.Fatalf("Health.Diverged = true on a local-only repo, want false")
	}
}

// --- git inspection helpers ---

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func gitLogCount(t *testing.T, root string) int {
	t.Helper()
	out := runGit(t, root, "rev-list", "--count", "HEAD")
	n := 0
	for _, c := range out {
		n = n*10 + int(c-'0')
	}
	return n
}

func gitShow(t *testing.T, root, format string) string {
	t.Helper()
	if format == "" {
		return runGit(t, root, "show", "--stat", "HEAD")
	}
	return runGit(t, root, "show", "-s", "--format="+format, "HEAD")
}

func gitLsFiles(t *testing.T, root string) string {
	t.Helper()
	return runGit(t, root, "ls-files")
}
