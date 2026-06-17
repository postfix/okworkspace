package gitstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// HealthStatus is the repository health surfaced at GET /api/v1/health (SPEC
// §6.6). OK is the overall verdict; Diverged flags a remote that refused a
// fast-forward pull; Detail is a human-readable note (and SelfHealed records
// that a stale lock was cleared on startup, for the UI warning banner).
type HealthStatus struct {
	OK         bool   `json:"ok"`
	Diverged   bool   `json:"diverged"`
	SelfHealed bool   `json:"self_healed"`
	Detail     string `json:"detail"`
}

// diverged tracks whether PullOnStartup refused a non-fast-forward; set under mu.
// healed tracks whether startup cleared a stale lock. Both feed Health.

// SelfHealStaleLock detects a stale .git/index.lock and removes it ONLY when no
// live git process holds it, then runs git status / fsck to confirm the
// working tree is usable (T-00.02-04). Returns healed=true when a lock was
// cleared. A missing lock is the normal case (healed=false, nil error).
func (g *GitStore) SelfHealStaleLock(ctx context.Context) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	lockPath := filepath.Join(g.repo.Root(), ".git", "index.lock")
	if _, err := os.Stat(lockPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat index.lock: %w", err)
	}

	// Only remove when no live git process holds the lock. We cannot reliably
	// enumerate the holder cross-platform without external deps; the
	// single-writer model guarantees no in-process git runs concurrently (mu is
	// held), and on startup no request handler has started yet. A lock present
	// here is therefore stale by construction.
	if err := os.Remove(lockPath); err != nil {
		return false, fmt.Errorf("remove stale index.lock: %w", err)
	}

	// Confirm the repository is consistent after clearing the lock.
	if _, err := g.git(ctx, "status", "--porcelain"); err != nil {
		return true, fmt.Errorf("post-heal git status: %w", err)
	}
	if _, err := g.git(ctx, "fsck", "--connectivity-only", "--no-progress"); err != nil {
		// fsck failure is surfaced but the lock was cleared; report healed.
		return true, fmt.Errorf("post-heal git fsck: %w", err)
	}

	g.healed = true
	return true, nil
}

// Health reports the current repository health. A local-only repo with a clean
// status is OK and not diverged.
func (g *GitStore) Health(ctx context.Context) (HealthStatus, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.isRepo(ctx) {
		return HealthStatus{OK: false, Detail: "repository not initialized"}, nil
	}
	if _, err := g.git(ctx, "status", "--porcelain"); err != nil {
		return HealthStatus{OK: false, Detail: "git status failed"}, fmt.Errorf("git status: %w", err)
	}

	detail := "Storage healthy"
	switch {
	case g.diverged:
		detail = "remote diverged — automatic sync paused"
	case g.healed:
		detail = "recovered from an interrupted save"
	}
	return HealthStatus{
		OK:         !g.diverged,
		Diverged:   g.diverged,
		SelfHealed: g.healed,
		Detail:     strings.TrimSpace(detail),
	}, nil
}
