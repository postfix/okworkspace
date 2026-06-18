package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/server"
	"github.com/postfix/okworkspace/internal/store"
	"github.com/postfix/okworkspace/internal/users"
)

// newServerWith builds a server and bootstraps the admin, returning the handler,
// the admin one-time password, and the user repository for seeding extra users.
func newServerWith(t *testing.T) (http.Handler, string, *users.Repository) {
	t.Helper()
	h, pw, repo, _ := newServerWithStore(t)
	return h, pw, repo
}

// newServerWithStore is like newServerWith but also returns the store so a test
// can assert audit_log rows were written by the handlers.
func newServerWithStore(t *testing.T) (http.Handler, string, *users.Repository, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	var cfg config.Config
	cfg.Auth.SessionCookieName = config.DefaultSessionCookieName
	cfg.Auth.SessionTTLHours = config.DefaultSessionTTLHours
	cfg.Admin.Username = "admin"
	repo := users.NewRepository(st.DB())
	_, pw, _, err := users.BootstrapAdmin(context.Background(), repo, cfg)
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	h, err := server.New(server.Deps{
		Store:    st,
		Config:   cfg,
		UserRepo: repo,
		Audit:    audit.New(st.DB(), nil),
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return h, pw, repo, st
}

// countAudit returns the number of audit_log rows with the given action.
func countAudit(t *testing.T, st *store.Store, action string) int {
	t.Helper()
	var n int
	if err := st.DB().QueryRow(`SELECT COUNT(1) FROM audit_log WHERE action=?`, action).Scan(&n); err != nil {
		t.Fatalf("count audit %q: %v", action, err)
	}
	return n
}

// loginAs signs in and returns the session cookies + a fresh CSRF token/cookies
// valid for the authenticated session.
func loginAs(t *testing.T, h http.Handler, username, password string) []*http.Cookie {
	t.Helper()
	token, csrfCookies := fetchCSRF(t, h)
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", token)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	for _, c := range csrfCookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login as %q failed: %d %s", username, rec.Code, rec.Body.String())
	}
	// Merge csrf cookies with the session cookies returned by login.
	cookies := append([]*http.Cookie{}, csrfCookies...)
	cookies = append(cookies, rec.Result().Cookies()...)
	return cookies
}

func doMutate(t *testing.T, h http.Handler, method, path string, body any, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	token, csrfCookies := fetchCSRF(t, h)
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", token)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	for _, c := range csrfCookies {
		req.AddCookie(c)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAdminListUsersAsAdmin(t *testing.T) {
	h, pw, _ := newServerWith(t)
	cookies := loginAs(t, h, "admin", pw)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin list users = %d, body=%s", rec.Code, rec.Body.String())
	}
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v (body=%s)", err, rec.Body.String())
	}
	if len(list) < 1 {
		t.Error("expected at least the admin user in the list")
	}
}

// TestEditorForbiddenFromAdminAPI is the server-side RBAC denial check
// (the curl-403 equivalent recorded in the SUMMARY).
func TestEditorForbiddenFromAdminAPI(t *testing.T) {
	h, adminPW, repo := newServerWith(t)
	// Create an editor and set a known password (clear must_change so login works).
	_, otp, err := users.Create(context.Background(), repo, users.NewUser{Username: "ed", DisplayName: "Ed", Role: users.RoleEditor})
	if err != nil {
		t.Fatalf("Create editor: %v", err)
	}
	ed, _ := repo.GetByUsername(context.Background(), "ed")
	if err := users.ChangeOwnPassword(context.Background(), repo, ed.ID, otp, "editor-long-password"); err != nil {
		t.Fatalf("set editor password: %v", err)
	}

	_ = adminPW
	cookies := loginAs(t, h, "ed", "editor-long-password")

	// GET list as editor -> 403.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("editor GET /admin/users = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "You don't have permission to do that.") {
		t.Errorf("403 body missing copy: %s", rec.Body.String())
	}

	// POST create as editor -> 403.
	rec = doMutate(t, h, http.MethodPost, "/api/v1/admin/users",
		map[string]string{"username": "x", "display_name": "X", "role": "reader"}, cookies)
	if rec.Code != http.StatusForbidden {
		t.Errorf("editor POST /admin/users = %d, want 403", rec.Code)
	}
}

