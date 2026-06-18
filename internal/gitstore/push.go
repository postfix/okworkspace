package gitstore

import (
	"context"
	"fmt"
	"strings"
)

// Push sends local commits to the configured remote after a commit (VER-04),
// reusing the Phase-0 ff-only / alert-on-divergence semantics (D-12). It is a
// no-op (nil error, no network) unless a remote is enabled, push_on_commit is
// set, AND a remote is configured — so a deployment with no remote never touches
// the network. On a non-fast-forward rejection (the remote has commits we do not
// have) it sets the diverged flag and returns nil: the system ALERTS through
// Health and never overwrites the remote (no rewrite flags) or auto-merges, so
// remote data is never clobbered (T-05-05). The push uses the existing g.git
// arg-slice wrapper, so the remote/branch are never interpolated into a shell.
func (g *GitStore) Push(ctx context.Context) error {
	if !g.cfg.RemoteEnabled || !g.cfg.PushOnCommit || g.cfg.Remote == "" {
		return nil
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// Plain `git push <remote> <branch>` — no rewrite flag is ever passed. A
	// non-ff push is rejected by git (non-zero exit); we treat that as divergence
	// and alert (never overwrite the remote).
	if _, err := g.git(ctx, "push", g.cfg.Remote, g.cfg.Branch); err != nil {
		if isNonFastForward(err) {
			// Divergence: the remote moved ahead. Do not overwrite the remote and
			// do not auto-merge. Flag diverged so Health surfaces the warning
			// banner (D-12 parity with PullOnStartup); return nil so the local
			// commit still succeeds (the save is not lost).
			g.diverged = true
			return nil
		}
		return fmt.Errorf("git push: %w", err)
	}
	return nil
}

// isNonFastForward reports whether a push error is a non-fast-forward / rejected
// push (the divergence case) rather than a transport/auth failure. git prints
// "rejected" and "non-fast-forward" (or "fetch first") to stderr on this case,
// which g.git folds into the wrapped error string.
func isNonFastForward(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "non-fast-forward") ||
		strings.Contains(msg, "rejected") ||
		strings.Contains(msg, "fetch first")
}
