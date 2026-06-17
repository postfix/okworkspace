// Package users owns the user record and its persistence in the operational
// SQLite store, plus first-run admin bootstrap.
package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/postfix/okworkspace/internal/auth"
)

// ErrNotFound is returned when a user lookup finds no matching row.
var ErrNotFound = errors.New("user not found")

// User is an operational user record. Roles are the fixed SPEC set
// admin/editor/reader (D-07).
type User struct {
	ID                 int64
	Username           string
	DisplayName        string
	Role               string
	PasswordHash       string
	MustChangePassword bool
	Active             bool
}

// Repository reads and writes users via the shared *sql.DB. All queries are
// parameterized (no string concatenation of user input) to prevent SQLi.
type Repository struct {
	db *sql.DB
}

// NewRepository returns a Repository backed by db.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// GetByUsername looks up a user by username, returning ErrNotFound if absent.
func (r *Repository) GetByUsername(ctx context.Context, username string) (*User, error) {
	const q = `SELECT id, username, display_name, role, password_hash, must_change_password, active
	           FROM users WHERE username = ?`
	var u User
	var mustChange, active int
	err := r.db.QueryRowContext(ctx, q, username).Scan(
		&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.PasswordHash, &mustChange, &active,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user %q: %w", username, err)
	}
	u.MustChangePassword = mustChange != 0
	u.Active = active != 0
	return &u, nil
}

// GetByID looks up a user by id, returning ErrNotFound if absent.
func (r *Repository) GetByID(ctx context.Context, id int64) (*User, error) {
	const q = `SELECT id, username, display_name, role, password_hash, must_change_password, active
	           FROM users WHERE id = ?`
	var u User
	var mustChange, active int
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.PasswordHash, &mustChange, &active,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user id %d: %w", id, err)
	}
	u.MustChangePassword = mustChange != 0
	u.Active = active != 0
	return &u, nil
}

// LookupForAuth implements auth.UserLookup: it returns the minimal fields the
// authenticator needs, mapping ErrNotFound to auth.ErrUserNotFound.
func (r *Repository) LookupForAuth(ctx context.Context, username string) (auth.AuthUser, error) {
	u, err := r.GetByUsername(ctx, username)
	if errors.Is(err, ErrNotFound) {
		return auth.AuthUser{}, auth.ErrUserNotFound
	}
	if err != nil {
		return auth.AuthUser{}, err
	}
	return auth.AuthUser{ID: u.ID, PasswordHash: u.PasswordHash, Active: u.Active}, nil
}

// Count returns the total number of user rows.
func (r *Repository) Count(ctx context.Context) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

// Create inserts a new user and returns its id.
func (r *Repository) Create(ctx context.Context, u User) (int64, error) {
	const q = `INSERT INTO users
		(username, display_name, role, password_hash, must_change_password, active, created_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'))`
	res, err := r.db.ExecContext(ctx, q,
		u.Username, u.DisplayName, u.Role, u.PasswordHash, boolToInt(u.MustChangePassword), boolToInt(u.Active))
	if err != nil {
		return 0, fmt.Errorf("create user %q: %w", u.Username, err)
	}
	return res.LastInsertId()
}

// List returns all users ordered by display name. Used by the admin screen.
func (r *Repository) List(ctx context.Context) ([]User, error) {
	const q = `SELECT id, username, display_name, role, password_hash, must_change_password, active
	           FROM users ORDER BY display_name COLLATE NOCASE`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []User
	for rows.Next() {
		var u User
		var mustChange, active int
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.PasswordHash, &mustChange, &active); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		u.MustChangePassword = mustChange != 0
		u.Active = active != 0
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return out, nil
}

// UpdateRole sets a user's role. Role validity is enforced by the caller
// (users.SetRole); this method only performs the parameterized update.
func (r *Repository) UpdateRole(ctx context.Context, id int64, role string) error {
	return r.execUpdate(ctx, `UPDATE users SET role = ? WHERE id = ?`, id, role, id)
}

// UpdatePassword sets a user's password hash and must_change_password flag.
func (r *Repository) UpdatePassword(ctx context.Context, id int64, hash string, mustChange bool) error {
	return r.execUpdate(ctx,
		`UPDATE users SET password_hash = ?, must_change_password = ? WHERE id = ?`,
		id, hash, boolToInt(mustChange), id)
}

// SetActive toggles a user's active flag (Deactivate/reactivate).
func (r *Repository) SetActive(ctx context.Context, id int64, active bool) error {
	return r.execUpdate(ctx, `UPDATE users SET active = ? WHERE id = ?`, id, boolToInt(active), id)
}

// UpdateDisplayName changes a user's display name.
func (r *Repository) UpdateDisplayName(ctx context.Context, id int64, displayName string) error {
	return r.execUpdate(ctx, `UPDATE users SET display_name = ? WHERE id = ?`, id, displayName, id)
}

// execUpdate runs a parameterized UPDATE and returns ErrNotFound when no row
// matched the id. The id is passed twice by callers (once for the WHERE value,
// once for the not-found check) — args carries the bind values in order.
func (r *Repository) execUpdate(ctx context.Context, q string, id int64, args ...any) error {
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("update user id %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected for user id %d: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
