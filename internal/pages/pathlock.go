package pages

import "sync"

// keyedMutex serializes mutations that share a key so a check-then-commit
// sequence is atomic against concurrent callers on the same key. It closes the
// TOCTOU windows in Save (revision check → commit) and Create/CreateFolder
// (uniqueness/existence check → commit): without it, two concurrent requests can
// both pass the precondition before either commits, and the single-writer commit
// queue then applies both — a silent lost update (last write wins, no 409) or a
// silent path clobber (two creates at one path).
//
// Keys are namespaced by the caller ("page:<path>" for a page write, "dir:<dir>"
// for a structural add to a directory) so writes that cannot collide do not block
// each other. Entries are reference-counted and removed when idle, so the map is
// bounded by the number of pages/folders under concurrent mutation, not by total
// history.
type keyedMutex struct {
	mu sync.Mutex
	m  map[string]*keyedEntry
}

type keyedEntry struct {
	mu   sync.Mutex
	refs int
}

// lock acquires the lock for key and returns an unlock function that MUST be
// called exactly once (defer it). The reference count is bumped under the map
// mutex BEFORE the per-key mutex is taken, so an entry a waiter is blocked on can
// never be deleted out from under it. The map is lazily created so the zero
// keyedMutex is usable (a Service built by struct literal in tests works without
// a constructor).
func (k *keyedMutex) lock(key string) func() {
	k.mu.Lock()
	if k.m == nil {
		k.m = make(map[string]*keyedEntry)
	}
	e := k.m[key]
	if e == nil {
		e = &keyedEntry{}
		k.m[key] = e
	}
	e.refs++
	k.mu.Unlock()

	e.mu.Lock()

	return func() {
		e.mu.Unlock()
		k.mu.Lock()
		e.refs--
		if e.refs == 0 {
			delete(k.m, key)
		}
		k.mu.Unlock()
	}
}
