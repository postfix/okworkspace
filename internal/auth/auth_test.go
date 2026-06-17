package auth_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/store"
	"github.com/postfix/okworkspace/internal/users"
)

func setup(t *testing.T) (*users.Repository, string) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	repo := users.NewRepository(st.DB())
	var cfg config.Config
	cfg.Admin.Username = "admin"
	_, pw, _, err := users.BootstrapAdmin(context.Background(), repo, cfg)
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	return repo, pw
}

func TestAuthenticateSuccess(t *testing.T) {
	repo, pw := setup(t)
	u, err := auth.Authenticate(context.Background(), repo, "admin", pw)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if u == nil || u.Username != "admin" {
		t.Fatalf("expected admin user, got %+v", u)
	}
}

func TestAuthenticateWrongPassword(t *testing.T) {
	repo, _ := setup(t)
	_, err := auth.Authenticate(context.Background(), repo, "admin", "definitely-wrong")
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthenticateUnknownUserSameError(t *testing.T) {
	repo, _ := setup(t)
	_, err := auth.Authenticate(context.Background(), repo, "nobody", "whatever")
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials (no enumeration), got %v", err)
	}
}