func TestAdminCreateAndDeactivateFlow(t *testing.T) {
	h, pw, _ := newServerWith(t)
	cookies := loginAs(t, h, "admin", pw)

	// Create a user.
	rec := doMutate(t, h, http.MethodPost, "/api/v1/admin/users",
		map[string]string{"username": "newbie", "display_name": "New Bie", "role": "reader"}, cookies)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create user = %d, body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID              int64  `json:"id"`
		OneTimePassword string `json:"one_time_password"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v (%s)", err, rec.Body.String())
	}
	if created.OneTimePassword == "" {
		t.Error("create response should include a one-time password")
	}
	if created.ID == 0 {
		t.Fatal("create response missing id")
	}

	// Reset password.
	rec = doMutate(t, h, http.MethodPost, "/api/v1/admin/users/"+itoa(created.ID)+"/reset-password", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Errorf("reset-password = %d, body=%s", rec.Code, rec.Body.String())
	}

	// Deactivate.
	rec = doMutate(t, h, http.MethodPost, "/api/v1/admin/users/"+itoa(created.ID)+"/deactivate", nil, cookies)
	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Errorf("deactivate = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestProfilePasswordRejectsRoleParam(t *testing.T) {
	h, pw, repo := newServerWith(t)
	// The admin must change password on first login; create a normal editor instead.
	_, otp, _ := users.Create(context.Background(), repo, users.NewUser{Username: "ed2", DisplayName: "Ed2", Role: users.RoleEditor})
	ed, _ := repo.GetByUsername(context.Background(), "ed2")
	if err := users.ChangeOwnPassword(context.Background(), repo, ed.ID, otp, "editor-long-password"); err != nil {
		t.Fatal(err)
	}
	_ = pw
	cookies := loginAs(t, h, "ed2", "editor-long-password")

	// Attempt to sneak a role into a profile update — it must be ignored.
	rec := doMutate(t, h, http.MethodPut, "/api/v1/profile",
		map[string]string{"display_name": "Ed Two", "role": "admin"}, cookies)
	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("profile update = %d, body=%s", rec.Code, rec.Body.String())
	}
	got, _ := repo.GetByID(context.Background(), ed.ID)
	if got.Role != users.RoleEditor {
		t.Errorf("profile update escalated role to %q", got.Role)
	}
	if got.DisplayName != "Ed Two" {
		t.Errorf("display name not updated: %q", got.DisplayName)
	}
}

// TestAuditRowsForLoginLogoutCreate asserts the audit wiring: a login, a logout,
// and an admin user-create each produce the corresponding audit_log row with the
// expected actor (and target for the create).
func TestAuditRowsForLoginLogoutCreate(t *testing.T) {
	h, _, repo, st := newServerWithStore(t)

	// Give the admin a known password and clear must_change so login is clean.
	admin, _ := repo.GetByUsername(context.Background(), "admin")
	if err := users.ChangeOwnPassword(context.Background(), repo, admin.ID, mustChangeBootstrapPW(t, repo), "admin-long-password"); err != nil {
		// The admin one-time password is unknown here; instead reset via repo.
		t.Logf("change admin password fallback: %v", err)
	}

	// Use a freshly created editor whose OTP we control for a clean login path.
	_, otp, err := users.Create(context.Background(), repo, users.NewUser{Username: "auditee", DisplayName: "Audit Ee", Role: users.RoleEditor})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	ed, _ := repo.GetByUsername(context.Background(), "auditee")
	if err := users.ChangeOwnPassword(context.Background(), repo, ed.ID, otp, "auditee-long-password"); err != nil {
		t.Fatalf("set editor password: %v", err)
	}

	// Login -> expect a login row.
	cookies := loginAs(t, h, "auditee", "auditee-long-password")
	if got := countAudit(t, st, audit.ActionLogin); got < 1 {
		t.Errorf("login audit rows = %d, want >=1", got)
	}

	// Logout -> expect a logout row.
	rec := doMutate(t, h, http.MethodPost, "/api/v1/auth/logout", nil, cookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d, body=%s", rec.Code, rec.Body.String())
	}
	if got := countAudit(t, st, audit.ActionLogout); got < 1 {
		t.Errorf("logout audit rows = %d, want >=1", got)
	}

	// Admin user-create -> expect a user_create row naming actor+target.
	adminCookies := loginAsAdmin(t, h, repo)
	rec = doMutate(t, h, http.MethodPost, "/api/v1/admin/users",
		map[string]string{"username": "created", "display_name": "Created", "role": "reader"}, adminCookies)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create = %d, body=%s", rec.Code, rec.Body.String())
	}
	if got := countAudit(t, st, audit.ActionUserCreate); got < 1 {
		t.Errorf("user_create audit rows = %d, want >=1", got)
	}
	var actor, target string
	if err := st.DB().QueryRow(
		`SELECT actor, target FROM audit_log WHERE action=? ORDER BY id DESC LIMIT 1`, audit.ActionUserCreate,
	).Scan(&actor, &target); err != nil {
		t.Fatalf("scan user_create row: %v", err)
	}
	if actor != "admin" {
		t.Errorf("user_create actor = %q, want admin", actor)
	}
	if target != "created" {
		t.Errorf("user_create target = %q, want created", target)
	}
}

// loginAsAdmin resets the admin to a known password (via the repo, the same path
// the admin CLI uses) and logs in, returning the session cookies.
func loginAsAdmin(t *testing.T, h http.Handler, repo *users.Repository) []*http.Cookie {
	t.Helper()
	admin, err := repo.GetByUsername(context.Background(), "admin")
	if err != nil {
		t.Fatalf("get admin: %v", err)
	}
	otp, err := users.ResetPassword(context.Background(), repo, admin.ID)
	if err != nil {
		t.Fatalf("reset admin: %v", err)
	}
	if err := users.ChangeOwnPassword(context.Background(), repo, admin.ID, otp, "admin-long-password"); err != nil {
		t.Fatalf("set admin password: %v", err)
	}
	return loginAs(t, h, "admin", "admin-long-password")
}

// mustChangeBootstrapPW is a placeholder returning an obviously-wrong password;
// the admin OTP is unknown in this helper, so the change is expected to fail and
// the test instead uses loginAsAdmin (repo-driven reset). Kept to document intent.
func mustChangeBootstrapPW(_ *testing.T, _ *users.Repository) string {
	return "unknown-bootstrap-password"
}

func itoa(i int64) string {
	return strconv64(i)
}

func strconv64(i int64) string {
	// tiny helper to avoid importing strconv in two places
	return strings.TrimSpace(formatInt(i))
}

func formatInt(i int64) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
