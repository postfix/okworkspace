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
	"strings"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/graph"
	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/okf"
	"github.com/postfix/okworkspace/internal/pages"
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/server"
	"github.com/postfix/okworkspace/internal/store"
	"github.com/postfix/okworkspace/internal/tagmeta"
	"github.com/postfix/okworkspace/internal/users"
	"gopkg.in/yaml.v3"
)

type tagAdminFixture struct {
	handler http.Handler
	repo    *repo.Repo
	store   *graph.Store
	db      *store.Store
	users   *users.Repository
	gs      *gitstore.GitStore
	pages   *pages.Service
	tagmeta *tagmeta.Store
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
	tagmetaStore := tagmeta.NewStore(contentRepo, st.DB(), w, false)

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
		Tagmeta:   tagmetaStore,
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return &tagAdminFixture{
		handler: h, repo: contentRepo, store: graphStore,
		db: st, users: userRepo, gs: gs, pages: ps, tagmeta: tagmetaStore,
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

// TestTagRename_DedupesWhenBothTagsPresent proves the rename conflict/dedupe
// branch (TC-01): a page carrying BOTH the old AND the new tag must collapse to a
// SINGLE new tag after rename — never a duplicate. Two orderings are seeded to
// exercise both dedupe paths in computeRenameTags: [devops, ops] hits the
// containsNormalized(out, newName) guard (handlers_tags_admin.go:132 — the new
// name is already emitted before the old-name entry is reached, so the rename
// drops rather than re-appends), and [ops, devops] hits the trailing else-branch
// skip. Both pages MUST end with exactly one "devops" tag on disk. (The merge
// twin of this dedupe is already covered; rename was the untested gap.)
func TestTagRename_DedupesWhenBothTagsPresent(t *testing.T) {
	f := newTagAdminServer(t)
	f.seedPage(t, "newfirst.md", []string{"devops", "ops"}) // new before old → :132 guard
	f.seedPage(t, "oldfirst.md", []string{"ops", "devops"}) // old before new → else-branch skip
	admin := f.adminCookies(t)

	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/rename",
		map[string]string{"old_name": "ops", "new_name": "devops"}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply rename: %d %s", rec.Code, rec.Body.String())
	}

	// The batch commit is awaited inside ApplyTagsBatch, so the rewritten pages
	// are on disk once the apply returns. "devops" appears ONLY in the tags region
	// of the seeded content (title is the path, body is "body"), so a count of 1
	// is exactly the single-deduped-tag assertion; a failed dedupe would leave 2
	// (the renamed ops→devops PLUS the original devops).
	for _, path := range []string{"newfirst.md", "oldfirst.md"} {
		raw, err := f.repo.Read(path)
		if err != nil {
			t.Fatalf("read %q: %v", path, err)
		}
		if n := strings.Count(string(raw), "devops"); n != 1 {
			t.Errorf("page %q has %d 'devops' occurrences after rename, want 1 (deduped); content:\n%s", path, n, raw)
		}
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

// --- Bulk multi-tag tests (Phase 31 / TF-01) ---

// tagBulkCommitCount counts git commits in the fixture's repo root.
func tagBulkCommitCount(t *testing.T, root string) int {
	t.Helper()
	cmd := exec.Command("git", "rev-list", "--count", "HEAD")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0
	}
	n := 0
	for _, c := range strings.TrimSpace(string(out)) {
		n = n*10 + int(c-'0')
	}
	return n
}

// pageTags reads a page's raw bytes and extracts the frontmatter tags list,
// parsing the YAML sequence directly (the test package cannot call the
// unexported server.readPageTags).
func pageTags(t *testing.T, f *tagAdminFixture, path string) []string {
	t.Helper()
	raw, err := f.repo.Read(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	doc, err := okf.Parse(raw)
	if err != nil || !doc.HasFrontmatter {
		return nil
	}
	var tags []string
	n := &doc.Front
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i]
		v := n.Content[i+1]
		if k.Value == "tags" && v.Kind == yaml.SequenceNode {
			for _, item := range v.Content {
				tags = append(tags, item.Value)
			}
		}
	}
	return tags
}

// TestBulkTagRBAC: editor gets 403 on both preview and apply.
func TestBulkTagRBAC(t *testing.T) {
	f := newTagAdminServer(t)
	f.seedPage(t, "a.md", []string{"ops"})
	editor := f.editorCookies(t)

	for _, tc := range []struct {
		name string
		body any
	}{
		{"preview", map[string]any{"page_paths": []string{"a.md"}, "add_tags": []string{"x"}, "preview": true}},
		{"apply", map[string]any{"page_paths": []string{"a.md"}, "add_tags": []string{"x"}}},
	} {
		rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/bulk", tc.body, editor)
		if rec.Code != http.StatusForbidden {
			t.Errorf("bulk %s: editor got %d, want 403", tc.name, rec.Code)
		}
	}
}

// TestBulkTagPreview: seed 3 pages, preview add [a,b] + remove [c] → total_affected=3,
// per-page current_tags/new_tags correct.
func TestBulkTagPreview(t *testing.T) {
	f := newTagAdminServer(t)
	f.seedPage(t, "a.md", []string{"c", "x"})
	f.seedPage(t, "b.md", []string{"c", "y"})
	f.seedPage(t, "d.md", []string{"c", "z"})
	admin := f.adminCookies(t)

	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/bulk",
		map[string]any{
			"page_paths":  []string{"a.md", "b.md", "d.md"},
			"add_tags":    []string{"a", "b"},
			"remove_tags": []string{"c"},
			"preview":     true,
		}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview bulk: %d %s", rec.Code, rec.Body.String())
	}
	var prev struct {
		TotalAffected int `json:"total_affected"`
		AffectedPages []struct {
			PagePath    string   `json:"page_path"`
			CurrentTags []string `json:"current_tags"`
			NewTags     []string `json:"new_tags"`
		} `json:"affected_pages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &prev); err != nil {
		t.Fatalf("unmarshal preview: %v", err)
	}
	if prev.TotalAffected != 3 {
		t.Errorf("preview TotalAffected = %d, want 3", prev.TotalAffected)
	}
	want := map[string][]string{
		"a.md": {"x", "a", "b"},
		"b.md": {"y", "a", "b"},
		"d.md": {"z", "a", "b"},
	}
	for _, p := range prev.AffectedPages {
		exp, ok := want[p.PagePath]
		if !ok {
			t.Errorf("unexpected page %q in preview", p.PagePath)
			continue
		}
		if strings.Join(p.NewTags, ",") != strings.Join(exp, ",") {
			t.Errorf("page %q new_tags = %v, want %v", p.PagePath, p.NewTags, exp)
		}
	}
}

// TestBulkTagGolden (TF-01 crit 4): add [a,b] + remove [c] across 3 pages →
// correct per-page frontmatter in 1 commit; graph rebuild shows `c` gone.
func TestBulkTagGolden(t *testing.T) {
	f := newTagAdminServer(t)
	f.seedPage(t, "a.md", []string{"c", "x"})
	f.seedPage(t, "b.md", []string{"c", "y"})
	f.seedPage(t, "d.md", []string{"c", "z"})
	admin := f.adminCookies(t)

	before := tagBulkCommitCount(t, f.repo.Root())

	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/bulk",
		map[string]any{
			"page_paths":  []string{"a.md", "b.md", "d.md"},
			"add_tags":    []string{"a", "b"},
			"remove_tags": []string{"c"},
		}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply bulk: %d %s", rec.Code, rec.Body.String())
	}

	// Give the async commit job a moment to land.
	time.Sleep(100 * time.Millisecond)

	after := tagBulkCommitCount(t, f.repo.Root())
	if after-before != 1 {
		t.Errorf("commit delta = %d, want exactly 1 (3 pages, one batch commit)", after-before)
	}

	// Verify per-page frontmatter tags.
	wantTags := map[string][]string{
		"a.md": {"x", "a", "b"},
		"b.md": {"y", "a", "b"},
		"d.md": {"z", "a", "b"},
	}
	for path, want := range wantTags {
		got := pageTags(t, f, path)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("page %q tags = %v, want %v", path, got, want)
		}
	}

	// Rebuild graph and verify `c` is gone from these pages.
	if err := f.store.RebuildGraph(context.Background()); err != nil {
		t.Fatalf("RebuildGraph: %v", err)
	}
	paths, _ := f.store.PagesByTag(context.Background(), "c")
	if len(paths) != 0 {
		t.Errorf("tag 'c' still on %d pages after bulk remove", len(paths))
	}
	// Tag 'a' now on all 3 pages.
	paths, _ = f.store.PagesByTag(context.Background(), "a")
	if len(paths) != 3 {
		t.Errorf("tag 'a' on %d pages, want 3", len(paths))
	}
}

// TestBulkTagStale: seed 2 pages, apply, but edit one page between preview and
// apply → that page reports stale, the other applies, batch completes.
func TestBulkTagStale(t *testing.T) {
	f := newTagAdminServer(t)
	f.seedPage(t, "p1.md", []string{"keep"})
	f.seedPage(t, "p2.md", []string{"keep"})
	admin := f.adminCookies(t)
	ctx := context.Background()

	// The bulk op rides ApplyTagsBatch, whose per-page optimistic-concurrency
	// floor excludes a stale page WITHOUT sinking the batch. The bulk handler
	// re-resolves the page set at apply time (like rename/merge/delete), so a
	// stale page is one whose revision moved DURING the apply — between
	// buildTagApplyItems reading it and ApplyTagsBatch re-checking it. We prove
	// that exclusion deterministically by feeding ApplyTagsBatch a stale
	// BaseRevision for p2 (the exact condition a mid-apply edit produces): p2
	// reports "stale", p1 applies, and the batch completes in one commit.
	pg2, err := f.pages.Get(ctx, "p2.md")
	if err != nil {
		t.Fatalf("Get p2: %v", err)
	}
	staleRev := pg2.Revision
	raw, _ := f.repo.Read("p2.md")
	if err := f.repo.Write("p2.md", append(raw, []byte("\nextra line\n")...)); err != nil {
		t.Fatalf("write p2: %v", err)
	}
	if err := f.gs.Commit(ctx, gitstore.CommitSpec{
		Paths: []string{"p2.md"}, Message: "out-of-band p2", User: "test", Action: "edit", Source: "test",
	}); err != nil {
		t.Fatalf("commit p2: %v", err)
	}

	// p1's current revision (fresh).
	pg1, err := f.pages.Get(ctx, "p1.md")
	if err != nil {
		t.Fatalf("Get p1: %v", err)
	}

	before := tagBulkCommitCount(t, f.repo.Root())

	// Apply through the SAME ApplyTagsBatch the bulk handler uses, with p2's
	// BaseRevision stale — the exact item shape buildTagApplyItems produces.
	items := []pages.TagApplyItem{
		{PagePath: "p1.md", Tags: []string{"keep", "new"}, BaseRevision: pg1.Revision, BodyOld: "", BodyNew: ""},
		{PagePath: "p2.md", Tags: []string{"keep", "new"}, BaseRevision: staleRev, BodyOld: "", BodyNew: ""},
	}
	results, err := f.pages.ApplyTagsBatch(ctx, items, "admin")
	if err != nil {
		t.Fatalf("ApplyTagsBatch: %v", err)
	}

	statuses := make(map[string]string)
	for _, r := range results {
		statuses[r.PagePath] = r.Status
	}
	if statuses["p1.md"] != "applied" {
		t.Errorf("p1 status = %q, want applied", statuses["p1.md"])
	}
	if statuses["p2.md"] != "stale" {
		t.Errorf("p2 status = %q, want stale", statuses["p2.md"])
	}

	// One commit for the single applied page (stale page excluded).
	after := tagBulkCommitCount(t, f.repo.Root())
	if after-before != 1 {
		t.Errorf("commit delta = %d, want 1 (stale excluded, one applied page)", after-before)
	}

	// The stale page was NOT clobbered: no "new" tag written.
	for _, t2 := range pageTags(t, f, "p2.md") {
		if t2 == "new" {
			t.Errorf("stale page p2 had 'new' written (clobbered)")
		}
	}

	// Verify the bulk handler itself returns per-page statuses via HTTP: a
	// follow-up bulk apply on p1 succeeds, proving the handler's response shape.
	if err := f.store.RebuildGraph(ctx); err != nil {
		t.Fatalf("RebuildGraph: %v", err)
	}
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/bulk",
		map[string]any{"page_paths": []string{"p1.md"}, "add_tags": []string{"bulk"}}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("bulk HTTP apply: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct{ Results []pages.TagApplyResult `json:"results"` }
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Results) != 1 || resp.Results[0].Status != "applied" {
		t.Errorf("bulk HTTP results = %+v, want one applied", resp.Results)
	}
}

// TestBulkTagFilterSelection: seed pages with tag `ops`, use filter_tags only
// (no explicit paths) → preview resolves to all pages with `ops`.
func TestBulkTagFilterSelection(t *testing.T) {
	f := newTagAdminServer(t)
	f.seedPage(t, "o1.md", []string{"ops"})
	f.seedPage(t, "o2.md", []string{"ops", "docs"})
	f.seedPage(t, "x.md", []string{"other"})
	admin := f.adminCookies(t)

	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/bulk",
		map[string]any{
			"filter_tags": []string{"ops"},
			"add_tags":    []string{"new"},
			"preview":     true,
		}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview bulk: %d %s", rec.Code, rec.Body.String())
	}
	var prev struct {
		TotalAffected int `json:"total_affected"`
		AffectedPages []struct {
			PagePath string `json:"page_path"`
		} `json:"affected_pages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &prev); err != nil {
		t.Fatalf("unmarshal preview: %v", err)
	}
	if prev.TotalAffected != 2 {
		t.Errorf("filter preview TotalAffected = %d, want 2", prev.TotalAffected)
	}
	got := make(map[string]bool)
	for _, p := range prev.AffectedPages {
		got[p.PagePath] = true
	}
	if !got["o1.md"] || !got["o2.md"] {
		t.Errorf("filter resolved pages = %v, want {o1.md, o2.md}", got)
	}
	if got["x.md"] {
		t.Errorf("x.md should not be in the ops filter set")
	}
}

