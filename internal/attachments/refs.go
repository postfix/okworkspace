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
