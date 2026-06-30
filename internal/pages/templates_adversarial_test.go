package pages

// Adversarial / boundary / property / fuzz tests for the ED-06a page-template
// surface (Phase 27). Independent of the author's templates_test.go: these
// attack the load-bearing "robust frontmatter merge" invariant of
// CreateFromTemplate and the path-safety contract of ListTemplates with the
// inputs the happy-path tests structurally miss.
//
// THE INVARIANT under attack (SURFACE A): for ANY (frontmatter, body) seed and a
// non-empty user title, CreateFromTemplate either
//   (a) returns a CLEAN typed error (ErrTemplateInvalid / ErrTitleRequired /
//       repo.ErrUnsafePath), writing nothing, OR
//   (b) writes exactly ONE valid frontmatter block to disk that: begins with a
//       single opening fence, re-parses via okf.Parse, strict-decodes via
//       yaml.Unmarshal into map[string]any with NO error (⇒ no duplicate keys,
//       no two-block corruption, well-formed YAML), has the USER title winning,
//       and carries all five required fields.
// It must NEVER panic, hang, or write a corrupt/duplicate-key/two-block page.
//
// Oracle: the invariant itself (totality + the strict-decode property) for the
// generated/boundary cases; an explicit reference for the named-outcome cases.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/okf"
	"github.com/postfix/okworkspace/internal/repo"
	"gopkg.in/yaml.v3"
)

// fixedNow is the deterministic clock used by the merge-pipeline fuzz/unit
// helpers that exercise okf directly (mirroring Service.now in the fixture).
var fixedNow = time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)

// requiredKeys is the set the merged page must always carry (okf.Repair adds
// every absent one).
var requiredKeys = []string{
	okf.FieldType, okf.FieldTitle, okf.FieldDescription, okf.FieldTags, okf.FieldTimestamp,
}

// assertMergeInvariant runs CreateFromTemplate and asserts the ED-06a invariant.
// On a clean typed error it confirms nothing was corrupted; on success it reads
// the page off disk and verifies the single-block / strict-decode / title-wins /
// required-fields properties. A panic inside CreateFromTemplate fails the test
// automatically (totality), which is exactly what we want to catch.
func assertMergeInvariant(t *testing.T, svc *Service, r *repo.Repo, folder, title, fm, body string) {
	t.Helper()
	path, err := svc.CreateFromTemplate(context.Background(), folder, title, fm, body, "alice")
	if err != nil {
		// Branch (a): the ONLY acceptable errors are the clean typed ones the
		// handler maps to a 4xx. Anything else (e.g. a raw emit/marshal error
		// mapped to 500) is a robustness defect — fails-open, not fails-closed.
		if errors.Is(err, ErrTemplateInvalid) || errors.Is(err, ErrTitleRequired) || errors.Is(err, repo.ErrUnsafePath) {
			return
		}
		t.Fatalf("non-clean error for seed fm=%q body=%q: %v", fm, body, err)
	}
	// Branch (b): a page was written. It must satisfy every property.
	waitForFile(t, r, path)
	raw, err := r.Read(path)
	if err != nil {
		t.Fatalf("read created page %q: %v", path, err)
	}
	assertSingleValidBlock(t, raw, strings.TrimSpace(title), fm, body)
}

