// Package gitstore is the single-writer Git service for OKF Workspace. It shells
// out to the host `git` CLI (LOCKED decision, CLAUDE.md) via exec.Command with
// explicit argument slices — never a shell string — always checking exit codes
// and capturing stderr. All repo writes funnel through Commit, serialized by an
// internal mutex so no two git invocations contend on .git/index.lock
// (PITFALLS.md Pitfall 2). Push is DEFERRED to Phase 1.
package gitstore

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/repo"
)

// GitStore owns serialized access to the Git repository at repo.Root().
type GitStore struct {
	repo *repo.Repo
	cfg  config.GitConfig

	// mu serializes every git invocation — this is the single-writer guard.
	// It also guards the health flags below.
	mu sync.Mutex

	// diverged is set when PullOnStartup refused a non-fast-forward pull; healed
	// is set when SelfHealStaleLock cleared a stale lock. Both feed Health.
	diverged bool
	healed   bool
}

// New constructs a GitStore over the safe-path repo and git config.
func New(r *repo.Repo, cfg config.GitConfig) *GitStore {
	branch := cfg.Branch
	if branch == "" {
		branch = "main"
		cfg.Branch = branch
	}
	return &GitStore{repo: r, cfg: cfg}
}

// git runs `git <args...>` in the repo root and returns combined stdout, or an
// error wrapping stderr. Arguments are passed as a slice (no shell), so user
// input is never interpolated into a command string (T-00.02-06).
func (g *GitStore) git(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.repo.Root()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("git %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// isRepo reports whether the root is already a Git work tree.
func (g *GitStore) isRepo(ctx context.Context) bool {
	out, err := g.git(ctx, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// Init initializes the Git repository at the repo root if it is not already one
// (idempotent), setting the default branch from git.branch and a local
// identity so commits succeed even when no global git identity is configured.
func (g *GitStore) Init(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.isRepo(ctx) {
		return nil
	}
	if _, err := g.git(ctx, "init", "-b", g.cfg.Branch); err != nil {
		// Older git without `-b`: fall back to init + symbolic-ref.
		if _, err2 := g.git(ctx, "init"); err2 != nil {
			return fmt.Errorf("git init: %w", err)
		}
		_, _ = g.git(ctx, "symbolic-ref", "HEAD", "refs/heads/"+g.cfg.Branch)
	}
	// A repo-local identity guarantees commits work on a fresh box without a
	// global ~/.gitconfig. Per-commit author is overridden in Commit.
	if _, err := g.git(ctx, "config", "user.name", "OKF Workspace"); err != nil {
		return fmt.Errorf("git config user.name: %w", err)
	}
	if _, err := g.git(ctx, "config", "user.email", "okf-workspace@localhost"); err != nil {
		return fmt.Errorf("git config user.email: %w", err)
	}
	return nil
}

// PullOnStartup performs a fast-forward-ONLY pull when a remote is enabled and
// pull_on_startup is set. On divergence it does NOT merge — Health surfaces the
// diverged flag and the caller alerts (D-12). When the remote is disabled this
// is a no-op (no network access). Push is deferred to Phase 1.
func (g *GitStore) PullOnStartup(ctx context.Context) error {
	if !g.cfg.RemoteEnabled || !g.cfg.PullOnStartup {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.cfg.Remote == "" {
		return nil
	}
	// Fetch then attempt a fast-forward-only merge. --ff-only refuses to create
	// a merge commit on divergence (returns non-zero), protecting local data.
	if _, err := g.git(ctx, "fetch", g.cfg.Remote, g.cfg.Branch); err != nil {
		return fmt.Errorf("git fetch: %w", err)
	}
	if _, err := g.git(ctx, "merge", "--ff-only", "FETCH_HEAD"); err != nil {
		// Divergence: leave the working tree untouched. Health reports diverged
		// so the caller can surface the warning banner (D-12); never auto-merge.
		g.diverged = true
		return nil
	}
	return nil
}
