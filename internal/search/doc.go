package search

// Deterministic document IDs. A page's ID is its workspace-relative path so an
// upsert (Index with the same ID) overwrites in place and a delete (Delete by ID)
// removes it. Attachment and heading IDs are namespaced to avoid colliding with a
// page path. A heading's ID is the page path concatenated with the (#-prefixed)
// anchor, which is stable for a given heading and unique within a page (the slug
// de-dup guarantees per-page anchor uniqueness).
func pageDocID(pagePath string) string { return pagePath }
func attachmentDocID(id string) string { return "att:" + id }
func headingDocID(pagePath, anchor string) string {
	return pagePath + anchor
}

// pageDoc builds the indexable map for a page document. body is the opaque
// okf.Doc.Body text (indexed verbatim, never re-emitted); tags are read
// sequence-aware from frontmatter (Pitfall 7).
func pageDoc(pagePath, title, body string, tags []string) map[string]interface{} {
	return map[string]interface{}{
		"type":      TypePage,
		"title":     title,
		"body":      body,
		"tags":      tags,
		"page_path": pagePath,
	}
}

// headingDoc builds the indexable map for a heading sub-document. The heading
// text is indexed under `title` (analyzed) so a heading-text query matches; the
// anchor (#slug) and the owning page's title/path are stored for the deep-link
// result (interface_contract: kind:"heading" → title, path, anchor, page_title).
func headingDoc(pagePath, pageTitle, headingText, anchor string) map[string]interface{} {
	return map[string]interface{}{
		"type":       TypeHeading,
		"title":      headingText,
		"page_path":  pagePath,
		"anchor":     anchor,
		"page_title": pageTitle,
	}
}

// attachmentDoc builds the indexable map for an attachment document. The
// original filename is indexed under `filename` (SRCH-04) and the extracted text
// under `extracted_text` (SRCH-05); the owning page's path/title are stored so a
// hit links to and is labelled by the page that owns the attachment
// (AttachmentMeta.PagePath — no tree scan). extracted may be empty (pending or
// no text layer) — the doc is still indexed for filename search.
func attachmentDoc(filename, extracted, pagePath, pageTitle string) map[string]interface{} {
	return map[string]interface{}{
		"type":           TypeAttachment,
		"filename":       filename,
		"extracted_text": extracted,
		"page_path":      pagePath,
		"page_title":     pageTitle,
	}
}
