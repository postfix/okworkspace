package pages

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/search"
	"github.com/postfix/okworkspace/internal/store"
)

// indexRecorder is a thread-safe KindIndex job handler registered on the real
// worker so a test can observe the FIRE-AND-FORGET search index enqueues a
// mutation makes, WITHOUT standing up a real bleve index. It records each payload
// the handler receives; helper methods wait for and assert the expected ops.
type indexRecorder struct {
	mu       sync.Mutex
	payloads []string
}

func (rec *indexRecorder) handler() jobs.Handler {
	return func(_ context.Context, payload string) error {
		rec.mu.Lock()
		rec.payloads = append(rec.payloads, payload)
		rec.mu.Unlock()
		return nil
	}
}

// waitForPayload polls until a payload equal to want has been recorded, or fails.
func (rec *indexRecorder) waitForPayload(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rec.mu.Lock()
		for _, p := range rec.payloads {
			if p == want {
				rec.mu.Unlock()
				return
			}
		}
		rec.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	rec.mu.Lock()
	got := append([]string(nil), rec.payloads...)
	rec.mu.Unlock()
	t.Fatalf("search index payload %q never enqueued; recorded: %v", want, got)
}

// newIndexedFixture is newServiceFixture plus a registered KindIndex recorder so a
// test can assert the search index enqueues every mutation makes.
func newIndexedFixture(t *testing.T) (*Service, *repo.Repo, *indexRecorder) {
	t.Helper()
	r, gs, _ := newTestRepoAndGit(t)

	st, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	w := jobs.New(st.DB(), jobs.Config{PollInterval: 5 * time.Millisecond})
	w.Register(KindCommit, CommitHandler(r, gs))
	rec := &indexRecorder{}
	w.Register(search.KindIndex, rec.handler())
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	t.Cleanup(func() { w.Stop(); cancel() })

	svc := NewService(r, gs, w, st.DB(), false)
	svc.now = func() time.Time { return time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC) }
	return svc, r, rec
}

// newServiceFixture builds a Service backed by a real repo + git + worker, with
// the CommitJob handler registered and the drain goroutine running. It returns
// the service and the repo so tests can assert on-disk state.
func newServiceFixture(t *testing.T, pushOnCommit bool) (*Service, *repo.Repo, *gitstore.GitStore) {
	t.Helper()
	r, gs, _ := newTestRepoAndGit(t)

	st, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	w := jobs.New(st.DB(), jobs.Config{PollInterval: 5 * time.Millisecond})
	w.Register(KindCommit, CommitHandler(r, gs))
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	t.Cleanup(func() { w.Stop(); cancel() })

	svc := NewService(r, gs, w, st.DB(), pushOnCommit)
	// Deterministic clock for scaffolded timestamps.
	svc.now = func() time.Time { return time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC) }
	return svc, r, gs
}

// waitForFile polls until the repo-relative path exists or the deadline passes.
func waitForFile(t *testing.T, r *repo.Repo, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ok, _ := r.Exists(path); ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("file %q never appeared (commit job did not drain)", path)
}

func TestCreateSaveRead(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	path, err := svc.Create(ctx, "runbooks", "Deploy Staging", "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if path != "runbooks/deploy-staging.md" {
		t.Fatalf("path = %q, want runbooks/deploy-staging.md", path)
	}
	waitForFile(t, r, path)

	page, err := svc.Get(ctx, path)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if page.Revision == "" {
		t.Fatal("Revision is empty after a committed create")
	}
	fm := page.Frontmatter
	for _, want := range []string{"type: Page", "title: Deploy Staging", "timestamp:", "description:"} {
		if !strings.Contains(fm, want) {
			t.Fatalf("frontmatter missing %q; got:\n%s", want, fm)
		}
	}
	if !strings.Contains(fm, "tags:") && !strings.Contains(fm, "tags") {
		t.Fatalf("frontmatter missing tags; got:\n%s", fm)
	}
	if !strings.Contains(fm, "2026-06-18") {
		t.Fatalf("timestamp not ISO-8601 from clock; got:\n%s", fm)
	}
}

func TestCreateCollision(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	p1, err := svc.Create(ctx, "runbooks", "Deploy Staging", "alice")
	if err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	waitForFile(t, r, p1)
	p2, err := svc.Create(ctx, "runbooks", "Deploy Staging", "alice")
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	if p2 != "runbooks/deploy-staging-2.md" {
		t.Fatalf("second path = %q, want runbooks/deploy-staging-2.md", p2)
	}
	waitForFile(t, r, p2)
	p3, err := svc.Create(ctx, "runbooks", "Deploy Staging", "alice")
	if err != nil {
		t.Fatalf("Create 3: %v", err)
	}
	if p3 != "runbooks/deploy-staging-3.md" {
		t.Fatalf("third path = %q, want runbooks/deploy-staging-3.md", p3)
	}
}

