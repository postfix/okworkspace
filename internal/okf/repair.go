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

// Field returns the value of a top-level scalar frontmatter field, or "" when
// the field is absent or not a scalar. Read-only inspection (e.g. recovering a
// page title for the navigation tree); it never mutates the document.
func Field(d *Doc, field string) string {
	mapping := topMapping(d)
	if mapping == nil {
		return ""
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == field {
			if mapping.Content[i+1].Kind == yaml.ScalarNode {
				return mapping.Content[i+1].Value
			}
			return ""
		}
	}
	return ""
}

// SetField surgically sets the value of a top-level scalar frontmatter field,
// adding the key if it is absent. It marks the document FrontDirty so Emit
// re-marshals the frontmatter. Used to fill the generated title on a freshly
// scaffolded page (Create) without disturbing the other repaired fields.
func SetField(d *Doc, field, value string) {
	mapping := topMapping(d)
	if mapping == nil {
		mapping = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		d.Front = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{mapping}}
		if !d.HasFrontmatter {
			d.HasFrontmatter = true
			d.openFence = fenceLine(d.EOLStyle)
			d.closeFence = fenceLine(d.EOLStyle)
		}
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == field {
			mapping.Content[i+1].Kind = yaml.ScalarNode
			mapping.Content[i+1].Tag = "!!str"
			mapping.Content[i+1].Value = value
			d.FrontDirty = true
			return
		}
	}
	key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: field}
	val := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	mapping.Content = append(mapping.Content, key, val)
	d.FrontDirty = true
}

// SetTags surgically sets the top-level `tags` key to a BLOCK-style YAML
// sequence (one `- tag` per line), creating the key if it is absent. It is the
// yaml.SequenceNode analog of SetField and the byte-stable write primitive for
// the per-page tag-apply path (TAG-03): it mutates ONLY the tags value node and
// marks the document FrontDirty so Emit re-marshals the frontmatter, leaving the
// body and every other frontmatter key byte-identical.
//
// Per the locked Phase-11 CONTEXT decision the canonical style for a NON-empty
// tags key is block style (distinct from appendField's empty-`[]` flow style).
// SetTags writes EXACTLY the slice it is given, in order — it does NOT normalize,
// dedupe, lowercase, or cap. Normalization is the caller's (Wave-2 handler's)
// responsibility; SetTags is a pure structural editor.
func SetTags(d *Doc, tags []string) {
	mapping := topMapping(d)
	if mapping == nil {
		mapping = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		d.Front = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{mapping}}
		if !d.HasFrontmatter {
			d.HasFrontmatter = true
			d.openFence = fenceLine(d.EOLStyle)
			d.closeFence = fenceLine(d.EOLStyle)
		}
	}

	// Build the block-style sequence value node: Style 0 (NOT FlowStyle) so it
	// renders as one `- tag` per line, with one !!str scalar per tag in order.
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, tag := range tags {
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: tag})
	}

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == FieldTags {
			// Replace the value node in place so the key keeps its position and
			// the surrounding keys/comments are untouched.
			mapping.Content[i+1] = seq
			d.FrontDirty = true
			return
		}
	}
	key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: FieldTags}
	mapping.Content = append(mapping.Content, key, seq)
	d.FrontDirty = true
}

// fenceLine returns a "---" fence line terminated with the document's EOL.
func fenceLine(style EOLStyle) []byte {
	if style == EOLCRLF {
		return []byte("---\r\n")
	}
	return []byte("---\n")
}
