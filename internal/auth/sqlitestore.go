package auth

import (
	"context"
	"database/sql"
	"time"
)

// sqliteSessionStore implements scs.CtxStore against the shared, pure-Go
// modernc.org/sqlite database. The upstream scs sqlite3store package depends on
// the cgo mattn/go-sqlite3 driver, which CLAUDE.md forbids; this thin store
// uses the same `sessions` table schema (token TEXT PK, data BLOB, expiry REAL)
// so sessions persist in app.db with no cgo.
type sqliteSessionStore struct {
	db *sql.DB
}

func newSQLiteSessionStore(db *sql.DB) *sqliteSessionStore {
	return &sqliteSessionStore{db: db}
}

func (s *sqliteSessionStore) Find(token string) ([]byte, bool, error) {
	return s.FindCtx(context.Background(), token)
}

func (s *sqliteSessionStore) Commit(token string, b []byte, expiry time.Time) error {
	return s.CommitCtx(context.Background(), token, b, expiry)
}

func (s *sqliteSessionStore) Delete(token string) error {
	return s.DeleteCtx(context.Background(), token)
}

func (s *sqliteSessionStore) FindCtx(ctx context.Context, token string) ([]byte, bool, error) {
	var data []byte
	row := s.db.QueryRowContext(ctx,
		`SELECT data FROM sessions WHERE token = ? AND expiry >= ?`, token, nowUnix())
	err := row.Scan(&data)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func (s *sqliteSessionStore) CommitCtx(ctx context.Context, token string, b []byte, expiry time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token, data, expiry) VALUES (?, ?, ?)
		 ON CONFLICT(token) DO UPDATE SET data = excluded.data, expiry = excluded.expiry`,
		token, b, float64(expiry.UnixNano())/1e9)
	return err
}

func (s *sqliteSessionStore) DeleteCtx(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	return err
}

func nowUnix() float64 {
	return float64(time.Now().UnixNano()) / 1e9
}
