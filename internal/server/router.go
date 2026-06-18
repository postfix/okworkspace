package server

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/pages"
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
	// Audit records key actions (login/logout/user-management/profile) to the
	// SQLite mirror + a structured slog line (SEC-05). Optional; when nil the
	// handlers use a no-op recorder so audit never breaks a request path.
	Audit *audit.Logger
	// Pages is the page lifecycle service backing the tree/get/create/save/folder
	// routes. Optional; when nil those routes return a 500 (the SPA is wired with
	// the real service in main.go).
	Pages *pages.Service
}

// New builds the HTTP handler: chi mux with the middleware stack (recover,
// request-id, session, CSRF) and the /api/v1 auth surface, plus the SPA
// catch-all.
func New(deps Deps) (http.Handler, error) {
	if deps.Store == nil || deps.UserRepo == nil {
		return nil, fmt.Errorf("server.New: Store and UserRepo are required")
	}

	sm := auth.NewSessionManager(deps.Store.DB(), deps.Config)
	var rec auditRecorder = deps.Audit
	if deps.Audit == nil {
		rec = nopAudit{}
	}
	h := &authHandlers{
		sessions: sm,
		users:    deps.UserRepo,
		config:   deps.Config,
		audit:    rec,
		pages:    deps.Pages,
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

			// Page reads — available to ANY authenticated user (readers included).
			// The `/pages/*` catch-all matches slash-bearing page paths; the
			// wildcard is read via chi.URLParam(r, "*"). A regex `{path:.*}` param
			// is NOT used because chi mis-routes multi-segment paths when a GET and
			// a PUT share the same regex-wildcard node (sibling-route conflict);
			// the plain `*` catch-all routes every depth correctly.
			authed.Get("/tree", h.handleTree)
			authed.Get("/pages/*", h.handleGetPage)
			// Trash listing — readable by any authenticated user (the trash view
			// surfaces provenance only, never page content or Git vocabulary).
			authed.Get("/trash", h.handleListTrash)

			// Page/folder MUTATIONS — editor-gated subgroup (mirrors the admin
			// subgroup). Authorization is read from the session role via
			// RequireRole, never client input (T-02-02). Admin passes the editor
			// gate (roleSatisfies).
			authed.Group(func(editor chi.Router) {
				editor.Use(auth.RequireRole(auth.RoleEditor))
				editor.Post("/pages", h.handleCreatePage)
				editor.Put("/pages/*", h.handleSavePage)
				// Rename/move share one endpoint: POST /pages/{path}/rename.
				// chi cannot host a `{path:.*}` regex node and the `/pages/*`
				// catch-all (used by GET/PUT) as siblings — they conflict and
				// yield a 405 (the same sibling-wildcard issue Plan 02 hit). So
				// rename is registered on the SAME `/pages/*` catch-all under POST,
				// and the handler strips the trailing `/rename` from the wildcard.
				editor.Post("/pages/*", h.handleRenamePage)
				editor.Delete("/pages/*", h.handleDeletePage)
				editor.Post("/folders", h.handleCreateFolder)
				// Restore a trashed page to its original folder (auto-suffix on a
				// live-page collision, D-10). {id} is the trash row id, not a path,
				// so this does not collide with the /pages/* wildcard.
				editor.Post("/trash/{id}/restore", h.handleRestoreFromTrash)
			})

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
	// set even for the /csrf GET). WR-07: the Secure flag for the CSRF cookie is
	// derived from the SAME auth.SecureCookies helper the session manager uses,
	// so the two cookies' Secure flags can never diverge.
	secure := auth.SecureCookies(deps.Config.Server.PublicURL)
	handler := sessionLoad(sm, r)
	handler = csrfProtect(handler, secure)
	return handler, nil
}
