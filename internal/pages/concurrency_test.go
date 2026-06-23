package pages

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// syncCommitWorker is a real single-writer drain for concurrency tests: it runs
// the actual CommitHandler inline under one mutex, so a commit truly lands (file
// written + git commit cut, advancing the blob revision) before the enqueueing
// goroutine returns — exactly the property the production worker guarantees and
// the property the keyed-mutex fix relies on. Non-commit kinds (search index
// upserts) are no-ops.
type syncCommitWorker struct {
	mu sync.Mutex
	h  func(context.Context, string) error
}

func (w *syncCommitWorker) Enqueue(ctx context.Context, kind, payload string) error {
	if kind != KindCommit {
		return nil // fire-and-forget index work is irrelevant to these tests
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.h(ctx, payload)
}

func (w *syncCommitWorker) EnqueueAndWait(ctx context.Context, kind, payload string, _ time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.h(ctx, payload)
}

func newConcurrencyService(t *testing.T) (*Service, string) {
	t.Helper()
	r, gs, _ := newTestRepoAndGit(t)
	svc := &Service{
		repo:   r,
		git:    gs,
		worker: &syncCommitWorker{h: CommitHandler(r, gs)},
		now:    time.Now,
	}
	return svc, ""
}

// TestSave_ConcurrentSameBaseRevision_OneWinner is the regression test for the
// lost-update race: N goroutines save the SAME page with the SAME base_revision
// concurrently. Optimistic concurrency must admit EXACTLY ONE (the 409 floor),
// never silently overwrite. Before the keyed-mutex fix all N returned nil.
func TestSave_ConcurrentSameBaseRevision_OneWinner(t *testing.T) {
	svc, _ := newConcurrencyService(t)
	ctx := context.Background()

	path, err := svc.Create(ctx, "", "race page", "seed")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	base, err := svc.Revision(ctx, path)
	if err != nil {
		t.Fatalf("Revision: %v", err)
	}

	const n = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start // release all goroutines at once for maximum contention
			errs[i] = svc.Save(ctx, path, "body from writer\n", "", base, "writer")
		}(i)
	}
	close(start)
	wg.Wait()

	winners, stale, other := 0, 0, 0
	for _, e := range errs {
		switch {
		case e == nil:
			winners++
		case errors.Is(e, ErrStaleRevision):
			stale++
		default:
			other++
		}
	}
	if winners != 1 || stale != n-1 || other != 0 {
		t.Fatalf("concurrent save with one base revision: winners=%d stale=%d other=%d; want winners=1 stale=%d other=0",
			winners, stale, other, n-1)
	}
}

// TestCreate_ConcurrentSameTitle_DistinctPaths is the regression test for the
// create-clobber race: N goroutines create pages with the SAME title in the same
// folder concurrently. uniquePath must hand each a DISTINCT path (foo.md,
// foo-2.md, …); none may collapse onto an existing one. Before the fix two
// concurrent creates both returned foo.md and one clobbered the other.
func TestCreate_ConcurrentSameTitle_DistinctPaths(t *testing.T) {
	svc, _ := newConcurrencyService(t)
	ctx := context.Background()

	const n = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	paths := make([]string, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			paths[i], errs[i] = svc.Create(ctx, "", "dup title", "writer")
		}(i)
	}
	close(start)
	wg.Wait()

	seen := make(map[string]bool, n)
	for i, e := range errs {
		if e != nil {
			t.Fatalf("Create #%d errored: %v", i, e)
		}
		if seen[paths[i]] {
			t.Fatalf("two concurrent creates resolved to the same path %q (clobber)", paths[i])
		}
		seen[paths[i]] = true
	}
	if len(seen) != n {
		t.Fatalf("expected %d distinct paths, got %d", n, len(seen))
	}
}

// TestRename_ConcurrentSamePage_NoDuplicate is the regression test for the
// structural duplication race: two goroutines rename the SAME page to different
// titles concurrently. Exactly one may succeed; the loser must see the source
// already gone (ErrPageNotFound), never produce a second live copy.
func TestRename_ConcurrentSamePage_NoDuplicate(t *testing.T) {
	svc, _ := newConcurrencyService(t)
	ctx := context.Background()

	path, err := svc.Create(ctx, "", "orig page", "seed")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	res := make([]struct {
		path string
		err  error
	}, 2)
	titles := []string{"alpha", "beta"}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			res[i].path, res[i].err = svc.Rename(ctx, path, titles[i], "writer")
		}(i)
	}
	close(start)
	wg.Wait()

	winners, notFound := 0, 0
	for _, r := range res {
		switch {
		case r.err == nil:
			winners++
		case errors.Is(r.err, ErrPageNotFound):
			notFound++
		default:
			t.Fatalf("unexpected rename error: %v", r.err)
		}
	}
	if winners != 1 || notFound != 1 {
		t.Fatalf("concurrent rename of one page: winners=%d notFound=%d; want 1 and 1 (no duplication)", winners, notFound)
	}
	// The original path must be gone (it was renamed away exactly once).
	if exists, _ := svc.repo.Exists(path); exists {
		t.Fatalf("source page %q still exists after a successful rename", path)
	}
}
