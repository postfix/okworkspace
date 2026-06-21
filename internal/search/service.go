package search

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/postfix/okworkspace/internal/okf"
)

// readTags returns the page's tags read sequence-aware from the frontmatter
// yaml.Node. okf.Field reads top-level SCALAR nodes only, but tags are typically a
// YAML sequence (`tags: [a, b]` or a block list), so okf.Field would return ""
// (Pitfall 7). This walks doc.Front directly: it finds the top-level `tags` key and
// collects its sequence items (or a single scalar, defensively). Empty/blank tags
// are skipped.
func readTags(doc *okf.Doc) []string {
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
// or nil when there is none (mirrors okf's unexported helper of the same name).
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
