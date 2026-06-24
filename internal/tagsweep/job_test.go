package tagsweep

// These tests are KEY-FREE: they inject a fake suggester (never a real model, env
// unset) and prove the load-bearing safety guarantee (PITFALLS Pitfall 5,
// go/no-go) — draining KindTagSuggest jobs stages ONLY pending rows and performs
// ZERO frontmatter writes / ZERO commits, even across a worker kill+restart.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/store"
)

// fakeSuggester records every SuggestTags call and returns a canned result (or a
// canned error / panic). It holds NO writer — proving the handler cannot write a
// file or commit through the suggestion seam.
type fakeSuggester struct {
	mu      sync.Mutex
	calls   []string
	tags    []string
	existing []bool
	baseRev string
	err     error
	panicOn bool
}

func (f *fakeSuggester) SuggestTags(ctx context.Context, path string) ([]string, []bool, string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, path)
	f.mu.Unlock()
	if f.panicOn {
		panic("boom")
	}
	if f.err != nil {
		return nil, nil, "", f.err
	}
	return f.tags, f.existing, f.baseRev, nil
}

func (f *fakeSuggester) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// TestSuggestHandlerHappyPath: a fake suggester returning a fixed result → the
// handler stages exactly one pending row whose ListPending entry matches.
func TestSuggestHandlerHappyPath(t *testing.T) {
	ts, _, _ := newTestStore(t)
	ctx := context.Background()

	fake := &fakeSuggester{tags: []string{"ops", "runbook"}, existing: []bool{true, false}, baseRev: "rev-1"}
	h := SuggestHandler(ts, fake)

	if err := h(ctx, SuggestPayload("a.md")); err != nil {
		t.Fatalf("handler: %v", err)
	}

	got, err := ts.ListPending(ctx)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListPending len = %d, want 1", len(got))
	}
	e := got[0]
	if e.PagePath != "a.md" || e.BaseRevision != "rev-1" || len(e.Suggestions) != 2 {
		t.Fatalf("staged entry = %+v, want a.md/rev-1/2 suggestions", e)
	}
	if e.Suggestions[0] != (Suggestion{Tag: "ops", Existing: true}) ||
		e.Suggestions[1] != (Suggestion{Tag: "runbook", Existing: false}) {
		t.Fatalf("staged suggestions = %+v, want ops(existing) + runbook(new)", e.Suggestions)
	}
}

// TestSuggestHandlerSuggesterError: a suggester error is RETURNED (so the worker
// would retry) and NO pending row is staged.
func TestSuggestHandlerSuggesterError(t *testing.T) {
	ts, _, _ := newTestStore(t)
	ctx := context.Background()

	fake := &fakeSuggester{err: errors.New("model unavailable")}
	h := SuggestHandler(ts, fake)

	if err := h(ctx, SuggestPayload("a.md")); err == nil {
		t.Fatal("handler returned nil, want the suggester error propagated")
	}

	got, err := ts.ListPending(ctx)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListPending after suggester error = %v, want empty (no partial row)", got)
	}
}

// TestSuggestHandlerPanicRecovered: a panic inside the suggester is recovered and
// returned as an error (the single drain goroutine survives).
func TestSuggestHandlerPanicRecovered(t *testing.T) {
	ts, _, _ := newTestStore(t)
	fake := &fakeSuggester{panicOn: true}
	h := SuggestHandler(ts, fake)

	if err := h(context.Background(), SuggestPayload("a.md")); err == nil {
		t.Fatal("handler returned nil on panic, want a recovered error")
	}
}

// snapshotRepo returns a sorted map of every regular file path → contents under
// dir (excluding .git), so a test can assert the working tree is byte-identical
// before and after a drain.
func snapshotRepo(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		rel, _ := filepath.Rel(dir, p)
		out[rel] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot repo: %v", err)
	}
	return out
}

