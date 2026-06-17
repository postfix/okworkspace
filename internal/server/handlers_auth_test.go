package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/server"
	"github.com/postfix/okworkspace/internal/store"
	"github.com/postfix/okworkspace/internal/users"
)

func newServer(t *testing.T) (http.Handler, string) {
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

	h, err := server.New(server.Deps{Store: st, Config: cfg, UserRepo: repo})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return h, pw
}

// csrfClient performs a GET /api/v1/csrf to obtain a token + the csrf cookie,
// returning a cookie jar (as a slice) and the token.
func fetchCSRF(t *testing.T, h http.Handler) (token string, cookies []*http.Cookie) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/csrf", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /csrf status = %d", rec.Code)
	}
	var body struct {
		Token string `json:"csrf_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode csrf: %v", err)
	}
	return body.Token, rec.Result().Cookies()
}

func TestLoginWithoutCSRFRejected(t *testing.T) {
	h, pw := newServer(t)
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": pw})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Errorf("login without CSRF token should be rejected, got 200")
	}
}

func TestLoginSuccessSetsSecureCookie(t *testing.T) {
	h, pw := newServer(t)
	token, cookies := fetchCSRF(t, h)

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": pw})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", token)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var sawSession bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == config.DefaultSessionCookieName {
			sawSession = true
			if !c.HttpOnly {
				t.Error("session cookie missing HttpOnly")
			}
			if c.SameSite != http.SameSiteLaxMode {
				t.Errorf("session cookie SameSite = %v, want Lax", c.SameSite)
			}
		}
	}
	if !sawSession {
		t.Error("no okf_session Set-Cookie on successful login")
	}
}

func TestLoginInvalidCredentialsGenericError(t *testing.T) {
	h, _ := newServer(t)
	token, cookies := fetchCSRF(t, h)

	post := func(username, password string) (int, string) {
		body, _ := json.Marshal(map[string]string{"username": username, "password": password})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", token)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		b, _ := io.ReadAll(rec.Body)
		return rec.Code, string(b)
	}

	codeWrong, bodyWrong := post("admin", "wrong-password")
	codeUnknown, bodyUnknown := post("ghost", "wrong-password")

	if codeWrong != http.StatusUnauthorized || codeUnknown != http.StatusUnauthorized {
		t.Errorf("expected 401 for both, got wrong=%d unknown=%d", codeWrong, codeUnknown)
	}
	if !strings.Contains(bodyWrong, "Invalid username or password.") {
		t.Errorf("wrong-password body missing generic message: %s", bodyWrong)
	}
	if bodyWrong != bodyUnknown {
		t.Errorf("response differs between wrong-password and unknown-user (enumeration):\n  wrong=%q\n  unknown=%q", bodyWrong, bodyUnknown)
	}
}

func TestMeAfterLogin(t *testing.T) {
	h, pw := newServer(t)
	token, cookies := fetchCSRF(t, h)

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": pw})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", token)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}
	sessionCookies := rec.Result().Cookies()

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	for _, c := range sessionCookies {
		meReq.AddCookie(c)
	}
	meRec := httptest.NewRecorder()
	h.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("/me status = %d", meRec.Code)
	}
	var me struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
	}
	if err := json.Unmarshal(meRec.Body.Bytes(), &me); err != nil {
		t.Fatalf("decode /me: %v", err)
	}
	if me.Username != "admin" || me.Role != "admin" || me.DisplayName == "" {
		t.Errorf("/me = %+v, want admin with display name and role", me)
	}
}

func TestMeWithoutSessionUnauthorized(t *testing.T) {
	h, _ := newServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("/me without session = %d, want 401", rec.Code)
	}
}
