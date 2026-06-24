package server_test

// KEY-FREE: the approve path NEVER calls the model — it re-validates the client
// tags via agent.ValidateTags (pure normalize) and routes them through the
// byte-stable batched apply. A real pages.Service + tagsweep.Store + worker + git
// (sharing ONE repo/db) exercise the full HTTP seam with no LLM.

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
	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/pages"
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/server"
	"github.com/postfix/okworkspace/internal/store"
	"github.com/postfix/okworkspace/internal/tagsweep"
	"github.com/postfix/okworkspace/internal/users"
)

// approveFixture wires a full server with a real pages.Service AND a real
// tagsweep.Store over the SAME repo + db + single worker, so the admin approve
// endpoint exercises the batched single-writer apply end to end against git.
type approveFixture struct {
	handler http.Handler
	repo    *repo.Repo
	store   *tagsweep.Store
	pages   *pages.Service
	users   *users.Repository
	root    string
}

func newApproveServer(t *testing.T) *approveFixture {
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

	w := jobs.New(st.DB(), jobs.Config{PollInterval: 5 * time.Millisecond})
	w.Register(pages.KindCommit, pages.CommitHandler(contentRepo, gs))
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	t.Cleanup(func() { w.Stop(); cancel() })

	pagesSvc := pages.NewService(contentRepo, gs, w, st.DB(), false)
	ts := tagsweep.OpenStore(st.DB())
	ts.SetRepo(contentRepo)

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
		Pages:          pagesSvc,
		TagSuggestions: ts,
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return &approveFixture{
		handler: h, repo: contentRepo, store: ts, pages: pagesSvc,
		users: userRepo, root: contentRepo.Root(),
	}
}

// seedApprovePage creates a committed page and stages a pending suggestion row
// for it (against the page's CURRENT revision), returning the path. This mirrors
// what a sweep would have produced: a live page + a pending queue row.
func (f *approveFixture) seedApprovePage(t *testing.T, title string, tags ...string) string {
	t.Helper()
	ctx := context.Background()
	p, err := f.pages.Create(ctx, "", title, "alice")
	if err != nil {
		t.Fatalf("seed Create %q: %v", title, err)
	}
	waitForApprovePath(t, f, p)
	pg, err := f.pages.Get(ctx, p)
	if err != nil {
		t.Fatalf("seed Get %q: %v", p, err)
	}
	sugg := make([]tagsweep.Suggestion, 0, len(tags))
	for _, tg := range tags {
		sugg = append(sugg, tagsweep.Suggestion{Tag: tg})
	}
	if err := f.store.StagePending(ctx, p, sugg, pg.Revision); err != nil {
		t.Fatalf("seed StagePending %q: %v", p, err)
	}
	return p
}

