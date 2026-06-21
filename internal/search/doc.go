package search

// Deterministic document IDs. A page's ID is its workspace-relative path so an
// upsert (Index with the same ID) overwrites in place and a delete (Delete by ID)
// removes it. Attachment and heading IDs are namespaced to avoid colliding with a
// page path (used by 03-03; defined here so the scheme is stable).
func pageDocID(pagePath string) string      { return pagePath }
func attachmentDocID(id string) string      { return "att:" + id }
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