// TestBulkTagValidation: add_tags with invalid charset → 400; empty page set +
// empty add/remove → 400.
func TestBulkTagValidation(t *testing.T) {
	f := newTagAdminServer(t)
	f.seedPage(t, "a.md", []string{"ops"})
	admin := f.adminCookies(t)

	// Invalid charset in add_tags.
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/bulk",
		map[string]any{
			"page_paths": []string{"a.md"},
			"add_tags":   []string{"<script>"},
		}, admin)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid charset: got %d, want 400", rec.Code)
	}

	// Empty page set + empty add/remove.
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/bulk",
		map[string]any{}, admin)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty request: got %d, want 400", rec.Code)
	}
}


// --- Tag metadata tests (Phase 32 / TF-02) ---

// tagListEntry mirrors the GET /admin/tags response shape.
type tagListEntry struct {
	Tag         string `json:"tag"`
	Count       int    `json:"count"`
	Color       string `json:"color"`
	Description string `json:"description"`
	Parent      string `json:"parent"`
}

// TestTagMetaRBAC: editor gets 403 on POST /admin/tags/meta.
func TestTagMetaRBAC(t *testing.T) {
	f := newTagAdminServer(t)
	editor := f.editorCookies(t)

	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/meta",
		map[string]string{"tag": "deploy", "color": "#4caf50", "description": "Deploy ops"}, editor)
	if rec.Code != http.StatusForbidden {
		t.Errorf("editor POST /admin/tags/meta: got %d, want 403", rec.Code)
	}
}

