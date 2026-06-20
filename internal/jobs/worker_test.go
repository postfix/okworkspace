package jobs_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "app.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	return st
}

func fastConfig() jobs.Config {
	return jobs.Config{
		PollInterval: 5 * time.Millisecond,
		BaseBackoff:  10 * time.Millisecond,
		MaxBackoff:   40 * time.Millisecond,
		MaxAttempts:  3,
	}
}

func TestEnqueueDrainsJob(t *testing.T) {
	st := newTestStore(t)
	w := jobs.New(st.DB(), fastConfig())

	var ran atomic.Int32
	done := make(chan struct{}, 1)
	w.Register("test", func(_ context.Context, _ string) error {
		ran.Add(1)
		select {
		case done <- struct{}{}:
		default:
		}
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	defer w.Stop()

	if err := w.Enqueue(ctx, "test", "payload-1"); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("job was not drained within timeout")
	}
	if got := ran.Load(); got != 1 {
		t.Fatalf("handler ran %d times, want 1", got)
	}
}

func TestRetryWithBackoffThenFail(t *testing.T) {
	st := newTestStore(t)
	w := jobs.New(st.DB(), fastConfig())

	var attempts atomic.Int32
	w.Register("always-fail", func(_ context.Context, _ string) error {
		attempts.Add(1)
		return errors.New("boom")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	defer w.Stop()

	if err := w.Enqueue(ctx, "always-fail", ""); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Wait for the job to reach a terminal failed state (does not loop forever).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if n := failedCount(t, st); n == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := failedCount(t, st); n != 1 {
		t.Fatalf("failed jobs = %d, want 1 (job did not terminate)", n)
	}
	if got := attempts.Load(); got != int32(fastConfig().MaxAttempts) {
		t.Fatalf("handler attempts = %d, want MaxAttempts=%d", got, fastConfig().MaxAttempts)
	}
}

func TestSerializedExecution(t *testing.T) {
	st := newTestStore(t)
	w := jobs.New(st.DB(), fastConfig())

	const n = 10
	var (
		concurrent atomic.Int32
		maxSeen    atomic.Int32
		completed  atomic.Int32
	)
	w.Register("serial", func(_ context.Context, _ string) error {
		cur := concurrent.Add(1)
		for {
			old := maxSeen.Load()
			if cur <= old || maxSeen.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		concurrent.Add(-1)
		completed.Add(1)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	defer w.Stop()

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.Enqueue(ctx, "serial", ""); err != nil {
				t.Errorf("Enqueue: %v", err)
			}
		}()
	}
	wg.Wait()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && completed.Load() < int32(n) {
		time.Sleep(10 * time.Millisecond)
	}
	if completed.Load() != int32(n) {
		t.Fatalf("completed %d jobs, want %d", completed.Load(), n)
	}
	if maxSeen.Load() != 1 {
		t.Fatalf("max concurrent handler executions = %d, want 1 (single-writer)", maxSeen.Load())
	}
}

func TestWaitForJobDone(t *testing.T) {
	st := newTestStore(t)
	w := jobs.New(st.DB(), fastConfig())
	w.Register("ok", func(_ context.Context, _ string) error { return nil })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	defer w.Stop()

	id, err := w.EnqueueJob(ctx, "ok", "")
	if err != nil {
		t.Fatalf("EnqueueJob: %v", err)
	}
	if id <= 0 {
		t.Fatalf("EnqueueJob returned id %d, want > 0", id)
	}
	if err := w.WaitForJob(ctx, id, 2*time.Second); err != nil {
		t.Fatalf("WaitForJob: %v", err)
	}
	// The job must actually be done now.
	var status string
	if err := st.DB().QueryRow(`SELECT status FROM jobs WHERE id=?`, id).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "done" {
		t.Fatalf("status = %q, want done", status)
	}
}

func TestWaitForJobFailed(t *testing.T) {
	st := newTestStore(t)
	w := jobs.New(st.DB(), fastConfig())
	w.Register("boom", func(_ context.Context, _ string) error {
		return errors.New("kaboom")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	defer w.Stop()

	id, err := w.EnqueueJob(ctx, "boom", "")
	if err != nil {
		t.Fatalf("EnqueueJob: %v", err)
	}
	// MaxAttempts retries take a few backoff cycles before the job is terminally
	// failed; give WaitForJob room to observe that terminal state.
	err = w.WaitForJob(ctx, id, 3*time.Second)
	if err == nil {
		t.Fatal("WaitForJob returned nil, want a failure error")
	}
	if errors.Is(err, jobs.ErrJobTimeout) {
		t.Fatalf("WaitForJob returned timeout, want failure: %v", err)
	}
	if !strings.Contains(err.Error(), "kaboom") {
		t.Fatalf("WaitForJob error = %q, want it to include the stored last error", err)
	}
}

func TestWaitForJobTimeout(t *testing.T) {
	st := newTestStore(t)
	// No worker started: the job stays pending forever, so WaitForJob must time
	// out rather than block indefinitely.
	w := jobs.New(st.DB(), fastConfig())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	id, err := w.EnqueueJob(ctx, "never-drained", "")
	if err != nil {
		t.Fatalf("EnqueueJob: %v", err)
	}
	err = w.WaitForJob(ctx, id, 80*time.Millisecond)
	if !errors.Is(err, jobs.ErrJobTimeout) {
		t.Fatalf("WaitForJob error = %v, want ErrJobTimeout", err)
	}
}

// failedCount returns the number of rows in jobs with status='failed'.
func failedCount(t *testing.T, st *store.Store) int {
	t.Helper()
	var n int
	if err := st.DB().QueryRow(`SELECT COUNT(1) FROM jobs WHERE status='failed'`).Scan(&n); err != nil {
		t.Fatalf("count failed jobs: %v", err)
	}
	return n
}
