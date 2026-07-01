// handlers_tags_admin_test.go — Phase 19 / v1.3 tag admin endpoint tests.
//
// Tests cover: RBAC (editor/reader → 403), preview→approve flow, tag rename,
// merge, delete, input validation, and edge cases (empty names, charset,
// case-only rename no-op).
package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/graph"
	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/pages"
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/server"
	"github.com/postfix/okworkspace/internal/store"
	"github.com/postfix/okworkspace/internal/users"
)

type tagAdminFixture struct {
	handler http.Handler
	repo    *repo.Repo
	store   *graph.Store
	db      *store.Store
	users   *users.Repository
	gs      *gitstore.GitStore
}

func newTagAdminServer(t *testing.T) *tagAdminFixture {
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
	w.Register(pages.KindCommit, pages.CommitHandler(contentRepo, gs))
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	t.Cleanup(func() { w.Stop(); cancel() })

	ps := pages.NewService(contentRepo, gs, w, st.DB(), false)

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
		Pages:     ps,
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return &tagAdminFixture{
		handler: h, repo: contentRepo, store: graphStore,
		db: st, users: userRepo, gs: gs,
	}
}

// seedPage writes a page with the given path and tags, commits it via Git,
// then rebuilds the graph so PagesByTag can find it.
func (f *tagAdminFixture) seedPage(t *testing.T, path string, tags []string) {
	t.Helper()
	content := "---\ntype: page\ntitle: " + path + "\ndescription: \ntimestamp: 2026-06-21T00:00:00Z\n---\n\nbody\n"
	if tags != nil && len(tags) > 0 {
		// Insert tags before body
		fm := ""
		for _, tag := range tags {
			fm += "  - " + tag + "\n"
		}
		content = "---\ntype: page\ntitle: " + path + "\ndescription: \ntags:\n" + fm + "timestamp: 2026-06-21T00:00:00Z\n---\n\nbody\n"
	}
	if err := f.repo.Write(path, []byte(content)); err != nil {
		t.Fatalf("write page %q: %v", path, err)
	}
	if err := f.gs.Commit(context.Background(), gitstore.CommitSpec{
		Paths:   []string{path},
		Message: "seed " + path,
		User:    "test",
		Action:  "seed",
		Source:  "test",
	}); err != nil {
		t.Fatalf("commit %q: %v", path, err)
	}
	if err := f.store.RebuildGraph(context.Background()); err != nil {
		t.Fatalf("RebuildGraph: %v", err)
	}
}

// adminCookies logs in as the bootstrap admin.
func (f *tagAdminFixture) adminCookies(t *testing.T) []*http.Cookie {
	t.Helper()
	return loginAsAdmin(t, f.handler, f.users)
}

// editorCookies creates and logs in an editor.
func (f *tagAdminFixture) editorCookies(t *testing.T) []*http.Cookie {
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

// --- RBAC tests ---

func TestTagAdmin_EditorForbidden(t *testing.T) {
	f := newTagAdminServer(t)
	editor := f.editorCookies(t)

	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/api/v1/admin/tags/rename", map[string]string{"old_name": "ops", "new_name": "devops"}},
		{http.MethodPost, "/api/v1/admin/tags/merge", map[string]string{"source": "ops", "target": "devops"}},
		{http.MethodPost, "/api/v1/admin/tags/delete", map[string]string{"tag": "ops"}},
	} {
		rec := doMutate(t, f.handler, tc.method, tc.path, tc.body, editor)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: editor got %d, want 403", tc.method, tc.path, rec.Code)
		}
	}
}

// --- Rename tests ---

