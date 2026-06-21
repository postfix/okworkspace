package attachments

import (
	"context"
	"testing"
)

// TestPageReferences (ATT-07 core): PageReferences counts the pages whose Markdown
// contains the canonical DownloadRefPath(id), matches that exact form, ignores
// other ids, and counts a page that references the same id twice as ONE referencing
// page (page-level semantics — the orphan decision only needs zero-vs-nonzero).
func TestPageReferences(t *testing.T) {
	svc, _, _ := newTestService(t, []string{"txt"}, 100)
	id := "01TARGETATTACHMENT"
	other := "01OTHERATTACHMENT"
	ref := DownloadRefPath(id)
	otherRef := DownloadRefPath(other)

	// page-a references the target once.
	mustWrite(t, svc, "page-a.md", "See ["+"file"+"]("+ref+").\n")
	// page-b references the target TWICE (still ONE referencing page).
	mustWrite(t, svc, "page-b.md", "[a]("+ref+") and again [b]("+ref+")\n")
	// page-c references only a DIFFERENT id — must not count.
	mustWrite(t, svc, "page-c.md", "[x]("+otherRef+")\n")
	// page-d references nothing.
	mustWrite(t, svc, "page-d.md", "Just prose, no links.\n")

	got, err := PageReferences(context.Background(), svc.repo, id)
	if err != nil {
		t.Fatalf("PageReferences: %v", err)
	}
	if got != 2 {
		t.Fatalf("PageReferences(%q) = %d, want 2 (page-a + page-b)", id, got)
	}

	// The OTHER id is referenced by exactly one page.
	gotOther, err := PageReferences(context.Background(), svc.repo, other)
	if err != nil {
		t.Fatalf("PageReferences(other): %v", err)
	}
	if gotOther != 1 {
		t.Fatalf("PageReferences(%q) = %d, want 1 (page-c)", other, gotOther)
	}

	// An id referenced nowhere is zero (the orphan trigger).
	gotNone, err := PageReferences(context.Background(), svc.repo, "01NOBODYREFERENCESME")
	if err != nil {
		t.Fatalf("PageReferences(none): %v", err)
	}
	if gotNone != 0 {
		t.Fatalf("PageReferences(unreferenced) = %d, want 0", gotNone)
	}
}

// mustWrite writes a page directly through the repo (test seed; not a commit path).
func mustWrite(t *testing.T, svc *Service, path, body string) {
	t.Helper()
	if err := svc.repo.Write(path, []byte(body)); err != nil {
		t.Fatalf("seed write %q: %v", path, err)
	}
}
