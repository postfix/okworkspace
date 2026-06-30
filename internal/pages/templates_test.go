package pages

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/postfix/okworkspace/internal/okf"
	"gopkg.in/yaml.v3"
)

// writeTemplate drops a raw template file on disk under the given repo-relative
// path (no commit needed — ListTemplates reads the filesystem via repo.Tree()).
func writeTemplate(t *testing.T, svc *Service, rel, content string) {
	t.Helper()
	if err := svc.repo.Write(rel, []byte(content)); err != nil {
		t.Fatalf("write template %q: %v", rel, err)
	}
}

func TestListTemplatesEmptyWhenNoFolder(t *testing.T) {
	svc, _, _ := newServiceFixture(t, false)
	got, err := svc.ListTemplates(context.Background())
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty list with no Templates/ folder, got %v", got)
	}
}

func TestListTemplatesFlatMdOnly(t *testing.T) {
	svc, _, _ := newServiceFixture(t, false)
	// Two real templates.
	writeTemplate(t, svc, "Templates/Meeting Notes.md", "---\ntype: Meeting\n---\nBody\n")
	writeTemplate(t, svc, "Templates/Daily.md", "Body only\n")
	// Noise that must be skipped: a non-.md file, a nested .md, and a page
	// outside Templates/.
	writeTemplate(t, svc, "Templates/logo.png", "not markdown")
	writeTemplate(t, svc, "Templates/nested/Deep.md", "nested\n")
	writeTemplate(t, svc, "runbooks/Deploy.md", "elsewhere\n")

	got, err := svc.ListTemplates(context.Background())
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	names := make(map[string]string, len(got))
	for _, ti := range got {
		names[ti.Name] = ti.Path
	}
	if len(got) != 2 {
		t.Fatalf("want exactly 2 flat .md templates, got %d: %v", len(got), got)
	}
	if names["Meeting Notes"] != "Templates/Meeting Notes.md" {
		t.Fatalf("Meeting Notes path = %q", names["Meeting Notes"])
	}
	if names["Daily"] != "Templates/Daily.md" {
		t.Fatalf("Daily path = %q", names["Daily"])
	}
	if _, ok := names["logo"]; ok {
		t.Fatal("non-.md file logo.png was listed as a template")
	}
	if _, ok := names["Deep"]; ok {
		t.Fatal("nested template was listed (must be flat-only)")
	}
	if _, ok := names["Deploy"]; ok {
		t.Fatal("a page outside Templates/ was listed as a template")
	}
}

// TestListTemplatesPathsAllResolve asserts every listed path resolves safely
// within the repo (the SEC-01/D-06 invariant: a listed template is always a
// resolver-gated content path, never an escape).
func TestListTemplatesPathsAllResolve(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	writeTemplate(t, svc, "Templates/A.md", "Body\n")
	got, err := svc.ListTemplates(context.Background())
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	for _, ti := range got {
		if !strings.HasPrefix(ti.Path, templatesDir+"/") {
			t.Fatalf("listed path %q escapes Templates/", ti.Path)
		}
		if _, err := r.Resolve(ti.Path); err != nil {
			t.Fatalf("listed path %q does not resolve: %v", ti.Path, err)
		}
	}
}

func TestCreateFromTemplateTitleRequired(t *testing.T) {
	svc, _, _ := newServiceFixture(t, false)
	if _, err := svc.CreateFromTemplate(context.Background(), "", "  ", "type: Meeting\n", "Body\n", "alice"); !errors.Is(err, ErrTitleRequired) {
		t.Fatalf("want ErrTitleRequired, got %v", err)
	}
}

// readCreatedPage reads a created page off disk and returns its frontmatter
// region + body, asserting it re-parses into exactly ONE frontmatter block with
// NO duplicate top-level keys (the load-bearing merge invariant): it must begin
// with a fence, okf.Parse must round-trip it, and a strict map decode (which
// rejects duplicate keys) must succeed.
func readCreatedPage(t *testing.T, r interface {
	Read(string) ([]byte, error)
}, path string) (front string, body string) {
	t.Helper()
	raw, err := r.Read(path)
	if err != nil {
		t.Fatalf("read created page %q: %v", path, err)
	}
	if !strings.HasPrefix(string(raw), "---\n") {
		t.Fatalf("page does not begin with a frontmatter fence:\n%s", raw)
	}
	doc, err := okf.Parse(raw)
	if err != nil {
		t.Fatalf("created page does not re-parse: %v\n%s", err, raw)
	}
	if !doc.HasFrontmatter {
		t.Fatalf("created page has no frontmatter block:\n%s", raw)
	}
	// Strict decode rejects duplicate keys — the "no duplicate keys" guarantee.
	var m map[string]any
	if err := yaml.Unmarshal(doc.RawFront, &m); err != nil {
		t.Fatalf("created page frontmatter has duplicate keys / malformed YAML: %v\n%s", err, doc.RawFront)
	}
	return string(doc.RawFront), string(doc.Body)
}