// TestTagMetaSetGet: admin sets color + description; GET /admin/tags reflects
// them; Tags/deploy.md exists on disk.
func TestTagMetaSetGet(t *testing.T) {
	f := newTagAdminServer(t)
	f.seedPage(t, "a.md", []string{"deploy"})
	admin := f.adminCookies(t)

	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/meta",
		map[string]string{"tag": "deploy", "color": "#4caf50", "description": "Deploy ops"}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("set meta: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Tag         string `json:"tag"`
		Color       string `json:"color"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal meta response: %v", err)
	}
	if resp.Color != "#4caf50" || resp.Description != "Deploy ops" {
		t.Errorf("meta response = %+v, want color=#4caf50 desc=Deploy ops", resp)
	}

	// GET /admin/tags must show the metadata.
	rec = doMutate(t, f.handler, http.MethodGet, "/api/v1/admin/tags", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("list tags: %d %s", rec.Code, rec.Body.String())
	}
	var entries []tagListEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("unmarshal tags list: %v", err)
	}
	var found bool
	for _, e := range entries {
		if e.Tag == "deploy" {
			found = true
			if e.Color != "#4caf50" {
				t.Errorf("tag list color = %q, want #4caf50", e.Color)
			}
			if e.Description != "Deploy ops" {
				t.Errorf("tag list description = %q, want Deploy ops", e.Description)
			}
		}
	}
	if !found {
		t.Error("deploy tag not in tag list")
	}

	// Tags/deploy.md must exist on disk.
	raw, err := f.repo.Read("Tags/deploy.md")
	if err != nil {
		t.Fatalf("Read Tags/deploy.md: %v", err)
	}
	if !strings.Contains(string(raw), "color: \"#4caf50\"") {
		t.Errorf("Tags/deploy.md missing color frontmatter: %s", raw)
	}
}

// TestTagMetaValidation: invalid color → 400; invalid description (newline,
// >200 chars, HTML metachar) → 400; invalid tag (a/b) → 400.
func TestTagMetaValidation(t *testing.T) {
	f := newTagAdminServer(t)
	admin := f.adminCookies(t)

	cases := []struct {
		name string
		body map[string]string
	}{
		{"invalid color url()", map[string]string{"tag": "deploy", "color": "url(x)"}},
		{"invalid color var()", map[string]string{"tag": "deploy", "color": "var(--x)"}},
		{"description with newline", map[string]string{"tag": "deploy", "description": "line1\nline2"}},
		{"description too long", map[string]string{"tag": "deploy", "description": strings.Repeat("x", 201)}},
		{"description html metachar", map[string]string{"tag": "deploy", "description": "<script>"}},
		{"invalid tag slash", map[string]string{"tag": "a/b", "color": "red"}},
		{"empty tag", map[string]string{"tag": ""}},
	}
	for _, tc := range cases {
		rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/meta", tc.body, admin)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400 (body=%v)", tc.name, rec.Code, tc.body)
		}
	}
}

// TestTagMetaClear: set metadata, then set with empty color + description →
// file deleted, GET /admin/tags shows empty metadata.
func TestTagMetaClear(t *testing.T) {
	f := newTagAdminServer(t)
	f.seedPage(t, "a.md", []string{"deploy"})
	admin := f.adminCookies(t)

	// Set metadata first.
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/meta",
		map[string]string{"tag": "deploy", "color": "#4caf50", "description": "Deploy ops"}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("set meta: %d %s", rec.Code, rec.Body.String())
	}

	// Clear: empty color + description.
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/meta",
		map[string]string{"tag": "deploy", "color": "", "description": ""}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear meta: %d %s", rec.Code, rec.Body.String())
	}

	// Tags/deploy.md must be gone.
	exists, err := f.repo.Exists("Tags/deploy.md")
	if err != nil {
		t.Fatalf("Exists Tags/deploy.md: %v", err)
	}
	if exists {
		t.Error("Tags/deploy.md still exists after clear")
	}

	// GET /admin/tags shows empty metadata for deploy.
	rec = doMutate(t, f.handler, http.MethodGet, "/api/v1/admin/tags", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("list tags: %d %s", rec.Code, rec.Body.String())
	}
	var entries []tagListEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("unmarshal tags list: %v", err)
	}
	for _, e := range entries {
		if e.Tag == "deploy" {
			if e.Color != "" || e.Description != "" {
				t.Errorf("after clear: deploy color=%q description=%q, want empty", e.Color, e.Description)
			}
		}
	}
}

// TestTagMetaAbsent: GET /admin/tags with no Tags/ folder → tags render with
// empty color/description (graceful).
func TestTagMetaAbsent(t *testing.T) {
	f := newTagAdminServer(t)
	f.seedPage(t, "a.md", []string{"ops"})
	admin := f.adminCookies(t)

	// No Tags/ folder exists — the list must still work with empty metadata.
	rec := doMutate(t, f.handler, http.MethodGet, "/api/v1/admin/tags", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("list tags: %d %s", rec.Code, rec.Body.String())
	}
	var entries []tagListEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("unmarshal tags list: %v", err)
	}
	var found bool
	for _, e := range entries {
		if e.Tag == "ops" {
			found = true
			if e.Color != "" {
				t.Errorf("absent meta: color=%q, want empty", e.Color)
			}
			if e.Description != "" {
				t.Errorf("absent meta: description=%q, want empty", e.Description)
			}
		}
	}
	if !found {
		t.Error("ops tag not in tag list")
	}
}

// --- Tag hierarchy tests (Phase 33 / TF-03) ---

// TestTagHierarchySetParent: admin sets parent="ops" on "deploy"; GET
// /admin/tags shows deploy with parent="ops".
func TestTagHierarchySetParent(t *testing.T) {
	f := newTagAdminServer(t)
	f.seedPage(t, "a.md", []string{"deploy"})
	f.seedPage(t, "b.md", []string{"ops"})
	admin := f.adminCookies(t)

	// Set parent="ops" on "deploy".
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/meta",
		map[string]string{"tag": "deploy", "color": "#4caf50", "description": "Deploy ops", "parent": "ops"}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("set parent: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Parent string `json:"parent"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Parent != "ops" {
		t.Errorf("response parent = %q, want ops", resp.Parent)
	}

	// GET /admin/tags must show the parent.
	rec = doMutate(t, f.handler, http.MethodGet, "/api/v1/admin/tags", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("list tags: %d %s", rec.Code, rec.Body.String())
	}
	var entries []tagListEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("unmarshal tags list: %v", err)
	}
	for _, e := range entries {
		if e.Tag == "deploy" {
			if e.Parent != "ops" {
				t.Errorf("tag list parent = %q, want ops", e.Parent)
			}
		}
	}
}

