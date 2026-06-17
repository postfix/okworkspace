package auth

import (
	"context"
	"net/http"
)

// Role constants are the fixed SPEC §6.5 set (D-07). Authorization is always
// derived from the SESSION-bound user's role — never from a client-supplied
// header, body field, or query parameter (T-00.03-01).
const (
	RoleAdmin  = "admin"
	RoleEditor = "editor"
	RoleReader = "reader"
)

// rbacForbiddenMessage is the single copy returned on an RBAC denial (UI-SPEC
// copywriting contract). It says what happened without leaking which action or
// resource was attempted.
const rbacForbiddenMessage = "You don't have permission to do that."

// SessionUser is the minimal shape RequireRole needs from the authenticated
// user. The server attaches a value implementing this to the request context
// after loading the session-bound user; *users.User satisfies it via a small
// adapter in the server package, keeping auth free of a users import.
type SessionUser interface {
	UserID() int64
	UserRole() string
}

// currentUserKey is the unexported context key under which the session-bound
// user is stored. Using a private type prevents collisions with other packages.
type currentUserKey struct{}

// WithCurrentUser returns a copy of ctx carrying the session-bound user. The
// server's session middleware calls this once per authenticated request.
func WithCurrentUser(ctx context.Context, u SessionUser) context.Context {
	return context.WithValue(ctx, currentUserKey{}, u)
}

// CurrentUser returns the session-bound user attached to ctx, or (nil, false)
// when the request is unauthenticated.
func CurrentUser(ctx context.Context) (SessionUser, bool) {
	u, ok := ctx.Value(currentUserKey{}).(SessionUser)
	if !ok || u == nil {
		return nil, false
	}
	return u, true
}

// RequireRole returns middleware that authorizes a request against want. It
// reads the role from the SESSION-bound user (never client input): it responds
// 401 when there is no authenticated user, and 403 with the standard copy when
// the user's role is insufficient. Admin is a superset of every role for this
// phase's gates (an admin passes any RequireRole check).
func RequireRole(want string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := CurrentUser(r.Context())
			if !ok {
				writeJSONError(w, http.StatusUnauthorized, "Your session expired. Sign in again to continue.")
				return
			}
			if !roleSatisfies(u.UserRole(), want) {
				writeJSONError(w, http.StatusForbidden, rbacForbiddenMessage)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// roleSatisfies reports whether a user holding have is allowed to act where
// want is required. Admin satisfies everything; otherwise an exact match is
// required (editor/reader content gating gains nuance in Phase 1, D-07).
func roleSatisfies(have, want string) bool {
	if have == RoleAdmin {
		return true
	}
	return have == want
}

// writeJSONError writes a minimal {"error": message} JSON body. Kept local to
// auth so the middleware does not import the server package.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Minimal hand-rolled JSON to avoid pulling encoding/json into a hot path
	// and to keep the message escaping trivially correct for our fixed copy.
	_, _ = w.Write([]byte(`{"error":` + jsonString(message) + `}`))
}

// jsonString quotes s as a JSON string. Our messages are fixed ASCII copy, but
// we escape the characters that matter for safety.
func jsonString(s string) string {
	b := make([]byte, 0, len(s)+2)
	b = append(b, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"', '\\':
			b = append(b, '\\', c)
		case '\n':
			b = append(b, '\\', 'n')
		case '\r':
			b = append(b, '\\', 'r')
		case '\t':
			b = append(b, '\\', 't')
		default:
			b = append(b, c)
		}
	}
	b = append(b, '"')
	return string(b)
}
