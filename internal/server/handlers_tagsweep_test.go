package server_test

// KEY-FREE: the sweep-start + review-queue endpoints enqueue/list — they do NOT
// call the model — so a fake enqueuer (recording kind+payload pairs) and a real
// tagsweep.Store over a temp DB exercise the full HTTP seam with no LLM.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/server"
	"github.com/postfix/okworkspace/internal/store"
	"github.com/postfix/okworkspace/internal/tagsweep"
	"github.com/postfix/okworkspace/internal/users"
)

// recordingEnqueuer records every (kind, payload) enqueue so a test can assert
// exactly which KindTagSuggest jobs the sweep-start handler queued. It writes
// nothing — proving sweep-start only enqueues.
type recordingEnqueuer struct {
	mu   sync.Mutex
	jobs []struct{ kind, payload string }
}

func (e *recordingEnqueuer) Enqueue(ctx context.Context, kind, payload string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.jobs = append(e.jobs, struct{ kind, payload string }{kind, payload})
	return nil
}

func (e *recordingEnqueuer) payloads(kind string) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for _, j := range e.jobs {
		if j.kind == kind {
			out = append(out, j.payload)
		}
	}
	return out
}

type tagSweepFixture struct {
	handler http.Handler
	repo    *repo.Repo
	store   *tagsweep.Store
	db      *store.Store
	enq     *recordingEnqueuer
	users   *users.Repository
}

