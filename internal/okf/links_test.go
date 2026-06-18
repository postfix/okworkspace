package okf_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/postfix/okworkspace/internal/okf"
)

func TestRewriteLinks(t *testing.T) {
	// A page at architecture/overview.md links to ../runbooks/deploy.md. We rename
	// that target to ../runbooks/deploy-prod.md. fromDir is the linking page's dir.
	body := []byte("# Overview\n\n" +
		"See [Deploy](../runbooks/deploy.md) for the steps.\n\n" +
		"Code span `../runbooks/deploy.md` must not change.\n\n" +
		"```\n../runbooks/deploy.md\n```\n\n" +
		"Coincidence: [bak](../runbooks/deploy.md.bak) stays.\n" +
		"External [site](https://example.com/runbooks/deploy.md) stays.\n")

	out, changed := okf.RewriteLinks(body, "architecture", "runbooks/deploy.md", "runbooks/deploy-prod.md")
	if !changed {
		t.Fatal("RewriteLinks reported no change but a matching link exists")
	}
	got := string(out)

	// The matched inline-link destination is rewritten.
	if !strings.Contains(got, "[Deploy](../runbooks/deploy-prod.md)") {
		t.Fatalf("matched link not rewritten:\n%s", got)
	}
	// The code span is byte-unchanged.
	if !strings.Contains(got, "`../runbooks/deploy.md`") {
		t.Fatalf("code span was rewritten (must be structural, not substring):\n%s", got)
	}
	// The fenced code block literal is byte-unchanged.
	if !strings.Contains(got, "```\n../runbooks/deploy.md\n```") {
		t.Fatalf("fenced code block was rewritten:\n%s", got)
	}
	// A partial/substring coincidence is NOT rewritten.
	if !strings.Contains(got, "[bak](../runbooks/deploy.md.bak)") {
		t.Fatalf("substring-coincidence link was wrongly rewritten:\n%s", got)
	}
	// An external URL that contains the old name is NOT rewritten.
	if !strings.Contains(got, "https://example.com/runbooks/deploy.md") {
		t.Fatalf("external URL was wrongly rewritten:\n%s", got)
	}
}

func TestRewriteLinks_NoMatchIsByteIdentical(t *testing.T) {
	body := []byte("# Page\n\n[Unrelated](../other/page.md) link.\n")
	out, changed := okf.RewriteLinks(body, "architecture", "runbooks/deploy.md", "runbooks/deploy-prod.md")
	if changed {
		t.Fatal("RewriteLinks changed a body with no matching link")
	}
	if !bytes.Equal(out, body) {
		t.Fatal("no-match RewriteLinks must return byte-identical body")
	}
}

func TestRewriteLinks_RelativeRecompute(t *testing.T) {
	// Page B lives at runbooks/x.md and links to ../a/old.md (i.e. a/old.md).
	// a/old.md moves to b/new.md. The link in B must be recomputed to ../b/new.md.
	body := []byte("Link to [Old](../a/old.md).\n")
	out, changed := okf.RewriteLinks(body, "runbooks", "a/old.md", "b/new.md")
	if !changed {
		t.Fatal("expected the link to be rewritten")
	}
	if got := string(out); !strings.Contains(got, "[Old](../b/new.md)") {
		t.Fatalf("relative path not recomputed from B's location:\n%s", got)
	}
}

func TestRewriteLinks_PreservesFragment(t *testing.T) {
	body := []byte("Jump to [Step 2](../runbooks/deploy.md#step-2).\n")
	out, changed := okf.RewriteLinks(body, "architecture", "runbooks/deploy.md", "runbooks/deploy-prod.md")
	if !changed {
		t.Fatal("expected rewrite with a fragment")
	}
	if got := string(out); !strings.Contains(got, "[Step 2](../runbooks/deploy-prod.md#step-2)") {
		t.Fatalf("fragment not preserved on rewrite:\n%s", got)
	}
}

func TestRewriteLinks_ReferenceStyle(t *testing.T) {
	body := []byte("See [the deploy guide][dep].\n\n[dep]: ../runbooks/deploy.md\n")
	out, changed := okf.RewriteLinks(body, "architecture", "runbooks/deploy.md", "runbooks/deploy-prod.md")
	if !changed {
		t.Fatal("expected reference-style definition to be rewritten")
	}
	if got := string(out); !strings.Contains(got, "[dep]: ../runbooks/deploy-prod.md") {
		t.Fatalf("reference-style definition not rewritten:\n%s", got)
	}
}

// TestRewriteLinks_Corpus exercises the testdata fixture: only genuine inline
// link destinations to the old page change; the code span, the fenced block, and
// the .bak coincidence stay byte-identical.
func TestRewriteLinks_Corpus(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "links", "with_codeblock.md"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	doc, err := okf.Parse(raw)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	// The fixture lives at architecture/with_codeblock.md (fromDir=architecture).
	newBody, changed := okf.RewriteLinks(doc.Body, "architecture", "runbooks/deploy.md", "runbooks/deploy-prod.md")
	if !changed {
		t.Fatal("expected the fixture to have a rewritable link")
	}
	got := string(newBody)
	if strings.Count(got, "deploy-prod.md") != 2 {
		t.Fatalf("expected exactly 2 inline links rewritten, got %d:\n%s",
			strings.Count(got, "deploy-prod.md"), got)
	}
	// Code span + fenced block + .bak coincidence retain the OLD name.
	if !strings.Contains(got, "`../runbooks/deploy.md`") {
		t.Fatal("code span must keep the old name")
	}
	if !strings.Contains(got, "cat ../runbooks/deploy.md") {
		t.Fatal("fenced code block must keep the old name")
	}
	if !strings.Contains(got, "../runbooks/deploy.md.bak") {
		t.Fatal(".bak coincidence must keep the old name")
	}

	// Re-emit and assert the frontmatter region is byte-stable (only body changed).
	doc.Body = newBody
	out, err := doc.Emit()
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("---\ntype: Page")) {
		t.Fatal("frontmatter region was disturbed by the body rewrite")
	}
}

func TestFindLinks_SkipsCodeAndEscapes(t *testing.T) {
	body := []byte("A [real](a.md) link, `[code](nope.md)`, and \\[escaped](skip.md).\n")
	links := okf.FindLinks(body)
	if len(links) != 1 {
		t.Fatalf("expected exactly 1 link (code span + escape skipped), got %d", len(links))
	}
	if links[0].Dest != "a.md" {
		t.Fatalf("wrong link captured: %q", links[0].Dest)
	}
}
