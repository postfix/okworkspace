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
	m.Cookie.Secure = strings.HasPrefix(strings.ToLower(cfg.Server.PublicURL), "https://")
	return m
}