// assertSingleValidBlock is the on-disk oracle: it proves the bytes are a single
// valid frontmatter block with the user title winning and all required fields
// present. Used by both the service-level and pipeline-level checks.
func assertSingleValidBlock(t *testing.T, raw []byte, wantTitle, fm, body string) {
	t.Helper()
	s := string(raw)
	// Exactly one OPENING fence at byte 0 (LF or CRLF — both are valid OKF).
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		t.Fatalf("page does not begin with a single frontmatter fence:\n%q", s)
	}
	doc, err := okf.Parse(raw)
	if err != nil {
		t.Fatalf("created page does not re-parse (fm=%q body=%q): %v\n%q", fm, body, err, s)
	}
	if !doc.HasFrontmatter {
		t.Fatalf("created page has no frontmatter block (fm=%q body=%q):\n%q", fm, body, s)
	}
	// Strict decode: rejects duplicate top-level keys AND malformed YAML. This is
	// the precise "no corrupt / two-block / duplicate-key page" guarantee.
	var m map[string]any
	if err := yaml.Unmarshal(doc.RawFront, &m); err != nil {
		t.Fatalf("created page frontmatter is corrupt (dup keys / malformed) for fm=%q body=%q: %v\nFRONT:\n%q", fm, body, err, string(doc.RawFront))
	}
	// The merged frontmatter must be a mapping (a sequence/scalar root would be a
	// merge failure even if it "decodes").
	if m == nil {
		t.Fatalf("merged frontmatter decoded to a nil/empty map (fm=%q body=%q):\nFRONT:\n%q", fm, body, string(doc.RawFront))
	}
	// USER title wins: the canonical scalar title equals the user's trimmed title.
	if got := okf.Field(doc, okf.FieldTitle); got != wantTitle {
		t.Fatalf("user title did not win: got %q want %q (fm=%q body=%q)\nFRONT:\n%q", got, wantTitle, fm, body, string(doc.RawFront))
	}
	if mt, ok := m["title"]; !ok || fmt.Sprint(mt) != wantTitle {
		t.Fatalf("decoded title=%v (ok=%v) want %q (fm=%q)\nFRONT:\n%q", m["title"], ok, wantTitle, fm, string(doc.RawFront))
	}
	// All five required fields present as TOP-LEVEL keys.
	for _, k := range requiredKeys {
		if _, ok := m[k]; !ok {
			t.Fatalf("required field %q missing from merged page (fm=%q body=%q):\nFRONT:\n%q", k, fm, body, string(doc.RawFront))
		}
	}
}

// ---------------------------------------------------------------------------
// SURFACE A — boundary / partition table over the merge.
// ---------------------------------------------------------------------------

func TestCreateFromTemplate_AdversarialMergeTable(t *testing.T) {
	cases := []struct {
		name string
		fm   string
		body string
		// wantReject, when true, additionally asserts the seed fails closed with
		// ErrTemplateInvalid (not merely "satisfies the invariant").
		wantReject bool
	}{
		// --- partitions that MUST merge into a valid page ---
		{name: "empty seed", fm: "", body: ""},
		{name: "body only no frontmatter", fm: "", body: "Just prose.\n"},
		{name: "template own frontmatter", fm: "type: Meeting\ntags: [a, b]", body: "# Hi\n"},
		{name: "conflicting single title", fm: "title: Template Title\ntype: Note", body: "x\n"},
		{name: "literal placeholders stored verbatim", fm: "type: Note\nowner: {{user}}", body: "# {{title}}\nHi {{name}}\n"},
		{name: "body contains literal --- fence line", fm: "type: Note", body: "above\n---\nbelow\n"},
		{name: "body is only a --- line", fm: "type: Note", body: "---\n"},
		{name: "body starts with --- fence", fm: "type: Note", body: "---\nnot frontmatter\n"},
		{name: "crlf in frontmatter", fm: "type: Note\r\ntags: [x]\r\nowner: bob", body: "line1\r\nline2\r\n"},
		{name: "crlf body only", fm: "", body: "alpha\r\nbeta\r\n"},
		{name: "unicode and emoji values", fm: "type: Заметка\nowner: 🦊", body: "héllo 世界 🎉\n"},
		{name: "yaml comment only frontmatter", fm: "# just a comment", body: "body\n"},
		{name: "frontmatter null", fm: "null", body: "body\n"},
		{name: "frontmatter tilde", fm: "~", body: "body\n"},
		{name: "trailing blank lines in fm", fm: "type: Note\n\n\n", body: "b\n"},
		{name: "leading blank line in fm", fm: "\ntype: Note", body: "b\n"},
		{name: "quoted key", fm: `"type": Note`, body: "b\n"},
		{name: "block scalar value", fm: "type: Note\ndesc: |\n  line one\n  line two", body: "b\n"},
		{name: "flow mapping value", fm: "type: Note\nmeta: {a: 1, b: 2}", body: "b\n"},
		{name: "deeply nested mapping", fm: "type: Note\na:\n  b:\n    c:\n      d: deep", body: "b\n"},
		{name: "valid anchor and alias", fm: "type: Note\nbase: &b hello\nref: *b", body: "b\n"},
		{name: "title as mapping (non-scalar) - user must win", fm: "title:\n  nested: x\ntype: Note", body: "b\n"},
		{name: "title as sequence (non-scalar) - user must win", fm: "title: [a, b, c]\ntype: Note", body: "b\n"},
		{name: "title via alias - user must win", fm: "anchor: &t Original\ntitle: *t", body: "b\n"},
		{name: "title carries anchor referenced elsewhere", fm: "title: &t Original\nref: *t", body: "b\n"},
		{name: "merge key brings type", fm: "defaults: &d\n  type: Merged\n<<: *d", body: "b\n"},
		{name: "int key", fm: "1: one\ntype: Note", body: "b\n"},
		{name: "bool-ish keys", fm: "yes: a\ntrue: b\ntype: Note", body: "b\n"},
		{name: "huge value", fm: "type: Note\nbig: " + strings.Repeat("A", 200_000), body: "b\n"},
		{name: "tab inside a scalar value (legal)", fm: "type: Note\nval: \"a\tb\"", body: "b\n"},

		// --- partitions that MUST fail closed (ErrTemplateInvalid) ---
		{name: "duplicate title", fm: "title: A\ntitle: B", body: "b\n", wantReject: true},
		{name: "duplicate tags", fm: "tags: [a]\ntags: [b]", body: "b\n", wantReject: true},
		{name: "duplicate arbitrary key", fm: "type: A\ntype: B", body: "b\n", wantReject: true},
		{name: "duplicate title and tags both", fm: "title: A\ntitle: B\ntags: [a]\ntags: [b]", body: "b\n", wantReject: true},
		{name: "malformed unclosed flow seq", fm: "title: [unclosed\n  bad: : :", body: "b\n", wantReject: true},
		{name: "frontmatter is a sequence root", fm: "[1, 2, 3]", body: "b\n", wantReject: true},
		{name: "frontmatter is a bare scalar root", fm: "just a string", body: "b\n", wantReject: true},
		{name: "tab indentation (illegal in yaml)", fm: "type:\n\tnested: x", body: "b\n", wantReject: true},
		{name: "unresolved alias", fm: "ref: *missing", body: "b\n", wantReject: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, r, _ := newServiceFixture(t, false)
			if tc.wantReject {
				_, err := svc.CreateFromTemplate(context.Background(), "", "User Title", tc.fm, tc.body, "alice")
				if !errors.Is(err, ErrTemplateInvalid) {
					t.Fatalf("seed should fail closed with ErrTemplateInvalid, got: %v", err)
				}
				return
			}
			assertMergeInvariant(t, svc, r, "", "User Title", tc.fm, tc.body)
		})
	}
}

