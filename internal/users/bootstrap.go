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
// length using the unambiguous alphabet. It uses rejection sampling over
// crypto/rand so the mapping from random bytes to alphabet indices is uniform —
// plain `byte % len(alphabet)` would skew the distribution (modulo bias) since
// 256 is not a multiple of the 56-character alphabet, lowering effective
// entropy. Bytes at or above the largest multiple of len(alphabet) <= 256 are
// rejected and re-drawn.
func generatePassword(length int) (string, error) {
	n := len(passwordAlphabet)
	// threshold is the largest multiple of n that fits in a byte (0..255).
	// Bytes >= threshold are rejected to keep buf[0]%n uniform.
	threshold := byte(256 - (256 % n))
	out := make([]byte, length)
	buf := make([]byte, 1)
	for i := 0; i < length; {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		if buf[0] >= threshold {
			continue // reject to remove modulo bias
		}
		out[i] = passwordAlphabet[int(buf[0])%n]
		i++
	}
	return string(out), nil
}
