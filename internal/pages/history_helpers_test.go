package pages

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/repo"
)

// capturingAllWorker records EVERY enqueued payload (not just the last), so a
// single mutation that enqueues one job can be inspected, and the slice can be
// reset between mutations.
type capturingAllWorker struct{ payloads []string }

func (c *capturingAllWorker) Enqueue(_ context.Context, kind, payload string) error {
	// Capture only commit payloads; mutations also fire-and-forget a search
	// KindIndex job (Enqueue), which is not what these commit-payload tests inspect.
	if kind == KindCommit {
		c.payloads = append(c.payloads, payload)
	}
	return nil
}

// EnqueueAndWait models the synchronous wait-for-commit path: capture the
// payload and return nil (the modeled job is immediately "done") so the service
// returns without standing up a real drain goroutine.
func (c *capturingAllWorker) EnqueueAndWait(_ context.Context, kind, payload string, _ time.Duration) error {
	if kind == KindCommit {
		c.payloads = append(c.payloads, payload)
	}
	return nil
}

// commitFileForSvc writes + commits a file through the real GitStore so a Service
// built with a fake worker still has a HEAD to read (for Save/Restore tests).
func commitFileForSvc(t *testing.T, gs *gitstore.GitStore, r *repo.Repo, path, content, user string) {
	t.Helper()
	if err := r.Write(path, []byte(content)); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
	err := gs.Commit(context.Background(), gitstore.CommitSpec{
		Paths:   []string{path},
		Message: "seed " + path,
		User:    user,
		Action:  "create",
		Source:  "web-ui",
	})
	if err != nil {
		t.Fatalf("seed Commit %q: %v", path, err)
	}
}

// runGitCmd runs a raw git command in dir for test setup.
func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v: %s", args, dir, err, out)
	}
}

// newRepoWithRemote builds a real repo + GitStore configured to push to remoteDir
// (RemoteEnabled+PushOnCommit set), with the repo initialized.
func newRepoWithRemote(t *testing.T, remoteDir string) (*repo.Repo, *gitstore.GitStore) {
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
	gs := gitstore.New(r, config.GitConfig{
		Enabled: true, Branch: "main",
		RemoteEnabled: true, PushOnCommit: true, Remote: remoteDir,
	})
	if err := gs.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return r, gs
}

// remoteCommitCount returns the number of commits on main in a bare remote (0
// when the ref does not yet exist).
func remoteCommitCount(t *testing.T, remoteDir string) int {
	t.Helper()
	cmd := exec.Command("git", "rev-list", "--count", "main")
	cmd.Dir = remoteDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0
	}
	n := 0
	for _, c := range strings.TrimSpace(string(out)) {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