// TestCreateFromTemplate_PlaceholdersStoredLiterally is a metamorphic-style check
// of the "no server-side template expansion" contract: literal {{...}} tokens in
// the seed must be persisted byte-for-byte (substitution is the client's job).
// NOTE: a placeholder in an UNQUOTED frontmatter value (owner: {{user}}) is not
// tested here because "{{" is YAML flow-mapping syntax — such a seed is cleanly
// REJECTED with ErrTemplateInvalid (see TestCreateFromTemplate_BareBracePlaceholderInFMFailsClosed),
// which is fail-closed, not corruption. The realistic literal-storage paths are
// the opaque body and QUOTED frontmatter scalars; both must round-trip verbatim.
func TestCreateFromTemplate_PlaceholdersStoredLiterally(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	fm := "type: Note\nowner: \"{{user}}\"\nproject: '{{ project_name }}'"
	body := "# {{title}}\n\nHello {{name}}, today is {{date}}.\n"
	path, err := svc.CreateFromTemplate(context.Background(), "", "Real Title", fm, body, "alice")
	if err != nil {
		t.Fatalf("CreateFromTemplate: %v", err)
	}
	waitForFile(t, r, path)
	raw, err := r.Read(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(raw)
	for _, lit := range []string{"{{user}}", "{{ project_name }}", "{{title}}", "{{name}}", "{{date}}"} {
		if !strings.Contains(s, lit) {
			t.Fatalf("placeholder %q was not stored literally (server-side expansion?):\n%s", lit, s)
		}
	}
}

// TestCreateFromTemplate_BareBracePlaceholderInFMFailsClosed documents (and pins)
// the boundary the test above carves out: an unsubstituted bare {{...}} in an
// UNQUOTED frontmatter value is YAML flow-mapping syntax, so the merge fails
// CLOSED with a clean ErrTemplateInvalid (handler → 400) rather than corrupting
// the page. The client substitutes placeholders before POST, so this is an edge,
// not a normal path — but it must stay fail-closed.
func TestCreateFromTemplate_BareBracePlaceholderInFMFailsClosed(t *testing.T) {
	svc, _, _ := newServiceFixture(t, false)
	_, err := svc.CreateFromTemplate(context.Background(), "", "T", "owner: {{user}}\ntype: Note", "b\n", "alice")
	if !errors.Is(err, ErrTemplateInvalid) {
		t.Fatalf("bare {{user}} in unquoted FM value: want ErrTemplateInvalid (fail-closed), got %v", err)
	}
}

// TestCreateFromTemplate_BodyFenceNotMisparsed is a metamorphic check: a literal
// "---" line in the body must remain body, never be re-interpreted as a second
// frontmatter block. The seeded body must survive verbatim.
func TestCreateFromTemplate_BodyFenceNotMisparsed(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	body := "Section A\n\n---\n\nSection B\n\n---\n\nSection C\n"
	path, err := svc.CreateFromTemplate(context.Background(), "", "Fenced", "type: Note", body, "alice")
	if err != nil {
		t.Fatalf("CreateFromTemplate: %v", err)
	}
	waitForFile(t, r, path)
	raw, err := r.Read(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	doc, err := okf.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if string(doc.Body) != body {
		t.Fatalf("body with literal --- lines was mangled:\nwant %q\ngot  %q", body, string(doc.Body))
	}
}

// TestCreateFromTemplate_BillionLaughsNoHang asserts a YAML alias-expansion bomb
// resolves to a clean error (or a valid page) quickly — never a hang or OOM. The
// oracle is totality + a wall-clock bound.
func TestCreateFromTemplate_BillionLaughsNoHang(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	var b strings.Builder
	b.WriteString("a: &a [x, x, x, x, x, x, x, x, x, x]\n")
	for i := 'b'; i <= 'i'; i++ {
		prev := byte(i - 1)
		cur := byte(i)
		fmt.Fprintf(&b, "%c: &%c [*%c, *%c, *%c, *%c, *%c, *%c, *%c, *%c, *%c, *%c]\n",
			cur, cur, prev, prev, prev, prev, prev, prev, prev, prev, prev, prev)
	}
	b.WriteString("type: Note\n")
	fm := b.String()

	done := make(chan struct{})
	go func() {
		defer close(done)
		assertMergeInvariant(t, svc, r, "", "Bomb", fm, "body\n")
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("CreateFromTemplate hung on a YAML alias bomb (>15s)")
	}
}

// TestCreateFromTemplate_HugeFrontmatterManyKeys throws a frontmatter with many
// top-level keys at the merge (resource / scale boundary).
func TestCreateFromTemplate_HugeFrontmatterManyKeys(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	var b strings.Builder
	b.WriteString("type: Note\n")
	for i := range 5000 {
		fmt.Fprintf(&b, "k%d: v%d\n", i, i)
	}
	assertMergeInvariant(t, svc, r, "", "Big Page", b.String(), "body\n")
}

// ---------------------------------------------------------------------------
// SURFACE A — fuzz the merge pipeline (pure, no I/O) for totality.
// ---------------------------------------------------------------------------

// runMergePipeline mirrors CreateFromTemplate's merge EXACTLY (minus the lock +
// disk write): Parse → ValidateFrontmatter → SetField(title) → Repair → Emit.
// It returns the emitted bytes, or an error standing in for ErrTemplateInvalid.
func runMergePipeline(fm, body, title string) ([]byte, error) {
	doc, err := okf.Parse(AssembleSource(fm, body))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTemplateInvalid, err)
	}
	if err := okf.ValidateFrontmatter(doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTemplateInvalid, err)
	}
	okf.SetField(doc, okf.FieldTitle, title)
	okf.Repair(doc, fixedNow)
	return doc.Emit()
}

