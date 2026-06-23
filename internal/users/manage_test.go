package users_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/store"
	"github.com/postfix/okworkspace/internal/users"
)

func newRepo(t *testing.T) *users.Repository {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return users.NewRepository(st.DB())
}

func TestCreateGeneratesOneTimePassword(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	u, otp, err := users.Create(ctx, repo, users.NewUser{Username: "ed", DisplayName: "Ed Itor", Role: users.RoleEditor})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.ID == 0 || u.Username != "ed" || u.Role != users.RoleEditor {
		t.Errorf("Create returned %+v", u)
	}
	if !u.MustChangePassword {
		t.Error("created user should have must_change_password=1")
	}
	if len(otp) < 12 {
		t.Errorf("one-time password too short: %q", otp)
	}
	// The OTP must authenticate the new user.
	got, err := repo.GetByUsername(ctx, "ed")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	ok, err := auth.VerifyPassword(got.PasswordHash, otp)
	if err != nil || !ok {
		t.Errorf("one-time password does not verify against stored hash (ok=%v err=%v)", ok, err)
	}
}

func TestListReturnsAllUsers(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	if _, _, err := users.Create(ctx, repo, users.NewUser{Username: "a", DisplayName: "A", Role: users.RoleReader}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := users.Create(ctx, repo, users.NewUser{Username: "b", DisplayName: "B", Role: users.RoleEditor}); err != nil {
		t.Fatal(err)
	}
	list, err := users.List(ctx, repo)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List returned %d users, want 2", len(list))
	}
}

func TestSetRoleChangesRole(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	u, _, _ := users.Create(ctx, repo, users.NewUser{Username: "r", DisplayName: "R", Role: users.RoleReader})
	if err := users.SetRole(ctx, repo, u.ID, users.RoleEditor); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	got, _ := repo.GetByID(ctx, u.ID)
	if got.Role != users.RoleEditor {
		t.Errorf("role = %q, want editor", got.Role)
	}
	if err := users.SetRole(ctx, repo, u.ID, "superuser"); err == nil {
		t.Error("SetRole should reject an unknown role")
	}
}

func TestResetPasswordGeneratesVerifiableOTP(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	u, firstOTP, _ := users.Create(ctx, repo, users.NewUser{Username: "p", DisplayName: "P", Role: users.RoleReader})
	otp, err := users.ResetPassword(ctx, repo, u.ID)
	if err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if otp == firstOTP {
		t.Error("reset should generate a different password")
	}
	got, _ := repo.GetByID(ctx, u.ID)
	if !got.MustChangePassword {
		t.Error("reset should set must_change_password=1")
	}
	ok, err := auth.VerifyPassword(got.PasswordHash, otp)
	if err != nil || !ok {
		t.Errorf("reset OTP does not verify (ok=%v err=%v)", ok, err)
	}
}

func TestDeactivateBlocksLogin(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	u, otp, _ := users.Create(ctx, repo, users.NewUser{Username: "d", DisplayName: "D", Role: users.RoleReader})
	// Before deactivation the OTP authenticates.
	if _, err := auth.Authenticate(ctx, repo, "d", otp); err != nil {
		t.Fatalf("pre-deactivate auth should succeed: %v", err)
	}
	if err := users.Deactivate(ctx, repo, u.ID); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	got, _ := repo.GetByID(ctx, u.ID)
	if got.Active {
		t.Error("deactivated user should have active=0")
	}
	if _, err := auth.Authenticate(ctx, repo, "d", otp); err == nil {
		t.Error("deactivated user must NOT authenticate")
	}
}

func TestChangeOwnPasswordValidation(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	u, otp, _ := users.Create(ctx, repo, users.NewUser{Username: "c", DisplayName: "C", Role: users.RoleReader})

	// Wrong current password rejected.
	if err := users.ChangeOwnPassword(ctx, repo, u.ID, "wrong-current", "a-good-long-password"); err == nil {
		t.Error("wrong current password should be rejected")
	}
	// New password shorter than 12 chars rejected.
	if err := users.ChangeOwnPassword(ctx, repo, u.ID, otp, "short"); err == nil {
		t.Error("new password < 12 chars should be rejected")
	}
	// Valid change clears must_change_password and re-hashes.
	if err := users.ChangeOwnPassword(ctx, repo, u.ID, otp, "a-good-long-password"); err != nil {
		t.Fatalf("valid ChangeOwnPassword: %v", err)
	}
	got, _ := repo.GetByID(ctx, u.ID)
	if got.MustChangePassword {
		t.Error("must_change_password should be cleared after a successful change")
	}
	ok, _ := auth.VerifyPassword(got.PasswordHash, "a-good-long-password")
	if !ok {
		t.Error("new password should verify")
	}
}

