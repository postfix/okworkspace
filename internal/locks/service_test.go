package locks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/repo"
)

const testTTL = 2 * time.Minute

// fixedClock is an advanceable clock the tests inject into the service so every
// expiry assertion is deterministic (no time.Sleep, no wall-clock flakiness).
type fixedClock struct{ t time.Time }

func (c *fixedClock) now() time.Time     { return c.t }
func (c *fixedClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// newTestService builds a Service over a real repo rooted at t.TempDir() (NO git
// — locks never commit) with an injected fixed clock starting at a stable epoch.
func newTestService(t *testing.T) (*Service, *repo.Repo, *fixedClock) {
	t.Helper()
	r, err := repo.New(t.TempDir())
	if err != nil {
		t.Fatalf("repo.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	clk := &fixedClock{t: time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)}
	svc := NewService(r, testTTL)
	svc.now = clk.now
	return svc, r, clk
}

func ownerA() Owner { return Owner{Username: "alice", UserID: 1, SessionID: "conn-a"} }
func ownerB() Owner { return Owner{Username: "bob", UserID: 2, SessionID: "conn-b"} }

const pageP = "runbooks/deploy.md"

func TestAcquireLifecycle(t *testing.T) {
	ctx := context.Background()
	svc, r, _ := newTestService(t)

	// A acquires.
	lock, res, err := svc.Acquire(ctx, pageP, ownerA())
	if err != nil {
		t.Fatalf("A Acquire: %v", err)
	}
	if res != ResultAcquired {
		t.Fatalf("A Acquire result = %q, want acquired", res)
	}
	if lock.Username != "alice" {
		t.Fatalf("lock holder = %q, want alice", lock.Username)
	}
	// Lock file exists on disk at the mirrored path.
	if ok, _ := r.Exists(".okf-workspace/locks/" + pageP + ".lock"); !ok {
		t.Fatal("lock file not written on Acquire")
	}

	// A DIFFERENT session B sees held-by-other, holder reported is A, A's lock unchanged.
	holder, res, err := svc.Acquire(ctx, pageP, ownerB())
	if err != nil {
		t.Fatalf("B Acquire: %v", err)
	}
	if res != ResultHeldByOther {
		t.Fatalf("B Acquire result = %q, want held-by-other", res)
	}
	if holder.SessionID != "conn-a" {
		t.Fatalf("held-by-other holder session = %q, want conn-a", holder.SessionID)
	}
	got, live, err := svc.Get(ctx, pageP)
	if err != nil || !live {
		t.Fatalf("Get after B Acquire: live=%v err=%v", live, err)
	}
	if got.SessionID != "conn-a" {
		t.Fatalf("on-disk lock overwritten by B; session = %q, want conn-a", got.SessionID)
	}

	// Same session A re-Acquire → acquired (refresh).
	_, res, err = svc.Acquire(ctx, pageP, ownerA())
	if err != nil || res != ResultAcquired {
		t.Fatalf("A re-Acquire: res=%q err=%v, want acquired", res, err)
	}

	// Release by B (wrong session) → A's lock still present.
	if err := svc.Release(ctx, pageP, "conn-b"); err != nil {
		t.Fatalf("B Release: %v", err)
	}
	if _, live, _ := svc.Get(ctx, pageP); !live {
		t.Fatal("B Release removed A's lock (wrong session must not delete)")
	}

	// Release by A → lock gone; Release again → no error (idempotent).
	if err := svc.Release(ctx, pageP, "conn-a"); err != nil {
		t.Fatalf("A Release: %v", err)
	}
	if _, live, _ := svc.Get(ctx, pageP); live {
		t.Fatal("A Release did not remove the lock")
	}
	if err := svc.Release(ctx, pageP, "conn-a"); err != nil {
		t.Fatalf("idempotent Release: %v", err)
	}
}

func TestRefreshHolderOnly(t *testing.T) {
	ctx := context.Background()
	svc, _, clk := newTestService(t)

	if _, _, err := svc.Acquire(ctx, pageP, ownerA()); err != nil {
		t.Fatalf("A Acquire: %v", err)
	}
	t0 := clk.now()

	// Advance, then A refreshes → ExpiresAt bumps to (new now)+ttl.
	clk.advance(30 * time.Second)
	if err := svc.Refresh(ctx, pageP, ownerA()); err != nil {
		t.Fatalf("A Refresh: %v", err)
	}
	got, live, err := svc.Get(ctx, pageP)
	if err != nil || !live {
		t.Fatalf("Get after refresh: live=%v err=%v", live, err)
	}
	wantExpiry := t0.Add(30 * time.Second).Add(testTTL)
	if !got.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("ExpiresAt after refresh = %v, want %v", got.ExpiresAt, wantExpiry)
	}

	// B Refresh → not-holder sentinel, A's lock unchanged.
	if err := svc.Refresh(ctx, pageP, ownerB()); err != ErrNotHolder {
		t.Fatalf("B Refresh err = %v, want ErrNotHolder", err)
	}
	got2, _, _ := svc.Get(ctx, pageP)
	if got2.SessionID != "conn-a" || !got2.ExpiresAt.Equal(wantExpiry) {
		t.Fatal("B Refresh mutated A's lock")
	}
}

func TestForceTakesOwnershipLockOnly(t *testing.T) {
	ctx := context.Background()
	svc, r, _ := newTestService(t)

	if _, _, err := svc.Acquire(ctx, pageP, ownerA()); err != nil {
		t.Fatalf("A Acquire: %v", err)
	}

	before := countFiles(t, r.Root())

	forced, err := svc.Force(ctx, pageP, ownerB())
	if err != nil {
		t.Fatalf("B Force: %v", err)
	}
	if forced.SessionID != "conn-b" {
		t.Fatalf("Force holder session = %q, want conn-b", forced.SessionID)
	}
	got, live, _ := svc.Get(ctx, pageP)
	if !live || got.SessionID != "conn-b" {
		t.Fatalf("on-disk lock after Force = %q live=%v, want conn-b", got.SessionID, live)
	}

	// Force is lock-only: the only file Force touched is the existing .lock — no
	// page artifact was created (the file count is unchanged because Force
	// overwrote the same .lock A already wrote).
	after := countFiles(t, r.Root())
	if after != before {
		t.Fatalf("Force created %d new file(s); it must touch only the lock file", after-before)
	}
}

func TestExpiryAndGC(t *testing.T) {
	ctx := context.Background()
	svc, r, clk := newTestService(t)

	if _, _, err := svc.Acquire(ctx, pageP, ownerA()); err != nil {
		t.Fatalf("A Acquire: %v", err)
	}
	lockRel := ".okf-workspace/locks/" + pageP + ".lock"

	// Advance past ExpiresAt → Get treats it as absent, but the file still exists.
	clk.advance(testTTL + time.Second)
	if _, live, _ := svc.Get(ctx, pageP); live {
		t.Fatal("expired lock still reported live by Get")
	}
	if ok, _ := r.Exists(lockRel); !ok {
		t.Fatal("expired lock file removed before GC ran")
	}

	// GC reaps the expired file.
	if err := svc.GC(ctx); err != nil {
		t.Fatalf("GC: %v", err)
	}
	if ok, _ := r.Exists(lockRel); ok {
		t.Fatal("GC did not remove the expired lock file")
	}

	// Second GC is a no-op.
	if err := svc.GC(ctx); err != nil {
		t.Fatalf("second GC: %v", err)
	}
}

func TestLockPathSafety(t *testing.T) {
	ctx := context.Background()
	svc, r, _ := newTestService(t)

	// Nested page path resolves under the mirrored lock tree.
	nested := "a/b/c.md"
	if _, res, err := svc.Acquire(ctx, nested, ownerA()); err != nil || res != ResultAcquired {
		t.Fatalf("nested Acquire: res=%q err=%v", res, err)
	}
	if ok, _ := r.Exists(".okf-workspace/locks/a/b/c.md.lock"); !ok {
		t.Fatal("nested lock not written at mirrored path")
	}

	// Snapshot every file under the temp root BEFORE the traversal attempt.
	before := countFiles(t, r.Root())

	// Traversal-shaped page path → repo.Resolve rejects it; nothing escapes.
	if _, _, err := svc.Acquire(ctx, "../../etc/passwd", ownerA()); err == nil {
		t.Fatal("traversal-shaped path was accepted; Acquire must error")
	}
	after := countFiles(t, r.Root())
	if after != before {
		t.Fatalf("traversal Acquire wrote %d file(s); it must write nothing", after-before)
	}
	// And nothing was written to the obvious escape target.
	if _, err := os.Stat(filepath.Join(r.Root(), "..", "..", "etc", "passwd")); err == nil {
		t.Fatal("traversal Acquire wrote outside the repo root")
	}
}

func TestTornLockTreatedAsNoLock(t *testing.T) {
	ctx := context.Background()
	svc, r, _ := newTestService(t)

	// Write garbage bytes to a lock path via the repo (not os.*).
	if err := r.Write(".okf-workspace/locks/"+pageP+".lock", []byte("{not json")); err != nil {
		t.Fatalf("write garbage: %v", err)
	}
	if _, live, err := svc.Get(ctx, pageP); err != nil || live {
		t.Fatalf("torn lock: live=%v err=%v, want no live lock and no error", live, err)
	}
}

func TestEditorsForSnapshot(t *testing.T) {
	ctx := context.Background()
	svc, _, clk := newTestService(t)

	if _, _, err := svc.Acquire(ctx, pageP, ownerA()); err != nil {
		t.Fatalf("A Acquire: %v", err)
	}

	// A different connection sees A as a remote editor (you=false).
	eds, err := svc.EditorsFor(ctx, pageP, "conn-b")
	if err != nil {
		t.Fatalf("EditorsFor(conn-b): %v", err)
	}
	if len(eds) != 1 || eds[0].Username != "alice" || eds[0].You {
		t.Fatalf("EditorsFor(conn-b) = %+v, want [{alice false}]", eds)
	}

	// A's own connection sees itself with you=true.
	eds, err = svc.EditorsFor(ctx, pageP, "conn-a")
	if err != nil {
		t.Fatalf("EditorsFor(conn-a): %v", err)
	}
	if len(eds) != 1 || !eds[0].You {
		t.Fatalf("EditorsFor(conn-a) = %+v, want self with you=true", eds)
	}

	// After expiry, the snapshot is empty.
	clk.advance(testTTL + time.Second)
	eds, err = svc.EditorsFor(ctx, pageP, "conn-b")
	if err != nil {
		t.Fatalf("EditorsFor after expiry: %v", err)
	}
	if len(eds) != 0 {
		t.Fatalf("EditorsFor after expiry = %+v, want empty", eds)
	}
}

// countFiles counts regular files under root (excluding directories), used to
// assert Force/path-safety wrote exactly what they should and nothing escaped.
func countFiles(t *testing.T, root string) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			n++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("countFiles: %v", err)
	}
	return n
}
