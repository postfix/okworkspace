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
	"github.com/postfix/okworkspace/internal/graph"
	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/server"
	"github.com/postfix/okworkspace/internal/store"
	"github.com/postfix/okworkspace/internal/users"
)

// graphFixture wires a full server with a real graph.Store + a real jobs worker
// (KindGraph handler registered) over a real content repo so POST
// /admin/graph/reindex and the authed graph READ endpoints are exercised end to
// end through the actual enqueue + query paths.
type graphFixture struct {
	handler http.Handler
	repoo   *users.Repository
	repo    *repo.Repo
	store   *graph.Store
}

// newGraphServer mirrors newSearchServer but stands up the graph store + worker
// (no search.Index needed for the reindex-RBAC test). The KindGraph handler is
// registered on the worker passed as GraphJobs, so the enqueue is real.
func newGraphServer(t *testing.T) *graphFixture {
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

	graphStore := graph.OpenStore(st.DB())
	graphStore.SetRepo(contentRepo)
	graphStore.SetGit(gs)

	w := jobs.New(st.DB(), jobs.Config{PollInterval: 5 * time.Millisecond})
	w.Register(graph.KindGraph, graph.GraphHandler(graphStore, contentRepo))
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
		Store:     st,
		Config:    cfg,
		UserRepo:  userRepo,
		Audit:     audit.New(st.DB(), nil),
		GraphJobs: w,
		Graph:     graphStore,
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return &graphFixture{handler: h, repoo: userRepo, repo: contentRepo, store: graphStore}
}

// seedAndRebuild writes a page to the repo and rebuilds the derived graph so the
// read endpoints have populated cache tables to query.
func (f *graphFixture) seedAndRebuild(t *testing.T, path, title, body string) {
	t.Helper()
	content := "---\ntype: page\ntitle: " + title + "\ndescription: \ntags:\n  - design\ntimestamp: 2026-06-21T00:00:00Z\n---\n\n" + body + "\n"
	if err := f.repo.Write(path, []byte(content)); err != nil {
		t.Fatalf("write page: %v", err)
	}
	if err := f.store.RebuildGraph(context.Background()); err != nil {
		t.Fatalf("RebuildGraph: %v", err)
	}
}

// authedGet issues an authed GET and returns the recorder.
func authedGet(t *testing.T, h http.Handler, url string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// loginGraphEditor creates an editor user and logs in (mirrors loginSearchEditor).
func loginGraphEditor(t *testing.T, f *graphFixture) []*http.Cookie {
	t.Helper()
	_, otp, err := users.Create(context.Background(), f.repoo, users.NewUser{Username: "ged", DisplayName: "GEd", Role: users.RoleEditor})
	if err != nil {
		t.Fatalf("Create editor: %v", err)
	}
	ed, _ := f.repoo.GetByUsername(context.Background(), "ged")
	if err := users.ChangeOwnPassword(context.Background(), f.repoo, ed.ID, otp, "editor-long-password"); err != nil {
		t.Fatalf("set editor password: %v", err)
	}
	return loginAs(t, f.handler, "ged", "editor-long-password")
}

// TestGraphReindexAdminOnly: POST /admin/graph/reindex returns 202 for an admin
// and 403 for a non-admin (editor) — RBAC is read from the session role via the
// admin subgroup, never client input. No Git/index vocabulary appears in the
// admin success body (hidden-Git rule). Clones TestReindexAdminOnly.
func TestGraphReindexAdminOnly(t *testing.T) {
	f := newGraphServer(t)

	// Editor → 403.
	editorCookies := loginGraphEditor(t, f)
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/graph/reindex", nil, editorCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("editor graph reindex = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}

	// Admin → 202.
	adminCookies := loginAsAdmin(t, f.handler, f.repoo)
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/graph/reindex", nil, adminCookies)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("admin graph reindex = %d, want 202 (body=%s)", rec.Code, rec.Body.String())
	}

	assertNoGitVocab(t, rec.Body.String())
}

