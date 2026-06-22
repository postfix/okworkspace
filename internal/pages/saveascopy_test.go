package pages

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestSaveAsCopyLeavesOriginal is the COLL-04 backend proof of "fresh revision,
// original untouched". Save-as-copy is two existing steps — Create (a NEW deduped
// path with a FRESH revision) then Save(newPath) at that fresh revision — so it
// can NEVER write the original path nor carry the conflicted base revision. The
// original page's body and committed revision must be byte-identical to before; a
// repeat copy must auto-dedup via uniquePath. The frontend handler (Task 4) drives
// exactly this sequence via createPage/getPage/savePage.
func TestSaveAsCopyLeavesOriginal(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	// 1. Create the original page and give it a known body.
	orig, err := svc.Create(ctx, "notes", "Plan", "alice")
	if err != nil {
		t.Fatalf("Create original: %v", err)
	}
	if orig != "notes/plan.md" {
		t.Fatalf("original path = %q, want notes/plan.md", orig)
	}
	waitForFile(t, r, orig)

	origRev0, err := svc.Revision(ctx, orig)
	if err != nil {
		t.Fatalf("Revision orig (pre-save): %v", err)
	}
	if serr := svc.Save(ctx, orig, "ORIGINAL\n", "type: Page\ntitle: Plan\n", origRev0, "alice"); serr != nil {
		t.Fatalf("Save original body: %v", serr)
	}
	origRev := waitForNewRevision(t, svc, orig, origRev0)
	assertBodyContains(t, r, orig, "ORIGINAL")
	origBody, _ := r.Read(orig)

	// 2. Save-as-copy sequence (the backend half of COLL-04): Create a deduped
	//    sibling, read its FRESH revision, then Save MY body into the copy.
	newPath, err := svc.Create(ctx, "notes", "Plan (Copy)", "alice")
	if err != nil {
		t.Fatalf("Create copy: %v", err)
	}
	if newPath == orig {
		t.Fatalf("copy path equals the original (%q) — save-as-copy must be a new path", newPath)
	}
	if !strings.HasPrefix(newPath, "notes/") {
		t.Fatalf("copy path %q is not a deduped sibling under notes/", newPath)
	}
	waitForFile(t, r, newPath)

	freshRev, err := svc.Revision(ctx, newPath)
	if err != nil {
		t.Fatalf("Revision copy (fresh): %v", err)
	}
	if serr := svc.Save(ctx, newPath, "MINE\n", "type: Page\ntitle: Plan (Copy)\n", freshRev, "alice"); serr != nil {
		t.Fatalf("Save into copy at fresh rev: %v", serr)
	}
	assertBodyContains(t, r, newPath, "MINE")

	// 3. The ORIGINAL is byte-identical: same committed revision AND same body
	//    (never modified, never carries the conflicted base revision).
	time.Sleep(50 * time.Millisecond) // let any stray job drain before asserting stability
	origRevAfter, err := svc.Revision(ctx, orig)
	if err != nil {
		t.Fatalf("Revision orig (post-copy): %v", err)
	}
	if origRevAfter != origRev {
		t.Fatalf("original revision changed by save-as-copy (%q != %q) — the original must be untouched", origRevAfter, origRev)
	}
	if bodyAfter, _ := r.Read(orig); string(bodyAfter) != string(origBody) {
		t.Fatalf("original body changed by save-as-copy:\nbefore:\n%s\nafter:\n%s", origBody, bodyAfter)
	}
	if !strings.Contains(string(origBody), "ORIGINAL") {
		t.Fatalf("sanity: original body lost ORIGINAL marker:\n%s", origBody)
	}

	// 4. Dedup: a SECOND save-as-copy of the same title returns a DIFFERENT path
	//    (uniquePath appends -2) — copies never collide.
	newPath2, err := svc.Create(ctx, "notes", "Plan (Copy)", "alice")
	if err != nil {
		t.Fatalf("Create second copy: %v", err)
	}
	if newPath2 == newPath {
		t.Fatalf("second copy collided with the first (%q) — uniquePath must dedup", newPath2)
	}
}
