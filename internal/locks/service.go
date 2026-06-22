package locks

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/postfix/okworkspace/internal/repo"
)

// ErrNotHolder is returned by Refresh when the caller's SessionID does not match
// the live on-disk holder. A non-holder must NOT silently take over — that is
// Force's job. Callers (a later HTTP slice) map this to held-by-other.
var ErrNotHolder = errors.New("locks: not the lock holder")

// lockSubtree is the mirror-the-tree root every lock file lives under. It is an
// application constant (never user input), so it is safe to join here; the page
// path appended to it is re-validated by repo.Resolve on every Read/Write/Remove.
const lockSubtree = ".okf-workspace/locks"

// AcquireResult reports the outcome of an Acquire: the lock was taken (or
// refreshed by the same session), or a DIFFERENT live session already holds it.
type AcquireResult string

const (
	// ResultAcquired means owner now holds the lock (fresh, expired-takeover, or
	// same-session refresh).
	ResultAcquired AcquireResult = "acquired"
	// ResultHeldByOther means a different live session holds the lock; the
	// existing lock was left untouched and is returned to the caller.
	ResultHeldByOther AcquireResult = "held-by-other"
)

// Service is the soft-lock store (COLL-02). It mirrors pages.Service: a *repo.Repo
// for all path-safe I/O, an injected clock for deterministic TTL/expiry tests,
// and the lock TTL. The service NEVER touches the filesystem directly — every
// lock-file read/write/remove routes through s.repo.* → repo.Resolve (SEC-01),
// so a traversal-shaped page path can never escape `.okf-workspace/locks/`.
type Service struct {
	repo *repo.Repo
	ttl  time.Duration

	// now is the clock used for LockedAt/ExpiresAt and expiry checks. Overridable
	// in tests for deterministic expiry assertions (same idiom as pages.Service).
	now func() time.Time
}

// NewService constructs the lock store rooted at r with the given lock TTL
// (ExpiresAt = acquire-time + ttl). now defaults to time.Now; tests overwrite it.
func NewService(r *repo.Repo, ttl time.Duration) *Service {
	return &Service{repo: r, ttl: ttl, now: time.Now}
}

// lockPath maps a page path to its mirror-the-tree lock-file path. The page path
// arrives pre-validated by the handler (a later slice) AND repo.Resolve
// re-validates on every Read/Write/Remove, so this plain join can never let a
// lock escape the subtree. NEVER call os.* on the result.
func (s *Service) lockPath(pagePath string) string {
	return lockSubtree + "/" + pagePath + ".lock"
}

// Get returns the current LIVE lock for pagePath. A missing file, a torn/garbage
// write (any unmarshal error), or a lock past ExpiresAt all report (Lock{},
// false, nil): an expired or unreadable lock is treated as no live lock (the
// torn-read fallback self-heals on the next heartbeat write).
func (s *Service) Get(ctx context.Context, pagePath string) (Lock, bool, error) {
	raw, err := s.repo.Read(s.lockPath(pagePath))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Lock{}, false, nil
		}
		return Lock{}, false, err
	}
	var l Lock
	if uerr := json.Unmarshal(raw, &l); uerr != nil {
		// Torn / non-atomic write or garbage: treat as no live lock (it self-heals
		// on the next holder heartbeat). Accepted per the threat register (T-05-05).
		return Lock{}, false, nil
	}
	if s.now().After(l.ExpiresAt) {
		return Lock{}, false, nil
	}
	return l, true, nil
}

// Acquire takes (or refreshes) the lock for owner. If a DIFFERENT live session
// already holds it, the existing lock is returned with ResultHeldByOther and is
// NEVER overwritten. Otherwise (no live lock, expired, or the same session) a
// fresh lock is written and ResultAcquired is returned.
func (s *Service) Acquire(ctx context.Context, pagePath string, owner Owner) (Lock, AcquireResult, error) {
	cur, live, err := s.Get(ctx, pagePath)
	if err != nil {
		return Lock{}, "", err
	}
	if live && cur.SessionID != owner.SessionID {
		// A different live session holds it — do not overwrite.
		return cur, ResultHeldByOther, nil
	}
	l, err := s.write(pagePath, owner)
	if err != nil {
		return Lock{}, "", err
	}
	return l, ResultAcquired, nil
}

