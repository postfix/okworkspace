package pages

import (
	"context"
	"reflect"
	"sort"
	"testing"
)

// writeRaw writes raw bytes directly to the repo working tree (no commit needed —
// PagesWithBodyTag/DetectStaleBodyTags read repo.Tree()/repo.Read off disk).
func writeRaw(t *testing.T, svc *Service, path, content string) {
	t.Helper()
	if err := svc.repo.Write(path, []byte(content)); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func page(fmTags, body string) string {
	fm := "---\ntype: page\ntitle: t\ndescription: \n"
	if fmTags != "" {
		fm += "tags:\n" + fmTags
	}
	fm += "timestamp: 2026-06-21T00:00:00Z\n---\n\n"
	return fm + body + "\n"
}

// TestPagesWithBodyTag is the TR-04 body-inclusive enumeration gate: it finds a
// page whose #tag is body-only (absent from frontmatter → invisible to
// PagesByTag), excludes a fenced-code occurrence, and skips trashed/non-.md.
func TestPagesWithBodyTag(t *testing.T) {
	svc, _, _ := newServiceFixture(t, false)
	ctx := context.Background()

	// body-only: #ops in prose, NOT in frontmatter → must be found.
	writeRaw(t, svc, "notes/bodyonly.md", page("", "see #ops here"))
	// frontmatter-only: ops in tags:, NOT in body → must NOT be found by a body scan.
	writeRaw(t, svc, "notes/fmonly.md", page("  - ops\n", "plain body, no tag"))
	// both: #ops in body AND frontmatter → found (has a body occurrence).
	writeRaw(t, svc, "notes/both.md", page("  - ops\n", "body #ops here"))
	// fenced-code: #ops only inside a fence → excluded → NOT found.
	writeRaw(t, svc, "notes/fenced.md", page("", "```\n#ops in code\n```\n"))
	// unrelated: a different body tag → not found for "ops".
	writeRaw(t, svc, "notes/other.md", page("", "body #docs here"))
	// trashed: #ops in body but under the trash dir → skipped.
	writeRaw(t, svc, trashDir+"/deleted.md", page("", "trashed #ops here"))
	// non-.md: #ops in a text file → skipped.
	writeRaw(t, svc, "notes/readme.txt", "loose #ops text")

	got, err := svc.PagesWithBodyTag(ctx, "ops")
	if err != nil {
		t.Fatalf("PagesWithBodyTag: %v", err)
	}
	sort.Strings(got)
	want := []string{"notes/bodyonly.md", "notes/both.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PagesWithBodyTag(ops) = %v, want %v", got, want)
	}
}

// TestPagesWithBodyTag_CaseAndEmpty: normalization + empty guard.
func TestPagesWithBodyTag_CaseAndEmpty(t *testing.T) {
	svc, _, _ := newServiceFixture(t, false)
	ctx := context.Background()
	writeRaw(t, svc, "notes/cap.md", page("", "body #Ops here")) // #Ops → normalized ops

	got, err := svc.PagesWithBodyTag(ctx, "OPS") // caller may pass any case
	if err != nil {
		t.Fatalf("PagesWithBodyTag: %v", err)
	}
	if len(got) != 1 || got[0] != "notes/cap.md" {
		t.Errorf("case-insensitive match = %v, want [notes/cap.md]", got)
	}

	empty, err := svc.PagesWithBodyTag(ctx, "   ")
	if err != nil {
		t.Fatalf("PagesWithBodyTag empty: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("empty tag returned %v, want none", empty)
	}
}

// TestDetectStaleBodyTags is the TR-06 detect+report gate: a pre-fix-corrupted
// page (body #ops, frontmatter WITHOUT ops) is reported; a coherent page (body
// #ops in frontmatter) and a frontmatter-only page are NOT flagged.
func TestDetectStaleBodyTags(t *testing.T) {
	svc, _, _ := newServiceFixture(t, false)
	ctx := context.Background()

	// pre-fix corruption: body has #ops, frontmatter has [devops] (a rename ran
	// on frontmatter but the body #ops was left stale).
	writeRaw(t, svc, "notes/corrupt.md", page("  - devops\n", "still says #ops here"))
	// coherent: body #ops IS in frontmatter (normal ED-02b state) → not flagged.
	writeRaw(t, svc, "notes/ok.md", page("  - ops\n", "body #ops here"))
	// frontmatter-only: no body tag at all → not flagged.
	writeRaw(t, svc, "notes/fmonly.md", page("  - ops\n", "no body tag"))
	// fenced-only #ghost → excluded from body scan → not flagged.
	writeRaw(t, svc, "notes/fenced.md", page("", "```\n#ghost\n```\n"))

	got, err := svc.DetectStaleBodyTags(ctx)
	if err != nil {
		t.Fatalf("DetectStaleBodyTags: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("DetectStaleBodyTags found %d, want 1: %+v", len(got), got)
	}
	if got[0].PagePath != "notes/corrupt.md" {
		t.Errorf("flagged %q, want notes/corrupt.md", got[0].PagePath)
	}
	if !reflect.DeepEqual(got[0].BodyOnlyTags, []string{"ops"}) {
		t.Errorf("BodyOnlyTags = %v, want [ops]", got[0].BodyOnlyTags)
	}
}
