package search

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// metaKeyLastIndexedHead is the search_meta key holding the Git HEAD SHA the index
// was last built against.
const metaKeyLastIndexedHead = "last_indexed_head"

// headProvider is the gitstore subset DriftCheck/StoreHead need: the current HEAD.
type headProvider interface {
	HeadSHA(ctx context.Context) (string, error)
}

// readMeta returns the stored value for key, or "" when absent.
func (s *Index) readMeta(ctx context.Context, key string) (string, error) {
	if s.db == nil {
		return "", nil
	}
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM search_meta WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("search: read meta %q: %w", key, err)
	}
	return v, nil
}

// writeMeta upserts key=value into search_meta.
func (s *Index) writeMeta(ctx context.Context, key, value string) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO search_meta (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("search: write meta %q: %w", key, err)
	}
	return nil
}

// StoreHead records the current Git HEAD as the last-indexed value. Called after a
// successful rebuild so a subsequent startup sees the index as in-sync.
func (s *Index) StoreHead(ctx context.Context, gs headProvider) error {
	head, err := gs.HeadSHA(ctx)
	if err != nil {
		return err
	}
	return s.writeMeta(ctx, metaKeyLastIndexedHead, head)
}

// DriftCheck reports whether the index is out of sync with the working tree and
// should be rebuilt-from-files. Drift is true when the stored last_indexed_head
// differs from the current Git HEAD (an out-of-band pull/restore, or a crash
// between commit and index). A missing stored value (fresh install) also counts as
// drift only when there is actually a HEAD to index against — an empty repo with
// no HEAD and no stored value is in sync (both empty).
func (s *Index) DriftCheck(ctx context.Context, gs headProvider) (bool, error) {
	stored, err := s.readMeta(ctx, metaKeyLastIndexedHead)
	if err != nil {
		return false, err
	}
	head, err := gs.HeadSHA(ctx)
	if err != nil {
		return false, err
	}
	return stored != head, nil
}