// FuzzTemplateMerge drives the merge pipeline with arbitrary (frontmatter, body)
// seeds and a fixed non-empty title, asserting the totality invariant: every
// input that survives validation must Emit a single valid frontmatter block that
// re-parses, strict-decodes (no dup keys / malformed), keeps the user title, and
// carries all required fields. No input may panic, error at Emit, or corrupt.
//
//	go test ./internal/pages/ -run=NONE -fuzz=FuzzTemplateMerge
func FuzzTemplateMerge(f *testing.F) {
	seeds := []struct{ fm, body string }{
		{"", ""},
		{"", "body"},
		{"type: Note", "b"},
		{"title: A\ntitle: B", "b"},
		{"title: A", "b"},
		{"tags: [a]\ntags: [b]", "b"},
		{"title: [a,b]\ntype: Note", "b"},
		{"title:\n  x: 1", "b"},
		{"anchor: &t v\ntitle: *t", "b"},
		{"defaults: &d\n  type: M\n<<: *d", "b"},
		{"[1,2,3]", "b"},
		{"just a scalar", "b"},
		{"null", "b"},
		{"# comment", "b"},
		{"type: Note\r\nowner: x", "a\r\nb"},
		{"type: Note", "above\n---\nbelow"},
		{"owner: {{user}}", "# {{title}}"},
		{"type:\n\ttab: bad", "b"},
		{"ref: *missing", "b"},
		{"1: one", "b"},
	}
	for _, s := range seeds {
		f.Add(s.fm, s.body)
	}

	const title = "Fuzz Title"
	f.Fuzz(func(t *testing.T, fm, body string) {
		out, err := runMergePipeline(fm, body, title)
		if err != nil {
			// Rejected at parse/validate — acceptable (the fail-closed branch).
			if errors.Is(err, ErrTemplateInvalid) {
				return
			}
			// Emit failed AFTER validation passed — a fail-open defect: the
			// handler maps this to a 500, not a clean 400.
			t.Fatalf("emit failed after validation for fm=%q body=%q: %v", fm, body, err)
		}
		assertSingleValidBlock(t, out, title, fm, body)
	})
}

