package search

import (
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
)

// Document type constants. Every indexed document carries a `type` field whose
// value is one of these; the type field both selects the per-type document
// mapping and drives the result-type facet (SRCH-06).
const (
	TypePage       = "page"
	TypeHeading    = "heading"
	TypeAttachment = "attachment"
)

// buildMapping returns the typed index mapping. TypeField="type" tells Bleve which
// field selects the per-type document mapping; DefaultAnalyzer="en" gives stemming
// + stop-word handling for the analyzed text fields (title/body/extracted_text/
// filename). The page mapping is fully populated by THIS plan; the heading and
// attachment mappings are declared now (empty-but-registered field sets) so 03-03
// only adds field population — never a mapping migration that would force a rebuild.
func buildMapping() mapping.IndexMapping {
	im := bleve.NewIndexMapping()
	im.TypeField = "type"
	im.DefaultAnalyzer = "en"

	// Reusable field mappings. Boost is applied at QUERY time, not here.
	titleFM := bleve.NewTextFieldMapping()
	titleFM.Store = true
	titleFM.IncludeTermVectors = true // required for phrase queries + highlighting

	bodyFM := bleve.NewTextFieldMapping()
	bodyFM.Store = true
	bodyFM.IncludeTermVectors = true

	keywordFM := bleve.NewKeywordFieldMapping() // whole-token exact (tags)
	keywordFM.Store = true

	filenameFM := bleve.NewTextFieldMapping()
	filenameFM.Store = true
	filenameFM.IncludeTermVectors = true

	textFM := bleve.NewTextFieldMapping() // extracted attachment text
	textFM.Store = true
	textFM.IncludeTermVectors = true

	typeFM := bleve.NewKeywordFieldMapping() // facetable, exact
	typeFM.Store = true

	pathFM := bleve.NewKeywordFieldMapping()
	pathFM.Store = true // stored for navigation; not analyzed for relevance

	// Page documents (THIS plan).
	page := bleve.NewDocumentMapping()
	page.AddFieldMappingsAt("type", typeFM)
	page.AddFieldMappingsAt("title", titleFM)
	page.AddFieldMappingsAt("body", bodyFM)
	page.AddFieldMappingsAt("tags", keywordFM)
	page.AddFieldMappingsAt("page_path", pathFM)

	// Heading documents (deep-link sub-docs; populated by 03-03).
	heading := bleve.NewDocumentMapping()
	heading.AddFieldMappingsAt("type", typeFM)
	heading.AddFieldMappingsAt("title", titleFM)
	heading.AddFieldMappingsAt("page_path", pathFM)
	heading.AddFieldMappingsAt("anchor", pathFM)
	heading.AddFieldMappingsAt("page_title", pathFM)

	// Attachment documents (populated by 03-03).
	attach := bleve.NewDocumentMapping()
	attach.AddFieldMappingsAt("type", typeFM)
	attach.AddFieldMappingsAt("filename", filenameFM)
	attach.AddFieldMappingsAt("extracted_text", textFM)
	attach.AddFieldMappingsAt("page_path", pathFM)
	attach.AddFieldMappingsAt("page_title", pathFM)

	im.AddDocumentMapping(TypePage, page)
	im.AddDocumentMapping(TypeHeading, heading)
	im.AddDocumentMapping(TypeAttachment, attach)
	return im
}
