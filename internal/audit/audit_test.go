package audit_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/store"
)

// newTestStore opens a migrated store in a temp dir.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestRecordWritesRowAndLine(t *testing.T) {
	st := newTestStore(t)

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	a := audit.New(st.DB(), logger)
	if err := a.Record(context.Background(), audit.Event{
		Action: audit.ActionLogin,
		Actor:  "admin",
		Source: "web-ui",
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// (1) Exactly one mirror row with the fields.
	var (
		action, actor, source string
		createdAt             string
		count                 int
	)
	if err := st.DB().QueryRow(`SELECT COUNT(1) FROM audit_log`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("audit_log rows = %d, want 1", count)
	}
	if err := st.DB().QueryRow(
		`SELECT action, actor, source, created_at FROM audit_log LIMIT 1`,
	).Scan(&action, &actor, &source, &createdAt); err != nil {
		t.Fatalf("scan row: %v", err)
	}
	if action != audit.ActionLogin || actor != "admin" || source != "web-ui" {
		t.Errorf("row = (%q,%q,%q), want (login,admin,web-ui)", action, actor, source)
	}
	if createdAt == "" {
		t.Error("created_at is empty; want a timestamp")
	}

	// (2) A structured slog line with the same fields.
	line := buf.String()
	if line == "" {
		t.Fatal("no slog line emitted")
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &rec); err != nil {
		t.Fatalf("slog line is not JSON: %v (line=%q)", err, line)
	}
	if rec["msg"] != "audit" {
		t.Errorf("slog msg = %v, want audit", rec["msg"])
	}
	if rec["action"] != "login" || rec["actor"] != "admin" || rec["source"] != "web-ui" {
		t.Errorf("slog attrs = action:%v actor:%v source:%v", rec["action"], rec["actor"], rec["source"])
	}
}

func TestRecordNeverLogsSecrets(t *testing.T) {
	st := newTestStore(t)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	a := audit.New(st.DB(), logger)

	// Even if a caller mistakenly stuffs a secret-looking string in Detail, the
	// Event has no password/token field by design — assert the line/row carry no
	// "password" or "token" KEY.
	if err := a.Record(context.Background(), audit.Event{
		Action: audit.ActionUserResetPassword,
		Actor:  "admin",
		Target: "bob",
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	line := strings.ToLower(buf.String())
	// The action name legitimately contains "password"; assert no secret KEYS.
	for _, forbidden := range []string{`"password"`, `"token"`, `"one_time_password"`, `"session"`, `"hash"`} {
		if strings.Contains(line, forbidden) {
			t.Errorf("slog line leaks secret key %s: %s", forbidden, line)
		}
	}
}

func TestRecordNonFatalOnDBError(t *testing.T) {
	// Open a store, then close the DB so a write fails — Record must return an
	// error but the test asserts it does NOT panic (audit failure must never
	// take down the calling request path; the caller ignores the error).
	st := newTestStore(t)
	db := st.DB()
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	a := audit.New(db, logger)

	// Must not panic. A returned error is acceptable; the contract is that the
	// caller can safely ignore it without a crash.
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("Record panicked on a closed DB: %v", rec)
			}
		}()
		_ = a.Record(context.Background(), audit.Event{Action: audit.ActionLogout, Actor: "admin"})
	}()

	// The failure should still surface as a warning in the log (observability).
	if !strings.Contains(strings.ToLower(buf.String()), "audit") {
		t.Errorf("expected an audit-related log line on DB failure, got %q", buf.String())
	}
}

// compile-time guard that the DB handle type used by New matches *sql.DB.
var _ = func(db *sql.DB, l *slog.Logger) *audit.Logger { return audit.New(db, l) }
