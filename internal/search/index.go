package search

import (
	"database/sql"
	"fmt"
	"os"
	"sync"

	"github.com/blevesearch/bleve/v2"

	"github.com/postfix/okworkspace/internal/repo"
)

// Index is the single, process-wide search index. It holds ONE shared bleve.Index
// (concurrency-safe for many concurrent readers + the single worker writer —
// Pitfall 4) plus the dependencies the lifecycle needs: the content repo (read
// files for rebuild via the SEC-01 resolver) and the SQLite db (last_indexed_head
// bookkeeping). repo/db are attached after open (SetRepo/SetDB) so the constructor
// keeps the simple OpenOrCreate(dir) signature the startup path uses.
//
// mu guards the idx pointer so an atomic rebuild swap can replace the underlying
// index while in-flight queries safely read the old one until the swap completes.
type Index struct {
	dir string

	mu  sync.RWMutex
	idx bleve.Index

	repo *repo.Repo
	db   *sql.DB
}

// OpenOrCreate opens the on-disk scorch index at dir, creating it (with the typed
// mapping) when the path does not yet exist. On any OTHER open error (e.g.
// corruption after an unclean shutdown) it returns the error; the caller logs and
// triggers a full rebuild into a fresh dir rather than repairing in place
// (Pitfall 3). The dir MUST live under <data_dir>/index/, outside the content/Git
// repo entirely (T-03-04).
func OpenOrCreate(dir string) (*Index, error) {
	bidx, err := openOrCreateBleve(dir)
	if err != nil {
		return nil, err
	}
	return &Index{dir: dir, idx: bidx}, nil
}

// openOrCreateBleve opens an existing scorch index or creates a new one.
func openOrCreateBleve(dir string) (bleve.Index, error) {
	bidx, err := bleve.Open(dir)
	if err == nil {
		return bidx, nil
	}
	if err == bleve.ErrorIndexPathDoesNotExist {
		return bleve.New(dir, buildMapping())
	}
	return nil, fmt.Errorf("search: open index %q: %w", dir, err)
}

// SetRepo attaches the content repo used by rebuild/upsert file reads.
func (s *Index) SetRepo(r *repo.Repo) { s.repo = r }

// SetDB attaches the SQLite db used for last_indexed_head bookkeeping.
func (s *Index) SetDB(db *sql.DB) { s.db = db }

// Close closes the underlying index. Call on shutdown AFTER the worker has
// stopped (no more writes) so no in-flight Index/Delete races the close.
func (s *Index) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idx == nil {
		return nil
	}
	err := s.idx.Close()
	s.idx = nil
	return err
}

// DocCount returns the number of documents in the index (idempotency assertions).
func (s *Index) DocCount() (uint64, error) {
	s.mu.RLock()
	idx := s.idx
	s.mu.RUnlock()
	if idx == nil {
		return 0, fmt.Errorf("search: index is closed")
	}
	return idx.DocCount()
}

// swapDir atomically replaces the live index with the freshly-built one at tmp.
// The new index is opened first (so a failure to open it leaves the old one
// intact), then under the write lock the old index is closed, the old dir removed,
// tmp renamed into place, and the new index installed. os.Rename is atomic on the
// same filesystem; tmp is a sibling of dir so that holds.
func (s *Index) swapDir(tmp string) error {
	newIdx, err := bleve.Open(tmp)
	if err != nil {
		return fmt.Errorf("search: open rebuilt index: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.idx != nil {
		_ = s.idx.Close()
		s.idx = nil
	}
	// Close the freshly-opened handle before the rename so no open handle holds the
	// tmp dir, then reopen at the final path after the swap.
	_ = newIdx.Close()

	if err := os.RemoveAll(s.dir); err != nil {
		return fmt.Errorf("search: remove stale index dir: %w", err)
	}
	if err := os.Rename(tmp, s.dir); err != nil {
		return fmt.Errorf("search: swap index dir: %w", err)
	}
	reopened, err := bleve.Open(s.dir)
	if err != nil {
		return fmt.Errorf("search: reopen swapped index: %w", err)
	}
	s.idx = reopened
	return nil
}

// withIndex runs fn against the current index pointer under a read lock so a
// concurrent swap cannot pull the index out from under a query.
func (s *Index) withIndex(fn func(idx bleve.Index) error) error {
	s.mu.RLock()
	idx := s.idx
	s.mu.RUnlock()
	if idx == nil {
		return fmt.Errorf("search: index is closed")
	}
	return fn(idx)
}
