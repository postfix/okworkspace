package users_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/store"
	"github.com/postfix/okworkspace/internal/users"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func testConfig() config.Config {
	var cfg config.Config
	cfg.Admin.Username = "admin"
	return cfg
}

func TestBootstrapAdminCreatesOneAdmin(t *testing.T) {
	st := newStore(t)
	repo := users.NewRepository(st.DB())
	ctx := context.Background()

	username, pw, created, err := users.BootstrapAdmin(ctx, repo, testConfig())
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	if !created {
		t.Fatal("expected created=true on empty DB")
	}
	if username != "admin" {
		t.Errorf("username = %q, want admin", username)
	}
	if len(pw) < 24 {
		t.Errorf("generated password too short: %d chars", len(pw))
	}

	// Exactly one user row.
	var count int
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("user count = %d, want 1", count)
	}

	// must_change_password flag persisted.
	u, err := repo.GetByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if !u.MustChangePassword {
		t.Error("expected must_change_password=true on bootstrap admin")
	}
	if u.Role != "admin" {
		t.Errorf("role = %q, want admin", u.Role)
	}

	// Stored hash verifies against the returned plaintext.
	ok, err := auth.VerifyPassword(u.PasswordHash, pw)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Error("stored hash does not verify against generated password")
	}
}

func TestBootstrapAdminNoopWhenUsersExist(t *testing.T) {
	st := newStore(t)
	repo := users.NewRepository(st.DB())
	ctx := context.Background()

	if _, _, created, err := users.BootstrapAdmin(ctx, repo, testConfig()); err != nil || !created {
		t.Fatalf("first bootstrap: created=%v err=%v", created, err)
	}

	_, pw, created, err := users.BootstrapAdmin(ctx, repo, testConfig())
	if err != nil {
		t.Fatalf("second BootstrapAdmin: %v", err)
	}
	if created {
		t.Error("expected created=false when a user already exists")
	}
	if pw != "" {
		t.Error("expected no password returned on no-op bootstrap")
	}

	var count int
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("user count = %d, want 1 (no new row)", count)
	}
}
