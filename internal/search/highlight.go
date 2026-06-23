package search

import (
	htmlfmt "github.com/blevesearch/bleve/v2/search/highlight/format/html"
	simplefrag "github.com/blevesearch/bleve/v2/search/highlight/fragmenter/simple"
	simplehl "github.com/blevesearch/bleve/v2/search/highlight/highlighter/simple"

	"github.com/blevesearch/bleve/v2/registry"
	"github.com/blevesearch/bleve/v2/search/highlight"
)

// highlightStyle is the registry name of the weight-only highlighter used by
// every search request (referenced via NewHighlightWithStyle). It is NOT the
// built-in "html" highlighter: Bleve's default wraps matches in <mark> (a
// background fill) which both violates the UI-SPEC weight-only rule and is a
// stored-XSS surface. Ours wraps matches in <strong> instead.
const highlightStyle = "okf-weight"

// fragmentSize bounds the snippet window around a match.
const fragmentSize = 200

// init registers the weight-only highlighter. It composes Bleve's simple
// fragmenter with the html FragmentFormatter configured to emit <strong>…</strong>
// around matches. Crucially, that formatter html-ESCAPES the surrounding fragment
// text (html.EscapeString), so any <script> or literal <mark> embedded in page
// content is neutralized in the returned snippet — the XSS guard (T-03-01). The
// SPA (03-02) renders only the known <strong> tag, with rehype-raw OFF.
func init() {
	err := registry.RegisterHighlighter(highlightStyle,
		func(config map[string]interface{}, cache *registry.Cache) (highlight.Highlighter, error) {
			fragmenter := simplefrag.NewFragmenter(fragmentSize)
			formatter := htmlfmt.NewFragmentFormatter("<strong>", "</strong>")
			return simplehl.NewHighlighter(fragmenter, formatter, simplehl.DefaultSeparator), nil
		})
	if err != nil {
		panic("search: register weight-only highlighter: " + err.Error())
	}
}
