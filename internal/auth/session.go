package auth

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/postfix/okworkspace/internal/config"
)

// SessionUserIDKey is the SCS session key holding the authenticated user id.
const SessionUserIDKey = "user_id"

// SessionConnectionIDKey is the SCS session key holding the client-generated
// connection id used as the soft-lock session_id (COLL-02 presence/lock owner).
const SessionConnectionIDKey = "connection_id"

// SecureCookies reports whether cookies should carry the Secure flag for the
// given public URL — true exactly when the URL is served over HTTPS. It is the
// SINGLE source of truth for the Secure decision (WR-07): both the SCS session
// cookie and the nosurf CSRF cookie derive their Secure flag from this helper,
// so the two can never diverge. The check is case-insensitive (e.g. an
// "HTTPS://host" config is correctly treated as secure for BOTH cookies).
func SecureCookies(publicURL string) bool {
	return strings.HasPrefix(strings.ToLower(publicURL), "https://")
}

// NewSessionManager builds an SCS session manager backed by the shared SQLite
// DB (sessions persist in app.db). The cookie is HttpOnly + SameSite=Lax, and
// Secure when server.public_url is https. Lifetime = auth.session_ttl_hours.
func NewSessionManager(db *sql.DB, cfg config.Config) *scs.SessionManager {
	m := scs.New()
	m.Store = newSQLiteSessionStore(db)
	m.Lifetime = time.Duration(cfg.Auth.SessionTTLHours) * time.Hour

	m.Cookie.Name = cfg.Auth.SessionCookieName
	if m.Cookie.Name == "" {
		m.Cookie.Name = config.DefaultSessionCookieName
	}
	m.Cookie.HttpOnly = true
	m.Cookie.SameSite = http.SameSiteLaxMode
	m.Cookie.Path = "/"
	m.Cookie.Secure = SecureCookies(cfg.Server.PublicURL)
	return m
}