// TestTagHierarchyCycle: build ops→infra, then try to set parent="ops" on
// "infra" → 400 (cycle).
func TestTagHierarchyCycle(t *testing.T) {
	f := newTagAdminServer(t)
	f.seedPage(t, "a.md", []string{"ops"})
	f.seedPage(t, "b.md", []string{"infra"})
	admin := f.adminCookies(t)

	// Set infra's parent to ops.
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/meta",
		map[string]string{"tag": "infra", "color": "", "description": "", "parent": "ops"}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("set infra parent: %d %s", rec.Code, rec.Body.String())
	}

	// Try to set ops's parent to infra → cycle (infra→ops→infra).
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/meta",
		map[string]string{"tag": "ops", "color": "", "description": "", "parent": "infra"}, admin)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("cycle set parent: got %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestTagHierarchySelfParent: set parent="deploy" on "deploy" → 400.
func TestTagHierarchySelfParent(t *testing.T) {
	f := newTagAdminServer(t)
	f.seedPage(t, "a.md", []string{"deploy"})
	admin := f.adminCookies(t)

	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/meta",
		map[string]string{"tag": "deploy", "color": "", "description": "", "parent": "deploy"}, admin)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("self-parent: got %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestTagHierarchyRootTag: set parent="" on "deploy" → valid (root tag).
func TestTagHierarchyRootTag(t *testing.T) {
	f := newTagAdminServer(t)
	f.seedPage(t, "a.md", []string{"deploy"})
	admin := f.adminCookies(t)

	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/meta",
		map[string]string{"tag": "deploy", "color": "#4caf50", "description": "Deploy ops", "parent": ""}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("set root tag: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Parent string `json:"parent"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Parent != "" {
		t.Errorf("root tag parent = %q, want empty", resp.Parent)
	}
}