// TestGraphReadEndpoints: the three authed graph reads return 200 with the lean
// JSON shape for any logged-in user; an unauthenticated request is rejected.
func TestGraphReadEndpoints(t *testing.T) {
	f := newGraphServer(t)
	f.seedAndRebuild(t, "a.md", "Alpha", "see [b](b.md)")
	f.seedAndRebuild(t, "b.md", "Bravo", "no links")
	cookies := loginGraphEditor(t, f)

	// GET /graph → nodes/edges body.
	rec := authedGet(t, f.handler, "/api/v1/graph", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /graph = %d, body=%s", rec.Code, rec.Body.String())
	}
	var g struct {
		Nodes []map[string]any `json:"nodes"`
		Edges []map[string]any `json:"edges"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &g); err != nil {
		t.Fatalf("decode graph: %v (body=%s)", err, rec.Body.String())
	}
	if len(g.Nodes) == 0 {
		t.Fatalf("expected page nodes, got none: %s", rec.Body.String())
	}
	// Lean-shape guard at the HTTP layer too: no bodies leak.
	if strings.Contains(rec.Body.String(), "body") || strings.Contains(rec.Body.String(), "frontmatter") {
		t.Fatalf("graph payload leaks body/frontmatter: %s", rec.Body.String())
	}

	// GET /graph/local?path=a.md&depth=1 → neighborhood.
	rec = authedGet(t, f.handler, "/api/v1/graph/local?path=a.md&depth=1", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /graph/local = %d, body=%s", rec.Code, rec.Body.String())
	}

	// GET /graph/backlinks?path=b.md → a.md links to b.md.
	rec = authedGet(t, f.handler, "/api/v1/graph/backlinks?path=b.md", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /graph/backlinks = %d, body=%s", rec.Code, rec.Body.String())
	}
	var bl []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &bl); err != nil {
		t.Fatalf("decode backlinks: %v (body=%s)", err, rec.Body.String())
	}
	if len(bl) != 1 || bl[0]["path"] != "a.md" {
		t.Fatalf("backlinks(b.md) = %v, want [a.md]", bl)
	}

	// Unauthenticated → rejected (not 200).
	rec = authedGet(t, f.handler, "/api/v1/graph", nil)
	if rec.Code == http.StatusOK {
		t.Fatalf("unauthenticated /graph returned 200, want rejection")
	}
}

// TestGraphReadMissingPath: /graph/local and /graph/backlinks without a path
// return 400 with generic copy.
func TestGraphReadMissingPath(t *testing.T) {
	f := newGraphServer(t)
	cookies := loginGraphEditor(t, f)

	for _, url := range []string{"/api/v1/graph/local", "/api/v1/graph/backlinks"} {
		rec := authedGet(t, f.handler, url, cookies)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("GET %s (no path) = %d, want 400 (body=%s)", url, rec.Code, rec.Body.String())
		}
	}
}

// TestGraphReadNilDependency: when the Graph dependency is nil, the read endpoints
// return 500 with the generic copy and NO Git/SQLite/Bleve/index tokens leak.
func TestGraphReadNilDependency(t *testing.T) {
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
	var cfg config.Config
	cfg.Auth.SessionCookieName = config.DefaultSessionCookieName
	cfg.Auth.SessionTTLHours = config.DefaultSessionTTLHours
	cfg.Admin.Username = "admin"
	userRepo := users.NewRepository(st.DB())
	if _, _, _, err := users.BootstrapAdmin(context.Background(), userRepo, cfg); err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	// Graph is intentionally nil here.
	h, err := server.New(server.Deps{Store: st, Config: cfg, UserRepo: userRepo, Audit: audit.New(st.DB(), nil)})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	cookies := loginAsAdmin(t, h, userRepo)

	rec := authedGet(t, h, "/api/v1/graph", cookies)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("nil-graph /graph = %d, want 500 (body=%s)", rec.Code, rec.Body.String())
	}
	assertNoGitVocab(t, rec.Body.String())
}
