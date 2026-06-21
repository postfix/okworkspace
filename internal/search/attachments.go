package search

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/blevesearch/bleve/v2"

	"github.com/postfix/okworkspace/internal/okf"
)

// attachmentDirPrefix is the flat repo-root directory holding every attachment's
// three artifacts (<id>.<ext> binary, <id>.json meta, <id>.txt extracted text).
// Kept as a local constant so the search package does not import the attachments
// package (avoids a heavy cross-package dependency for two path shapes).
const attachmentDirPrefix = "attachments/"

// attachmentMeta is the subset of the <id>.json meta sidecar search needs: the
// original filename (SRCH-04) and the owning page path (SRCH-05 — links the
// attachment result to the page it belongs to, no tree scan). The on-disk meta
// is owned by the attachments package; only these fields are read here.
type attachmentMeta struct {
	ID           string `json:"id"`
	OriginalName string `json:"original_name"`
	PagePath     string `json:"page_path"`
}

// metaPath / txtPath mirror attachments.MetaPath / attachments.TxtPath without
// importing that package.
func metaPath(id string) string { return attachmentDirPrefix + id + ".json" }
func txtPath(id string) string  { return attachmentDirPrefix + id + ".txt" }

// indexAttachment upserts a single attachment document. It reads the meta
// sidecar (for the original filename + owning page path) through the SEC-01
// resolver, reads the extracted text if present (tolerating a missing .txt for a
// pending/empty extraction — filename-only indexing in that case), resolves the
// owning page's title, and indexes under the namespaced "att:"+id id.
func (s *Index) indexAttachment(id string) error {
	if s.repo == nil {
		return fmt.Errorf("search: indexAttachment requires a content repo")
	}
	meta, err := s.readAttachmentMeta(id)
	if err != nil {
		return err
	}
	extracted := s.readExtractedText(id)
	pageTitle := s.pageTitle(meta.PagePath)
	doc := attachmentDoc(meta.OriginalName, extracted, meta.PagePath, pageTitle)
	return s.withIndex(func(idx bleve.Index) error {
		return idx.Index(attachmentDocID(id), doc)
	})
}

// deleteAttachment removes an attachment document by id.
func (s *Index) deleteAttachment(id string) error {
	return s.withIndex(func(idx bleve.Index) error {
		return idx.Delete(attachmentDocID(id))
	})
}

// readAttachmentMeta reads and parses an attachment's <id>.json sidecar via the
// resolver.
func (s *Index) readAttachmentMeta(id string) (attachmentMeta, error) {
	raw, err := s.repo.Read(metaPath(id))
	if err != nil {
		return attachmentMeta{}, fmt.Errorf("search: read attachment meta %q: %w", id, err)
	}
	var m attachmentMeta
	if err := json.Unmarshal(raw, &m); err != nil {
		return attachmentMeta{}, fmt.Errorf("search: parse attachment meta %q: %w", id, err)
	}
	return m, nil
}

// readExtractedText reads an attachment's <id>.txt sidecar, returning "" when it
// is absent (pending/empty extraction) — a missing .txt is NOT an error
// (T-03-11): the attachment is still indexed by filename.
func (s *Index) readExtractedText(id string) string {
	exists, err := s.repo.Exists(txtPath(id))
	if err != nil || !exists {
		return ""
	}
	raw, err := s.repo.Read(txtPath(id))
	if err != nil {
		return ""
	}
	return string(raw)
}

// pageTitle resolves a page's frontmatter title, returning "" when the page is
// missing or unparsable (the result then carries an empty page_title rather than
// failing the whole index operation).
func (s *Index) pageTitle(pagePath string) string {
	if pagePath == "" {
		return ""
	}
	raw, err := s.repo.Read(pagePath)
	if err != nil {
		return ""
	}
	doc, err := okf.Parse(raw)
	if err != nil {
		return ""
	}
	return okf.Field(doc, okf.FieldTitle)
}

// allAttachmentIDs enumerates the attachment ids present on disk by listing the
// <id>.json meta sidecars under the attachments directory (skipping the .txt and
// binary artifacts). Used by the rebuild walk to index every attachment doc.
func (s *Index) allAttachmentIDs() ([]string, error) {
	items, err := s.repo.Tree()
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, it := range items {
		if it.IsDir || !strings.HasPrefix(it.Path, attachmentDirPrefix) {
			continue
		}
		name := strings.TrimPrefix(it.Path, attachmentDirPrefix)
		if strings.Contains(name, "/") {
			continue // nested dirs are not attachment artifacts
		}
		if id, ok := strings.CutSuffix(name, ".json"); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
