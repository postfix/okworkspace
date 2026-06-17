// Package auth provides password hashing (Argon2id), server-side session
// management (SCS), and credential verification for OKF Workspace. Argon2id is
// the default and only password hasher (SEC-03); bcrypt is forbidden as the
// default per CLAUDE.md.
package auth

import "github.com/alexedwards/argon2id"

// HashPassword hashes a plaintext password with Argon2id, returning a
// PHC-format string ("$argon2id$...") with embedded parameters. The plaintext
// is never stored or logged.
func HashPassword(plain string) (string, error) {
	return argon2id.CreateHash(plain, argon2id.DefaultParams)
}

// VerifyPassword reports whether plain matches the given PHC-format Argon2id
// hash. The comparison is constant-time (argon2id.ComparePasswordAndHash).
func VerifyPassword(hash, plain string) (bool, error) {
	return argon2id.ComparePasswordAndHash(plain, hash)
}
