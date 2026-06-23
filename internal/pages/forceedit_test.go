package pages

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/locks"
	"github.com/postfix/okworkspace/internal/repo"
)

// TestForceEditStillRejectsStaleSave is the COLL-03 load-bearing proof: force-edit
// is LOCK-ONLY and never bypasses pages.Save's optimistic-concurrency floor. A
// forced lock plus a landed commit must still make a stale save return
// ErrStaleRevision (the 409 floor at service.go:200 is the sole data-loss
// authority and is not lock-aware), the stale write must never land, and a save at
// the current revision must succeed (force-edit is not itself blocked — only a
// STALE save is rejected). pages/service.go is NOT modified by this slice; the
// invariant holds by construction because Save reads the committed revision itself
// and nothing in the save path consults the lock.
func TestForceEditStillRejectsStaleSave(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	// A locks.Service over the SAME repo as the page service, so a forced lock and
	// the page body share one filesystem root.
	lockSvc := locks.NewService(r, 2*time.Minute)

	ownerA := locks.Owner{Username: "alice", UserID: 1, SessionID: "conn-a"}
	ownerB := locks.Owner{Username: "bob", UserID: 2, SessionID: "conn-b"}

	// 1. Create a page and capture its committed revision (rev0) + on-disk body.
	path, err := svc.Create(ctx, "", "Notes", "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitForFile(t, r, path)

	rev0, err := svc.Revision(ctx, path)
	if err != nil {
		t.Fatalf("Revision rev0: %v", err)
	}
	if rev0 == "" {
		t.Fatal("rev0 is empty after a committed create")
	}
	bodyAtRev0, _ := r.Read(path)

	// 2. A acquires the lock; B force-takes it (lock-only). Assert Force changed
	//    ONLY the lock — the page's committed revision and on-disk body are
	//    untouched (Force never reads/writes the page body or any revision).
	if _, res, aerr := lockSvc.Acquire(ctx, path, ownerA); aerr != nil || res != locks.ResultAcquired {
		t.Fatalf("A Acquire: res=%v err=%v, want acquired/nil", res, aerr)
	}
	forced, ferr := lockSvc.Force(ctx, path, ownerB)
	if ferr != nil {
		t.Fatalf("B Force: %v", ferr)
	}
	if forced.SessionID != ownerB.SessionID {
		t.Fatalf("Force holder session = %q, want %q (B did not take the lock)", forced.SessionID, ownerB.SessionID)
	}
	revAfterForce, err := svc.Revision(ctx, path)
	if err != nil {
		t.Fatalf("Revision after force: %v", err)
	}
	if revAfterForce != rev0 {
		t.Fatalf("Force changed the page revision (%q != %q) — Force must be lock-only", revAfterForce, rev0)
	}
	if bodyNow, _ := r.Read(path); string(bodyNow) != string(bodyAtRev0) {
		t.Fatal("Force mutated the page body — Force must touch only the lock file")
	}

	// 3. Land a real commit: a successful save by someone else (carol) at rev0
	//    advances the committed revision to rev1. This is the "someone else saved
	//    while B was editing" event.
	if serr := svc.Save(ctx, path, "v2\n", "type: Page\ntitle: Notes\n", rev0, "carol"); serr != nil {
		t.Fatalf("landing Save (carol@rev0): %v", serr)
	}
	rev1 := waitForNewRevision(t, svc, path, rev0)
	assertBodyContains(t, r, path, "v2")

	// 4. The FORCED editor B saves with the now-stale rev0 → must 409
	//    (ErrStaleRevision), even though B holds the (forced) lock. The stale write
	//    must NOT land: the on-disk body is still "v2".
	staleErr := svc.Save(ctx, path, "B's edit\n", "type: Page\ntitle: Notes\n", rev0, "bob")
	if !errors.Is(staleErr, ErrStaleRevision) {
		t.Fatalf("forced stale save err = %v, want ErrStaleRevision — force-edit MUST NOT bypass the 409 floor", staleErr)
	}
	// Give any (erroneously) enqueued job a moment; assert B's stale write never landed.
	time.Sleep(50 * time.Millisecond)
	if body, _ := r.Read(path); !strings.Contains(string(body), "v2") || strings.Contains(string(body), "B's edit") {
		t.Fatalf("B's stale forced write landed — the 409 floor must reject it before any write; body:\n%s", body)
	}

	// 5. Control: B saves at the CURRENT revision (rev1) → succeeds. Force-edit is
	//    NOT blocked; only a STALE save is rejected. The write lands ("B's edit").
	if serr := svc.Save(ctx, path, "B's edit\n", "type: Page\ntitle: Notes\n", rev1, "bob"); serr != nil {
		t.Fatalf("control Save (B@rev1) err = %v, want nil (a fresh-revision save must succeed)", serr)
	}
	rev2 := waitForNewRevision(t, svc, path, rev1)
	if rev2 == rev1 {
		t.Fatalf("revision did not advance after B's valid save (rev2 == rev1 == %q)", rev1)
	}
	assertBodyContains(t, r, path, "B's edit")
}

// waitForNewRevision polls Revision until it differs from prev (the commit job has
// landed) or the deadline passes, returning the new revision.
func waitForNewRevision(t *testing.T, svc *Service, path, prev string) string {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rev, err := svc.Revision(ctx, path)
		if err == nil && rev != prev && rev != "" {
			return rev
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("revision never advanced past %q (commit job did not drain)", prev)
	return prev
}

// assertBodyContains waits for the on-disk body of path to contain want (a commit
// job may still be draining) and fails if it never does.
func assertBodyContains(t *testing.T, r *repo.Repo, path, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		body, _ := r.Read(path)
		if strings.Contains(string(body), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	body, _ := r.Read(path)
	t.Fatalf("on-disk body never contained %q; got:\n%s", want, body)
}
