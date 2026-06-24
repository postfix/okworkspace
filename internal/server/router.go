package server

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/postfix/okworkspace/internal/agent"
	"github.com/postfix/okworkspace/internal/attachments"
	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/auth"
	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/graph"
	"github.com/postfix/okworkspace/internal/locks"
	"github.com/postfix/okworkspace/internal/pages"
	"github.com/postfix/okworkspace/internal/search"
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
	// Attachments is the attachment lifecycle service backing the upload/list/
	// download routes. Optional; when nil those routes return a 500.
	Attachments *attachments.Service
	// Search is the full-text search index backing GET /search and the admin
	// reindex. Optional; when nil those routes return a 500 (generic copy),
	// following the existing optional-dependency pattern.
	Search *search.Index
	// SearchJobs enqueues the rebuild job for POST /admin/search/reindex
	// (fire-and-forget). Optional; when nil reindex returns a 500.
	SearchJobs searchEnqueuer
	// GraphJobs enqueues the from-files rebuild job for POST /admin/graph/reindex
	// (fire-and-forget). Optional; when nil the graph reindex returns a 500. In
	// main.go this is the SAME single worker passed as SearchJobs (the KindGraph
	// handler is registered on it), not a second store/worker.
	GraphJobs graphEnqueuer
	// Graph is the derived link/tag adjacency store backing the authed graph READ
	// endpoints (GET /graph, /graph/local, /graph/backlinks). Optional; when nil
	// those reads return a 500 with the generic copy. In main.go this is the SAME
	// graphStore already constructed for the KindGraph maintenance handler — it is
	// reused for the reads, not a second store.
	Graph *graph.Store
	// Agent is the Eino agent service backing POST /agent/chat (Ask). Optional;
	// when nil the route returns a 500. When constructed-but-disabled (cfg.Agent.
	// Enabled false) the handler returns a structured "agent off" error.
	Agent *agent.Service
	// Locks is the soft-lock store backing the acquire/force/release endpoints
	// (COLL-02). Optional; when nil those routes return a 500, following the
	// optional-dependency pattern.
	Locks *locks.Service
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
		sessions:    sm,
		users:       deps.UserRepo,
		config:      deps.Config,
		audit:       rec,
		pages:       deps.Pages,
		attachments: deps.Attachments,
		search:      deps.Search,
		searchJobs:  deps.SearchJobs,
		graphJobs:   deps.GraphJobs,
		graph:       deps.Graph,
		agent:       deps.Agent,
		locks:       deps.Locks,
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
			// GET /pages/* serves a plain page read, the version history
			// (".../history"), or an old version (".../version/{v}") — the
			// dispatcher routes on the wildcard suffix since chi cannot host
			// sibling wildcard routes (VER-02/03 reads, any authenticated user).
			authed.Get("/pages/*", h.handleGetPageOrHistory)
			// Trash listing — readable by any authenticated user (the trash view
			// surfaces provenance only, never page content or Git vocabulary).
			authed.Get("/trash", h.handleListTrash)

			// Attachment reads — available to ANY authenticated user. The single
			// `/attachments/*` catch-all hosts BOTH the per-page list and the
			// byte-exact download: chi cannot host a `{id}/download` route next to
			// the slash-bearing `{pagePath}` list wildcard (the sibling-wildcard
			// conflict the page routes also hit), so both are dispatched on the same
			// catch-all by handleGetAttachment (download iff the wildcard ends in
			// "/download", the SSE extraction-status stream iff it ends in
			// "/status", else a page list). The SSE stream is dispatched here for
			// the same reason: a sibling {id}/status route cannot coexist with the
			// slash-bearing list wildcard.
			authed.Get("/attachments/*", h.handleGetAttachment)

			// Full-text search — available to ANY authenticated user (matches the
			// page-read authorization model, Area 4). q is a query param (not a
			// path), so no cleanPathParam is needed.
			authed.Get("/search", h.handleSearch)

			// Derived link/tag graph reads — available to ANY authenticated user
			// (the same any-authed read model as /tree + /search, NOT editor/admin
			// gated). All three are mounted here in the authed read group. path and
			// depth are QUERY params (not path segments); backlinks is a query-param
			// read (NOT a /pages/{path}/backlinks route) precisely to avoid the
			// /pages/* sibling-wildcard conflict (chi cannot host a sibling wildcard
			// next to the /pages/* catch-all). The payload is lean (ids+labels+typed
			// edges, no page bodies) and built only from the cache tables.
			authed.Get("/graph", h.handleGraph)
			authed.Get("/graph/local", h.handleGraphLocal)
			authed.Get("/graph/backlinks", h.handleGraphBacklinks)

			// Agent Ask (AGNT-01) — any authenticated user may ask a question
			// about the current page; the answer streams back as SSE. POST (it
			// carries a prompt body and so inherits the global nosurf CSRF on
			// mutating methods), but it is READ-ONLY: the agent reaches the
			// workspace only through the five read-only tools, no write/apply tool
			// is reachable, and authorization/scope are read from the SESSION.
			authed.Post("/agent/chat", h.handleAgentChat)

			// Single-shot agent modes (AGNT-05/06/07/08) — any-authed, awaited
			// (JSON, not SSE). Summarize is read-only; Rewrite returns a proposal
			// for the diff dialog and Draft a body for the editor — NEITHER writes
			// (the apply path is a separate gated endpoint). They reach the
			// workspace only through the role-scoped pages/attachments services.
			authed.Post("/agent/summarize-page", h.handleSummarizePage)
			authed.Post("/agent/summarize-attachment", h.handleSummarizeAttachment)
			authed.Post("/agent/rewrite", h.handleRewrite)
			authed.Post("/agent/draft", h.handleDraft)

			// Per-page tag suggestion (TAG-01) — suggest is READ-ONLY (any-authed,
			// like rewrite): it returns a vocab-biased capped proposal + the base
			// revision + per-tag existing flags and NEVER writes. Apply is the
			// consequential editor+CSRF write (registered in the editor subgroup
			// below). NEITHER is an Eino tool — the read-only 5-tool boundary is
			// unchanged (AGNT-11).
			authed.Post("/agent/suggest-tags", h.handleSuggestTags)

			// Page/folder MUTATIONS — editor-gated subgroup (mirrors the admin
			// subgroup). Authorization is read from the session role via
			// RequireRole, never client input (T-02-02). Admin passes the editor
			// gate (roleSatisfies).
			authed.Group(func(editor chi.Router) {
				editor.Use(auth.RequireRole(auth.RoleEditor))

				// Agent SAFETY-CORE gate (AGNT-09/10/11) — editor-gated, inheriting
				// the global nosurf CSRF on these mutating POSTs. /propose-patch
				// returns old+new bodies + the captured base revision so the FRONTEND
				// renders a real diff; /apply-patch is the ONE consequential write,
				// reusing pages.Save → the single-writer commit and 409-ing on a moved
				// revision. NEITHER is an Eino tool — the read-only 5-tool allow-list is
				// unchanged (apply/write is never tool-reachable, AGNT-11).
				editor.Post("/agent/propose-patch", h.handleProposePatch)
				editor.Post("/agent/apply-patch", h.handleApplyPatch)

				// Apply approved tags (TAG-01 write / TAG-03 reuse) — the consequential
				// editor+CSRF write. It re-validates+normalizes the client tags
				// server-side (the client list is never trusted), writes ONLY the tags
				// lines byte-stably via okf.SetTags through pages.Save → the single-
				// writer commit, and 409s on a moved base revision. NOT an Eino tool.
				editor.Post("/agent/apply-tags", h.handleApplyTags)

				editor.Post("/pages", h.handleCreatePage)
				editor.Put("/pages/*", h.handleSavePage)
				// Rename/move share one endpoint: POST /pages/{path}/rename.
				// chi cannot host a `{path:.*}` regex node and the `/pages/*`
				// catch-all (used by GET/PUT) as siblings — they conflict and
				// yield a 405 (the same sibling-wildcard issue Plan 02 hit). So
				// rename is registered on the SAME `/pages/*` catch-all under POST,
				// and the handler strips the trailing `/rename` from the wildcard.
				// Folder rename/move (`/rename-folder`, `/move-folder`) share this same
				// catch-all dispatch (a folder is a dir/index.md page, so no sibling
				// wildcard route) and inherit this editor RBAC gate (TREE-02/06,
				// authorization read from the SESSION role, never client input).
				editor.Post("/pages/*", h.handleRenamePage)
				editor.Delete("/pages/*", h.handleDeletePage)
				editor.Post("/folders", h.handleCreateFolder)
				// Attachment upload — editor-gated (RBAC from the session, never
				// client input — T-02-05/SEC §V4), mirroring the page-mutation gate.
				editor.Post("/attachments", h.handleUploadAttachment)
				// Attachment replace (PUT, ATT-05) and remove (DELETE, ATT-06/07) —
				// editor-gated from the session (T-02-14). Registered on the SAME
				// `/attachments/*` catch-all as the GET reads (the id is the
				// wildcard): a `{id}` sibling route cannot coexist with the
				// slash-bearing list wildcard the GET uses (the sibling-wildcard
				// conflict the page routes also hit), so PUT/DELETE reuse the
				// catch-all and read the id via chi.URLParam(r, "*").
				editor.Put("/attachments/*", h.handleReplaceAttachment)
				editor.Delete("/attachments/*", h.handleDeleteAttachment)
				// Restore a trashed page to its original folder (auto-suffix on a
				// live-page collision, D-10). {id} is the trash row id, not a path,
				// so this does not collide with the /pages/* wildcard.
				editor.Post("/trash/{id}/restore", h.handleRestoreFromTrash)
				// Restore a whole folder-delete as a unit (TREE-05), index.md first,
				// per-page collision auto-suffix. {id} is the OPAQUE delete_group_id
				// (not a path), bound parameterized in SQL — no wildcard conflict with
				// /pages/* and no SQLi (T-07-05). Editor-gated like the page restore.
				editor.Post("/trash/group/{id}/restore", h.handleRestoreFolderGroup)
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
				// Rebuild the search index from files — admin-only operational
				// action (T-03-07), already behind RequireRole(admin) + nosurf CSRF.
				admin.Post("/admin/search/reindex", h.handleReindex)
				// Rebuild the derived link/tag graph from files — admin-only
				// operational action (LINK-03), inheriting the SAME RequireRole(admin)
				// gate + nosurf CSRF; RBAC is read from the session role, never client
				// input. Enqueues a from-files rebuild fire-and-forget (202).
				admin.Post("/admin/graph/reindex", h.handleGraphReindex)
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
