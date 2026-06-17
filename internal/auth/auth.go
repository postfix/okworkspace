package auth

import (
	"context"
	"errors"
)

// ErrInvalidCredentials is returned for BOTH an unknown username and a wrong
// password, so the API cannot be used to enumerate accounts (T-00.01-02).
var ErrInvalidCredentials = errors.New("invalid username or password")

// AuthUser is the minimal user shape Authenticate needs. The users package's
// *users.User satisfies this via an adapter (see UserLookup), keeping auth free
// of an import dependency on users (avoids an import cycle, since users
// bootstrap depends on auth for password hashing).
type AuthUser struct {
	ID           int64
	PasswordHash string
	Active       bool
}

// UserLookup resolves a username to the fields needed for authentication. It
// returns ErrUserNotFound when no such user exists.
type UserLookup interface {
	LookupForAuth(ctx context.Context, username string) (AuthUser, error)
}

// ErrUserNotFound is returned by a UserLookup when the username is absent. It is
// mapped to ErrInvalidCredentials by Authenticate (no enumeration).
var ErrUserNotFound = errors.New("user not found")

// dummyHash is a valid Argon2id hash compared against on the unknown-user path
// so that timing does not reveal whether the username exists. Computed once at
// init from a throwaway secret.
var dummyHash string

func init() {
	// Best-effort; if hashing fails we fall back to an empty string (Verify
	// still runs in constant-ish time over the input).
	if h, err := HashPassword("okf-workspace-timing-equalizer"); err == nil {
		dummyHash = h
	}
}

// Authenticate looks up the user via the UserLookup and verifies the password
// with Argon2id. It returns ErrInvalidCredentials for an unknown user, a wrong
// password, or a deactivated account — performing a dummy hash comparison on
// the unknown-user path to equalize timing (no account enumeration). On success
// it returns the user's id.
func Authenticate(ctx context.Context, lookup UserLookup, username, plain string) (int64, error) {
	u, err := lookup.LookupForAuth(ctx, username)
	if errors.Is(err, ErrUserNotFound) {
		// Equalize timing: perform a comparison against a dummy hash.
		_, _ = VerifyPassword(dummyHash, plain)
		return 0, ErrInvalidCredentials
	}
	if err != nil {
		return 0, err
	}

	ok, err := VerifyPassword(u.PasswordHash, plain)
	if err != nil {
		return 0, err
	}
	if !ok || !u.Active {
		return 0, ErrInvalidCredentials
	}
	return u.ID, nil
}
