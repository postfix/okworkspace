package users

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/postfix/okworkspace/internal/auth"
)

// Role constants for the management surface. RoleAdmin is also declared in
// bootstrap.go (kept there for the bootstrap path); RoleEditor and RoleReader
// complete the fixed SPEC §6.5 set (D-07).
const (
	RoleEditor = "editor"
	RoleReader = "reader"
)

// minPasswordLen is the minimum length for a user-chosen password (UI-SPEC
// copy: "Choose a longer password — at least 12 characters.").
const minPasswordLen = 12

// managedPasswordLen is the length of a generated one-time password for created
// or reset accounts (same strong-random scheme as the bootstrap admin).
const managedPasswordLen = 28

// ErrInvalidRole is returned when a role outside the fixed set is supplied.
var ErrInvalidRole = errors.New("invalid role")

// ErrWeakPassword is returned when a new password is shorter than minPasswordLen.
var ErrWeakPassword = errors.New("password too short")

// ErrWrongPassword is returned by ChangeOwnPassword when the current password
// does not verify.
var ErrWrongPassword = errors.New("current password is incorrect")

// ErrEmptyDisplayName is returned when an update would blank the display name.
var ErrEmptyDisplayName = errors.New("display name must not be empty")

// NewUser carries the admin-supplied fields for creating a user. No password is
// accepted — Create always generates a one-time password (D-05).
type NewUser struct {
	Username    string
	DisplayName string
	Role        string
}

// validRole reports whether role is one of the fixed admin/editor/reader set.
func validRole(role string) bool {
	switch role {
	case RoleAdmin, RoleEditor, RoleReader:
		return true
	default:
		return false
	}
}

// Create inserts a new user with the given role, generating a strong one-time
// password (Argon2id-hashed, never stored in plaintext) and setting
// must_change_password=1 so the user is forced to set their own password on
// first sign-in (D-02/D-05). It returns the created user and the plaintext
// one-time password exactly once.
func Create(ctx context.Context, repo *Repository, nu NewUser) (*User, string, error) {
	username := strings.TrimSpace(nu.Username)
	displayName := strings.TrimSpace(nu.DisplayName)
	if username == "" {
		return nil, "", fmt.Errorf("create user: username must not be empty")
	}
	if displayName == "" {
		return nil, "", ErrEmptyDisplayName
	}
	if !validRole(nu.Role) {
		return nil, "", fmt.Errorf("create user %q: %w: %q", username, ErrInvalidRole, nu.Role)
	}

	otp, err := generatePassword(managedPasswordLen)
	if err != nil {
		return nil, "", fmt.Errorf("generate one-time password: %w", err)
	}
	hash, err := auth.HashPassword(otp)
	if err != nil {
		return nil, "", fmt.Errorf("hash one-time password: %w", err)
	}

	u := User{
		Username:           username,
		DisplayName:        displayName,
		Role:               nu.Role,
		PasswordHash:       hash,
		MustChangePassword: true,
		Active:             true,
	}
	id, err := repo.Create(ctx, u)
	if err != nil {
		return nil, "", err
	}
	u.ID = id
	return &u, otp, nil
}

// List returns all users ordered by display name. Password hashes are present
// on the records but callers (handlers) must never serialize them.
func List(ctx context.Context, repo *Repository) ([]User, error) {
	return repo.List(ctx)
}

// SetRole changes a target user's role. It rejects roles outside the fixed set.
func SetRole(ctx context.Context, repo *Repository, id int64, role string) error {
	if !validRole(role) {
		return fmt.Errorf("set role: %w: %q", ErrInvalidRole, role)
	}
	return repo.UpdateRole(ctx, id, role)
}

// ResetPassword generates a new one-time password for the target user, hashes
// it, sets must_change_password=1, and returns the plaintext once so the caller
// (admin API or CLI) can show it to the operator. Never a fixed default value
// (T-00.03-05).
func ResetPassword(ctx context.Context, repo *Repository, id int64) (string, error) {
	if _, err := repo.GetByID(ctx, id); err != nil {
		return "", err
	}
	otp, err := generatePassword(managedPasswordLen)
	if err != nil {
		return "", fmt.Errorf("generate one-time password: %w", err)
	}
	hash, err := auth.HashPassword(otp)
	if err != nil {
		return "", fmt.Errorf("hash one-time password: %w", err)
	}
	if err := repo.UpdatePassword(ctx, id, hash, true); err != nil {
		return "", err
	}
	return otp, nil
}

// Deactivate sets active=0 for the target user. A deactivated user can no
// longer authenticate (auth.Authenticate rejects inactive accounts).
func Deactivate(ctx context.Context, repo *Repository, id int64) error {
	if _, err := repo.GetByID(ctx, id); err != nil {
		return err
	}
	return repo.SetActive(ctx, id, false)
}

// UpdateOwnProfile changes the caller's display name. It accepts NO role
// parameter — a user can never change their own role (D-06, T-00.03-02).
func UpdateOwnProfile(ctx context.Context, repo *Repository, id int64, displayName string) error {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return ErrEmptyDisplayName
	}
	return repo.UpdateDisplayName(ctx, id, displayName)
}

// ChangeOwnPassword verifies the caller's current password, enforces the
// minimum length on the new password, re-hashes it, and clears
// must_change_password. It accepts NO role parameter (D-06).
func ChangeOwnPassword(ctx context.Context, repo *Repository, id int64, oldPassword, newPassword string) error {
	if len(newPassword) < minPasswordLen {
		return ErrWeakPassword
	}
	u, err := repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	ok, err := auth.VerifyPassword(u.PasswordHash, oldPassword)
	if err != nil {
		return fmt.Errorf("verify current password: %w", err)
	}
	if !ok {
		return ErrWrongPassword
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}
	return repo.UpdatePassword(ctx, id, hash, false)
}
