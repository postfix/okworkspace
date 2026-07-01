package pages

import (
	"context"
	"strings"
	"testing"

	"github.com/postfix/okworkspace/internal/okf"
)

// seedBodyPage creates a page then Saves the given body + frontmatter tags,
// returning its path and CURRENT committed revision (the base_revision a batch
// would carry). Drives the real single-writer commit path so the page exists in
// git history with the given body.
func seedBodyPage(t *testing.T, svc *Service, r interface {
	Read(string) ([]byte, error)
}, title, body string, tags []string) (path, rev string) {
	t.Helper()
	ctx := context.Background()
	p, err := svc.Create(ctx, "", title, "alice")
	if err != nil {
		t.Fatalf("seed Create %q: %v", title, err)
	}
	waitForFileGeneric(t, svc, p)
	pg, err := svc.Get(ctx, p)
	if err != nil {
		t.Fatalf("seed Get %q: %v", p, err)
	}
	front := pg.Frontmatter
	if tags != nil {
		nf, ferr := SetTagsFrontmatter(pg.Frontmatter, body, tags)
		if ferr != nil {
			t.Fatalf("seed SetTagsFrontmatter: %v", ferr)
		}
		front = nf
	}
	if err := svc.Save(ctx, p, body, front, pg.Revision, "alice"); err != nil {
		t.Fatalf("seed Save %q: %v", p, err)
	}
	waitForRevisionChanged(t, svc, p, pg.Revision)
	pg2, err := svc.Get(ctx, p)
	if err != nil {
		t.Fatalf("seed re-Get %q: %v", p, err)
	}
	return p, pg2.Revision
}

func waitForFileGeneric(t *testing.T, svc *Service, path string) {
	t.Helper()
	deadline := 300
	for range deadline {
		if ok, _ := svc.repo.Exists(path); ok {
			return
		}
	}
	// Fall back to a real poll if the tight loop raced the worker.
	waitForRevisionChangedExists(t, svc, path)
}

func waitForRevisionChangedExists(t *testing.T, svc *Service, path string) {
	t.Helper()
	for range 600 {
		if ok, _ := svc.repo.Exists(path); ok {
			return
		}
	}
	t.Fatalf("file %q never appeared", path)
}

// TestBodyRewriteRenameGolden is the TR-03 regression-closed golden: rename
// #ops→#devops on a page with body #ops (and ops in frontmatter) rewrites BOTH
// the body (#ops→#devops) and the frontmatter ([devops]) in one commit. Feeding
// the committed body back through the recognizer shows `ops` is gone — so the
// client reconcile on the next (unchanged-body) save cannot re-materialize it.
func TestBodyRewriteRenameGolden(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	p, rev := seedBodyPage(t, svc, r, "Runbook", "Deploy notes\n\nsee #ops here\n", []string{"ops"})

	results, err := svc.ApplyTagsBatch(ctx, []TagApplyItem{{
		PagePath: p, Tags: []string{"devops"}, BaseRevision: rev,
		BodyOld: "ops", BodyNew: "devops",
	}}, "alice")
	if err != nil {
		t.Fatalf("ApplyTagsBatch: %v", err)
	}
	if results[0].Status != TagApplyApplied {
		t.Fatalf("status = %q, want applied", results[0].Status)
	}

	body := readBody(t, r, p)
	fm := readFrontmatter(t, r, p)
	if !strings.Contains(body, "#devops") {
		t.Errorf("body missing #devops: %q", body)
	}
	if strings.Contains(body, "#ops") {
		t.Errorf("body still contains #ops (regression NOT closed): %q", body)
	}
	if !strings.Contains(fm, "devops") || strings.Contains(fm, "- ops\n") {
		t.Errorf("frontmatter not [devops]: %q", fm)
	}
	// Regression proof: the recognizer over the committed body yields no `ops`, so
	// reconcile (fmTags ∪ inlineTags) cannot re-add it.
	if bodyHasTag([]byte("---\n"+fm+"---\n"+body), "ops") {
		t.Error("committed body still recognizes #ops — reconcile would re-add it")
	}
}

// TestBodyRewriteDeleteGolden: delete #ops removes it from body + frontmatter.
func TestBodyRewriteDeleteGolden(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()
	p, rev := seedBodyPage(t, svc, r, "DelPage", "text #ops here\n", []string{"ops"})

	results, err := svc.ApplyTagsBatch(ctx, []TagApplyItem{{
		PagePath: p, Tags: []string{}, BaseRevision: rev, BodyOld: "ops", BodyNew: "",
	}}, "alice")
	if err != nil {
		t.Fatalf("ApplyTagsBatch: %v", err)
	}
	if results[0].Status != TagApplyApplied {
		t.Fatalf("status = %q, want applied", results[0].Status)
	}
	body := readBody(t, r, p)
	if strings.Contains(body, "#ops") {
		t.Errorf("body still contains #ops after delete: %q", body)
	}
	// D-03a: space-bounded → "text here".
	if !strings.Contains(body, "text here") {
		t.Errorf("delete whitespace rule not applied: %q", body)
	}
}