func TestTagRename_PreviewThenApply(t *testing.T) {
	f := newTagAdminServer(t)
	f.seedPage(t, "a.md", []string{"ops"})
	f.seedPage(t, "b.md", []string{"ops", "docs"})
	admin := f.adminCookies(t)

	// Preview
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/rename",
		map[string]any{"old_name": "ops", "new_name": "devops", "preview": true}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview rename: %d %s", rec.Code, rec.Body.String())
	}
	var prev struct {
		Tag           string `json:"tag"`
		TotalAffected int    `json:"total_affected"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &prev); err != nil {
		t.Fatalf("unmarshal preview: %v", err)
	}
	if prev.TotalAffected != 2 {
		t.Errorf("preview TotalAffected = %d, want 2", prev.TotalAffected)
	}

	// Apply
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/rename",
		map[string]string{"old_name": "ops", "new_name": "devops"}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply rename: %d %s", rec.Code, rec.Body.String())
	}

	// Rebuild graph to reflect changes (the graph upsert is async)
	if err := f.store.RebuildGraph(context.Background()); err != nil {
		t.Fatalf("RebuildGraph after rename: %v", err)
	}

	// Verify: old tag gone from graph cache
	paths, err := f.store.PagesByTag(context.Background(), "ops")
	if err != nil {
		t.Fatalf("PagesByTag ops: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("old tag 'ops' still on %d pages after rename", len(paths))
	}

	// Verify: new tag present
	paths, err = f.store.PagesByTag(context.Background(), "devops")
	if err != nil {
		t.Fatalf("PagesByTag devops: %v", err)
	}
	if len(paths) != 2 {
		t.Errorf("new tag 'devops' on %d pages, want 2", len(paths))
	}
}

func TestTagRename_EmptyName(t *testing.T) {
	f := newTagAdminServer(t)
	admin := f.adminCookies(t)

	for _, body := range []map[string]string{
		{"old_name": "", "new_name": "x"},
		{"old_name": "x", "new_name": ""},
		{"old_name": "   ", "new_name": "   "},
	} {
		rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/rename", body, admin)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("rename %v: got %d, want 400", body, rec.Code)
		}
	}
}

func TestTagRename_CaseOnlyNoOp(t *testing.T) {
	f := newTagAdminServer(t)
	f.seedPage(t, "a.md", []string{"Ops"})
	admin := f.adminCookies(t)

	// Case-only rename: "Ops" → "ops" should be a no-op after normalization
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/rename",
		map[string]string{"old_name": "Ops", "new_name": "ops"}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("case-only rename: %d %s", rec.Code, rec.Body.String())
	}

	// Tag should still exist
	paths, _ := f.store.PagesByTag(context.Background(), "ops")
	if len(paths) != 1 {
		t.Errorf("expected 1 page with 'ops' after case-only rename, got %d", len(paths))
	}
}

func TestTagRename_BadCharset(t *testing.T) {
	f := newTagAdminServer(t)
	admin := f.adminCookies(t)

	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/rename",
		map[string]string{"old_name": "x", "new_name": "<script>"}, admin)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad charset rename: got %d, want 400", rec.Code)
	}
}

// TestTagRename_InlineGrammarGate proves the ENDPOINT enforces the stricter
// inline-tag grammar (ValidateInlineTag), not merely the legacy charset gate.
// Each token here is ACCEPTED by the old ValidateTagCharset (^[a-zA-Z0-9_ -]+$)
// but REJECTED by ValidateInlineTag — so a 400 can only come from the new gate
// being the active HTTP validator (TR-03).
func TestTagRename_InlineGrammarGate(t *testing.T) {
	f := newTagAdminServer(t)
	admin := f.adminCookies(t)

	for _, newName := range []string{
		"a b",  // space: legal charset, illegal body alphabet
		"1984", // all-digit: legal charset, no qualifying non-digit
	} {
		rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/rename",
			map[string]string{"old_name": "x", "new_name": newName}, admin)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("rename new_name=%q: got %d, want 400", newName, rec.Code)
		}
	}
}

// TestTagRename_FrontmatterCharsetGate proves the rename ENDPOINT rejects a
// target that is body-splice-safe (passes ValidateInlineTag) but NOT
// frontmatter-storable — a nested tag like "a/b" whose '/' the frontmatter
// charset (^[a-zA-Z0-9_ -]+$) forbids. Before SEC-v1.5-F01 this passed the
// inline gate (preview 200) then 500'd at the frontmatter write on apply; the
// added ValidateTagCharset gate makes it a clean 400 at the gate instead.
func TestTagRename_FrontmatterCharsetGate(t *testing.T) {
	f := newTagAdminServer(t)
	admin := f.adminCookies(t)

	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/rename",
		map[string]string{"old_name": "x", "new_name": "a/b"}, admin)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("rename new_name=%q: got %d, want 400", "a/b", rec.Code)
	}
}

// --- Merge tests ---

func TestTagMerge_PreviewThenApply(t *testing.T) {
	f := newTagAdminServer(t)
	f.seedPage(t, "a.md", []string{"ops"})
	f.seedPage(t, "b.md", []string{"devops"})
	f.seedPage(t, "c.md", []string{"ops", "devops"})
	admin := f.adminCookies(t)

	// Preview
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/merge",
		map[string]any{"source": "ops", "target": "devops", "preview": true}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview merge: %d %s", rec.Code, rec.Body.String())
	}

	// Apply
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/merge",
		map[string]string{"source": "ops", "target": "devops"}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply merge: %d %s", rec.Code, rec.Body.String())
	}

	// Rebuild graph
	if err := f.store.RebuildGraph(context.Background()); err != nil {
		t.Fatalf("RebuildGraph after merge: %v", err)
	}

	// Source tag gone
	paths, _ := f.store.PagesByTag(context.Background(), "ops")
	if len(paths) != 0 {
		t.Errorf("source tag 'ops' still on %d pages after merge", len(paths))
	}

	// Target tag on all 3 pages
	paths, _ = f.store.PagesByTag(context.Background(), "devops")
	if len(paths) != 3 {
		t.Errorf("target tag 'devops' on %d pages, want 3", len(paths))
	}
}

func TestTagMerge_SameSourceTarget(t *testing.T) {
	f := newTagAdminServer(t)
	admin := f.adminCookies(t)

	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/merge",
		map[string]string{"source": "ops", "target": "ops"}, admin)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("same source/target merge: got %d, want 400", rec.Code)
	}
}

// TestTagMerge_InlineGrammarGate proves the merge ENDPOINT enforces the inline
// grammar on its target. Merge previously had NO validator-reject handler test;
// these targets pass the legacy charset gate but fail ValidateInlineTag, so a
// 400 confirms the new gate is the active HTTP validator on merge too (TR-03).
func TestTagMerge_InlineGrammarGate(t *testing.T) {
	f := newTagAdminServer(t)
	admin := f.adminCookies(t)

	for _, target := range []string{
		"a b",  // space: legal charset, illegal body alphabet
		"1984", // all-digit: legal charset, no qualifying non-digit
	} {
		rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/merge",
			map[string]string{"source": "ops", "target": target}, admin)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("merge target=%q: got %d, want 400", target, rec.Code)
		}
	}
}

// TestTagMerge_FrontmatterCharsetGate proves the merge ENDPOINT rejects a target
// that is body-splice-safe (passes ValidateInlineTag) but not frontmatter-storable
// — a nested tag like "a/b" whose '/' the frontmatter charset forbids. Before
// SEC-v1.5-F01 this 500'd at the frontmatter write on apply; the added
// ValidateTagCharset gate makes it a clean 400 at the gate (see rename twin).
func TestTagMerge_FrontmatterCharsetGate(t *testing.T) {
	f := newTagAdminServer(t)
	admin := f.adminCookies(t)

	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/merge",
		map[string]string{"source": "ops", "target": "a/b"}, admin)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("merge target=%q: got %d, want 400", "a/b", rec.Code)
	}
}

// --- Delete tests ---

func TestTagDelete_PreviewThenApply(t *testing.T) {
	f := newTagAdminServer(t)
	f.seedPage(t, "a.md", []string{"ops", "docs"})
	f.seedPage(t, "b.md", []string{"ops"})
	admin := f.adminCookies(t)

	// Preview
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/delete",
		map[string]any{"tag": "ops", "preview": true}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview delete: %d %s", rec.Code, rec.Body.String())
	}
	var prev struct {
		TotalAffected int `json:"total_affected"`
	}
	json.Unmarshal(rec.Body.Bytes(), &prev)
	if prev.TotalAffected != 2 {
		t.Errorf("preview TotalAffected = %d, want 2", prev.TotalAffected)
	}

	// Apply
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/delete",
		map[string]string{"tag": "ops"}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply delete: %d %s", rec.Code, rec.Body.String())
	}

	// Rebuild graph
	if err := f.store.RebuildGraph(context.Background()); err != nil {
		t.Fatalf("RebuildGraph after delete: %v", err)
	}

	// Tag gone
	paths, _ := f.store.PagesByTag(context.Background(), "ops")
	if len(paths) != 0 {
		t.Errorf("tag 'ops' still on %d pages after delete", len(paths))
	}

	// Other tag untouched
	paths, _ = f.store.PagesByTag(context.Background(), "docs")
	if len(paths) != 1 {
		t.Errorf("unrelated tag 'docs' on %d pages, want 1", len(paths))
	}
}

func TestTagDelete_NonexistentTag(t *testing.T) {
	f := newTagAdminServer(t)
	admin := f.adminCookies(t)

	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/delete",
		map[string]string{"tag": "noexist"}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete nonexistent: %d %s", rec.Code, rec.Body.String())
	}
}