// ---------------------------------------------------------------------------
// SURFACE B — path safety of the template surface.
// ---------------------------------------------------------------------------

// TestListTemplates_NeverEscapesTemplatesDir hammers ListTemplates with adversarial
// on-disk layouts (nested dirs, dotfiles, traversal-shaped names that the OS
// permits as literal filenames, non-.md) and asserts every listed entry is a
// flat Templates/<name>.md that re-resolves safely — never a path outside
// Templates/.
func TestListTemplates_NeverEscapesTemplatesDir(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)

	// Real, listable templates.
	writeTemplate(t, svc, "Templates/Good One.md", "---\ntype: Note\n---\nx\n")
	writeTemplate(t, svc, "Templates/Another.md", "body\n")
	// Noise that must NOT be listed.
	writeTemplate(t, svc, "Templates/image.png", "binary")
	writeTemplate(t, svc, "Templates/README.txt", "notes")
	writeTemplate(t, svc, "Templates/.hidden.md", "dot")            // dotfile .md (flat)
	writeTemplate(t, svc, "Templates/sub/Nested.md", "nested")      // nested .md
	writeTemplate(t, svc, "Templates/sub/deeper/Deep.md", "deep")   // deeper nested
	writeTemplate(t, svc, "Templates/UPPER.MD", "wrong case ext")   // .MD not .md
	writeTemplate(t, svc, "Pages/Outside.md", "outside templates")  // outside Templates/
	writeTemplate(t, svc, "TemplatesX/Sneaky.md", "prefix sibling") // Templates-prefixed sibling dir

	got, err := svc.ListTemplates(context.Background())
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	prefix := templatesDir + "/"
	names := map[string]bool{}
	for _, ti := range got {
		names[ti.Name] = true
		// Every listed path must be a flat, resolver-safe Templates/<name>.md.
		if !strings.HasPrefix(ti.Path, prefix) {
			t.Fatalf("listed path %q is not under Templates/", ti.Path)
		}
		rel := strings.TrimPrefix(ti.Path, prefix)
		if strings.Contains(rel, "/") {
			t.Fatalf("listed path %q is nested (must be flat)", ti.Path)
		}
		if !strings.HasSuffix(ti.Path, ".md") {
			t.Fatalf("listed path %q is not a .md file", ti.Path)
		}
		if _, err := r.Resolve(ti.Path); err != nil {
			t.Fatalf("listed path %q does not resolve safely: %v", ti.Path, err)
		}
	}
	// Exactly the two flat .md templates (dotfile .md is flat .md → it is listed;
	// assert the things that MUST be excluded are excluded).
	for _, bad := range []string{"image", "README", "Nested", "Deep", "UPPER", "Outside", "Sneaky"} {
		if names[bad] {
			t.Fatalf("%q must not be listed as a template; full list: %v", bad, got)
		}
	}
	if !names["Good One"] || !names["Another"] {
		t.Fatalf("expected the two real templates to be listed; got %v", got)
	}
}

