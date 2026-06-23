package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/postfix/okworkspace/internal/auth"
)

// stubUser implements auth.SessionUser for the RBAC tests.
type stubUser struct {
	id   int64
	role string
}

func (s stubUser) UserID() int64  { return s.id }
func (s stubUser) UserRole() string { return s.role }

// withUser returns a request whose context carries the given session user,
// simulating what the session middleware attaches.
func withUser(r *http.Request, u auth.SessionUser) *http.Request {
	return r.WithContext(auth.WithCurrentUser(r.Context(), u))
}

func nextOK() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func TestRequireRoleAdminAllowsAdmin(t *testing.T) {
	h := auth.RequireRole(auth.RoleAdmin)(nextOK())
	req := withUser(httptest.NewRequest(http.MethodGet, "/admin", nil), stubUser{id: 1, role: auth.RoleAdmin})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin should pass RequireRole(admin), got %d", rec.Code)
	}
}

func TestRequireRoleAdminDeniesEditorAndReader(t *testing.T) {
	for _, role := range []string{auth.RoleEditor, auth.RoleReader} {
		h := auth.RequireRole(auth.RoleAdmin)(nextOK())
		req := withUser(httptest.NewRequest(http.MethodGet, "/admin", nil), stubUser{id: 2, role: role})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("role %q should get 403, got %d", role, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "You don't have permission to do that.") {
			t.Errorf("role %q 403 body missing copy: %s", role, rec.Body.String())
		}
	}
}

func TestRequireRoleNoSessionUnauthorized(t *testing.T) {
	h := auth.RequireRole(auth.RoleAdmin)(nextOK())
	req := httptest.NewRequest(http.MethodGet, "/admin", nil) // no user in context
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no session should get 401, got %d", rec.Code)
	}
}

func TestCurrentUserRoundTrip(t *testing.T) {
	ctx := auth.WithCurrentUser(context.Background(), stubUser{id: 7, role: auth.RoleEditor})
	u, ok := auth.CurrentUser(ctx)
	if !ok {
		t.Fatal("CurrentUser should find the attached user")
	}
	if u.UserID() != 7 || u.UserRole() != auth.RoleEditor {
		t.Errorf("CurrentUser = %+v, want id=7 role=editor", u)
	}
	if _, ok := auth.CurrentUser(context.Background()); ok {
		t.Error("CurrentUser on empty context should report not-ok")
	}
}
