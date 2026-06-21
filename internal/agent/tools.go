// tools.go is the SINGLE source of truth for the agent's tool surface. It is the
// ONLY place Eino tools are constructed, and every tool here is READ-ONLY: each
// closure routes through a repo.Resolve-backed service (pages.Get / pages.Tree /
// search.Query / the attachment `.txt` accessor) — NEVER os.ReadFile, and NEVER
// a write/apply/commit/push/shell tool.
//
// Structural write boundary (AGNT-11 / T-04-03): apply-page is NOT a tool — it
// is a separate approval-gated HTTP endpoint (slice 5). The set-equality test in
// tools_test.go is the build gate that fails the moment a 6th (or mutating) tool
// name appears in readToolNames. Keep the tool list and the name list derived in
// THIS one function so they cannot drift.
package agent

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/postfix/okworkspace/internal/pages"
)

// readToolMaxResults caps how many search hits a single tool turn returns to the
// model. Small + flat keeps DeepSeek's tool-call JSON reliable (RESEARCH §Item 5)
// and bounds the context fed back into the ReAct loop.
const readToolMaxResults = 5

// --- flat typed in/out structs (jsonschema-tagged so InferTool derives + the
// framework validates the model's tool-call args before the closure runs). Keep
// these SMALL and FLAT — DeepSeek tool-calling is less consistent than GPT-4 and
// nested/complex schemas degrade it (RESEARCH §Item 5). ---

type listTreeIn struct{}

type listTreeOut struct {
	Paths []string `json:"paths" jsonschema:"description=Workspace-relative paths of every readable page."`
}

type readPageIn struct {
	Path string `json:"path" jsonschema:"description=Workspace-relative path of the page to read (e.g. 'guides/onboarding.md')."`
}

type readPageOut struct {
	Found bool   `json:"found" jsonschema:"description=True when the page exists and was read."`
	Body  string `json:"body" jsonschema:"description=The page's Markdown body (empty when not found)."`
}

type searchIn struct {
	Query string `json:"query" jsonschema:"description=Free-text search query."`
}

type searchHit struct {
	Path    string `json:"path" jsonschema:"description=Workspace-relative path of the matching page (or the attachment's owning page)."`
	Title   string `json:"title" jsonschema:"description=Page title or attachment filename."`
	Snippet string `json:"snippet" jsonschema:"description=A short matching fragment."`
}

type searchOut struct {
	Hits []searchHit `json:"hits" jsonschema:"description=Up to five most-relevant matches."`
}

type readAttachmentIn struct {
	ID string `json:"id" jsonschema:"description=The attachment id whose extracted text to read."`
}

type readAttachmentOut struct {
	Found bool   `json:"found" jsonschema:"description=True when extracted text is available."`
	Text  string `json:"text" jsonschema:"description=The attachment's extracted plain text (empty when none/pending)."`
}

// readToolNames is the canonical read-only allow-list. It is derived alongside
// the tool slice in readTools so the two cannot drift, and it is the exact set
// tools_test.go asserts. Adding a name here without a matching read-only tool
// (or vice-versa) fails the build gate.
//
// THIS LIST IS A SECURITY BOUNDARY: it must contain ONLY read tools. A write/
// apply tool must NEVER be added here — apply is a non-tool HTTP endpoint.
var readToolNames = []string{
	"list_tree",
	"read_page",
	"search_pages",
	"search_attachments",
	"read_attachment_text",
}

