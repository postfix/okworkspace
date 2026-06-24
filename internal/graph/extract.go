package graph

import (
	"path"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/postfix/okworkspace/internal/okf"
)

// outboundLinks returns the resolved, deduped repo-relative .md destinations that
// the page at srcPath links to. It REUSES okf.FindLinks (the byte scanner that
// already skips fenced/inline code spans — locked: no new scanner) and resolves
// each destination with path.Clean(path.Join(path.Dir(srcPath), dest)), mirroring
// okf.RewriteLinks's resolution recipe EXACTLY so forward edges agree with the
// rename link-rewrite.
//
// A destination is kept as an edge ONLY when, after stripping any #fragment, it is
// a repo-relative link (not external/absolute), resolves to a path ending in .md,
// is not a self-edge, and exists(resolved) reports true. A dangling link to a
// nonexistent .md is silently dropped (locked: dangling links are not surfaced
// this phase). exists is the existence check (repo.Exists in production; a
// closure over the live-page set during a from-scratch rebuild so rebuild and
// incremental agree).
func outboundLinks(srcPath string, body []byte, exists func(repoRelPath string) (bool, error)) ([]string, error) {
	fromDir := path.Dir(srcPath)
	seen := map[string]struct{}{}
	var out []string
	for _, l := range okf.FindLinks(body) {
		dest, _ := splitFragment(l.Dest)
		if dest == "" {
			continue
		}
		if isAbsoluteOrExternal(dest) {
			continue
		}
		resolved := path.Clean(path.Join(fromDir, dest))
		if !strings.HasSuffix(resolved, ".md") {
			continue
		}
		if resolved == srcPath {
			// Self-edge: a page linking to itself is not a graph edge.
			continue
		}
		if _, dup := seen[resolved]; dup {
			continue
		}
		ok, err := exists(resolved)
		if err != nil {
			return nil, err
		}
		if !ok {
			// Dangling link to a nonexistent .md — dropped, not surfaced (locked).
			continue
		}
		seen[resolved] = struct{}{}
		out = append(out, resolved)
	}
	return out, nil
}

// splitFragment splits a destination into its path part and an optional
// #fragment. Re-implemented here (a copy of okf's unexported splitFragment) so
// graph resolves links identically without importing search/okf internals.
func splitFragment(dest string) (pathPart, frag string) {
	if idx := strings.IndexByte(dest, '#'); idx >= 0 {
		return dest[:idx], dest[idx:]
	}
	return dest, ""
}

// isAbsoluteOrExternal reports whether a destination is an absolute path, a
// scheme URL (http:, mailto:, etc.), or a protocol-relative URL — none of which
// are repo-relative links. This is the SAME predicate okf.RewriteLinks uses
// (re-implemented locally, not imported), so a link okf would skip rewriting is
// also skipped here as a forward edge.
func isAbsoluteOrExternal(dest string) bool {
	if dest == "" {
		return true
	}
	// A leading '/' covers both an absolute repo path ("/x") and a
	// protocol-relative URL ("//host/x").
	if dest[0] == '/' {
		return true
	}
	// scheme:... — a colon before any slash indicates a URL scheme.
	for i := 0; i < len(dest); i++ {
		switch dest[i] {
		case ':':
			return true
		case '/':
			return false
		}
	}
	return false
}

// pageTags returns the page's frontmatter tags read sequence-aware from the
// yaml.Node. This re-implements search.readTags's walk EXACTLY (find the
// top-level `tags` key, collect SequenceNode scalar items trimmed/non-empty, or a
// single ScalarNode) so page_tags membership matches what search understands for
// the same doc — a parity requirement (do NOT import search). Returns nil when
// there are no tags.
func pageTags(doc *okf.Doc) []string {
	mapping := topMapping(&doc.Front)
	if mapping == nil {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != okf.FieldTags {
			continue
		}
		val := mapping.Content[i+1]
		switch val.Kind {
		case yaml.SequenceNode:
			out := make([]string, 0, len(val.Content))
			for _, item := range val.Content {
				if item.Kind == yaml.ScalarNode {
					if t := strings.TrimSpace(item.Value); t != "" {
						out = append(out, t)
					}
				}
			}
			return out
		case yaml.ScalarNode:
			if t := strings.TrimSpace(val.Value); t != "" {
				return []string{t}
			}
		}
		return nil
	}
	return nil
}

// topMapping returns the top-level mapping node of a parsed frontmatter document,
// or nil when there is none (mirrors search.topMapping / okf's unexported helper).
func topMapping(front *yaml.Node) *yaml.Node {
	n := front
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return nil
		}
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return nil
	}
	return n
}
