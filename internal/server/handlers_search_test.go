package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/search"
	"github.com/postfix/okworkspace/internal/server"
	"github.com/postfix/okworkspace/internal/store"
	"github.com/postfix/okworkspace/internal/users"
)

// searchFixture wires a full server with a real search.Index over a real content
// repo so GET /search and POST /admin/search/reindex are exercised end to end.
type searchFixture struct {
	handler http.Handler
	repo    *repo.Repo
	idx     *search.Index
	repoo   *users.Repository
}

func newSearchServer(t *testing.T) *searchFixture {
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

	idx, err := search.OpenOrCreate(filepath.Join(base, "index"))
	if err != nil {
		t.Fatalf("OpenOrCreate: %v", err)
	}
	idx.SetRepo(contentRepo)
	idx.SetDB(st.DB())
	t.Cleanup(func() { _ = idx.Close() })

	w := jobs.New(st.DB(), jobs.Config{PollInterval: 5 * time.Millisecond})
	w.Register(search.KindIndex, search.IndexHandler(idx, contentRepo))
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	t.Cleanup(func() { w.Stop(); cancel() })

	var cfg config.Config
	cfg.Auth.SessionCookieName = config.DefaultSessionCookieName
	cfg.Auth.SessionTTLHours = config.DefaultSessionTTLHours
	cfg.Admin.Username = "admin"
	userRepo := users.NewRepository(st.DB())
	if _, _, _, err := users.BootstrapAdmin(context.Background(), userRepo, cfg); err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}

	h, err := server.New(server.Deps{
		Store:      st,
		Config:     cfg,
		UserRepo:   userRepo,
		Audit:      audit.New(st.DB(), nil),
		Search:     idx,
		SearchJobs: w,
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return &searchFixture{handler: h, repo: contentRepo, idx: idx, repoo: userRepo}
}

// seedAndIndex writes a page to the repo and rebuilds the index so it is queryable.
func (f *searchFixture) seedAndIndex(t *testing.T, path, title, body string) {
	t.Helper()
	content := "---\ntype: page\ntitle: " + title + "\ndescription: \ntags:\n  []\ntimestamp: 2026-06-21T00:00:00Z\n---\n\n" + body + "\n"
	if err := f.repo.Write(path, []byte(content)); err != nil {
		t.Fatalf("write page: %v", err)
	}
	if err := f.idx.RebuildIndex(context.Background()); err != nil {
		t.Fatalf("RebuildIndex: %v", err)
	}
}

// loginSearchEditor creates an editor user and logs in.
func loginSearchEditor(t *testing.T, f *searchFixture) []*http.Cookie {
	t.Helper()
	_, otp, err := users.Create(context.Background(), f.repoo, users.NewUser{Username: "ed", DisplayName: "Ed", Role: users.RoleEditor})
	if err != nil {
		t.Fatalf("Create editor: %v", err)
	}
	ed, _ := f.repoo.GetByUsername(context.Background(), "ed")
	if err := users.ChangeOwnPassword(context.Background(), f.repoo, ed.ID, otp, "editor-long-password"); err != nil {
		t.Fatalf("set editor password: %v", err)
	}
	return loginAs(t, f.handler, "ed", "editor-long-password")
}

// TestSearchEndpoint: an authed GET /search returns 200 + a JSON array of typed
// page results; an unauthenticated request is rejected by the authed group; an
// empty q returns 200 [].
func TestSearchEndpoint(t *testing.T) {
	f := newSearchServer(t)
	f.seedAndIndex(t, "guide.md", "Findable Guide", "this page contains the findword term")
	cookies := loginSearchEditor(t, f)

	// Authed query with a hit.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=findword", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authed search = %d, body=%s", rec.Code, rec.Body.String())
	}
	var results []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode results: %v (body=%s)", err, rec.Body.String())
	}
	if len(results) == 0 {
		t.Fatalf("expected at least one result, got none: %s", rec.Body.String())
	}
	if kind, _ := results[0]["kind"].(string); kind != "page" {
		t.Fatalf("result kind = %q, want page", kind)
	}

	// Empty q → 200 [].
	req = httptest.NewRequest(http.MethodGet, "/api/v1/search?q=", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("empty-q search = %d", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("empty-q body = %q, want []", rec.Body.String())
	}

	// Unauthenticated → rejected (not 200).
	req = httptest.NewRequest(http.MethodGet, "/api/v1/search?q=findword", nil)
	rec = httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("unauthenticated search returned 200, want rejection")
	}
}

// TestReindexAdminOnly: POST /admin/search/reindex returns 202 for an admin and
// 403 for a non-admin (editor); no Git/index vocabulary appears in error bodies.
func TestReindexAdminOnly(t *testing.T) {
	f := newSearchServer(t)

	// Editor → 403.
	editorCookies := loginSearchEditor(t, f)
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/search/reindex", nil, editorCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("editor reindex = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}

	// Admin → 202.
	adminCookies := loginAsAdmin(t, f.handler, f.repoo)
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/search/reindex", nil, adminCookies)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("admin reindex = %d, want 202 (body=%s)", rec.Code, rec.Body.String())
	}

	assertNoGitVocab(t, rec.Body.String())
}

// assertNoGitVocab fails if a user-facing body leaks index/Git internals.
func assertNoGitVocab(t *testing.T, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	for _, term := range []string{"bleve", "index", "head", "commit", "git", "repo"} {
		if strings.Contains(lower, term) {
			t.Fatalf("response body leaks hidden-Git vocabulary %q: %s", term, body)
		}
	}
}