// readTools constructs the five read-only Eino tools and returns them together
// with their parallel name list (== readToolNames). It is the ONLY tool-
// construction site in the codebase.
//
// Every closure is repo.Resolve-backed and FAILS SOFT: a missing page/attachment
// returns Found:false (not a hard error) so the ReAct agent can recover instead
// of aborting the turn, and a path/id supplied by the model can never escape the
// resolver or leak raw bytes off an arbitrary path (T-04-04).
//
// trace (optional, nil-safe) records the workspace-relative page paths the agent
// actually retrieved via read_page / search_pages / search_attachments. It backs
// the workspace-Ask "Reasoned over:" citation (D3 / RESEARCH Q2) — citations are
// derived from the real tool-call trace, never from trusting the model to cite.
// nil trace (the allow-list test, page/selection/attachment scopes) is a no-op.
func readTools(deps Deps, trace *scopeTrace) ([]tool.BaseTool, []string, error) {
	listTree, err := utils.InferTool(
		"list_tree",
		"List the workspace-relative paths of all readable pages.",
		func(ctx context.Context, _ listTreeIn) (listTreeOut, error) {
			if deps.Pages == nil {
				return listTreeOut{Paths: []string{}}, nil
			}
			nodes, terr := deps.Pages.Tree(ctx)
			if terr != nil {
				// Soft-miss: an unreadable tree returns no paths rather than
				// aborting the agent turn.
				return listTreeOut{Paths: []string{}}, nil
			}
			return listTreeOut{Paths: flattenPagePaths(nodes)}, nil
		},
	)
	if err != nil {
		return nil, nil, err
	}

	readPage, err := utils.InferTool(
		"read_page",
		"Read the Markdown body of a workspace page by path.",
		func(ctx context.Context, in readPageIn) (readPageOut, error) {
			if deps.Pages == nil {
				return readPageOut{Found: false}, nil
			}
			p, gerr := deps.Pages.Get(ctx, in.Path)
			if gerr != nil {
				return readPageOut{Found: false}, nil // soft-miss
			}
			trace.add(in.Path) // citation: a page the agent actually read.
			return readPageOut{Found: true, Body: p.Body}, nil
		},
	)
	if err != nil {
		return nil, nil, err
	}

	searchPages, err := utils.InferTool(
		"search_pages",
		"Search workspace pages for a query; returns up to five page matches.",
		func(ctx context.Context, in searchIn) (searchOut, error) {
			return runSearch(ctx, deps, trace, in.Query, "page"), nil
		},
	)
	if err != nil {
		return nil, nil, err
	}

	searchAttachments, err := utils.InferTool(
		"search_attachments",
		"Search workspace attachments for a query; returns up to five attachment matches.",
		func(ctx context.Context, in searchIn) (searchOut, error) {
			return runSearch(ctx, deps, trace, in.Query, "attachment"), nil
		},
	)
	if err != nil {
		return nil, nil, err
	}

	readAttachmentText, err := utils.InferTool(
		"read_attachment_text",
		"Read the extracted plain text of an attachment by id.",
		func(ctx context.Context, in readAttachmentIn) (readAttachmentOut, error) {
			if deps.Attachments == nil {
				return readAttachmentOut{Found: false}, nil
			}
			text, terr := deps.Attachments.ExtractedText(ctx, in.ID)
			if terr != nil || text == "" {
				return readAttachmentOut{Found: false}, nil // soft-miss
			}
			return readAttachmentOut{Found: true, Text: text}, nil
		},
	)
	if err != nil {
		return nil, nil, err
	}

	tools := []tool.BaseTool{
		listTree,
		readPage,
		searchPages,
		searchAttachments,
		readAttachmentText,
	}
	// Derive the name list from the SAME construction site (mirrors readToolNames)
	// so the slice and the allow-list cannot drift apart silently.
	names := append([]string(nil), readToolNames...)
	return tools, names, nil
}

// runSearch runs the role-scoped query and maps the kind-filtered hits to the
// flat searchHit DTO (top readToolMaxResults). A nil searcher or query error
// returns an empty (non-nil) hit list — soft-miss, never a hard tool error.
//
// Every surfaced hit's page path is recorded on the (nil-safe) trace so the
// workspace-Ask citation line names exactly the pages RAG actually drew from
// (D3 / RESEARCH Q2). deps.Search is constructed role-scoped from the server
// session by the caller, so a hit can only be a page the session role may read
// — out-of-role pages never enter the hit list, the prompt, or the citation.
func runSearch(ctx context.Context, deps Deps, trace *scopeTrace, query, kind string) searchOut {
	out := searchOut{Hits: []searchHit{}}
	if deps.Search == nil {
		return out
	}
	results, err := deps.Search.Query(ctx, query)
	if err != nil {
		return out
	}
	for _, r := range results {
		if r.Kind != kind {
			continue
		}
		out.Hits = append(out.Hits, searchHit{Path: r.Path, Title: r.Title, Snippet: r.Snippet})
		trace.add(r.Path) // citation: a page RAG surfaced into the answer context.
		if len(out.Hits) >= readToolMaxResults {
			break
		}
	}
	return out
}

// flattenPagePaths walks the nested page/folder tree and collects the workspace-
// relative path of every page node (folders are skipped — only readable pages
// are surfaced to the model). Always returns a non-nil slice.
func flattenPagePaths(nodes []pages.Node) []string {
	paths := []string{}
	var walk func([]pages.Node)
	walk = func(ns []pages.Node) {
		for _, n := range ns {
			if n.Type == "page" {
				paths = append(paths, n.Path)
			}
			if len(n.Children) > 0 {
				walk(n.Children)
			}
		}
	}
	walk(nodes)
	return paths
}