func TestCreateTitleRequired(t *testing.T) {
	svc, _, _ := newServiceFixture(t, false)
	if _, err := svc.Create(context.Background(), "", "   ", "alice"); err != ErrTitleRequired {
		t.Fatalf("Create blank title err = %v, want ErrTitleRequired", err)
	}
}

func TestSaveStaleRevision(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	path, err := svc.Create(ctx, "", "Notes", "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitForFile(t, r, path)

	// A base_revision that does not match the current revision must be rejected
	// with ErrStaleRevision and write nothing.
	before, _ := r.Read(path)
	err = svc.Save(ctx, path, "changed body\n", "type: Page\ntitle: Notes\n", "deadbeef-not-the-revision", "alice")
	if err != ErrStaleRevision {
		t.Fatalf("Save stale err = %v, want ErrStaleRevision", err)
	}
	// Give any (erroneously) enqueued job a moment; assert the file is unchanged.
	time.Sleep(50 * time.Millisecond)
	after, _ := r.Read(path)
	if string(before) != string(after) {
		t.Fatal("stale save mutated the file; the 409 floor must write nothing")
	}
}

func TestSaveRepairsFrontmatter(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	path, err := svc.Create(ctx, "", "Notes", "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitForFile(t, r, path)
	page, err := svc.Get(ctx, path)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Save a body whose frontmatter is missing `description`. After the save the
	// re-read page must have `description` present (repaired).
	err = svc.Save(ctx, path, "# Body\n", "type: Page\ntitle: Notes\ntags: []\ntimestamp: 2026-06-18T12:00:00Z\n", page.Revision, "alice")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Wait for the new revision to differ.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := svc.Get(ctx, path)
		if strings.Contains(got.Frontmatter, "description") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("description was not repaired into the saved frontmatter")
}

func TestCreateFolder(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	if err := svc.CreateFolder(ctx, "", "architecture", "alice"); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	waitForFile(t, r, "architecture/index.md")
}

func TestPushFlagThreaded(t *testing.T) {
	// A fake worker captures the enqueued payload so we can assert Push reflects
	// the constructor's pushOnCommit without draining a real commit.
	for _, push := range []bool{true, false} {
		r, gs, _ := newTestRepoAndGit(t)
		fake := &capturingWorker{}
		svc := &Service{repo: r, git: gs, worker: fake, pushOnCommit: push, now: time.Now}

		if _, err := svc.Create(context.Background(), "", "Title", "alice"); err != nil {
			t.Fatalf("Create (push=%v): %v", push, err)
		}
		if fake.last == nil {
			t.Fatalf("no payload enqueued (push=%v)", push)
		}
		var p commitPayload
		if err := json.Unmarshal([]byte(*fake.last), &p); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if p.Push != push {
			t.Fatalf("payload Push = %v, want %v", p.Push, push)
		}
	}
}

// TestMutationsWaitForCommitOnDisk proves user-facing mutations return only
// AFTER their commit job has landed on disk: immediately after Create (and
// Rename/Delete) returns, the target file exists with NO polling or sleep. This
// is the fix for the "tree needs a manual refresh" race — the handler no longer
// returns before the worker writes the file.
func TestMutationsWaitForCommitOnDisk(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	// Create: file present the instant Create returns.
	path, err := svc.Create(ctx, "", "Wait Test", "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ok, err := r.Exists(path); err != nil || !ok {
		t.Fatalf("after Create returned, %q exists=%v err=%v; want it on disk with no poll", path, ok, err)
	}

	// Rename: new path present, old path gone, the instant Rename returns.
	newPath, err := svc.Rename(ctx, path, "Renamed Wait Test", "alice")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if ok, _ := r.Exists(newPath); !ok {
		t.Fatalf("after Rename returned, new path %q is not on disk", newPath)
	}
	if ok, _ := r.Exists(path); ok {
		t.Fatalf("after Rename returned, old path %q still on disk", path)
	}

	// Delete: source gone the instant Delete returns (it was moved to trash).
	if err := svc.Delete(ctx, newPath, "alice"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if ok, _ := r.Exists(newPath); ok {
		t.Fatalf("after Delete returned, %q still on disk; want moved to trash synchronously", newPath)
	}
}

// capturingWorker is a fake enqueuer that records the last payload. Both the
// fire-and-forget Enqueue and the synchronous EnqueueAndWait capture the payload
// and return nil — modeling a job that reaches "done" immediately, so the
// service's wait-for-commit path returns without a real drain goroutine.
type capturingWorker struct{ last *string }

func (c *capturingWorker) Enqueue(_ context.Context, kind, payload string) error {
	// Capture only commit payloads; mutations also fire-and-forget a search
	// KindIndex job (Enqueue), which would otherwise overwrite the captured commit.
	if kind == KindCommit {
		c.last = &payload
	}
	return nil
}

func (c *capturingWorker) EnqueueAndWait(_ context.Context, kind, payload string, _ time.Duration) error {
	if kind == KindCommit {
		c.last = &payload
	}
	return nil
}

// TestCreateEnqueuesIndexUpsert: Create fire-and-forget enqueues a search.KindIndex
// upsert for the new page so it is searchable without a restart (SRCH live).
func TestCreateEnqueuesIndexUpsert(t *testing.T) {
	svc, _, rec := newIndexedFixture(t)
	path, err := svc.Create(context.Background(), "runbooks", "Deploy Staging", "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	rec.waitForPayload(t, search.UpsertPagePayload(path))
}

// TestSaveEnqueuesIndexUpsert: Save re-indexes the edited page (new body bytes).
func TestSaveEnqueuesIndexUpsert(t *testing.T) {
	svc, r, rec := newIndexedFixture(t)
	ctx := context.Background()
	path, err := svc.Create(ctx, "", "Edit Me", "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitForFile(t, r, path)
	page, err := svc.Get(ctx, path)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if err := svc.Save(ctx, path, "A new searchable word: zylophone.", page.Frontmatter, page.Revision, "alice"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	rec.waitForPayload(t, search.UpsertPagePayload(path))
}

// TestRenameEnqueuesIndexMove: Rename is an index MOVE — a delete for the old path
// and an upsert for the new path.
func TestRenameEnqueuesIndexMove(t *testing.T) {
	svc, r, rec := newIndexedFixture(t)
	ctx := context.Background()
	oldPath, err := svc.Create(ctx, "", "Rename Source", "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitForFile(t, r, oldPath)
	newPath, err := svc.Rename(ctx, oldPath, "Renamed Target", "alice")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	rec.waitForPayload(t, search.DeletePagePayload(oldPath))
	rec.waitForPayload(t, search.UpsertPagePayload(newPath))
}

// TestMoveEnqueuesIndexMove: Move likewise deletes the old path and upserts the new.
func TestMoveEnqueuesIndexMove(t *testing.T) {
	svc, r, rec := newIndexedFixture(t)
	ctx := context.Background()
	if err := svc.CreateFolder(ctx, "", "Dest", "alice"); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	oldPath, err := svc.Create(ctx, "", "Move Source", "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitForFile(t, r, oldPath)
	newPath, err := svc.Move(ctx, oldPath, "dest", "alice")
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	rec.waitForPayload(t, search.DeletePagePayload(oldPath))
	rec.waitForPayload(t, search.UpsertPagePayload(newPath))
}

// TestDeleteEnqueuesIndexDelete: deleting a page to trash enqueues an index delete
// for the original page path so it (and its headings) leave results immediately.
func TestDeleteEnqueuesIndexDelete(t *testing.T) {
	svc, r, rec := newIndexedFixture(t)
	ctx := context.Background()
	path, err := svc.Create(ctx, "", "Delete Me", "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitForFile(t, r, path)
	if err := svc.Delete(ctx, path, "alice"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	rec.waitForPayload(t, search.DeletePagePayload(path))
}

// TestRestoreEnqueuesIndexUpsert: restoring a trashed page re-indexes it so it
// reappears in search.
func TestRestoreEnqueuesIndexUpsert(t *testing.T) {
	svc, r, rec := newIndexedFixture(t)
	ctx := context.Background()
	path, err := svc.Create(ctx, "", "Restore Me", "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitForFile(t, r, path)
	if err := svc.Delete(ctx, path, "alice"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	entries, err := svc.ListTrash(ctx)
	if err != nil || len(entries) == 0 {
		t.Fatalf("ListTrash: %v (len=%d)", err, len(entries))
	}
	restored, err := svc.Restore(ctx, entries[0].ID, "alice")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	rec.waitForPayload(t, search.UpsertPagePayload(restored))
}
