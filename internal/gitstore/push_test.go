package gitstore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/repo"
)

// writeFile is a tiny test helper to seed a file in a non-repo dir.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

// runGit runs a raw git command in dir for test setup, failing on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v: %s", args, dir, err, out)
	}
}

func TestPushDisabled(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	root := t.TempDir()
	r, err := repo.New(root)
	if err != nil {
		t.Fatalf("repo.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	// RemoteEnabled / PushOnCommit unset → Push must be a no-op (no network, nil).
	gs := New(r, config.GitConfig{Enabled: true, Branch: "main"})
	if err := gs.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := gs.Push(context.Background()); err != nil {
		t.Fatalf("Push (disabled) should be a no-op nil, got %v", err)
	}

	// Even with PushOnCommit set, an empty Remote stays a no-op.
	gs2 := New(r, config.GitConfig{Enabled: true, Branch: "main", RemoteEnabled: true, PushOnCommit: true, Remote: ""})
	if err := gs2.Push(context.Background()); err != nil {
		t.Fatalf("Push (no remote) should be a no-op nil, got %v", err)
	}
}

func TestPushEnabledReachesRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	// Bare remote.
	remoteDir := t.TempDir()
	runGit(t, remoteDir, "init", "--bare", "-b", "main")

	// Working repo pointed at the bare remote.
	root := t.TempDir()
	r, err := repo.New(root)
	if err != nil {
		t.Fatalf("repo.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	gs := New(r, config.GitConfig{
		Enabled: true, Branch: "main",
		RemoteEnabled: true, PushOnCommit: true, Remote: remoteDir,
	})
	ctx := context.Background()
	if err := gs.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	commitFile(t, gs, r, "page.md", "# hello\n", "create", "Sam")

	if err := gs.Push(ctx); err != nil {
		t.Fatalf("Push (enabled): %v", err)
	}
	// The commit must now exist in the bare remote.
	cmd := exec.Command("git", "log", "-1", "--format=%s", "main")
	cmd.Dir = remoteDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("remote log: %v: %s", err, out)
	}
	if len(out) == 0 {
		t.Fatal("remote received no commit (push did not reach the remote)")
	}
}

func TestPushDiverged(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	// Bare remote with an initial commit pushed from a SEPARATE clone, so the
	// local repo's later push is a non-fast-forward (the remote moved ahead).
	remoteDir := t.TempDir()
	runGit(t, remoteDir, "init", "--bare", "-b", "main")

	// A "other" clone that seeds the remote and then advances it.
	otherDir := t.TempDir()
	runGit(t, otherDir, "clone", remoteDir, ".")
	runGit(t, otherDir, "config", "user.email", "o@x.local")
	runGit(t, otherDir, "config", "user.name", "Other")
	if err := writeFile(filepath.Join(otherDir, "seed.md"), "# seed\n"); err != nil {
		t.Fatal(err)
	}
	runGit(t, otherDir, "add", "seed.md")
	runGit(t, otherDir, "commit", "-m", "seed")
	runGit(t, otherDir, "push", "origin", "main")

	// Local repo: init, set the remote, make a DIVERGENT commit (does not contain
	// the remote's seed), and try to push → non-fast-forward rejection.
	root := t.TempDir()
	r, err := repo.New(root)
	if err != nil {
		t.Fatalf("repo.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	gs := New(r, config.GitConfig{
		Enabled: true, Branch: "main",
		RemoteEnabled: true, PushOnCommit: true, Remote: remoteDir,
	})
	ctx := context.Background()
	if err := gs.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	commitFile(t, gs, r, "local.md", "# local only\n", "create", "Sam")

	// Push must NOT error (alert, never force) but must flag diverged.
	if err := gs.Push(ctx); err != nil {
		t.Fatalf("Push on divergence should return nil (alert, never force), got %v", err)
	}
	health, err := gs.Health(ctx)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !health.Diverged {
		t.Fatal("Health.Diverged = false after a non-fast-forward push; want true (alert)")
	}
	if health.OK {
		t.Fatal("Health.OK = true after divergence; want false")
	}
}