// Refresh is the holder heartbeat: it bumps ExpiresAt to now+ttl. ONLY the
// current live holder (matching SessionID) may refresh; a non-holder gets
// ErrNotHolder and the on-disk lock is left untouched (takeover is Force's job).
func (s *Service) Refresh(ctx context.Context, pagePath string, owner Owner) error {
	cur, live, err := s.Get(ctx, pagePath)
	if err != nil {
		return err
	}
	if !live || cur.SessionID != owner.SessionID {
		return ErrNotHolder
	}
	_, err = s.write(pagePath, owner)
	return err
}

// Force unconditionally takes ownership of the lock for owner. It is
// LOCK-FILE-ONLY: it never reads, writes, or references the page body or any
// revision — force-edit (who may type) is decoupled from save authority (is the
// write safe), which lives untouched in pages.Save.
func (s *Service) Force(ctx context.Context, pagePath string, owner Owner) (Lock, error) {
	return s.write(pagePath, owner)
}

// Release deletes the lock for pagePath ONLY when the on-disk SessionID matches
// sessionID, so a session that lost the lock to a TTL takeover cannot clobber the
// new holder. Idempotent: a missing lock file is not an error.
func (s *Service) Release(ctx context.Context, pagePath, sessionID string) error {
	raw, err := s.repo.Read(s.lockPath(pagePath))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var l Lock
	if uerr := json.Unmarshal(raw, &l); uerr != nil {
		// Garbage on disk has no owner we can match; leave it for GC rather than
		// deleting a file we cannot attribute to this caller.
		return nil
	}
	if l.SessionID != sessionID {
		return nil
	}
	return s.repo.Remove(s.lockPath(pagePath))
}

// List returns every lock file currently on disk (live or expired — GC uses the
// expired ones). A non-existent locks subtree yields an empty slice, not an
// error. Torn/garbage files are skipped. Resolution of the walk root routes
// through repo.Resolve so the walk can never start outside the repo.
func (s *Service) List(ctx context.Context) ([]Lock, error) {
	root, err := s.repo.Resolve(lockSubtree)
	if err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(root); statErr != nil {
		if os.IsNotExist(statErr) {
			return nil, nil
		}
		return nil, statErr
	}
	var locks []Lock
	walkErr := filepath.WalkDir(root, func(abs string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() || !strings.HasSuffix(abs, ".lock") {
			return nil
		}
		raw, rerr := os.ReadFile(abs)
		if rerr != nil {
			// A single unreadable lock must not abort the whole scan (GC/presence
			// are best-effort over files) — skip it.
			return nil
		}
		var l Lock
		if json.Unmarshal(raw, &l) != nil {
			return nil // torn/garbage — skip
		}
		locks = append(locks, l)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return locks, nil
}

// EditorsFor returns the live holders of pagePath's lock for a presence snapshot,
// with self excluded by connection id. One lock per page, so this is the single
// holder when live (and empty once it expires). You is true when the holder's
// SessionID matches connID, so the caller never sees their own lock as a remote
// editor (two-tabs-one-browser).
func (s *Service) EditorsFor(ctx context.Context, pagePath, connID string) ([]Editor, error) {
	l, live, err := s.Get(ctx, pagePath)
	if err != nil {
		return nil, err
	}
	if !live {
		return nil, nil
	}
	return []Editor{{Username: l.Username, You: l.SessionID == connID}}, nil
}

// write marshals a fresh lock for owner (LockedAt = now, ExpiresAt = now+ttl) and
// persists it through the repo (parent dirs auto-created, path re-validated). It
// is the single place a lock file is written — Acquire/Refresh/Force all funnel
// through it so the on-disk shape and TTL stamping stay consistent.
func (s *Service) write(pagePath string, owner Owner) (Lock, error) {
	now := s.now()
	l := Lock{
		Username:  owner.Username,
		UserID:    owner.UserID,
		SessionID: owner.SessionID,
		LockedAt:  now,
		ExpiresAt: now.Add(s.ttl),
	}
	data, err := json.Marshal(l)
	if err != nil {
		return Lock{}, err
	}
	if werr := s.repo.Write(s.lockPath(pagePath), data); werr != nil {
		return Lock{}, werr
	}
	return l, nil
}