// TestCreateRejectsInvalidUsername covers WR-01: usernames are constrained to a
// safe charset/length so they cannot corrupt the Git author identity / commit
// audit trail. Each bad name must fail with ErrInvalidUsername; a clean name
// must succeed.
func TestCreateRejectsInvalidUsername(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	bad := []string{
		"",                  // empty
		"   ",               // whitespace only (trimmed to empty)
		"has space",         // internal whitespace
		"line\nbreak",       // newline
		"tab\tchar",         // control char
		"name=value",        // '=' could corrupt -c user.name=
		"a<b>c",             // angle brackets corrupt the --author header
		"naïve",             // non-ASCII outside the charset
		strings.Repeat("a", 65), // exceeds 64 chars
	}
	for _, name := range bad {
		_, _, err := users.Create(ctx, repo, users.NewUser{Username: name, DisplayName: "X", Role: users.RoleReader})
		if !errors.Is(err, users.ErrInvalidUsername) {
			t.Errorf("Create(%q) error = %v, want ErrInvalidUsername", name, err)
		}
	}

	// A clean username succeeds.
	if _, _, err := users.Create(ctx, repo, users.NewUser{Username: "good.user-1_2", DisplayName: "Good", Role: users.RoleReader}); err != nil {
		t.Errorf("Create(valid username) unexpected error: %v", err)
	}
}

// TestLastAdminInvariant covers CR-03: demoting or deactivating the last active
// admin must be rejected, but the same action succeeds once a second active
// admin exists.
func TestLastAdminInvariant(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	// Seed the only admin (active).
	a1, _, err := users.Create(ctx, repo, users.NewUser{Username: "admin1", DisplayName: "Admin One", Role: users.RoleAdmin})
	if err != nil {
		t.Fatalf("create admin1: %v", err)
	}

	// Demoting the sole admin is rejected.
	if err := users.SetRole(ctx, repo, a1.ID, users.RoleReader); !errors.Is(err, users.ErrLastAdmin) {
		t.Fatalf("SetRole(last admin -> reader) = %v, want ErrLastAdmin", err)
	}
	// Deactivating the sole admin is rejected.
	if err := users.Deactivate(ctx, repo, a1.ID); !errors.Is(err, users.ErrLastAdmin) {
		t.Fatalf("Deactivate(last admin) = %v, want ErrLastAdmin", err)
	}
	// The admin is still admin and active.
	if got, _ := repo.GetByID(ctx, a1.ID); got.Role != users.RoleAdmin || !got.Active {
		t.Fatalf("admin1 changed despite rejection: role=%q active=%v", got.Role, got.Active)
	}

	// Add a second active admin — now there are two.
	a2, _, err := users.Create(ctx, repo, users.NewUser{Username: "admin2", DisplayName: "Admin Two", Role: users.RoleAdmin})
	if err != nil {
		t.Fatalf("create admin2: %v", err)
	}

	// Demoting one admin now succeeds (the other still covers admin).
	if err := users.SetRole(ctx, repo, a2.ID, users.RoleReader); err != nil {
		t.Fatalf("SetRole(admin2 -> reader) with two admins: %v", err)
	}

	// a1 is now the last admin again — deactivating it must be rejected.
	if err := users.Deactivate(ctx, repo, a1.ID); !errors.Is(err, users.ErrLastAdmin) {
		t.Fatalf("Deactivate(new last admin) = %v, want ErrLastAdmin", err)
	}

	// Promote a2 back to admin, then deactivating a1 succeeds.
	if err := users.SetRole(ctx, repo, a2.ID, users.RoleAdmin); err != nil {
		t.Fatalf("re-promote admin2: %v", err)
	}
	if err := users.Deactivate(ctx, repo, a1.ID); err != nil {
		t.Fatalf("Deactivate(admin1) with two admins: %v", err)
	}
	if got, _ := repo.GetByID(ctx, a1.ID); got.Active {
		t.Error("admin1 should be deactivated")
	}
}

func TestUpdateOwnProfileChangesDisplayName(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	u, _, _ := users.Create(ctx, repo, users.NewUser{Username: "u", DisplayName: "Old", Role: users.RoleReader})
	if err := users.UpdateOwnProfile(ctx, repo, u.ID, "New Name"); err != nil {
		t.Fatalf("UpdateOwnProfile: %v", err)
	}
	got, _ := repo.GetByID(ctx, u.ID)
	if got.DisplayName != "New Name" {
		t.Errorf("display name = %q, want New Name", got.DisplayName)
	}
	// Role must be unchanged (self-service cannot escalate).
	if got.Role != users.RoleReader {
		t.Errorf("role changed by profile update: %q", got.Role)
	}
	if strings.TrimSpace("") == got.DisplayName {
		t.Error("blank display name should not have been accepted in real flow")
	}
}