func waitForApprovePath(t *testing.T, f *approveFixture, path string) {
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

func approveCommitCount(t *testing.T, root string) int {
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

func (f *approveFixture) readPage(t *testing.T, path string) string {
	t.Helper()
	raw, err := f.repo.Read(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return string(raw)
}

func approveLoginEditor(t *testing.T, f *approveFixture) []*http.Cookie {
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

type approval struct {
	PagePath string   `json:"page_path"`
	Tags     []string `json:"tags"`
}

func approveBody(approvals ...approval) map[string]any {
	return map[string]any{"approvals": approvals}
}

func decodeResults(t *testing.T, rec interface{ Bytes() []byte }) map[string]string {
	t.Helper()
	var out []struct {
		PagePath string `json:"page_path"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(rec.Bytes(), &out); err != nil {
		t.Fatalf("decode results: %v (body=%s)", err, string(rec.Bytes()))
	}
	m := map[string]string{}
	for _, r := range out {
		m[r.PagePath] = r.Status
	}
	return m
}

// TestApproveTagSuggestionsAdminOnly: non-admin → 403; admin → 200.
func TestApproveTagSuggestionsAdminOnly(t *testing.T) {
	f := newApproveServer(t)
	p := f.seedApprovePage(t, "Alpha", "ops")

	editor := approveLoginEditor(t, f)
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/approve",
		approveBody(approval{PagePath: p, Tags: []string{"ops"}}), editor)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("editor approve = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}

	admin := loginAsAdmin(t, f.handler, f.users)
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/approve",
		approveBody(approval{PagePath: p, Tags: []string{"ops"}}), admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin approve = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestApproveBatchedOneCommit is the BATCHED-COMMIT GATE through the HTTP seam:
// approving 3 pages → 200 all applied, git history grew by EXACTLY ONE commit
// (delta==1, not 3), and ListPending now returns zero (rows resolved).
func TestApproveBatchedOneCommit(t *testing.T) {
	f := newApproveServer(t)
	p1 := f.seedApprovePage(t, "Alpha", "ops")
	p2 := f.seedApprovePage(t, "Bravo", "docs")
	p3 := f.seedApprovePage(t, "Charlie", "runbook")

	before := approveCommitCount(t, f.root)
	admin := loginAsAdmin(t, f.handler, f.users)
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/approve",
		approveBody(
			approval{PagePath: p1, Tags: []string{"ops"}},
			approval{PagePath: p2, Tags: []string{"docs"}},
			approval{PagePath: p3, Tags: []string{"runbook"}},
		), admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	got := decodeResults(t, rec.Body)
	for _, p := range []string{p1, p2, p3} {
		if got[p] != "applied" {
			t.Fatalf("page %q status = %q, want applied", p, got[p])
		}
	}

	// THE GATE: 3 approved pages → exactly ONE commit.
	after := approveCommitCount(t, f.root)
	if after-before != 1 {
		t.Fatalf("commit delta = %d, want exactly 1 (batched-commit invariant, Pitfall 6)", after-before)
	}

	// The applied rows are resolved → the pending queue is empty.
	pending, err := f.store.ListPending(context.Background())
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("ListPending after approve = %+v, want empty (rows resolved)", pending)
	}
}

// TestApproveStaleDoesNotSinkBatch: stage 3, mutate page 2 out-of-band, approve
// all 3 → page 2 status=stale and STAYS pending (for a re-run); pages 1+3 applied
// in ONE commit and resolved.
func TestApproveStaleDoesNotSinkBatch(t *testing.T) {
	f := newApproveServer(t)
	ctx := context.Background()
	p1 := f.seedApprovePage(t, "Alpha", "ops")
	p2 := f.seedApprovePage(t, "Bravo", "docs")
	p3 := f.seedApprovePage(t, "Charlie", "runbook")

	// Mutate page 2 out-of-band so its STAGED base_revision is now stale.
	pg2, err := f.pages.Get(ctx, p2)
	if err != nil {
		t.Fatalf("Get p2: %v", err)
	}
	if err := f.pages.Save(ctx, p2, "moved body\n", pg2.Frontmatter, pg2.Revision, "mallory"); err != nil {
		t.Fatalf("out-of-band Save p2: %v", err)
	}
	waitForApproveRevisionChanged(t, f, p2, pg2.Revision)

	before := approveCommitCount(t, f.root)
	admin := loginAsAdmin(t, f.handler, f.users)
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/approve",
		approveBody(
			approval{PagePath: p1, Tags: []string{"ops"}},
			approval{PagePath: p2, Tags: []string{"docs"}},
			approval{PagePath: p3, Tags: []string{"runbook"}},
		), admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	got := decodeResults(t, rec.Body)
	if got[p1] != "applied" || got[p3] != "applied" {
		t.Fatalf("p1=%q p3=%q, want both applied", got[p1], got[p3])
	}
	if got[p2] != "stale" {
		t.Fatalf("p2 status = %q, want stale", got[p2])
	}

	// One commit for the two applied pages (the stale page added nothing).
	if after := approveCommitCount(t, f.root); after-before != 1 {
		t.Fatalf("commit delta = %d, want exactly 1 (two applied pages, one commit)", after-before)
	}

	// The stale page STAYS pending (re-runnable); the applied pages are resolved.
	pending, err := f.store.ListPending(ctx)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 1 || pending[0].PagePath != p2 {
		t.Fatalf("ListPending = %+v, want only the stale page %q still pending", pending, p2)
	}
}

// TestApproveRevalidatesServerSide: send a tampered/over-cap/garbage tag list →
// the server normalizes/caps before write (proven by reading the page frontmatter),
// never the raw client list.
func TestApproveRevalidatesServerSide(t *testing.T) {
	f := newApproveServer(t)
	p := f.seedApprovePage(t, "Alpha", "ops")

	// A tampered list: uppercase/whitespace (normalized), a dup, and >5 entries
	// (capped to MaxSuggestedTags=5). The written tags must be the CLEANED set.
	tampered := []string{"  OPS  ", "ops", "Docs", "runbook", "alpha", "beta", "gamma", "delta"}
	admin := loginAsAdmin(t, f.handler, f.users)
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/approve",
		approveBody(approval{PagePath: p, Tags: tampered}), admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if got := decodeResults(t, rec.Body); got[p] != "applied" {
		t.Fatalf("status = %q, want applied", got[p])
	}

	raw := f.readPage(t, p)
	// The raw tampered tokens must NOT appear; the normalized "ops" must, and the
	// list is capped at 5 (so at most 5 tag list items were written).
	if strings.Contains(raw, "OPS") || strings.Contains(raw, "  ops  ") {
		t.Fatalf("raw client token written verbatim; frontmatter:\n%s", raw)
	}
	if !strings.Contains(raw, "ops") {
		t.Fatalf("normalized tag 'ops' not written; frontmatter:\n%s", raw)
	}
	// Count list-item tag lines ("- ") in the frontmatter region: cap is 5.
	fmEnd := strings.Index(raw[3:], "\n---\n")
	front := raw
	if fmEnd >= 0 {
		front = raw[:fmEnd+3]
	}
	n := strings.Count(front, "\n  - ") + strings.Count(front, "\n- ")
	if n > 5 {
		t.Fatalf("wrote %d tags, want <= 5 (cap); frontmatter:\n%s", n, front)
	}
}

// TestApproveUsesStagedBaseRevision: the handler IGNORES any client-supplied
// base_revision and uses the STAGED one (the request omits base_revision entirely
// — the page still applies against the staged revision).
func TestApproveUsesStagedBaseRevision(t *testing.T) {
	f := newApproveServer(t)
	p := f.seedApprovePage(t, "Alpha", "ops")

	// The request body carries NO base_revision at all (the type has no such field):
	// the server reads it from the staged queue row. If it had trusted a client
	// value (absent → ""), the apply would 409 (stale). It must succeed.
	admin := loginAsAdmin(t, f.handler, f.users)
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/admin/tags/approve",
		approveBody(approval{PagePath: p, Tags: []string{"ops"}}), admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if got := decodeResults(t, rec.Body); got[p] != "applied" {
		t.Fatalf("status = %q, want applied (staged base_revision used)", got[p])
	}
	if raw := f.readPage(t, p); !strings.Contains(raw, "ops") {
		t.Fatalf("tag not applied via staged base_revision; frontmatter:\n%s", raw)
	}
}

func waitForApproveRevisionChanged(t *testing.T, f *approveFixture, path, old string) {
	t.Helper()
	for i := 0; i < 600; i++ {
		cur, err := f.pages.Revision(context.Background(), path)
		if err == nil && cur != old {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("revision for %q never moved off %q", path, old)
}
