package search

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blevesearch/bleve/v2"

	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/okf"
	"github.com/postfix/okworkspace/internal/repo"
)

// KindIndex is the job kind registered on the EXISTING single jobs worker for
// search index maintenance. Mutation code paths enqueue it FIRE-AND-FORGET
// (worker.Enqueue, never EnqueueAndWait from inside a handler — the CR-01 deadlock
// lesson) so a page save / attachment change / startup-drift triggers an index
// update without blocking the single drain goroutine.
const KindIndex = "index"

// indexPayload is the JSON enqueued for a KindIndex job. Op selects the action;
// Kind/Path/ID identify the document. For a page upsert/delete, Path is the page
// path (and the doc ID). A rebuild ignores the other fields.
type indexPayload struct {
	Op   string `json:"op"`   // "upsert" | "delete" | "rebuild"
	Kind string `json:"kind"` // "page" | "attachment"
	Path string `json:"path"` // page path
	ID   string `json:"id"`   // attachment id (for attachment ops) / page id (delete)
}

// RebuildPayload returns the JSON payload for a KindIndex rebuild job. Startup
// drift recovery and the admin reindex endpoint both enqueue this.
func RebuildPayload() string {
	raw, _ := json.Marshal(indexPayload{Op: "rebuild"})
	return string(raw)
}

// UpsertPagePayload / DeletePagePayload build the payloads 03-03's mutation hooks
// will enqueue. Defined here so the payload shape is owned by one package.
func UpsertPagePayload(pagePath string) string {
	raw, _ := json.Marshal(indexPayload{Op: "upsert", Kind: TypePage, Path: pagePath})
	return string(raw)
}

func DeletePagePayload(pagePath string) string {
	raw, _ := json.Marshal(indexPayload{Op: "delete", Kind: TypePage, Path: pagePath, ID: pagePath})
	return string(raw)
}

// UpsertAttachmentPayload / DeleteAttachmentPayload build the payloads 03-04's
// attachment mutation hooks will enqueue. ID is the attachment id; the doc id is
// namespaced ("att:"+id) by the handler.
func UpsertAttachmentPayload(id string) string {
	raw, _ := json.Marshal(indexPayload{Op: "upsert", Kind: TypeAttachment, ID: id})
	return string(raw)
}

func DeleteAttachmentPayload(id string) string {
	raw, _ := json.Marshal(indexPayload{Op: "delete", Kind: TypeAttachment, ID: id})
	return string(raw)
}

// IndexHandler returns the jobs.Handler registered for KindIndex (mirrors
// attachments.ExtractHandler's constructor shape). It attaches the content repo to
// the index (so subsequent rebuilds read files through the SEC-01 resolver) and
// dispatches each job by Op. The whole body runs under a defer recover() because
// Bleve can panic on a corrupt segment (Pitfall 3) — a panic becomes a returned
// error so the single drain goroutine survives (the same defense ExtractHandler
// uses). This handler NEVER re-enqueues; if it ever did, it would use w.Enqueue
// (fire-and-forget), never EnqueueAndWait (CR-01).
func IndexHandler(idx *Index, r *repo.Repo) jobs.Handler {
	if idx.repo == nil {
		idx.SetRepo(r)
	}
	return func(ctx context.Context, payload string) (err error) {
		defer func() {
			if rec := recover(); rec != nil {
				err = fmt.Errorf("search: index handler panic: %v", rec)
			}
		}()

		var p indexPayload
		if uerr := json.Unmarshal([]byte(payload), &p); uerr != nil {
			return fmt.Errorf("search: index payload: %w", uerr)
		}

		switch p.Op {
		case "rebuild":
			return idx.RebuildIndex(ctx)
		case "upsert":
			switch p.Kind {
			case TypePage:
				return idx.indexPage(ctx, p.Path)
			case TypeAttachment:
				return idx.indexAttachment(p.ID)
			default:
				// An unknown kind is a no-op so a stray payload never wedges the worker.
				return nil
			}
		case "delete":
			id := p.ID
			if id == "" {
				id = p.Path
			}
			switch p.Kind {
			case TypeAttachment:
				return idx.deleteAttachment(id)
			case TypePage, "":
				return idx.deletePage(ctx, id)
			default:
				return nil
			}
		default:
			return fmt.Errorf("search: unknown index op %q", p.Op)
		}
	}
}

// indexPage upserts a single page document (Index with the page path as the ID
// overwrites in place). It reads the file through the resolver, parses the
// frontmatter for title + sequence-aware tags, indexes the opaque body, and
// re-indexes the page's heading sub-documents (deleting any stale ones via the
// page_headings prior-set discipline) so heading deep-links stay fresh.
func (s *Index) indexPage(ctx context.Context, pagePath string) error {
	if s.repo == nil {
		return fmt.Errorf("search: indexPage requires a content repo")
	}
	raw, err := s.repo.Read(pagePath)
	if err != nil {
		return fmt.Errorf("search: read page %q: %w", pagePath, err)
	}
	doc, err := okf.Parse(raw)
	if err != nil {
		return fmt.Errorf("search: parse page %q: %w", pagePath, err)
	}
	title := okf.Field(doc, okf.FieldTitle)
	d := pageDoc(pagePath, title, string(doc.Body), readTags(doc))
	if err := s.withIndex(func(idx bleve.Index) error {
		return idx.Index(pageDocID(pagePath), d)
	}); err != nil {
		return err
	}
	return s.indexHeadings(ctx, pagePath, title, doc.Body)
}

// deletePage removes a page document by ID and clears its heading sub-documents
// (and their page_headings rows) so a deleted page leaves no stale heading
// deep-links behind.
func (s *Index) deletePage(ctx context.Context, id string) error {
	if err := s.withIndex(func(idx bleve.Index) error {
		return idx.Delete(id)
	}); err != nil {
		return err
	}
	return s.deleteHeadings(ctx, id)
}
