package pages

import (
	"context"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/postfix/okworkspace/internal/okf"
)

// tagscan.go — body-inclusive tag enumeration (TR-04) + pre-fix stale-body
// detection (TR-06), living in the PAGES SERVICE because it reads page bodies
// through the SEC-01 resolver (repo.Tree() + repo.Read), mirroring the
// tree.go / templates.go enumeration pattern. It deliberately does NOT touch
// internal/graph/query.go, whose "reads NO bodies — only path enumeration"
// invariant (query.go:256) must hold: a body-only `#tag` is invisible to the
// frontmatter-only page_tags cache (PagesByTag), so the affected-set scan for a
// server-side body rewrite must read bodies here instead.
//
// Recognition is single-sourced through okf.FindHashtags — the SAME region-aware
// scanner RewriteHashtags uses — so a fenced/inline/indented/HTML/escaped `#tag`
// is excluded identically and there is never a second `#tag` parser.

// isLivePage reports whether a repo Tree entry is a live page: a non-directory
// `.md` file that is not under the trash dir. Mirrors the graph rebuild's skip
// rules (live .md only; no trashed/non-.md; the resolver rejects path escape).
func isLivePage(path string, isDir bool) bool {
	if isDir || !strings.HasSuffix(path, ".md") {
		return false
	}
	if path == trashDir || strings.HasPrefix(path, trashDir+"/") {
		return false
	}
	return true
}

// bodyHasTag reports whether raw page bytes carry `#tag` in the BODY per the
// region-aware scan (fenced/inline/indented/HTML/escape excluded). tag is a
// normalized (lower-cased) bare tag name. It parses the frontmatter off first so
// only the body is scanned (a frontmatter `tags:` value is not a body `#tag`).
func bodyHasTag(raw []byte, tag string) bool {
	doc, err := okf.Parse(raw)
	if err != nil {
		return false
	}
	for _, h := range okf.FindHashtags(doc.Body) {
		if h.Name == tag {
			return true
		}
	}
	return false
}

// PagesWithBodyTag returns the repo-relative paths of every live page whose BODY
// contains `#tag` (region-aware), in sorted order. tag is normalized (trimmed +
// lower-cased) by the caller convention; it is lower-cased here defensively. It
// is a one-shot per-operation scan (never per-keystroke): it walks the repo tree
// once and reads each live page body through the SEC-01 resolver.
//
// This is the TR-04 blind-spot closer: a page carrying `#tag` only in its body
// (absent from frontmatter → invisible to graph.PagesByTag) is found here, so
// the rename/merge/delete affected set includes it and its body is rewritten.
func (s *Service) PagesWithBodyTag(ctx context.Context, tag string) ([]string, error) {
	want := strings.ToLower(strings.TrimSpace(tag))
	if want == "" {
		return nil, nil
	}
	items, err := s.repo.Tree()
	if err != nil {
		return nil, fmt.Errorf("pages: body-tag scan tree: %w", err)
	}
	out := make([]string, 0)
	for _, it := range items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !isLivePage(it.Path, it.IsDir) {
			continue
		}
		raw, rerr := s.repo.Read(it.Path)
		if rerr != nil {
			// A transient read error on ONE page must not sink the whole scan
			// (mirrors the graph rebuild's skip-on-error); it is simply not
			// reported as body-affected.
			continue
		}
		if bodyHasTag(raw, want) {
			out = append(out, it.Path)
		}
	}
	return out, nil
}

// StaleBodyTag is one pre-fix corruption finding (TR-06): a page whose body
// carries `#tag` occurrences whose frontmatter counterpart is ABSENT — the
// half-reverted state a pre-fix rename/delete left (the frontmatter op ran but
// the body #tag was never rewritten, so the client re-unions it on next save).
type StaleBodyTag struct {
	PagePath string   `json:"page_path"`
	// BodyOnlyTags are the lower-cased body `#tag` names not present in the page's
	// frontmatter `tags:` — the disagreement indicative of a pre-fix op.
	BodyOnlyTags []string `json:"body_only_tags"`
}

// DetectStaleBodyTags is the TR-06 DETECT+REPORT scan: a bounded one-shot
// workspace walk that flags every live page whose body `#tags` disagree with its
// frontmatter (a body `#tag` whose frontmatter counterpart is absent), which is
// the signature of a pre-fix rename/delete that rewrote frontmatter but left the
// body tag stale. It DETECTS + REPORTS ONLY — it never writes. The historical
// corruption is thereby surfaced, not silently claimed away (the ROADMAP
// "for all pages including never-reopened" over-claim is softened to "for all
// FUTURE operations" + this surfaced set).
//
// A body `#tag` that IS in frontmatter is normal (the ED-02b materialized state)
// and never flagged; only the disagreement is reported.
func (s *Service) DetectStaleBodyTags(ctx context.Context) ([]StaleBodyTag, error) {
	items, err := s.repo.Tree()
	if err != nil {
		return nil, fmt.Errorf("pages: stale-body scan tree: %w", err)
	}
	out := make([]StaleBodyTag, 0)
	for _, it := range items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !isLivePage(it.Path, it.IsDir) {
			continue
		}
		raw, rerr := s.repo.Read(it.Path)
		if rerr != nil {
			continue
		}
		doc, perr := okf.Parse(raw)
		if perr != nil {
			continue
		}
		// Frontmatter tag set (normalized), for membership tests.
		fm := map[string]bool{}
		for _, t := range frontmatterTags(doc) {
			fm[strings.ToLower(strings.TrimSpace(t))] = true
		}
		// Body tags (region-aware, de-duped, in first-seen order).
		var bodyOnly []string
		seen := map[string]bool{}
		for _, h := range okf.FindHashtags(doc.Body) {
			if fm[h.Name] || seen[h.Name] {
				continue
			}
			seen[h.Name] = true
			bodyOnly = append(bodyOnly, h.Name)
		}
		if len(bodyOnly) > 0 {
			out = append(out, StaleBodyTag{PagePath: it.Path, BodyOnlyTags: bodyOnly})
		}
	}
	return out, nil
}

// frontmatterTags returns a page's frontmatter `tags:` read sequence-aware from
// the parsed yaml.Node — the SAME walk graph.pageTags / server.extractTagsYAML
// use (find the top-level `tags` key; collect SequenceNode scalar items
// trimmed/non-empty, or a single ScalarNode). Kept local to avoid importing the
// graph/search packages; parity with them is a membership requirement, not an
// import one. Returns nil when there are no tags.
func frontmatterTags(doc *okf.Doc) []string {
	n := &doc.Front
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return nil
		}
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value != okf.FieldTags {
			continue
		}
		val := n.Content[i+1]
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
