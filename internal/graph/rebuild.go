package graph

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/postfix/okworkspace/internal/okf"
)

// trashPrefix is the working-tree location deleted pages move into; pages under
// it are excluded from the graph (live pages only). Mirrors search.trashPrefix /
// pages.trashDir (kept local to avoid importing those packages).
const trashPrefix = ".okf-workspace/trash"

// isTrashed reports whether a workspace-relative path lives under the trash dir.
func isTrashed(path string) bool {
	return path == trashPrefix || strings.HasPrefix(path, trashPrefix+"/")
}

// RebuildGraph rebuilds the derived link/tag adjacency from the files on disk,
// idempotently — the correctness backstop that keeps the cache honest (deleting
// both tables and rebuilding reproduces byte-identical rows). It walks the live
// pages (skipping dirs, non-.md, and anything under the trash prefix), reads each
// through the SEC-01 resolver (repo.Read, never os.*), parses it, and accumulates
// its outbound edges + tags. Edges resolve dangling targets against the SAME
// live-page set the walk produced (so rebuild and incremental upsert agree on
// which links exist). Then in ONE transaction it DELETEs both tables and
// bulk-inserts the accumulated rows. After a successful rewrite it records the
// current HEAD as last_graphed_head (done LAST so a partial rebuild never claims
// an in-sync HEAD). Mirrors search.RebuildIndex's walk + skip-on-error (WR-06).
func (s *Store) RebuildGraph(ctx context.Context) error {
	if s.repo == nil {
		return fmt.Errorf("graph: rebuild requires a content repo (call SetRepo)")
	}

	items, err := s.repo.Tree()
	if err != nil {
		return fmt.Errorf("graph: walk repo: %w", err)
	}

	// First pass: collect the live-page set so link resolution can drop dangling
	// edges using the same set the rebuild walks (rebuild==incremental agreement).
	live := map[string]struct{}{}
	for _, it := range items {
		if it.IsDir || isTrashed(it.Path) || !strings.HasSuffix(it.Path, ".md") {
			continue
		}
		live[it.Path] = struct{}{}
	}
	existsInSet := func(p string) (bool, error) {
		_, ok := live[p]
		return ok, nil
	}

	// Second pass: accumulate edges + tags for every live page.
	type linkRow struct{ src, dst string }
	type tagRow struct{ page, tag string }
	var linkRows []linkRow
	var tagRows []tagRow
	for _, it := range items {
		if it.IsDir || isTrashed(it.Path) || !strings.HasSuffix(it.Path, ".md") {
			continue
		}
		raw, rerr := s.repo.Read(it.Path)
		if rerr != nil {
			// A transient/IO read error on ONE file must not abort the whole rebuild
			// (WR-06): the rebuild is the correctness backstop. Skip it.
			slog.WarnContext(ctx, "graph: skipping unreadable page during rebuild",
				slog.String("path", it.Path), slog.String("error", rerr.Error()))
			continue
		}
		doc, perr := okf.Parse(raw)
		if perr != nil {
			// A malformed page is skipped, not fatal (best-effort over files).
			slog.WarnContext(ctx, "graph: skipping unparseable page during rebuild",
				slog.String("path", it.Path), slog.String("error", perr.Error()))
			continue
		}
		links, lerr := outboundLinks(it.Path, doc.Body, existsInSet)
		if lerr != nil {
			// existsInSet never errors, but be defensive: skip the one page.
			slog.WarnContext(ctx, "graph: skipping page with unresolvable links during rebuild",
				slog.String("path", it.Path), slog.String("error", lerr.Error()))
			continue
		}
		for _, dst := range links {
			linkRows = append(linkRows, linkRow{src: it.Path, dst: dst})
		}
		for _, tag := range pageTags(doc) {
			tagRows = append(tagRows, tagRow{page: it.Path, tag: tag})
		}
	}

	if s.db == nil {
		// No-db harness: nothing to persist (the walk above is still the backstop).
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM page_links`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM page_tags`); err != nil {
		return err
	}
	for _, lr := range linkRows {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO page_links (src_path, dst_path) VALUES (?, ?)`,
			lr.src, lr.dst); err != nil {
			return err
		}
	}
	for _, tr := range tagRows {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO page_tags (page_path, tag) VALUES (?, ?)`,
			tr.page, tr.tag); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("graph: commit rebuild: %w", err)
	}

	// Record the HEAD this rebuild graphed against so the startup DriftCheck only
	// rebuilds on a real mismatch. Done LAST so a partial rebuild never claims an
	// in-sync HEAD it did not fully graph.
	if s.gs != nil {
		if err := s.StoreHead(ctx, s.gs); err != nil {
			return fmt.Errorf("graph: record last-graphed head: %w", err)
		}
	}
	return nil
}
