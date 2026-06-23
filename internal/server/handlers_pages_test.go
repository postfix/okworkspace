package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/pages"
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/server"
	"github.com/postfix/okworkspace/internal/store"
	"github.com/postfix/okworkspace/internal/users"
)

// pageFixture wires a full server with a real pages.Service (repo + git + worker)
// so the page/tree/folder routes are exercised end to end.
type pageFixture struct {
	handler http.Handler
	repo    *repo.Repo
	repoo   *users.Repository
}

func newPageServer(t *testing.T) *pageFixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}

	st, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	contentRepo, err := repo.New(filepath.Join(t.TempDir(), "repo"))
	if err != nil {
		t.Fatalf("repo.New: %v", err)
	}
	t.Cleanup(func() { _ = contentRepo.Close() })
	gs := gitstore.New(contentRepo, config.GitConfig{Enabled: true, Branch: "main"})
	if err := gs.Init(context.Background()); err != nil {
		t.Fatalf("gitstore.Init: %v", err)
	}

	w := jobs.New(st.DB(), jobs.Config{PollInterval: 5 * time.Millisecond})
	w.Register(pages.KindCommit, pages.CommitHandler(contentRepo, gs))
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	t.Cleanup(func() { w.Stop(); cancel() })

	pagesSvc := pages.NewService(contentRepo, gs, w, st.DB(), false)

	var cfg config.Config
	cfg.Auth.SessionCookieName = config.DefaultSessionCookieName
	cfg.Auth.SessionTTLHours = config.DefaultSessionTTLHours
	cfg.Admin.Username = "admin"
	userRepo := users.NewRepository(st.DB())
	if _, _, _, err := users.BootstrapAdmin(context.Background(), userRepo, cfg); err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	h, err := server.New(server.Deps{
		Store:    st,
		Config:   cfg,
		UserRepo: userRepo,
		Audit:    audit.New(st.DB(), nil),
		Pages:    pagesSvc,
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return &pageFixture{handler: h, repo: contentRepo, repoo: userRepo}
}

// loginEditor creates an editor user and logs in, returning the session cookies.
func loginEditor(t *testing.T, f *pageFixture) []*http.Cookie {
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

// loginReader creates a reader user and logs in, returning the session cookies.
func loginReader(t *testing.T, f *pageFixture) []*http.Cookie {
	t.Helper()
	_, otp, err := users.Create(context.Background(), f.repoo, users.NewUser{Username: "rd", DisplayName: "Reader", Role: users.RoleReader})
	if err != nil {
		t.Fatalf("Create reader: %v", err)
	}
	rd, _ := f.repoo.GetByUsername(context.Background(), "rd")
	if err := users.ChangeOwnPassword(context.Background(), f.repoo, rd.ID, otp, "reader-long-password"); err != nil {
		t.Fatalf("set reader password: %v", err)
	}
	return loginAs(t, f.handler, "rd", "reader-long-password")
}

// waitForPath polls the repo until a page path exists in the tree (i.e. the
// commit job has drained).
func waitForPath(t *testing.T, f *pageFixture, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ok, _ := f.repo.Exists(path); ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("page %q never appeared (commit did not drain)", path)
}

func createPageAs(t *testing.T, f *pageFixture, cookies []*http.Cookie, folder, title string) string {
	t.Helper()
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/pages",
		map[string]string{"folder": folder, "title": title}, cookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create page = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode create resp: %v", err)
	}
	waitForPath(t, f, resp.Path)
	return resp.Path
}

func TestCreatePageRBAC(t *testing.T) {
	f := newPageServer(t)

	// Reader: 403 with the permission copy.
	readerCookies := loginReader(t, f)
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/pages",
		map[string]string{"folder": "", "title": "Nope"}, readerCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("reader create = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}

	// Editor: 201 and the page appears in the tree.
	editorCookies := loginEditor(t, f)
	path := createPageAs(t, f, editorCookies, "runbooks", "Deploy Staging")

	treeRec := doGet(t, f.handler, "/api/v1/tree", editorCookies)
	if treeRec.Code != http.StatusOK {
		t.Fatalf("GET tree = %d", treeRec.Code)
	}
	if !containsPath(t, treeRec.Body.Bytes(), path) {
		t.Fatalf("created page %q not in tree: %s", path, treeRec.Body.String())
	}
}

func TestGetPage(t *testing.T) {
	f := newPageServer(t)
	editorCookies := loginEditor(t, f)
	path := createPageAs(t, f, editorCookies, "runbooks", "Deploy Staging")

	rec := doGet(t, f.handler, "/api/v1/pages/"+path, editorCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET page = %d, body=%s", rec.Code, rec.Body.String())
	}
	var page struct {
		Frontmatter string `json:"frontmatter"`
		Body        string `json:"body"`
		Revision    string `json:"revision"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode page: %v", err)
	}
	if page.Revision == "" {
		t.Fatal("GET page returned empty revision")
	}
	if page.Frontmatter == "" {
		t.Fatal("GET page returned empty frontmatter")
	}
}

func TestSavePageConflict(t *testing.T) {
	f := newPageServer(t)
	editorCookies := loginEditor(t, f)
	path := createPageAs(t, f, editorCookies, "", "Notes")

	// On-disk content before the (rejected) save.
	before, _ := f.repo.Read(path)

	// Save with a stale base_revision -> 409, file unchanged.
	rec := doMutate(t, f.handler, http.MethodPut, "/api/v1/pages/"+path,
		map[string]string{
			"body":          "changed\n",
			"frontmatter":   "type: Page\ntitle: Notes\n",
			"base_revision": "not-the-real-revision",
		}, editorCookies)
	if rec.Code != http.StatusConflict {
		t.Fatalf("stale save = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
	}
	time.Sleep(50 * time.Millisecond)
	after, _ := f.repo.Read(path)
	if string(before) != string(after) {
		t.Fatal("stale save mutated the on-disk file; 409 floor must write nothing")
	}
}

func TestSavePageAudits(t *testing.T) {
	st := newPageServerStore(t)
	f := st.fixture
	editorCookies := loginEditor(t, f)
	path := createPageAs(t, f, editorCookies, "", "Notes")

	// Read the current revision so the save is accepted.
	getRec := doGet(t, f.handler, "/api/v1/pages/"+path, editorCookies)
	var page struct {
		Revision string `json:"revision"`
	}
	_ = json.Unmarshal(getRec.Body.Bytes(), &page)

	rec := doMutate(t, f.handler, http.MethodPut, "/api/v1/pages/"+path,
		map[string]string{
			"body":          "# Updated\n",
			"frontmatter":   "type: Page\ntitle: Notes\ntags: []\ntimestamp: 2026-06-18T12:00:00Z\ndescription: \"\"\n",
			"base_revision": page.Revision,
		}, editorCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("save = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
	if n := countAudit(t, st.store, audit.ActionPageEdit); n < 1 {
		t.Fatalf("expected a page_edit audit row, got %d", n)
	}
}

func TestTreeHandler(t *testing.T) {
	f := newPageServer(t)
	editorCookies := loginEditor(t, f)
	createPageAs(t, f, editorCookies, "", "Home")

	// Reader can read the tree (reads are open to any authenticated user).
	readerCookies := loginReader(t, f)
	rec := doGet(t, f.handler, "/api/v1/tree", readerCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("reader GET tree = %d, want 200", rec.Code)
	}
	var nodes []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &nodes); err != nil {
		t.Fatalf("decode tree: %v (body=%s)", err, rec.Body.String())
	}
}

func TestWildcardPath(t *testing.T) {
	f := newPageServer(t)
	editorCookies := loginEditor(t, f)
	// A slash-bearing page path must resolve via {path:.*}.
	path := createPageAs(t, f, editorCookies, "a/b", "Deep Page")
	if path != "a/b/deep-page.md" {
		t.Fatalf("nested create path = %q, want a/b/deep-page.md", path)
	}
	rec := doGet(t, f.handler, "/api/v1/pages/"+path, editorCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET nested page = %d, body=%s", rec.Code, rec.Body.String())
	}
}

// TestGetPageInVersionFolder is the CR-01 regression: a page that literally
// lives inside a folder named "version" (docs/version/notes.md) must read as the
// PAGE, not be mis-routed to the view-version handler. The dispatch is anchored
// on the ".md/version/" boundary, so a real "/version/" path segment no longer
// hijacks the read.
func TestGetPageInVersionFolder(t *testing.T) {
	f := newPageServer(t)
	editorCookies := loginEditor(t, f)

	// Page under a folder named "version".
	versionPagePath := createPageAs(t, f, editorCookies, "docs/version", "Notes")
	if versionPagePath != "docs/version/notes.md" {
		t.Fatalf("create path = %q, want docs/version/notes.md", versionPagePath)
	}
	rec := doGet(t, f.handler, "/api/v1/pages/"+versionPagePath, editorCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET page under /version/ folder = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var vp struct {
		Body     string `json:"body"`
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &vp); err != nil {
		t.Fatalf("decode page: %v (body=%s)", err, rec.Body.String())
	}
	if vp.Revision == "" {
		t.Fatal("page under /version/ folder read returned empty revision (mis-routed to view-version?)")
	}

	// A page in a folder named "history" must also read as a page (the "/history"
	// suffix only triggers history when it directly follows the ".md").
	histPagePath := createPageAs(t, f, editorCookies, "docs/history", "Log")
	if histPagePath != "docs/history/log.md" {
		t.Fatalf("create path = %q, want docs/history/log.md", histPagePath)
	}
	rec = doGet(t, f.handler, "/api/v1/pages/"+histPagePath, editorCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET page under /history folder = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	// Sanity: the real sub-routes still work on this page. History returns a
	// version list ...
	entries := waitForHistoryLen(t, f, editorCookies, versionPagePath, 1)
	version, _ := entries[len(entries)-1]["version"].(string)
	if version == "" {
		t.Fatal("history of page under /version/ folder returned no version token")
	}
	// ... and view-version returns the page at that opaque token.
	vrec := doGet(t, f.handler, "/api/v1/pages/"+versionPagePath+"/version/"+version, editorCookies)
	if vrec.Code != http.StatusOK {
		t.Fatalf("GET .md/version/<token> = %d, want 200 (body=%s)", vrec.Code, vrec.Body.String())
	}
	var vv struct {
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal(vrec.Body.Bytes(), &vv); err != nil {
		t.Fatalf("decode view-version: %v (body=%s)", err, vrec.Body.String())
	}
	if vv.Revision == "" {
		t.Fatal("view-version of page under /version/ folder returned empty revision")
	}
}

// --- small helpers ---

func doGet(t *testing.T, h http.Handler, path string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func containsPath(t *testing.T, treeJSON []byte, path string) bool {
	t.Helper()
	var nodes []map[string]any
	if err := json.Unmarshal(treeJSON, &nodes); err != nil {
		t.Fatalf("decode tree: %v", err)
	}
	var walk func(ns []map[string]any) bool
	walk = func(ns []map[string]any) bool {
		for _, n := range ns {
			if p, _ := n["path"].(string); p == path {
				return true
			}
			if children, ok := n["children"].([]any); ok {
				cs := make([]map[string]any, 0, len(children))
				for _, c := range children {
					if cm, ok := c.(map[string]any); ok {
						cs = append(cs, cm)
					}
				}
				if walk(cs) {
					return true
				}
			}
		}
		return false
	}
	return walk(nodes)
}

// pageServerStore bundles a fixture with its store for audit assertions.
type pageServerStore struct {
	fixture *pageFixture
	store   *store.Store
}

func newPageServerStore(t *testing.T) *pageServerStore {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	contentRepo, err := repo.New(filepath.Join(t.TempDir(), "repo"))
	if err != nil {
		t.Fatalf("repo.New: %v", err)
	}
	t.Cleanup(func() { _ = contentRepo.Close() })
	gs := gitstore.New(contentRepo, config.GitConfig{Enabled: true, Branch: "main"})
	if err := gs.Init(context.Background()); err != nil {
		t.Fatalf("gitstore.Init: %v", err)
	}
	w := jobs.New(st.DB(), jobs.Config{PollInterval: 5 * time.Millisecond})
	w.Register(pages.KindCommit, pages.CommitHandler(contentRepo, gs))
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	t.Cleanup(func() { w.Stop(); cancel() })
	pagesSvc := pages.NewService(contentRepo, gs, w, st.DB(), false)

	var cfg config.Config
	cfg.Auth.SessionCookieName = config.DefaultSessionCookieName
	cfg.Auth.SessionTTLHours = config.DefaultSessionTTLHours
	cfg.Admin.Username = "admin"
	userRepo := users.NewRepository(st.DB())
	if _, _, _, err := users.BootstrapAdmin(context.Background(), userRepo, cfg); err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	h, err := server.New(server.Deps{
		Store:    st,
		Config:   cfg,
		UserRepo: userRepo,
		Audit:    audit.New(st.DB(), nil),
		Pages:    pagesSvc,
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return &pageServerStore{fixture: &pageFixture{handler: h, repo: contentRepo, repoo: userRepo}, store: st}
}
