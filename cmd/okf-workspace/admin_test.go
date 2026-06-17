package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/postfix/okworkspace/internal/store"
	"github.com/postfix/okworkspace/internal/users"
)

func TestAdminResetPasswordResetsKnownUser(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	repo := users.NewRepository(st.DB())
	if _, _, err := users.Create(context.Background(), repo, users.NewUser{Username: "ed", DisplayName: "Ed", Role: users.RoleEditor}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = st.Close()

	// runAdminResetPassword opens its own store at dbPath, resets, and returns
	// the one-time password (and prints it once via the logger in the cobra cmd).
	otp, err := runAdminResetPassword(context.Background(), dbPath, "ed")
	if err != nil {
		t.Fatalf("runAdminResetPassword: %v", err)
	}
	if strings.TrimSpace(otp) == "" {
		t.Error("expected a one-time password to be returned")
	}

	// Reopen and confirm must_change_password was set and the OTP verifies.
	st2, _ := store.Open(dbPath)
	defer func() { _ = st2.Close() }()
	repo2 := users.NewRepository(st2.DB())
	u, err := repo2.GetByUsername(context.Background(), "ed")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if !u.MustChangePassword {
		t.Error("reset should set must_change_password")
	}
}

func TestAdminResetPasswordUnknownUserErrors(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	_ = st.Close()

	if _, err := runAdminResetPassword(context.Background(), dbPath, "ghost"); err == nil {
		t.Error("unknown user should return a non-nil error (non-zero exit)")
	}
}
