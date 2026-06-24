package graph

import (
	"context"
	"testing"
)

// TestStore_TablesExistAndAcceptInserts asserts migration 0009 created the three
// derived-cache tables and that they accept inserts (the schema is wired).
func TestStore_TablesExistAndAcceptInserts(t *testing.T) {
	h := newHarness(t)
	db := h.db.DB()

	if _, err := db.Exec(`INSERT INTO page_links (src_path, dst_path) VALUES (?, ?)`,
		"a.md", "b.md"); err != nil {
		t.Fatalf("insert page_links: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO page_tags (page_path, tag) VALUES (?, ?)`,
		"a.md", "ops"); err != nil {
		t.Fatalf("insert page_tags: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO graph_meta (key, value) VALUES (?, ?)`,
		"probe", "v"); err != nil {
		t.Fatalf("insert graph_meta: %v", err)
	}

	links := h.snapshotLinks(t)
	if len(links) != 1 || links[0] != "a.md|b.md" {
		t.Fatalf("page_links = %v, want [a.md|b.md]", links)
	}
	tags := h.snapshotTags(t)
	if len(tags) != 1 || tags[0] != "a.md|ops" {
		t.Fatalf("page_tags = %v, want [a.md|ops]", tags)
	}
}

// TestStore_MigrationIdempotent asserts re-running Migrate is a no-op (0009 is
// recorded in schema_migrations and not re-applied).
func TestStore_MigrationIdempotent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// A second Migrate must not error (IF NOT EXISTS + schema_migrations guard).
	if err := h.db.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	var n int
	if err := h.db.DB().QueryRow(
		`SELECT COUNT(1) FROM schema_migrations WHERE version=?`, 9).Scan(&n); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if n != 1 {
		t.Fatalf("schema_migrations rows for version 9 = %d, want 1", n)
	}
}

// TestStore_DriftAndMeta asserts StoreHead/DriftCheck against graph_meta: after
// storing the current HEAD, drift is false; advancing HEAD makes drift true.
func TestStore_DriftAndMeta(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// A fresh repo with a HEAD but no stored head => drift (must rebuild).
	h.writePage(t, "a.md", "A", []string{"ops"}, "hello")
	if err := h.gs.Commit(ctx, commitSpec("seed a.md", "a.md")); err != nil {
		t.Fatalf("commit: %v", err)
	}
	drift, err := h.st.DriftCheck(ctx, h.gs)
	if err != nil {
		t.Fatalf("DriftCheck: %v", err)
	}
	if !drift {
		t.Fatal("expected drift=true before StoreHead")
	}

	// After recording the current HEAD, drift is false.
	if err := h.st.StoreHead(ctx, h.gs); err != nil {
		t.Fatalf("StoreHead: %v", err)
	}
	drift, err = h.st.DriftCheck(ctx, h.gs)
	if err != nil {
		t.Fatalf("DriftCheck after StoreHead: %v", err)
	}
	if drift {
		t.Fatal("expected drift=false after StoreHead")
	}

	// Advancing HEAD (a new commit) re-introduces drift.
	h.writePage(t, "b.md", "B", nil, "world")
	if err := h.gs.Commit(ctx, commitSpec("seed b.md", "b.md")); err != nil {
		t.Fatalf("commit b: %v", err)
	}
	drift, err = h.st.DriftCheck(ctx, h.gs)
	if err != nil {
		t.Fatalf("DriftCheck after new commit: %v", err)
	}
	if !drift {
		t.Fatal("expected drift=true after advancing HEAD")
	}
}
