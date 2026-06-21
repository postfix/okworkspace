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

	// headingRows accumulates the page_headings rows the fresh index implies, so
	// the table is rewritten to match a from-scratch rebuild (the backstop that
	// keeps the incremental cleanup honest).
	headingRows := map[string][]string{}

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
		title := okf.Field(doc, okf.FieldTitle)
		if berr := batch.Index(pageDocID(it.Path), pageDoc(
			it.Path, title, string(doc.Body), readTags(doc),
		)); berr != nil {
			_ = tmpIdx.Close()
			_ = os.RemoveAll(tmp)
			return fmt.Errorf("search: batch index %q: %w", it.Path, berr)
		}
		// Heading sub-docs for the page (deep-link targets).
		for _, hd := range okf.ScanHeadings(doc.Body) {
			id := headingDocID(it.Path, hd.Anchor)
			if berr := batch.Index(id, headingDoc(it.Path, title, hd.Text, hd.Anchor)); berr != nil {
				_ = tmpIdx.Close()
				_ = os.RemoveAll(tmp)
				return fmt.Errorf("search: batch index heading %q: %w", id, berr)
			}
			headingRows[it.Path] = append(headingRows[it.Path], id)
		}
	}

	// Attachment docs (filename + extracted text → owning page). Enumerated from
	// the on-disk meta sidecars; a missing .txt is tolerated (filename-only).
	attIDs, aerr := s.allAttachmentIDs()
	if aerr != nil {
		_ = tmpIdx.Close()
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("search: enumerate attachments: %w", aerr)
	}
	for _, id := range attIDs {
		meta, merr := s.readAttachmentMeta(id)
		if merr != nil {
			// A malformed/missing meta is skipped, not fatal (best-effort over files).
			continue
		}
		extracted := s.readExtractedText(id)
		pageTitle := s.pageTitle(meta.PagePath)
		if berr := batch.Index(attachmentDocID(id),
			attachmentDoc(meta.OriginalName, extracted, meta.PagePath, pageTitle)); berr != nil {
			_ = tmpIdx.Close()
			_ = os.RemoveAll(tmp)
			return fmt.Errorf("search: batch index attachment %q: %w", id, berr)
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

	// Rewrite page_headings to mirror the freshly-built index. Done AFTER the
	// successful swap so a failed rebuild never corrupts the tracking table.
	if err := s.rewriteAllHeadingRows(ctx, headingRows); err != nil {
		return err
	}
	return nil
}

// rewriteAllHeadingRows replaces the entire page_headings table with the rows a
// from-scratch rebuild produced (clearing pages that no longer contribute any
// heading). A no-db harness is a no-op.
func (s *Index) rewriteAllHeadingRows(ctx context.Context, rows map[string][]string) error {
	if s.db == nil {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM page_headings`); err != nil {
		return err
	}
	for pagePath, ids := range rows {
		for _, id := range ids {
			if _, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO page_headings (page_path, heading_id) VALUES (?, ?)`,
				pagePath, id); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
