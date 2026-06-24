// Package graph maintains the DERIVED link/tag adjacency cache for the workspace:
// forward edges between pages (page_links) and per-page frontmatter tag
// membership (page_tags), backed by two SQLite cache tables. The cache is
// rebuildable-from-files: deleting both tables and running RebuildGraph over the
// on-disk .md files reproduces byte-identical adjacency, so SQLite is NEVER the
// source of truth (locked invariant — the files on disk are truth).
//
// Extraction REUSES the existing parsers (okf.FindLinks for forward links, the
// sequence-aware tag read mirrored from search.readTags) rather than introducing
// a new scanner. The KindGraph job rides the EXISTING single jobs worker and its
// handler defers a recover() so one corrupt page can never wedge the drain
// goroutine. This package deliberately does NOT import internal/search.
package graph

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/postfix/okworkspace/internal/repo"
)

// metaKeyLastGraphedHead is the graph_meta key holding the Git HEAD SHA the graph
// was last built against (the startup drift backstop — clones search's
// last_indexed_head).
const metaKeyLastGraphedHead = "last_graphed_head"

// headProvider is the gitstore subset DriftCheck/StoreHead need: the current
// HEAD. Kept local (identical to search's) so this package does not import
// internal/search.
type headProvider interface {
	HeadSHA(ctx context.Context) (string, error)
}

// Store is the process-wide derived link/tag adjacency cache. It owns the shared
// *sql.DB (where the page_links/page_tags/graph_meta tables live), the content
// repo (read .md files for rebuild/upsert via the SEC-01 resolver), and the Git
// HEAD accessor (record last_graphed_head after a successful rebuild). repo/db/gs
// are attached after open via SetRepo/SetGit, mirroring search.Index's shape.
type Store struct {
	db   *sql.DB
	repo *repo.Repo
	gs   headProvider
}

// OpenStore constructs a Store over the shared *sql.DB. repo and the Git head
// provider are attached afterwards via SetRepo/SetGit (mirroring search.Index's
// SetRepo/SetDB/SetGit setters), keeping the constructor a simple single-arg call
// for the startup wiring path.
func OpenStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// SetRepo attaches the content repo used by rebuild/upsert file reads.
func (s *Store) SetRepo(r *repo.Repo) { s.repo = r }

// SetGit attaches the Git HEAD accessor used to record last_graphed_head after a
// successful rebuild (the drift-detection backstop — CR-01). Without it,
// RebuildGraph cannot persist the HEAD it graphed against and DriftCheck would
// always report drift on startup.
func (s *Store) SetGit(gs headProvider) { s.gs = gs }

// readMeta returns the stored value for key, or "" when absent. A nil db (a
// no-db harness) is a no-op returning "".
func (s *Store) readMeta(ctx context.Context, key string) (string, error) {
	if s.db == nil {
		return "", nil
	}
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM graph_meta WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("graph: read meta %q: %w", key, err)
	}
	return v, nil
}

// writeMeta upserts key=value into graph_meta. A nil db is a no-op.
func (s *Store) writeMeta(ctx context.Context, key, value string) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO graph_meta (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("graph: write meta %q: %w", key, err)
	}
	return nil
}

// StoreHead records the current Git HEAD as the last-graphed value. Called after
// a successful rebuild so a subsequent startup sees the graph as in-sync.
func (s *Store) StoreHead(ctx context.Context, gs headProvider) error {
	head, err := gs.HeadSHA(ctx)
	if err != nil {
		return err
	}
	return s.writeMeta(ctx, metaKeyLastGraphedHead, head)
}

// DriftCheck reports whether the graph is out of sync with the working tree and
// should be rebuilt-from-files. Drift is true when the stored last_graphed_head
// differs from the current Git HEAD (an out-of-band pull/restore, or a crash
// between commit and graph update). A missing stored value (fresh install) also
// counts as drift only when there is actually a HEAD to graph against — an empty
// repo with no HEAD and no stored value is in sync (both empty).
func (s *Store) DriftCheck(ctx context.Context, gs headProvider) (bool, error) {
	stored, err := s.readMeta(ctx, metaKeyLastGraphedHead)
	if err != nil {
		return false, err
	}
	head, err := gs.HeadSHA(ctx)
	if err != nil {
		return false, err
	}
	return stored != head, nil
}