// TestBodyRewriteMergeGolden: merge #ops→#devops rewrites body #ops to #devops.
func TestBodyRewriteMergeGolden(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()
	p, rev := seedBodyPage(t, svc, r, "MergePage", "body #ops here\n", []string{"ops"})

	results, err := svc.ApplyTagsBatch(ctx, []TagApplyItem{{
		PagePath: p, Tags: []string{"devops"}, BaseRevision: rev, BodyOld: "ops", BodyNew: "devops",
	}}, "alice")
	if err != nil {
		t.Fatalf("ApplyTagsBatch: %v", err)
	}
	if results[0].Status != TagApplyApplied {
		t.Fatalf("status = %q, want applied", results[0].Status)
	}
	body := readBody(t, r, p)
	if !strings.Contains(body, "#devops") || strings.Contains(body, "#ops") {
		t.Errorf("merge did not rewrite body: %q", body)
	}
}

// TestBodyRewriteBodyOnlyGolden: a page with #ops ONLY in the body (frontmatter
// has NO ops) is still written — the body is rewritten and the frontmatter tags
// region is left byte-identical (SkipFrontmatter). This is the body-only page
// criterion (previously dropped at changed=false).
func TestBodyRewriteBodyOnlyGolden(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()
	// Frontmatter has a DIFFERENT tag; body has #ops only.
	p, rev := seedBodyPage(t, svc, r, "BodyOnly", "prose #ops here\n", []string{"docs"})
	fmBefore := frontRegion(t, r, p)

	results, err := svc.ApplyTagsBatch(ctx, []TagApplyItem{{
		PagePath: p, Tags: []string{"docs"}, BaseRevision: rev,
		SkipFrontmatter: true, BodyOld: "ops", BodyNew: "devops",
	}}, "alice")
	if err != nil {
		t.Fatalf("ApplyTagsBatch: %v", err)
	}
	if results[0].Status != TagApplyApplied {
		t.Fatalf("status = %q, want applied", results[0].Status)
	}
	body := readBody(t, r, p)
	if !strings.Contains(body, "#devops") || strings.Contains(body, "#ops") {
		t.Errorf("body-only page body not rewritten: %q", body)
	}
	// Frontmatter tags region byte-identical (SkipFrontmatter).
	if fm := frontRegion(t, r, p); fm != fmBefore {
		t.Errorf("body-only page frontmatter region changed:\nbefore=%q\nafter =%q", fmBefore, fm)
	}
}

// frontRegion returns ONLY the parsed frontmatter region (doc.RawFront) of a
// page, so a test can assert the frontmatter is byte-identical independent of a
// body change.
func frontRegion(t *testing.T, r interface{ Read(string) ([]byte, error) }, path string) string {
	t.Helper()
	raw, err := r.Read(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	doc, err := okf.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", path, err)
	}
	return string(doc.RawFront)
}

// TestBodyRewriteStaleClient409 is the stale-open-client invariant: a Save
// carrying the PRE-rewrite body at the PRE-rewrite base_revision is rejected
// ErrStaleRevision (409) after the server rewrite lands — never clobbering the
// rewrite. This is the path a client-only fix cannot cover.
func TestBodyRewriteStaleClient409(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()
	p, rev := seedBodyPage(t, svc, r, "Stale", "see #ops here\n", []string{"ops"})

	// Server rewrite lands (moves the revision).
	if _, err := svc.ApplyTagsBatch(ctx, []TagApplyItem{{
		PagePath: p, Tags: []string{"devops"}, BaseRevision: rev, BodyOld: "ops", BodyNew: "devops",
	}}, "alice"); err != nil {
		t.Fatalf("ApplyTagsBatch: %v", err)
	}
	waitForRevisionChanged(t, svc, p, rev)

	// A stale open client saves the PRE-rewrite body at the PRE-rewrite revision.
	pgNow, _ := svc.Get(ctx, p)
	err := svc.Save(ctx, p, "see #ops here\n", pgNow.Frontmatter, rev /* stale */, "staleclient")
	if err == nil {
		t.Fatal("stale save succeeded; want ErrStaleRevision (409)")
	}
	if err != ErrStaleRevision {
		t.Fatalf("stale save err = %v, want ErrStaleRevision", err)
	}
	// The server rewrite was NOT clobbered.
	if body := readBody(t, r, p); strings.Contains(body, "#ops") {
		t.Errorf("rewrite clobbered by stale client: %q", body)
	}
}
