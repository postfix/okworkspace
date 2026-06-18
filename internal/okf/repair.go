package okf

import (
	"time"

	"gopkg.in/yaml.v3"
)

// Repair surgically adds ONLY the required frontmatter fields that are absent
// (PAGE-09). Existing keys, values, comments, and ordering are left untouched —
// the function appends missing fields to the end of the top-level mapping and
// sets FrontDirty only when something was added. Defaults: `type` → "Page",
// `timestamp` → now in ISO-8601 (RFC 3339), `description` → empty string,
// `tags` → empty sequence, `title` → empty string. now is supplied by the
// caller so the result is deterministic in tests.
//
// Repair never re-orders or rewrites the user's existing frontmatter; combined
// with Emit's verbatim-unless-dirty behavior, a page that already has all five
// required fields Emits byte-identically to its input.
func Repair(d *Doc, now time.Time) {
	mapping := topMapping(d)
	if mapping == nil {
		// No usable mapping (e.g. body-only file, or empty/non-mapping
		// frontmatter). Materialize a fresh document + mapping so missing
		// required fields can be added. This promotes a body-only file to one
		// with a frontmatter region.
		mapping = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		d.Front = yaml.Node{
			Kind:    yaml.DocumentNode,
			Content: []*yaml.Node{mapping},
		}
		if !d.HasFrontmatter {
			d.HasFrontmatter = true
			d.openFence = fenceLine(d.EOLStyle)
			d.closeFence = fenceLine(d.EOLStyle)
		}
	}

	present := existingKeys(mapping)
	added := false
	for _, field := range requiredFields {
		if present[field] {
			continue
		}
		appendField(mapping, field, now)
		added = true
	}
	if added {
		d.FrontDirty = true
	}
}

// topMapping returns the top-level mapping node of the parsed frontmatter, or
// nil when there is none (unparsed, empty, or a non-mapping root).
func topMapping(d *Doc) *yaml.Node {
	n := &d.Front
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

// existingKeys returns the set of top-level keys present in a mapping node.
func existingKeys(mapping *yaml.Node) map[string]bool {
	keys := make(map[string]bool, len(mapping.Content)/2)
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		keys[mapping.Content[i].Value] = true
	}
	return keys
}

// appendField appends a key/value pair for one missing required field with its
// default value, mutating the mapping node in place.
func appendField(mapping *yaml.Node, field string, now time.Time) {
	key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: field}
	var val *yaml.Node
	switch field {
	case FieldType:
		val = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: DefaultType}
	case FieldTimestamp:
		val = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: now.UTC().Format(time.RFC3339)}
	case FieldTags:
		// An empty sequence renders as "[]" via flow style, keeping the
		// repaired frontmatter compact and unambiguous.
		val = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	default:
		// description, title, and any future string default → empty scalar.
		val = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: ""}
	}
	mapping.Content = append(mapping.Content, key, val)
}

// fenceLine returns a "---" fence line terminated with the document's EOL.
func fenceLine(style EOLStyle) []byte {
	if style == EOLCRLF {
		return []byte("---\r\n")
	}
	return []byte("---\n")
}
