package users

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/config"
)

// RoleAdmin is the administrative role.
const RoleAdmin = "admin"

// generatedPasswordLen is the length of the bootstrap admin password. >= 24
// chars of base58-ish alphabet gives strong entropy; never a fixed default.
const generatedPasswordLen = 28

// passwordAlphabet excludes visually ambiguous characters (0/O, 1/l/I) so the
// one-time password can be transcribed from a log without errors.
const passwordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"

// BootstrapAdmin creates the admin user ONLY when no users exist (D-01). It
// generates a strong random password (crypto/rand), hashes it with Argon2id,
// inserts the admin row with must_change_password=1 (D-02), and returns the
// plaintext exactly once so the caller can print it. On a non-empty DB it is a
// no-op (created=false, empty password). Never ships a fixed default password.
func BootstrapAdmin(ctx context.Context, repo *Repository, cfg config.Config) (username, generatedPassword string, created bool, err error) {
	n, err := repo.Count(ctx)
	if err != nil {
		return "", "", false, err
	}
	if n > 0 {
		return "", "", false, nil
	}

	adminUsername := cfg.Admin.Username
	if adminUsername == "" {
		adminUsername = config.DefaultAdminUsername
	}

	pw, err := generatePassword(generatedPasswordLen)
	if err != nil {
		return "", "", false, fmt.Errorf("generate admin password: %w", err)
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return "", "", false, fmt.Errorf("hash admin password: %w", err)
	}

	_, err = repo.Create(ctx, User{
		Username:           adminUsername,
		DisplayName:        "Administrator",
		Role:               RoleAdmin,
		PasswordHash:       hash,
		MustChangePassword: true,
		Active:             true,
	})
	if err != nil {
		return "", "", false, err
	}
	return adminUsername, pw, true, nil
}

// generatePassword returns a cryptographically-random password of the given
// length using the unambiguous alphabet.
func generatePassword(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, length)
	for i, b := range buf {
		out[i] = passwordAlphabet[int(b)%len(passwordAlphabet)]
	}
	return string(out), nil
}
