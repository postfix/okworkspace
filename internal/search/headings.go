package search

import (
	"context"

	"github.com/blevesearch/bleve/v2"

	"github.com/postfix/okworkspace/internal/okf"
)

// indexHeadings (re)indexes the heading sub-documents for a page. It scans the
// opaque body for ATX headings, then — because Bleve has no delete-by-prefix
// (Pitfall 5) — reads the page's PRIOR heading-id set from the page_headings
// table, deletes each stale heading doc, indexes the fresh set, and rewrites the
// page_headings rows. The rebuild path starts from an empty index so it never
// needs the prior-set delete; this prior-set discipline is what keeps the
// INCREMENTAL upsert path free of stale heading docs after a rename/removal.
func (s *Index) indexHeadings(ctx context.Context, pagePath, pageTitle string, body []byte) error {
	prior, err := s.priorHeadingIDs(ctx, pagePath)
	if err != nil {
		return err
	}

	headings := okf.ScanHeadings(body)
	fresh := make([]string, 0, len(headings))
	freshSet := make(map[string]struct{}, len(headings))

	if err := s.withIndex(func(idx bleve.Index) error {
		// Delete stale heading docs no longer present in the new set.
		for id := range prior {
			if err := idx.Delete(id); err != nil {
				return err
			}
		}
		for _, hd := range headings {
			id := headingDocID(pagePath, hd.Anchor)
			if _, dup := freshSet[id]; dup {
				continue // anchor de-dup guarantees uniqueness, but be defensive
			}
			if err := idx.Index(id, headingDoc(pagePath, pageTitle, hd.Text, hd.Anchor)); err != nil {
				return err
			}
			freshSet[id] = struct{}{}
			fresh = append(fresh, id)
		}
		return nil
	}); err != nil {
		return err
	}

	return s.replaceHeadingRows(ctx, pagePath, fresh)
}

// deleteHeadings removes every heading doc a page contributed and clears its
// page_headings rows (page delete path).
func (s *Index) deleteHeadings(ctx context.Context, pagePath string) error {
	prior, err := s.priorHeadingIDs(ctx, pagePath)
	if err != nil {
		return err
	}
	if err := s.withIndex(func(idx bleve.Index) error {
		for id := range prior {
			if err := idx.Delete(id); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return s.replaceHeadingRows(ctx, pagePath, nil)
}

// priorHeadingIDs returns the set of heading doc ids currently recorded for a
// page in page_headings. An empty set (or no db) yields an empty map.
func (s *Index) priorHeadingIDs(ctx context.Context, pagePath string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	if s.db == nil {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT heading_id FROM page_headings WHERE page_path=?`, pagePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

// replaceHeadingRows replaces the page_headings rows for a page with ids (which
// may be empty to clear them). A no-db harness is a no-op.
func (s *Index) replaceHeadingRows(ctx context.Context, pagePath string, ids []string) error {
	if s.db == nil {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM page_headings WHERE page_path=?`, pagePath); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO page_headings (page_path, heading_id) VALUES (?, ?)`, pagePath, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
