package server

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/store"
	"github.com/postfix/okworkspace/internal/users"
)

// Deps holds the server's dependencies (DI wiring).
type Deps struct {
	Store    *store.Store
	Config   config.Config
	UserRepo *users.Repository
	// SPAHandler serves the embedded React app + history fallback for non-API
	// routes. Optional; when nil a minimal 404 is served (Task 3 wires the
	// real embedded SPA).
	SPAHandler http.Handler
	// Health reports repository health for GET /api/v1/health. Optional; when
	// nil the endpoint reports a default healthy status.
	Health HealthChecker
}

// New builds the HTTP handler: chi mux with the middleware stack (recover,
// request-id, session, CSRF) and the /api/v1 auth surface, plus the SPA
// catch-all.
func New(deps Deps) (http.Handler, error) {
	if deps.Store == nil || deps.UserRepo == nil {
		return nil, fmt.Errorf("server.New: Store and UserRepo are required")
	}

	sm := auth.NewSessionManager(deps.Store.DB(), deps.Config)
	h := &authHandlers{
		sessions: sm,
		users:    deps.UserRepo,
		config:   deps.Config,
	}
	health := &healthHandler{checker: deps.Health}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	// API surface.
	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/csrf", h.handleCSRF)
		api.Get("/health", health.handleHealth)
		api.Route("/auth", func(a chi.Router) {
			a.Post("/login", h.handleLogin)
			a.Post("/logout", h.handleLogout)
			a.Get("/me", h.handleMe)
		})

		// Authenticated surface: loadCurrentUser resolves the session-bound user
		// and attaches it to the context so RequireRole authorizes from the
		// SESSION, never client input (T-00.03-01).
		api.Group(func(authed chi.Router) {
			authed.Use(h.loadCurrentUser)

			// Self-service profile (any authenticated user, self only — D-06).
			authed.Put("/profile", h.handleUpdateProfile)
			authed.Put("/profile/password", h.handleChangePassword)

			// Admin-only user management (D-05). Every route is gated by
			// RequireRole(admin) in addition to the global nosurf CSRF on
			// mutating methods (T-00.03-03).
			authed.Group(func(admin chi.Router) {
				admin.Use(auth.RequireRole(auth.RoleAdmin))
				admin.Get("/admin/users", h.handleListUsers)
				admin.Post("/admin/users", h.handleCreateUser)
				admin.Put("/admin/users/{id}/role", h.handleSetRole)
				admin.Post("/admin/users/{id}/reset-password", h.handleResetPassword)
				admin.Post("/admin/users/{id}/deactivate", h.handleDeactivate)
			})
		})
	})

	// SPA catch-all for non-API routes.
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		if deps.SPAHandler != nil && !isAPIPath(req) {
			deps.SPAHandler.ServeHTTP(w, req)
			return
		}
		http.NotFound(w, req)
	})

	// Wrap with session load/save, then CSRF (outermost so the token cookie is
	// set even for the /csrf GET).
	secure := deps.Config.Server.PublicURL != "" &&
		len(deps.Config.Server.PublicURL) >= 8 && deps.Config.Server.PublicURL[:8] == "https://"
	handler := sessionLoad(sm, r)
	handler = csrfProtect(handler, secure)
	return handler, nil
}
