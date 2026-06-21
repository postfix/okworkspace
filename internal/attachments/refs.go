package attachments

import (
	"context"
	"fmt"
	"strings"

	"github.com/postfix/okworkspace/internal/repo"
)

// PageReferences counts how many pages reference the attachment id by scanning
// every page's Markdown for the canonical DownloadRefPath(id) substring. The
// orphan-delete logic of slice 02-04 calls this: when the count hits zero on
// unlink, the binary + both sidecars are removed in ONE commit (ATT-07). Defining
// the match against DownloadRefPath here (the same string the upload-time insert
// uses) guarantees insert and scan can never drift (Pitfall 6).
//
// Slice 02-01 ships the canonical-match implementation now so the contract is
// fixed; the remove/orphan-delete wiring that consumes it lands in 02-04.
func PageReferences(ctx context.Context, r *repo.Repo, id string) (int, error) {
	return PageReferencesExcluding(ctx, r, id, "")
}

// PageReferencesExcluding is PageReferences but skips a single page (excludePath)
// when counting. Remove uses it after unlinking pagePath so the recount NEVER
// depends on a possibly-stale working-tree read of the page it just edited (WR-02):
// the unlink commit may not have landed on disk yet (enqueueCommit soft-succeeds
// on ErrJobTimeout), so re-reading pagePath could still find the old link and
// keep a now-orphaned file. The caller knows pagePath is unlinked (the edited body
// no longer contains the canonical ref, or the link was already absent), so we
// treat pagePath as definitively non-referencing and count remaining references
// from the OTHER pages only. excludePath == "" excludes nothing (the original
// whole-tree count).
func PageReferencesExcluding(ctx context.Context, r *repo.Repo, id, excludePath string) (int, error) {
	ref := DownloadRefPath(id)
	items, err := r.Tree()
	if err != nil {
		return 0, fmt.Errorf("attachments: scan page references: %w", err)
	}
	count := 0
	for _, it := range items {
		if it.IsDir || !strings.HasSuffix(it.Path, ".md") {
			continue
		}
		if excludePath != "" && it.Path == excludePath {
			// The just-unlinked page is treated as definitively non-referencing,
			// regardless of commit-wait latency on its unlink edit (WR-02).
			continue
		}
		raw, err := r.Read(it.Path)
		if err != nil {
			return 0, fmt.Errorf("attachments: read page %q for ref scan: %w", it.Path, err)
		}
		if strings.Contains(string(raw), ref) {
			count++
		}
	}
	return count, nil
}
