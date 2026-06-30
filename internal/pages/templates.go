package pages

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/postfix/okworkspace/internal/okf"
)

// ErrTemplateInvalid is returned by CreateFromTemplate when the seeded template
// frontmatter is not well-formed YAML (malformed, or — crucially — carrying
// duplicate top-level keys, which the faithful okf.Doc model would otherwise
// re-emit as a corrupt duplicate-key page). Handlers map it to a 400 so a bad
// template fails closed rather than producing a two-block / duplicate-key page
// (ED-06a robust-merge requirement, D-04).
var ErrTemplateInvalid = errors.New("template invalid")

// templatesDir is the single top-level folder whose flat *.md files are offered
// as page templates (D-01). Nested subfolders are NOT templates in v1.4.
const templatesDir = "Templates"

// TemplateInfo is one entry in the templates list: the display Name (the
// filename without its ".md" extension) and the repo-relative Path (always under
// Templates/), which the client passes back to getPage to read the chosen
// template's content.
type TemplateInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ListTemplates enumerates the flat *.md files directly under the top-level
// Templates/ folder (D-02). It is non-recursive for v1.4: nested entries are
// skipped, as are non-.md files and directories. A repo with no Templates/
// folder yields an empty list (not an error) so the picker simply shows "Blank".
//
// Enumeration reuses the existing repo.Tree() listing, and every candidate path
// is re-validated through repo.Resolve (constrained to the Templates/ subtree by
// the prefix filter) so the SEC-01 invariant — every content path is
// resolver-gated — holds for template reads too (D-06).
func (s *Service) ListTemplates(_ context.Context) ([]TemplateInfo, error) {
	items, err := s.repo.Tree()
	if err != nil {
		return nil, err
	}
	prefix := templatesDir + "/"
	out := make([]TemplateInfo, 0)
	for _, it := range items {
		if it.IsDir {
			continue
		}
		rel, ok := strings.CutPrefix(it.Path, prefix)
		if !ok {
			continue // not under Templates/
		}
		if strings.Contains(rel, "/") {
			continue // nested — flat-only in v1.4
		}
		if !strings.HasSuffix(rel, ".md") {
			continue // non-template file (e.g. an image dropped in Templates/)
		}
		// Defense-in-depth: confirm the path resolves safely within the repo.
		// Tree() entries are within-root by construction, but routing through
		// Resolve keeps every content path resolver-gated (SEC-01/D-06); a path
		// that somehow fails to resolve is silently dropped, never listed.
		if _, err := s.repo.Resolve(it.Path); err != nil {
			continue
		}
		out = append(out, TemplateInfo{
			Name: strings.TrimSuffix(rel, ".md"),
			Path: it.Path,
		})
	}
	return out, nil
}

// CreateFromTemplate creates a new page seeded from a template's (already
// placeholder-substituted) frontmatter + body. It MIRRORS Create exactly — same
// uniquePath slug from the user's title, same single-writer enqueueWrite
// "create" + index/graph upsert — differing ONLY in that the doc is seeded from
// the submitted content via okf.Parse instead of an empty doc.
//
// The robust frontmatter merge (D-04, the load-bearing ED-06a risk) happens here
// in the canonical okf engine:
//   - okf.Parse builds the (single) frontmatter block from the assembled source.
//   - okf.ValidateFrontmatter strict-decodes it, rejecting malformed YAML AND
//     duplicate top-level keys with ErrTemplateInvalid (fail closed — okf's
//     faithful yaml.Node model accepts duplicates and would otherwise emit a
//     corrupt page).
//   - okf.SetField(title) sets the canonical title so the USER's title wins over
//     any template `title:` (in-place, no second key).
//   - okf.Repair adds the remaining required fields.
//
// The result is ONE valid frontmatter block — no double ---, no duplicate keys,
// no malformed YAML — and the path still derives from the user's title. Server
// re-parses+repairs; a client-sent template PATH is never trusted (only the
// substituted CONTENT is submitted).
func (s *Service) CreateFromTemplate(ctx context.Context, folder, title, seedFrontmatter, seedBody, user string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", ErrTitleRequired
	}

	// Parse + validate the (untrusted) seed BEFORE taking the mutation lock so a
	// bad template fails fast with a clean 400 and never holds the serialization
	// lock. Parsing is pure (no shared state), so order vs. the lock is safe.
	doc, err := okf.Parse(AssembleSource(seedFrontmatter, seedBody))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrTemplateInvalid, err)
	}
	if err := okf.ValidateFrontmatter(doc); err != nil {
		return "", fmt.Errorf("%w: %v", ErrTemplateInvalid, err)
	}

	// Serialize against all namespace mutations so uniquePath's existence check is
	// atomic with the commit (identical to Create).
	defer s.lockMutation()()

	path, err := s.uniquePath(folder, title)
	if err != nil {
		return "", err
	}

	// User title wins over any template title:, then repair the rest.
	okf.SetField(doc, okf.FieldTitle, title)
	okf.Repair(doc, s.now())

	bytesOut, err := doc.Emit()
	if err != nil {
		return "", fmt.Errorf("pages: emit template page: %w", err)
	}

	if err := s.enqueueWrite(ctx, path, bytesOut, "create", user); err != nil {
		return "", err
	}
	s.enqueueIndexUpsert(ctx, path)
	s.enqueueGraphUpsert(ctx, path)
	return path, nil
}
