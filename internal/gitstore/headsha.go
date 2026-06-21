package gitstore

import (
	"context"
	"strings"
)

// HeadSHA returns the full commit SHA of the current HEAD (`git rev-parse HEAD`).
// It is the drift-bookkeeping accessor (RESEARCH Open Question Q1): the search
// index persists the last-indexed HEAD and compares it to this value on startup
// to detect that the working tree moved out-of-band (a pull, a restore, a crash
// between commit and index) and trigger a rebuild-from-files.
//
// It mirrors BlobRevision (read-only, serialized under the single-writer mu) and
// the empty-repo handling of IsEmpty: when there is no HEAD yet (a brand-new repo
// with zero commits) it returns an empty string with a nil error, so callers can
// treat "no HEAD" as "no last-indexed value" without special-casing the error.
func (g *GitStore) HeadSHA(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	out, err := g.git(ctx, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		// No HEAD yet (empty repo) or rev-parse failed: treat as "no revision".
		return "", nil
	}
	return strings.TrimSpace(out), nil
}
