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
	gs   headProvider
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

// SetGit attaches the Git HEAD accessor used to record last_indexed_head after a
// successful rebuild (the drift-detection backstop — CR-01). Without it,
// RebuildIndex cannot persist the HEAD it indexed against and DriftCheck would
// always report drift on startup.
func (s *Index) SetGit(gs headProvider) { s.gs = gs }

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
// intact), then under the write lock the old index is closed, the old dir is
// moved aside to a backup, tmp is renamed into place, and the new index is
// installed. os.Rename is atomic on the same filesystem; tmp is a sibling of dir
// so that holds.
//
// The old dir is renamed to a .old backup FIRST (rather than RemoveAll'd) so a
// failed tmp→dir rename can roll the backup back and REOPEN it — otherwise a
// failed swap left s.idx nil and the on-disk index gone, killing search for the
// whole process lifetime with no in-process recovery (WR-01). On any failure
// after the old index is closed, s.idx is restored to a usable handle (the rolled-
// back old index, else a fresh empty index) so search degrades gracefully instead
// of returning "index is closed" forever.
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

	// Move the live dir aside instead of destroying it, so a failed swap can roll
	// back to a working index.
	backup := s.dir + ".old"
	_ = os.RemoveAll(backup)
	if err := os.Rename(s.dir, backup); err != nil {
		// The old dir could not even be moved aside. Try to reopen it in place so
		// search survives; if that fails, fall back to a fresh empty index.
		s.recoverIndex(s.dir)
		return fmt.Errorf("search: move stale index dir aside: %w", err)
	}

	if err := os.Rename(tmp, s.dir); err != nil {
		// Roll the old index back into place and reopen it (recoverIndex) so search
		// keeps working with the pre-rebuild data until the next rebuild.
		if rbErr := os.Rename(backup, s.dir); rbErr == nil {
			s.recoverIndex(s.dir)
		} else {
			// Backup is stranded at <dir>.old; recover whatever we can so the
			// service is not left with a nil index.
			s.recoverIndex(backup)
		}
		return fmt.Errorf("search: swap index dir: %w", err)
	}

	reopened, err := bleve.Open(s.dir)
	if err != nil {
		// The swapped-in index will not open; restore the backup if it is still
		// around so search survives, then surface the error.
		_ = os.RemoveAll(s.dir)
		if rbErr := os.Rename(backup, s.dir); rbErr == nil {
			s.recoverIndex(s.dir)
		}
		return fmt.Errorf("search: reopen swapped index: %w", err)
	}
	s.idx = reopened
	_ = os.RemoveAll(backup)
	return nil
}

// recoverIndex installs a usable index handle into s.idx (caller holds s.mu) after
// a failed swap, so the service never gets stuck with a nil "index is closed"
// handle. It tries to reopen the index at dir; if that fails it creates a fresh
// empty index at s.dir (a degraded but live state — a subsequent rebuild repopulates
// it). Best-effort: if even the fresh create fails, s.idx is left nil and the next
// rebuild/startup recovers.
func (s *Index) recoverIndex(dir string) {
	if reopened, err := bleve.Open(dir); err == nil {
		s.idx = reopened
		return
	}
	_ = os.RemoveAll(s.dir)
	if fresh, err := bleve.New(s.dir, buildMapping()); err == nil {
		s.idx = fresh
	}
}

// withIndex runs fn against the current index pointer while HOLDING the read lock
// for the WHOLE duration of fn, so a concurrent atomic swap (which takes the write
// lock to close the old handle and reopen the new one) cannot close the index
// out from under an in-flight query/upsert/delete (T-03-19). Holding only long
// enough to snapshot the pointer was a race: the snapshotted bleve handle could be
// Close()d by swapDir while fn was still reading it, surfacing "index closed".
// Many readers still proceed concurrently (RLock); only a swap blocks them, and
// only briefly. Bleve's own per-index operations are concurrency-safe, so holding
// the RLock across fn adds correctness without serializing readers against each
// other.
func (s *Index) withIndex(fn func(idx bleve.Index) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.idx == nil {
		return fmt.Errorf("search: index is closed")
	}
	return fn(s.idx)
}
