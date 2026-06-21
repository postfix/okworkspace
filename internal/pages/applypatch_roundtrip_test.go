// applypatch_roundtrip_test.go is the CR-01 regression gate: the agent
// propose→apply contract is BODY-ONLY (the proposed new_body carries no
// frontmatter region; the page's original frontmatter is server-owned and
// re-attached by pages.Save exactly once). Before CR-01 the propose path returned
// a FULL source (frontmatter + body) as new_body AND apply re-prepended the
// frontmatter again — Save assembled `---FM---` + `---FM---body`, okf.Parse
// consumed the first fence and the literal second `---FM---` block leaked into the
// saved body. The page was corrupted with a stray YAML fence (Failure Mode #4 /
// patch corruption), and the existing agent apply_test.go missed it because it
// modeled Save with an abstract stub instead of the real assemble/okf round-trip.
//
// This test drives the REAL pages.Service.Save (→ assemble → okf.Parse → Repair →
// okf.Emit → on-disk write) the way handleApplyPatch does, then re-reads the saved
// file from disk via the real okf path and asserts: EXACTLY ONE frontmatter fence
// block, the one-line body change applied, and NO stray `---` fence inside the
// body. It would FAIL against the pre-CR-01 double-write.
package pages

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/okf"
	"github.com/postfix/okworkspace/internal/repo"
)

func TestApplyPatchBodyOnlyRoundTrip(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	// Seed a page WITH frontmatter and a multi-line body (a fenced code block whose
	// content contains a literal "---" line, to prove the body stays opaque).
	path, err := svc.Create(ctx, "", "Notes", "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitForFile(t, r, path)

	// Establish a known body+frontmatter via a first Save (the page now has a real
	// frontmatter region the apply path will preserve).
	page, err := svc.Get(ctx, path)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	const originalBody = "# Notes\n\nA short intro paragraph.\n\n```\nnot --- a fence\n```\n"
	if err := svc.Save(ctx, path, originalBody, page.Frontmatter, page.Revision, "alice"); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	// Re-read so we have the committed frontmatter region + the fresh revision that
	// the propose path would capture as base_revision.
	page = waitForBody(t, svc, path, "A short intro paragraph.")
	frontmatter := page.Frontmatter
	baseRev := page.Revision

	// The agent proposes a BODY-ONLY one-line change (no frontmatter region). This
	// is exactly what ProposePatch now returns and what handleApplyPatch forwards as
	// req.NewBody, with req.Frontmatter = the page's original frontmatter region.
	proposedBody := strings.Replace(originalBody, "A short intro paragraph.", "A revised intro paragraph.", 1)
	if strings.HasPrefix(strings.TrimSpace(proposedBody), "---") {
		t.Fatal("test precondition: the proposed body must NOT carry a frontmatter fence")
	}

	// Apply exactly as handleApplyPatch does: Save(path, body_only, original_fm, rev).
	if err := svc.Save(ctx, path, proposedBody, frontmatter, baseRev, "alice"); err != nil {
		t.Fatalf("apply Save: %v", err)
	}

	// Re-read the SAVED FILE from disk through the REAL okf path and assert there is
	// exactly one frontmatter fence + the body change applied + no stray fence.
	saved := waitForBody(t, svc, path, "A revised intro paragraph.")

	doc, err := okf.Parse([]byte(rawFile(t, r, path)))
	if err != nil {
		t.Fatalf("okf.Parse saved file: %v", err)
	}
	if !doc.HasFrontmatter {
		t.Fatal("saved file lost its frontmatter region")
	}
	// The opaque body must NOT itself open with a `---` fence (the CR-01 stray
	// second frontmatter block). okf.Parse on the body alone must report no
	// frontmatter — i.e. the body is pure content.
	bodyDoc, err := okf.Parse(doc.Body)
	if err != nil {
		t.Fatalf("okf.Parse saved body: %v", err)
	}
	if bodyDoc.HasFrontmatter {
		t.Fatalf("saved body contains a STRAY frontmatter fence (CR-01 double-write); body:\n%s", doc.Body)
	}

	// Belt-and-suspenders: the whole saved source must contain EXACTLY ONE `---`
	// fence PAIR (two fence lines: the open and close of the single frontmatter
	// region). The pre-CR-01 bug produced four fence lines.
	wholeFile := rawFile(t, r, path)
	if n := countFenceLines(wholeFile); n != 2 {
		t.Fatalf("saved file has %d frontmatter fence lines, want exactly 2 (one region); file:\n%s", n, wholeFile)
	}

	// And the body change is present while the original line is gone.
	if !strings.Contains(saved.Body, "A revised intro paragraph.") {
		t.Fatalf("body change not applied; saved body:\n%s", saved.Body)
	}
	if strings.Contains(saved.Body, "A short intro paragraph.") {
		t.Fatalf("original line still present after apply; saved body:\n%s", saved.Body)
	}
	// The fenced code block's literal "---" content must survive untouched (opaque
	// body, never re-parsed as a fence).
	if !strings.Contains(saved.Body, "not --- a fence") {
		t.Fatalf("opaque body content was mangled; saved body:\n%s", saved.Body)
	}
}

// waitForBody polls Get until the page body contains want, returning the page.
func waitForBody(t *testing.T, svc *Service, path, want string) Page {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, err := svc.Get(context.Background(), path)
		if err == nil && strings.Contains(got.Body, want) {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	got, _ := svc.Get(context.Background(), path)
	t.Fatalf("page body never contained %q; got:\n%s", want, got.Body)
	return Page{}
}

// rawFile reads the on-disk bytes of a repo-relative path (the exact saved file).
func rawFile(t *testing.T, r *repo.Repo, path string) string {
	t.Helper()
	b, err := r.Read(path)
	if err != nil {
		t.Fatalf("read saved file %q: %v", path, err)
	}
	return string(b)
}

// countFenceLines counts lines that are exactly a "---" frontmatter fence marker
// (a line whose only content is "---"). The single frontmatter region contributes
// exactly two (open + close); a CR-01 double-write contributes four.
func countFenceLines(src string) int {
	n := 0
	for _, line := range strings.Split(src, "\n") {
		if strings.TrimRight(line, "\r") == "---" {
			n++
		}
	}
	return n
}
