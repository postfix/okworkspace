package search

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/blevesearch/bleve/v2"

	"github.com/postfix/okworkspace/internal/okf"
)

// trashPrefix is the working-tree location deleted pages move into; pages under it
// are excluded from the index (live pages only — Area 4 / T-03-02). Mirrors
// pages.trashDir (kept as a local constant to avoid importing the pages package).
const trashPrefix = ".okf-workspace/trash"

// isTrashed reports whether a workspace-relative path lives under the trash dir.
func isTrashed(path string) bool {
	return path == trashPrefix || strings.HasPrefix(path, trashPrefix+"/")
}

// RebuildIndex rebuilds the index from the files on disk, idempotently. It builds
// a NEW index into a sibling <dir>.tmp, walks the live pages (skipping dirs,
// non-.md files, and anything under the trash prefix), reads each through the
// SEC-01 resolver (repo.Read, never os.*), parses it, batch-indexes the page doc,
// then atomically swaps the tmp dir into place. A fresh tmp each run makes the
// result deterministic regardless of the prior index state. After a successful
// swap it records the current HEAD as last_indexed_head.
func (s *Index) RebuildIndex(ctx context.Context) error {
	if s.repo == nil {
		return fmt.Errorf("search: rebuild requires a content repo (call SetRepo)")
	}

	tmp := s.dir + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return fmt.Errorf("search: clear tmp index dir: %w", err)
	}
	tmpIdx, err := bleve.New(tmp, buildMapping())
	if err != nil {
		return fmt.Errorf("search: create tmp index: %w", err)
	}

	items, err := s.repo.Tree()
	if err != nil {
		_ = tmpIdx.Close()
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("search: walk repo: %w", err)
	}

	batch := tmpIdx.NewBatch()
	for _, it := range items {
		if it.IsDir || isTrashed(it.Path) || !strings.HasSuffix(it.Path, ".md") {
			continue
		}
		raw, rerr := s.repo.Read(it.Path)
		if rerr != nil {
			_ = tmpIdx.Close()
			_ = os.RemoveAll(tmp)
			return fmt.Errorf("search: read %q: %w", it.Path, rerr)
		}
		doc, perr := okf.Parse(raw)
		if perr != nil {
			// A malformed page is skipped, not fatal: a single bad file must not
			// abort the whole rebuild (the index is best-effort over files).
			continue
		}
		if berr := batch.Index(pageDocID(it.Path), pageDoc(
			it.Path, okf.Field(doc, okf.FieldTitle), string(doc.Body), readTags(doc),
		)); berr != nil {
			_ = tmpIdx.Close()
			_ = os.RemoveAll(tmp)
			return fmt.Errorf("search: batch index %q: %w", it.Path, berr)
		}
	}
	if berr := tmpIdx.Batch(batch); berr != nil {
		_ = tmpIdx.Close()
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("search: apply rebuild batch: %w", berr)
	}
	if cerr := tmpIdx.Close(); cerr != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("search: close tmp index: %w", cerr)
	}

	if serr := s.swapDir(tmp); serr != nil {
		_ = os.RemoveAll(tmp)
		return serr
	}
	return nil
}