func newTagSweepServer(t *testing.T) *tagSweepFixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	base := t.TempDir()

	st, err := store.Open(filepath.Join(base, "app.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	contentRepo, err := repo.New(filepath.Join(base, "repo"))
	if err != nil {
		t.Fatalf("repo.New: %v", err)
	}
	t.Cleanup(func() { _ = contentRepo.Close() })
	gs := gitstore.New(contentRepo, config.GitConfig{Enabled: true, Branch: "main"})
	if err := gs.Init(context.Background()); err != nil {
		t.Fatalf("gitstore.Init: %v", err)
	}

	ts := tagsweep.OpenStore(st.DB())
	ts.SetRepo(contentRepo)
	enq := &recordingEnqueuer{}

	var cfg config.Config
	cfg.Auth.SessionCookieName = config.DefaultSessionCookieName
	cfg.Auth.SessionTTLHours = config.DefaultSessionTTLHours
	cfg.Admin.Username = "admin"
	userRepo := users.NewRepository(st.DB())
	if _, _, _, err := users.BootstrapAdmin(context.Background(), userRepo, cfg); err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}

	h, err := server.New(server.Deps{
		Store:          st,
		Config:         cfg,
		UserRepo:       userRepo,
		Audit:          audit.New(st.DB(), nil),
		TagSuggestions: ts,
		TagSweepJobs:   enq,
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return &tagSweepFixture{handler: h, repo: contentRepo, store: ts, db: st, enq: enq, users: userRepo}
}

func (f *tagSweepFixture) writePage(t *testing.T, path string) {
	t.Helper()
	content := "---\ntype: page\ntitle: " + path + "\n---\n\nbody\n"
	if err := f.repo.Write(path, []byte(content)); err != nil {
		t.Fatalf("write page %q: %v", path, err)
	}
}

func (f *tagSweepFixture) seedTag(t *testing.T, pagePath, tag string) {
	t.Helper()
	if _, err := f.db.DB().Exec(`INSERT INTO page_tags (page_path, tag) VALUES (?, ?)`, pagePath, tag); err != nil {
		t.Fatalf("seed page_tags %q: %v", pagePath, err)
	}
}

// loginTagSweepEditor creates an editor user and logs in (a non-admin for the
// RBAC gate tests).
func loginTagSweepEditor(t *testing.T, f *tagSweepFixture) []*http.Cookie {
	t.Helper()
	_, otp, err := users.Create(context.Background(), f.users, users.NewUser{Username: "ed", DisplayName: "Ed", Role: users.RoleEditor})
	if err != nil {
		t.Fatalf("Create editor: %v", err)
	}
	ed, _ := f.users.GetByUsername(context.Background(), "ed")
	if err := users.ChangeOwnPassword(context.Background(), f.users, ed.ID, otp, "editor-long-password"); err != nil {
		t.Fatalf("set editor password: %v", err)
	}
	return loginAs(t, f.handler, "ed", "editor-long-password")
}

// TestStartTagSweepAdminOnly: a non-admin (editor) → 403; an admin → 202.
func TestStartTagSweepAdminOnly(t *testing.T) {
	f := newTagSweepServer(t)

	editor := loginTagSweepEditor(t, f)
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/sweep", map[string]bool{"all": false}, editor)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("editor sweep = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}

	admin := loginAsAdmin(t, f.handler, f.users)
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/sweep", map[string]bool{"all": false}, admin)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("admin sweep = %d, want 202 (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestStartTagSweepEnqueuesPerPage: with K untagged live pages, POST {all:false}
// → 202 {queued:K} and the enqueuer recorded EXACTLY K KindTagSuggest jobs with
// the correct per-page payloads; no write/commit (the handler only enqueues).
func TestStartTagSweepEnqueuesPerPage(t *testing.T) {
	f := newTagSweepServer(t)
	f.writePage(t, "a.md") // untagged → target
	f.writePage(t, "b.md") // tagged → not a target
	f.writePage(t, "c.md") // untagged → target
	f.seedTag(t, "b.md", "ops")

	admin := loginAsAdmin(t, f.handler, f.users)
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/sweep", map[string]bool{"all": false}, admin)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("sweep = %d, want 202 (body=%s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		OK     bool `json:"ok"`
		Queued int  `json:"queued"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rec.Body.String())
	}
	if resp.Queued != 2 {
		t.Fatalf("queued = %d, want 2", resp.Queued)
	}

	got := f.enq.payloads(tagsweep.KindTagSuggest)
	want := map[string]bool{
		tagsweep.SuggestPayload("a.md"): true,
		tagsweep.SuggestPayload("c.md"): true,
	}
	if len(got) != 2 {
		t.Fatalf("enqueued %d jobs, want 2: %v", len(got), got)
	}
	for _, p := range got {
		if !want[p] {
			t.Fatalf("unexpected enqueued payload %q (want a.md/c.md only)", p)
		}
	}
}

// TestStartTagSweepAllScope: {all:true} enqueues one job per live page (tagged +
// untagged).
func TestStartTagSweepAllScope(t *testing.T) {
	f := newTagSweepServer(t)
	f.writePage(t, "a.md")
	f.writePage(t, "b.md")
	f.seedTag(t, "b.md", "ops")

	admin := loginAsAdmin(t, f.handler, f.users)
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/sweep", map[string]bool{"all": true}, admin)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("sweep all = %d, want 202 (body=%s)", rec.Code, rec.Body.String())
	}
	got := f.enq.payloads(tagsweep.KindTagSuggest)
	if len(got) != 2 {
		t.Fatalf("all-scope enqueued %d jobs, want 2 (every live page): %v", len(got), got)
	}
}

// TestStartTagSweepZeroTargets: everything tagged + {all:false} → 202 {queued:0},
// zero jobs enqueued.
func TestStartTagSweepZeroTargets(t *testing.T) {
	f := newTagSweepServer(t)
	f.writePage(t, "a.md")
	f.writePage(t, "b.md")
	f.seedTag(t, "a.md", "ops")
	f.seedTag(t, "b.md", "ops")

	admin := loginAsAdmin(t, f.handler, f.users)
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/sweep", map[string]bool{"all": false}, admin)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("zero-target sweep = %d, want 202 (body=%s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Queued int `json:"queued"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Queued != 0 {
		t.Fatalf("queued = %d, want 0", resp.Queued)
	}
	if got := f.enq.payloads(tagsweep.KindTagSuggest); len(got) != 0 {
		t.Fatalf("zero-target enqueued %d jobs, want 0: %v", len(got), got)
	}
}

// getJSON issues an authed GET and returns the recorder.
func getJSON(t *testing.T, h http.Handler, path string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestListTagSuggestionsAdminOnly: non-admin → 403; admin → 200.
func TestListTagSuggestionsAdminOnly(t *testing.T) {
	f := newTagSweepServer(t)

	editor := loginTagSweepEditor(t, f)
	rec := getJSON(t, f.handler, "/api/v1/admin/tags/suggestions", editor)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("editor list = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}

	admin := loginAsAdmin(t, f.handler, f.users)
	rec = getJSON(t, f.handler, "/api/v1/admin/tags/suggestions", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin list = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestListTagSuggestionsReturnsPending: two staged pending rows → 200 with both
// entries (page_path + suggestions + base_revision) in deterministic order.
func TestListTagSuggestionsReturnsPending(t *testing.T) {
	f := newTagSweepServer(t)
	ctx := context.Background()
	if err := f.store.StagePending(ctx, "z.md", []tagsweep.Suggestion{{Tag: "zeta", Existing: false}}, "rev-z"); err != nil {
		t.Fatalf("StagePending z: %v", err)
	}
	if err := f.store.StagePending(ctx, "a.md", []tagsweep.Suggestion{{Tag: "alpha", Existing: true}}, "rev-a"); err != nil {
		t.Fatalf("StagePending a: %v", err)
	}

	admin := loginAsAdmin(t, f.handler, f.users)
	rec := getJSON(t, f.handler, "/api/v1/admin/tags/suggestions", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var got []tagsweep.PendingEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rec.Body.String())
	}
	if len(got) != 2 || got[0].PagePath != "a.md" || got[1].PagePath != "z.md" {
		t.Fatalf("list = %+v, want [a.md z.md] ordered", got)
	}
	if got[0].BaseRevision != "rev-a" || len(got[0].Suggestions) != 1 || got[0].Suggestions[0].Tag != "alpha" || !got[0].Suggestions[0].Existing {
		t.Fatalf("a.md entry = %+v, want alpha(existing)/rev-a", got[0])
	}
}
