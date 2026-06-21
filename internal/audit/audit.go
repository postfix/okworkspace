// Package audit implements the SEC-05 audit log: it records key Phase-0 actions
// (login, logout, admin bootstrap, repo seed, user management, profile/config
// changes, password resets) to BOTH a SQLite mirror row in audit_log AND a
// structured slog line per event.
//
// Two hard rules (PITFALLS / T-00.04-02):
//   - The audit trail captures WHO did WHAT (actor + target + action) — it must
//     never omit a key action.
//   - It must NEVER record a password plaintext, a session token, or any other
//     secret. The Event type deliberately has no password/token field; callers
//     pass only non-secret provenance (actor, target, a short detail).
//
// Record is non-fatal (T-00.04-03): a DB write failure is logged but never
// propagated in a way that breaks the calling request path. Auth must not go
// down because the audit mirror is briefly unwritable.
package audit

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// Action constants enumerate the Phase-0 audit events. New phases add their own
// (page/attachment changes, agent actions) without touching this set.
const (
	ActionLogin             = "login"
	ActionLogout            = "logout"
	ActionBootstrap         = "bootstrap"
	ActionSeed              = "seed"
	ActionUserCreate        = "user_create"
	ActionUserRoleChange    = "user_role_change"
	ActionUserResetPassword = "user_reset_password"
	ActionUserDeactivate    = "user_deactivate"
	ActionProfileChange     = "profile_change"
	ActionConfigChange      = "config_change"
	ActionPageCreate        = "page_create"
	ActionPageEdit          = "page_edit"
	ActionPageRename        = "rename"
	ActionPageMove          = "move"
	ActionPageTrash         = "trash"
	ActionPageRestore       = "restore"
	ActionFolderCreate      = "folder_create"
	ActionAttachUpload      = "attach_upload"
	ActionAttachReplace     = "attach_replace"
	ActionAttachDelete      = "attach_delete"
	ActionSearchReindex     = "search_reindex"
)

// Event is one audit record. It carries only non-secret provenance — there is
// intentionally no password/token field, so a secret can never be recorded by
// construction.
type Event struct {
	// Action is one of the Action* constants (what happened).
	Action string
	// Actor is who performed the action (a username, or "system"/"cli" for
	// non-interactive paths).
	Actor string
	// Target is the subject of the action when applicable (e.g. the managed
	// user's username). Optional.
	Target string
	// Detail is a short, non-secret human note (e.g. "role=editor"). Optional.
	// Callers MUST NOT place a password, hash, or token here.
	Detail string
	// Source is the origin of the action ("web-ui", "bootstrap", "cli"). Optional.
	Source string
	// At is the event time; defaults to time.Now().UTC() when zero.
	At time.Time
}

// Logger writes audit events to the SQLite mirror and a structured slog line.
type Logger struct {
	db  *sql.DB
	log *slog.Logger
}

// New returns a Logger backed by the shared operational *sql.DB and the given
// structured logger. If log is nil, slog.Default() is used.
func New(db *sql.DB, log *slog.Logger) *Logger {
	if log == nil {
		log = slog.Default()
	}
	return &Logger{db: db, log: log}
}

// Record writes one mirror row into audit_log AND emits one structured slog
// line for the event. It is non-fatal: a DB write error is logged at warn level
// and returned, but the caller is expected to ignore it (the user-facing request
// must not fail because the audit mirror is briefly unwritable, T-00.04-03).
//
// No secret is ever written: only Action/Actor/Target/Detail/Source/created_at.
func (l *Logger) Record(ctx context.Context, e Event) error {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	createdAt := e.At.UTC().Format(time.RFC3339Nano)

	// (1) Structured slog line first — observability survives even if the DB
	// write below fails. Only non-secret attributes are logged.
	l.log.LogAttrs(ctx, slog.LevelInfo, "audit",
		slog.String("action", e.Action),
		slog.String("actor", e.Actor),
		slog.String("target", e.Target),
		slog.String("source", e.Source),
		slog.String("detail", e.Detail),
		slog.String("at", createdAt),
	)

	// (2) SQLite mirror row. A failure is logged but not allowed to break the
	// caller's request path.
	if l.db == nil {
		return nil
	}
	const q = `INSERT INTO audit_log (action, actor, target, detail, source, created_at)
	           VALUES (?, ?, ?, ?, ?, ?)`
	if _, err := l.db.ExecContext(ctx, q,
		e.Action, e.Actor, e.Target, e.Detail, e.Source, createdAt); err != nil {
		// Non-fatal: surface for observability, return for callers that care,
		// but callers MUST be able to ignore this without crashing.
		l.log.WarnContext(ctx, "audit mirror write failed",
			slog.String("action", e.Action),
			slog.Any("error", err),
		)
		return fmt.Errorf("audit: write mirror row: %w", err)
	}
	return nil
}
