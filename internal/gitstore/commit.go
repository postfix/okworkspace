package gitstore

import (
	"context"
	"fmt"
	"strings"
)

// CommitSpec describes one commit through the single-writer service. Every
// commit carries User/Action/Source provenance (SPEC §14.2): User drives the
// Git author identity; Action and Source are embedded in the message body so
// the hidden Git history is an audit trail (T-00.02-07).
type CommitSpec struct {
	Paths   []string // repo-relative paths to stage (resolved before staging)
	Message string   // human-readable subject line
	User    string   // acting user (becomes the Git author name)
	Action  string   // e.g. "seed", "edit", "agent-apply"
	Source  string   // e.g. "bootstrap", "ui", "agent"
}

// Commit stages the spec's paths and creates exactly one commit, serialized by
// the single-writer mutex so no concurrent git invocation contends on
// index.lock. Each path is validated through the repo resolver before staging
// (no path bypasses the SEC-01 chokepoint).
func (g *GitStore) Commit(ctx context.Context, spec CommitSpec) error {
	if len(spec.Paths) == 0 {
		return fmt.Errorf("gitstore: commit requires at least one path")
	}
	if strings.TrimSpace(spec.Message) == "" {
		return fmt.Errorf("gitstore: commit requires a message")
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// Validate + stage each path through the resolver. We add by the relative
	// path (git runs in the repo root); Resolve enforces the path is in-root.
	addArgs := []string{"add", "--"}
	for _, p := range spec.Paths {
		if _, err := g.repo.Resolve(p); err != nil {
			return fmt.Errorf("gitstore: unsafe commit path %q: %w", p, err)
		}
		addArgs = append(addArgs, p)
	}
	if _, err := g.git(ctx, addArgs...); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	author := spec.User
	if author == "" {
		author = "OKF Workspace"
	}
	message := buildMessage(spec)

	args := []string{
		"-c", "user.name=" + author,
		"-c", "user.email=" + authorEmail(author),
		"commit",
		"--author", fmt.Sprintf("%s <%s>", author, authorEmail(author)),
		"-m", message,
	}
	if _, err := g.git(ctx, args...); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	return nil
}

// buildMessage renders the commit subject plus an action/source trailer so the
// provenance is recoverable from `git log` (SPEC §14.2).
func buildMessage(spec CommitSpec) string {
	var b strings.Builder
	b.WriteString(spec.Message)
	b.WriteString("\n\n")
	if spec.Action != "" {
		fmt.Fprintf(&b, "Action: %s\n", spec.Action)
	}
	if spec.Source != "" {
		fmt.Fprintf(&b, "Source: %s\n", spec.Source)
	}
	if spec.User != "" {
		fmt.Fprintf(&b, "User: %s\n", spec.User)
	}
	return b.String()
}

// authorEmail derives a stable synthetic email for the acting user so commits
// are attributable without requiring real email addresses.
func authorEmail(user string) string {
	slug := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(user), " ", "-"))
	if slug == "" {
		slug = "okf-workspace"
	}
	return slug + "@okf-workspace.local"
}
