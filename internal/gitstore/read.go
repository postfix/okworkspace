package gitstore

import (
	"context"
	"fmt"
	"strings"
)

// BlobRevision returns the Git blob SHA of the file at path as it exists in the
// current HEAD commit (`git rev-parse HEAD:<path>`). This is the per-file
// optimistic-concurrency revision (RESEARCH Pattern 3): it changes whenever the
// committed content of that path changes and costs no extra hashing. The path is
// validated through the repo resolver before it is handed to git so no path
// bypasses the SEC-01 chokepoint. An empty string (with nil error) is returned
// when the path does not exist at HEAD (e.g. a never-committed page).
func (g *GitStore) BlobRevision(ctx context.Context, path string) (string, error) {
	if _, err := g.repo.Resolve(path); err != nil {
		return "", fmt.Errorf("gitstore: unsafe revision path %q: %w", path, err)
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// `git rev-parse HEAD:<path>` prints the blob SHA for the path at HEAD, or
	// exits non-zero when HEAD or the path is absent. A missing path is not an
	// error here — it means "no committed revision yet" (empty revision).
	out, err := g.git(ctx, "rev-parse", "--verify", "--quiet", "HEAD:"+path)
	if err != nil {
		// No HEAD or path not present at HEAD: treat as no revision.
		return "", nil
	}
	return strings.TrimSpace(out), nil
}