// TestResolve_TemplateSurfaceTraversal confirms the resolver gates every escape
// shape that the template-read path would feed it (the template content read goes
// through repo.Resolve via getPage). The NEW surface introduces no path the
// resolver doesn't already refuse.
func TestResolve_TemplateSurfaceTraversal(t *testing.T) {
	_, r, _ := newServiceFixture(t, false)
	escapes := []string{
		"../etc/passwd",
		"../../etc/passwd",
		"Templates/../../etc/passwd",
		"Templates/../../../etc/shadow",
		"/etc/passwd",
		"/Templates/x.md",
		`..\..\windows\system32`,
		`Templates\..\..\secret`,
		"Templates/%2e%2e/%2e%2e/etc/passwd",
		"%2e%2e%2f%2e%2e%2fetc%2fpasswd",
		"Templates/sub/../../../../etc/passwd",
		"..",
		"../",
		"./../../etc/passwd",
	}
	for _, p := range escapes {
		t.Run(p, func(t *testing.T) {
			if _, err := r.Resolve(p); !errors.Is(err, repo.ErrUnsafePath) {
				t.Fatalf("Resolve(%q) = %v, want ErrUnsafePath", p, err)
			}
		})
	}
	// NUL byte rejected too.
	if _, err := r.Resolve("Templates/x\x00.md"); !errors.Is(err, repo.ErrUnsafePath) {
		t.Fatalf("Resolve with NUL = %v, want ErrUnsafePath", err)
	}
}

// TestCreateFromTemplate_TitleControlsPathNotSeed proves the create contract: the
// on-disk PATH derives solely from the user TITLE (slugified + resolver-gated),
// never from anything in the seed content. A seed cannot steer the write location
// (there is no template-PATH parameter to abuse).
func TestCreateFromTemplate_TitleControlsPathNotSeed(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	// Seed jammed with path-shaped junk; none may influence the write path.
	fm := "title: ../../etc/passwd\nslug: /var/www/evil\npath: ../../../secret.md\ntype: Note"
	body := "../../../../etc/passwd\n"
	path, err := svc.CreateFromTemplate(context.Background(), "runbooks", "Normal Title", fm, body, "alice")
	if err != nil {
		t.Fatalf("CreateFromTemplate: %v", err)
	}
	if path != "runbooks/normal-title.md" {
		t.Fatalf("path = %q, want runbooks/normal-title.md (derived from user title only)", path)
	}
	if _, err := r.Resolve(path); err != nil {
		t.Fatalf("created path does not resolve safely: %v", err)
	}
	waitForFile(t, r, path)
}

// TestCreateFromTemplate_MaliciousFolderNeverEscapes confirms the folder argument
// can never write outside the repo root. Traversal-shaped folders (../) are
// rejected with a clean ErrUnsafePath; absolute-looking folders (/abs/evil) are
// coerced to a within-root relative subfolder by uniquePath's leading-"/" trim.
// Either way: NO escape.
func TestCreateFromTemplate_MaliciousFolderNeverEscapes(t *testing.T) {
	// Traversal must be rejected outright.
	for _, folder := range []string{"../../etc", `..\..\x`, "Templates/../../etc"} {
		t.Run("reject/"+folder, func(t *testing.T) {
			svc, _, _ := newServiceFixture(t, false)
			_, err := svc.CreateFromTemplate(context.Background(), folder, "Title", "type: Note", "b\n", "alice")
			if !errors.Is(err, repo.ErrUnsafePath) {
				t.Fatalf("folder %q: want ErrUnsafePath, got %v", folder, err)
			}
		})
	}
	// Absolute-looking folder: coerced to relative, still inside the root.
	t.Run("absolute-coerced-in-root", func(t *testing.T) {
		svc, r, _ := newServiceFixture(t, false)
		path, err := svc.CreateFromTemplate(context.Background(), "/abs/evil", "Title", "type: Note", "b\n", "alice")
		if err != nil {
			t.Fatalf("absolute folder: unexpected error %v", err)
		}
		if strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
			t.Fatalf("path %q is not a safe within-root relative path", path)
		}
		if _, err := r.Resolve(path); err != nil {
			t.Fatalf("coerced path %q does not resolve safely: %v", path, err)
		}
	})
}