func sameSnapshot(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

func gitHead(t *testing.T, repoDir string) string {
	t.Helper()
	// HEAD may not exist in a fresh repo with no commits — return "" then.
	b, err := os.ReadFile(filepath.Join(repoDir, ".git", "HEAD"))
	if err != nil {
		return ""
	}
	// Resolve the ref to its commit SHA if present.
	head := string(b)
	if len(head) > 5 && head[:5] == "ref: " {
		ref := head[5 : len(head)-1]
		sha, rerr := os.ReadFile(filepath.Join(repoDir, ".git", ref))
		if rerr != nil {
			return "" // unborn branch (no commits yet)
		}
		return string(sha)
	}
	return head
}

// drainAll runs the worker until the jobs queue has no pending/running rows (or a
// deadline), so the safety gate can assert the post-drain state.
func drainAll(t *testing.T, st *store.Store) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		if err := st.DB().QueryRow(
			`SELECT COUNT(1) FROM jobs WHERE status IN ('pending','running')`).Scan(&n); err != nil {
			t.Fatalf("count pending jobs: %v", err)
		}
		if n == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("jobs did not drain within deadline")
}

// TestSafetyGate_NoAutoWrite is the go/no-go safety invariant (PITFALLS Pitfall
// 5): wiring a REAL jobs.Worker over a temp store, registering KindTagSuggest,
// enqueueing jobs for several pages, and draining produces exactly one pending row
// per page AND ZERO frontmatter writes / ZERO commits — proven by asserting the
// content repo working tree + Git HEAD are byte-identical before and after the
// drain. Then a worker kill+restart (Stop, re-enqueue, Start again) re-stages but
// still never writes.
func TestSafetyGate_NoAutoWrite(t *testing.T) {
	ts, st, r := newTestStore(t)
	ctx := context.Background()

	// A sentinel content file proves the handler never mutates the working tree.
	writePage(t, r, "a.md")
	writePage(t, r, "b.md")
	writePage(t, r, "c.md")

	repoRoot := r.Root()
	before := snapshotRepo(t, repoRoot)
	headBefore := gitHead(t, repoRoot)

	fake := &fakeSuggester{tags: []string{"ops"}, existing: []bool{true}, baseRev: "rev-1"}

	worker := jobs.New(st.DB(), jobs.Config{PollInterval: 10 * time.Millisecond})
	worker.Register(KindTagSuggest, SuggestHandler(ts, fake))
	worker.Start(ctx)

	pages := []string{"a.md", "b.md", "c.md"}
	for _, p := range pages {
		if err := worker.Enqueue(ctx, KindTagSuggest, SuggestPayload(p)); err != nil {
			t.Fatalf("enqueue %q: %v", p, err)
		}
	}
	drainAll(t, st)

	// (a) exactly one pending row per enqueued page.
	pending, err := ts.ListPending(ctx)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != len(pages) {
		t.Fatalf("pending rows = %d, want %d (one per page)", len(pending), len(pages))
	}

	// (b) ZERO frontmatter writes / ZERO commits: working tree + HEAD unchanged.
	after := snapshotRepo(t, repoRoot)
	if !sameSnapshot(before, after) {
		t.Fatal("working tree changed during sweep drain — a frontmatter write leaked (Pitfall 5 violation)")
	}
	if gitHead(t, repoRoot) != headBefore {
		t.Fatal("Git HEAD advanced during sweep drain — a commit leaked (Pitfall 5 violation)")
	}
	if fake.callCount() != len(pages) {
		t.Fatalf("suggester called %d times, want %d", fake.callCount(), len(pages))
	}

	// Kill + restart simulation: stop the worker, re-enqueue the same pages, start a
	// fresh worker, drain again, and re-assert NO write happened — only re-staging.
	worker.Stop()
	for _, p := range pages {
		if err := worker.Enqueue(ctx, KindTagSuggest, SuggestPayload(p)); err != nil {
			t.Fatalf("re-enqueue %q: %v", p, err)
		}
	}
	worker2 := jobs.New(st.DB(), jobs.Config{PollInterval: 10 * time.Millisecond})
	worker2.Register(KindTagSuggest, SuggestHandler(ts, fake))
	worker2.Start(ctx)
	drainAll(t, st)
	defer worker2.Stop()

	pending2, err := ts.ListPending(ctx)
	if err != nil {
		t.Fatalf("ListPending after restart: %v", err)
	}
	if len(pending2) != len(pages) {
		t.Fatalf("pending rows after restart = %d, want %d (re-stage, not duplicate)", len(pending2), len(pages))
	}
	afterRestart := snapshotRepo(t, repoRoot)
	if !sameSnapshot(before, afterRestart) {
		t.Fatal("working tree changed across kill+restart — a write leaked (Pitfall 5 violation)")
	}
	if gitHead(t, repoRoot) != headBefore {
		t.Fatal("Git HEAD advanced across kill+restart — a commit leaked (Pitfall 5 violation)")
	}
}
