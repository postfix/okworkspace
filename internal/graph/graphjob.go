package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/okf"
	"github.com/postfix/okworkspace/internal/repo"
)

// KindGraph is the job kind registered on the EXISTING single jobs worker for
// link/tag graph maintenance. Mutation code paths enqueue it FIRE-AND-FORGET
// (worker.Enqueue, never EnqueueAndWait from inside a handler — the CR-01 deadlock
// lesson) so a page save / delete / startup-drift triggers a graph update without
// blocking the single drain goroutine. Mirrors search.KindIndex exactly.
const KindGraph = "graph"

// graphPayload is the JSON enqueued for a KindGraph job. Op selects the action;
// Path identifies the page for an upsert/delete. A rebuild ignores Path. The
// payload shape is owned by this package (mirrors search.indexPayload).
type graphPayload struct {
	Op   string `json:"op"`   // "upsert" | "delete" | "rebuild"
	Path string `json:"path"` // page path (upsert/delete)
}

// RebuildPayload returns the JSON payload for a KindGraph rebuild job. Startup
// drift recovery and the admin rebuild endpoint (08-03) both enqueue this.
func RebuildPayload() string {
	raw, _ := json.Marshal(graphPayload{Op: "rebuild"})
	return string(raw)
}

// UpsertPagePayload / DeletePagePayload build the payloads 08-02's mutation hooks
// will enqueue. Defined here so the payload shape is owned by one package.
func UpsertPagePayload(pagePath string) string {
	raw, _ := json.Marshal(graphPayload{Op: "upsert", Path: pagePath})
	return string(raw)
}

func DeletePagePayload(pagePath string) string {
	raw, _ := json.Marshal(graphPayload{Op: "delete", Path: pagePath})
	return string(raw)
}

// GraphHandler returns the jobs.Handler registered for KindGraph (mirrors
// search.IndexHandler's constructor shape). It attaches the content repo to the
// store (so rebuilds/upserts read files through the SEC-01 resolver) and
// dispatches each job by Op. The whole body runs under a defer recover() so a
// parser/SQLite panic on one corrupt page becomes a returned error — the single
// drain goroutine survives (T-08-02). This handler NEVER re-enqueues; if it ever
// did, it would use w.Enqueue (fire-and-forget), never EnqueueAndWait (CR-01).
func GraphHandler(store *Store, r *repo.Repo) jobs.Handler {
	if store.repo == nil {
		store.SetRepo(r)
	}
	return func(ctx context.Context, payload string) (err error) {
		defer func() {
			if rec := recover(); rec != nil {
				err = fmt.Errorf("graph: handler panic: %v", rec)
			}
		}()

		var p graphPayload
		if uerr := json.Unmarshal([]byte(payload), &p); uerr != nil {
			return fmt.Errorf("graph: payload: %w", uerr)
		}

		switch p.Op {
		case "rebuild":
			return store.RebuildGraph(ctx)
		case "upsert":
			return store.upsertPage(ctx, p.Path)
		case "delete":
			return store.deletePage(ctx, p.Path)
		default:
			return fmt.Errorf("graph: unknown op %q", p.Op)
		}
	}
}

// upsertPage re-scans one page and atomically replaces ONLY that page's rows: its
// outbound link rows (src_path=path) and its tag rows (page_path=path). It reads
// the file through the resolver; a page that no longer exists on disk is treated
// as a delete (a stale upsert for a removed page is a no-op delete). A parse error
// on the one page is logged and swallowed (best-effort over files, like search).
func (s *Store) upsertPage(ctx context.Context, pagePath string) error {
	if s.repo == nil {
		return fmt.Errorf("graph: upsertPage requires a content repo")
	}
	if s.db == nil {
		return nil
	}

	exists, err := s.repo.Exists(pagePath)
	if err != nil {
		return fmt.Errorf("graph: stat page %q: %w", pagePath, err)
	}
	if !exists {
		// Stale upsert for a removed page: clear its rows (no-op delete).
		return s.deletePage(ctx, pagePath)
	}

	raw, err := s.repo.Read(pagePath)
	if err != nil {
		return fmt.Errorf("graph: read page %q: %w", pagePath, err)
	}
	doc, perr := okf.Parse(raw)
	if perr != nil {
		// A malformed page is skipped, not fatal (best-effort over files). Leave the
		// existing rows in place rather than clobbering on a transient parse error.
		slog.WarnContext(ctx, "graph: skipping unparseable page during upsert",
			slog.String("path", pagePath), slog.String("error", perr.Error()))
		return nil
	}

	links, err := outboundLinks(pagePath, doc.Body, s.repo.Exists)
	if err != nil {
		return fmt.Errorf("graph: resolve links for %q: %w", pagePath, err)
	}
	tags := pageTags(doc)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM page_links WHERE src_path=?`, pagePath); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM page_tags WHERE page_path=?`, pagePath); err != nil {
		return err
	}
	for _, dst := range links {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO page_links (src_path, dst_path) VALUES (?, ?)`,
			pagePath, dst); err != nil {
			return err
		}
	}
	for _, tag := range tags {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO page_tags (page_path, tag) VALUES (?, ?)`,
			pagePath, tag); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// deletePage removes a page from the graph in one transaction: its outbound AND
// inbound link rows (page_links WHERE src_path=path OR dst_path=path) and its tag
// rows (page_tags WHERE page_path=path). Edges from other pages pointing at the
// deleted page are removed too, so no dangling backlink survives.
func (s *Store) deletePage(ctx context.Context, pagePath string) error {
	if s.db == nil {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM page_links WHERE src_path=? OR dst_path=?`, pagePath, pagePath); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM page_tags WHERE page_path=?`, pagePath); err != nil {
		return err
	}
	return tx.Commit()
}