func TestCreateFromTemplateOwnFrontmatterSingleBlock(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()
	// Template carries its OWN frontmatter incl. a conflicting title:.
	fm := "title: Template Title\ntype: Meeting\ntags: [project, q3]"
	body := "# {{title}}\n\nNotes for the meeting.\n"
	path, err := svc.CreateFromTemplate(ctx, "runbooks", "Sprint Planning", fm, body, "alice")
	if err != nil {
		t.Fatalf("CreateFromTemplate: %v", err)
	}
	if path != "runbooks/sprint-planning.md" {
		t.Fatalf("path = %q, want path derived from USER title", path)
	}
	waitForFile(t, r, path)

	front, gotBody := readCreatedPage(t, r, path)
	// USER title wins over the template title:.
	if !strings.Contains(front, "title: Sprint Planning") {
		t.Fatalf("user title did not win; frontmatter:\n%s", front)
	}
	if strings.Contains(front, "Template Title") {
		t.Fatalf("template title leaked into the page:\n%s", front)
	}
	// Template's own fields carried through.
	if !strings.Contains(front, "type: Meeting") {
		t.Fatalf("template type: did not carry through:\n%s", front)
	}
	if !strings.Contains(front, "project") || !strings.Contains(front, "q3") {
		t.Fatalf("template tags did not carry through:\n%s", front)
	}
	// Required fields repaired.
	if !strings.Contains(front, "description:") || !strings.Contains(front, "timestamp:") {
		t.Fatalf("required fields not repaired:\n%s", front)
	}
	if gotBody == "" || !strings.Contains(gotBody, "Notes for the meeting.") {
		t.Fatalf("body not seeded: %q", gotBody)
	}
}

func TestCreateFromTemplateNoFrontmatterRepaired(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()
	path, err := svc.CreateFromTemplate(ctx, "", "Body Only Page", "", "Just a body, no frontmatter.\n", "alice")
	if err != nil {
		t.Fatalf("CreateFromTemplate: %v", err)
	}
	waitForFile(t, r, path)
	front, body := readCreatedPage(t, r, path)
	for _, want := range []string{"title: Body Only Page", "type: Page", "description:", "tags:", "timestamp:"} {
		if !strings.Contains(front, want) {
			t.Fatalf("repaired frontmatter missing %q:\n%s", want, front)
		}
	}
	if !strings.Contains(body, "Just a body") {
		t.Fatalf("body not promoted: %q", body)
	}
}

// TestCreateFromTemplateDuplicateKeysRejected is the core robust-merge guard:
// a template with duplicate top-level frontmatter keys must FAIL CLOSED with
// ErrTemplateInvalid — NEVER produce a corrupt duplicate-key page.
func TestCreateFromTemplateDuplicateKeysRejected(t *testing.T) {
	svc, _, _ := newServiceFixture(t, false)
	fm := "title: First\ntitle: Second\ntags: [a]\ntags: [b]"
	_, err := svc.CreateFromTemplate(context.Background(), "", "User Page", fm, "Body\n", "alice")
	if !errors.Is(err, ErrTemplateInvalid) {
		t.Fatalf("duplicate-key template: want ErrTemplateInvalid, got %v", err)
	}
}

// TestCreateFromTemplateMalformedYAMLRejected: malformed YAML in the seed fails
// closed with a clean error (never a corrupt page).
func TestCreateFromTemplateMalformedYAMLRejected(t *testing.T) {
	svc, _, _ := newServiceFixture(t, false)
	fm := "title: [unclosed\n  bad: : :"
	_, err := svc.CreateFromTemplate(context.Background(), "", "User Page", fm, "Body\n", "alice")
	if !errors.Is(err, ErrTemplateInvalid) {
		t.Fatalf("malformed YAML template: want ErrTemplateInvalid, got %v", err)
	}
}

// TestCreateFromTemplateConflictingTitleUserWins is the explicit conflicting-
// title case (single non-duplicate title: in the template).
func TestCreateFromTemplateConflictingTitleUserWins(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()
	path, err := svc.CreateFromTemplate(ctx, "", "My Real Title", "title: Ignore Me\ntype: Note", "Hi\n", "alice")
	if err != nil {
		t.Fatalf("CreateFromTemplate: %v", err)
	}
	waitForFile(t, r, path)
	front, _ := readCreatedPage(t, r, path)
	if !strings.Contains(front, "title: My Real Title") {
		t.Fatalf("user title did not win:\n%s", front)
	}
	if strings.Contains(front, "Ignore Me") {
		t.Fatalf("template title leaked:\n%s", front)
	}
}
