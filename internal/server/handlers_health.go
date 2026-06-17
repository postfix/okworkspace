package server

import (
	"context"
	"net/http"
)

// RepoHealth is the repository health payload surfaced to the SPA (SPEC §6.6).
// It mirrors gitstore.HealthStatus without importing gitstore into the server
// package, keeping the dependency one-directional.
type RepoHealth struct {
	OK         bool   `json:"ok"`
	Diverged   bool   `json:"diverged"`
	SelfHealed bool   `json:"self_healed"`
	Detail     string `json:"detail"`
}

// HealthChecker reports current repository health. Implemented by *gitstore.GitStore
// via an adapter in main, so the server package does not depend on gitstore.
type HealthChecker interface {
	RepoHealth(ctx context.Context) (RepoHealth, error)
}

// handleHealth returns the repository health status. It is a GET, so it needs no
// CSRF token; it is admin-visible in the UI (full RBAC gating lands in Plan 03).
func (h *healthHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if h.checker == nil {
		writeJSON(w, http.StatusOK, RepoHealth{OK: true, Detail: "Storage healthy"})
		return
	}
	status, err := h.checker.RepoHealth(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, RepoHealth{OK: false, Detail: "Storage health check failed"})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

type healthHandler struct {
	checker HealthChecker
}
