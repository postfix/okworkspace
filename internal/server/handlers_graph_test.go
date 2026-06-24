package server_test

import (
	"context"
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
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/server"
	"github.com/postfix/okworkspace/internal/store"
	"github.com/postfix/okworkspace/internal/users"
)

// graphFixture wires a full server with a real graph.Store + a real jobs worker
// (KindGraph handler registered) over a real content repo so POST
// /admin/graph/reindex is exercised end to end through the actual enqueue path.
type graphFixture struct {
	handler http.Handler
	repoo   *users.Repository
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
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return &graphFixture{handler: h, repoo: userRepo}
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
