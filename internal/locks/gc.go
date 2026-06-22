package locks

import (
	"context"

	"github.com/postfix/okworkspace/internal/jobs"
)

// KindGC is the job-kind string for the lock reaper (like pages.KindCommit /
// search.KindIndex). A time.Ticker in main.go fire-and-forget enqueues this on a
// cadence inside the TTL envelope so a crashed/idle session's lock is reaped
// instead of pinning a page forever.
const KindGC = "lock_gc"

// GC reaps every expired lock file: it walks all locks on disk and Removes those
// whose ExpiresAt is at or before now (idempotent — a concurrently-deleted file
// is tolerated by repo.Remove). Each file is removed through repo.Remove by its
// repo-relative path (the lock record carries no page path, so GC uses the path
// the walk found it at). It is safe to run repeatedly; Get already treats an
// expired lock as absent, so GC only frees the on-disk file. The first hard
// error aborts; per-file not-exist is not an error.
func (s *Service) GC(ctx context.Context) error {
	entries, err := s.walk()
	if err != nil {
		return err
	}
	now := s.now()
	for _, e := range entries {
		if now.After(e.lock.ExpiresAt) {
			if rerr := s.repo.Remove(e.rel); rerr != nil {
				return rerr
			}
		}
	}
	return nil
}

// GCHandler adapts Service.GC to a jobs.Handler with an empty payload (the GC
// scans everything; there is nothing to encode), mirroring how the search index
// handler is fed a payload-driven kind. Registered in main.go before the worker
// starts.
func GCHandler(store *Service) jobs.Handler {
	return func(ctx context.Context, _ string) error {
		return store.GC(ctx)
	}
}
