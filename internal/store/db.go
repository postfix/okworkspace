// Package store owns the single shared *sql.DB for OKF Workspace operational
// data (users, sessions, and — in later phases — jobs and the audit mirror).
// It uses the pure-Go modernc.org/sqlite driver so the binary stays CGO-free
// and statically linkable. SQLite is operational data ONLY, never wiki content.
package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (driver name "sqlite")
)

// Store wraps the shared *sql.DB. All packages share one Store rather than
// each opening SQLite independently.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at dsn (a file path) and
// configures foreign keys and WAL mode. The returned Store must be closed.
func Open(dsn string) (*Store, error) {
	// modernc.org/sqlite accepts PRAGMA settings via query params on the DSN.
	connStr := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dsn)
	db, err := sql.Open("sqlite", connStr)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", dsn, err)
	}
	// SQLite is a single-file embedded DB; keep a single connection to avoid
	// "database is locked" under WAL with concurrent writers at this scale.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite %q: %w", dsn, err)
	}
	return &Store{db: db}, nil
}

// DB returns the underlying *sql.DB for query execution and for wiring the SCS
// session store.
func (s *Store) DB() *sql.DB { return s.db }

// Close closes the underlying database.
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// PingContext verifies connectivity.
func (s *Store) PingContext(ctx context.Context) error {
	return s.db.PingContext(ctx)
}
