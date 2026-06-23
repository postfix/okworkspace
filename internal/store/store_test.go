package store

import (
	"context"
	"path/filepath"
	"testing"
)

func openTempStore(t *testing.T) *Store {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "app.db")
	st, err := Open(dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestMigrateIdempotent(t *testing.T) {
	st := openTempStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	// Running again must succeed and remain a no-op.
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	for _, tbl := range []string{"users", "sessions"} {
		var name string
		row := st.DB().QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl)
		if err := row.Scan(&name); err != nil {
			t.Fatalf("expected table %q present after migrate: %v", tbl, err)
		}
		if name != tbl {
			t.Errorf("table lookup got %q, want %q", name, tbl)
		}
	}
}

// TestMigrateTrashGroupColumn asserts migration 0008 added the nullable
// delete_group_id column to the trash table (TREE-04/05 grouped folder delete)
// and that the migration runner recorded schema version 8. The column must be
// nullable so existing/solo per-page deletes read NULL.
func TestMigrateTrashGroupColumn(t *testing.T) {
	st := openTempStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// The delete_group_id column exists on trash and is nullable.
	rows, err := st.DB().QueryContext(ctx, `PRAGMA table_info(trash)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(trash): %v", err)
	}
	defer rows.Close()
	var found bool
	for rows.Next() {
		var (
			cid       int
			name, typ string
			notNull   int
			dflt      interface{}
			pk        int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		if name == "delete_group_id" {
			found = true
			if notNull != 0 {
				t.Errorf("delete_group_id must be nullable (notNull=%d, want 0)", notNull)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table_info: %v", err)
	}
	if !found {
		t.Fatal("trash table is missing the delete_group_id column (migration 0008 not applied)")
	}

	// The migration runner recorded version 8.
	var name string
	if err := st.DB().QueryRowContext(ctx,
		`SELECT name FROM schema_migrations WHERE version=8`).Scan(&name); err != nil {
		t.Fatalf("schema_migrations missing version 8: %v", err)
	}
}

func TestUsersWriteThenRead(t *testing.T) {
	st := openTempStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	_, err := st.DB().ExecContext(ctx,
		`INSERT INTO users (username, display_name, role, password_hash, must_change_password, active, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"alice", "Alice Example", "admin", "$argon2id$fake", 1, 1, "2026-06-17T00:00:00Z")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	var displayName, role string
	var mustChange int
	row := st.DB().QueryRowContext(ctx,
		`SELECT display_name, role, must_change_password FROM users WHERE username=?`, "alice")
	if err := row.Scan(&displayName, &role, &mustChange); err != nil {
		t.Fatalf("read user: %v", err)
	}
	if displayName != "Alice Example" || role != "admin" || mustChange != 1 {
		t.Errorf("read-back mismatch: display=%q role=%q mustChange=%d", displayName, role, mustChange)
	}
}
