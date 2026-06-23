package pages

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"log/slog"

	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/okf"
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/search"
)

// Sentinel errors mapped to HTTP status codes by handlers_pages.go via
// errors.Is (mirrors the users package pattern).
var (
	// ErrPageNotFound is returned by Get/Save when the page .md does not exist.
	ErrPageNotFound = errors.New("page not found")
	// ErrStaleRevision is the optimistic-concurrency floor: Save rejects a write
	// whose base_revision no longer matches the current committed revision (409)
	// BEFORE any write is enqueued, so a concurrent edit is never silently lost.
	ErrStaleRevision = errors.New("stale revision")
	// ErrTitleRequired is returned by Create when the supplied title is blank.
	ErrTitleRequired = errors.New("title required")
)

// commitWaitTimeout bounds how long a user-facing mutation blocks waiting for
// its commit job to land on disk before falling back to async semantics. The
// commit itself completes well before the (optional, best-effort) remote push,
// so a hung push must not exceed this and break the save (VER-04): on timeout we
// log and return success, leaving the queued job to finish.
const commitWaitTimeout = 5 * time.Second

// enqueuer is the subset of *jobs.Worker the service needs. Defined as an
// interface so a test can inject a fake worker that captures the enqueued
// payload (TestPushFlagThreaded) without standing up the real drain goroutine.
//
// EnqueueAndWait enqueues a job and blocks until it is terminal: nil on done, a
// non-nil error on a job that reports failed, and jobs.ErrJobTimeout when the
// job neither completes nor fails within timeout. User-facing mutations route
// through this so the HTTP handler returns only after the commit is on disk (the
// file-tree refetch then sees the change instead of racing the worker).
type enqueuer interface {
	Enqueue(ctx context.Context, kind, payload string) error
	EnqueueAndWait(ctx context.Context, kind, payload string, timeout time.Duration) error
}

// reviser reads the committed blob revision of a path (the optimistic-
// concurrency token) AND the page's version history / old blobs (VER-02/03).
// *gitstore.GitStore satisfies it; kept as an interface so the service does not
// need a live git repo in every unit test. The History/ShowAt methods back the
// hidden-Git history view and forward-commit restore.
type reviser interface {
	BlobRevision(ctx context.Context, path string) (string, error)
	History(ctx context.Context, path string) ([]gitstore.Commit, error)
	ShowAt(ctx context.Context, ref, path string) ([]byte, error)
}

// Service is the page lifecycle service (SPEC §17.2/§17.3). Every mutation
// (Create/Save/CreateFolder) flows through the single-writer CommitJob (D-04) —
// the service NEVER writes the filesystem or shells out to git directly. All
// path I/O routes through repo.* (SEC-01 chokepoint).
type Service struct {
	repo   *repo.Repo
	git    reviser
	worker enqueuer
	db     *sql.DB

	// pushOnCommit records config.Git.PushOnCommit so every EnqueueCommit call
	// site can set commitPayload.Push = s.pushOnCommit. Push is wired but inert
	// until Plan 05 activates gitstore.Push; threading it here means Plan 05 only
	// flips the config value, no service change.
	pushOnCommit bool

	// now is the clock used for scaffolded/repaired timestamps. Overridable in
	// tests for deterministic frontmatter.
	now func() time.Time

	// keyed serializes same-target mutations so the precondition check (revision
	// for Save, uniqueness/existence for Create/CreateFolder) is atomic with the
	// commit. Without it, concurrent requests can both pass the check before either
	// commits, producing a silent lost update or path clobber (the single-writer
	// commit queue serializes git, but not the service-level decision). Held by
	// value so the zero value is usable; *Service is never copied (pointer methods).
	keyed keyedMutex
}

// NewService constructs the page service. pushOnCommit is config.Git.PushOnCommit
// (recorded so Create/Save/CreateFolder — and the later rename/move/trash/restore
// in subsequent plans — can thread Push onto the commit payload).
func NewService(r *repo.Repo, g *gitstore.GitStore, w *jobs.Worker, db *sql.DB, pushOnCommit bool) *Service {
	return &Service{
		repo:         r,
		git:          g,
		worker:       w,
		db:           db,
		pushOnCommit: pushOnCommit,
		now:          time.Now,
	}
}

// Page is the GET /pages response shape (SPEC §17.3): the parsed frontmatter
// region as raw YAML text, the opaque body, and the optimistic-concurrency
// revision.
type Page struct {
	Frontmatter string `json:"frontmatter"`
	Body        string `json:"body"`
	Revision    string `json:"revision"`
}

// lockMutation serializes every namespace-changing mutation (create, rename,
// move, delete, restore — for pages AND folders) on one global key, so each op's
// precondition check (uniqueness, existence, source-present) is atomic with its
// commit. The git layer is already single-writer; this just extends that
// serialization to cover the check, closing the structural TOCTOU races where two
// concurrent ops both pass their check and the second silently duplicates or
// clobbers. Content edits (Save) deliberately use a per-page key instead so
// independent pages still save concurrently. The returned func MUST be called once
// (defer it).
func (s *Service) lockMutation() func() { return s.keyed.lock("mutation") }

// Create slugifies title into a filename inside folder (D-12), scaffolds valid
// required frontmatter (D-13), and enqueues a single CommitJob that writes the
// new .md and cuts a hidden commit. It returns the repo-relative path of the new
// page. Collisions are suffixed (-2, -3, …) silently. Filenames/paths are never
// surfaced to the user — the caller maps the returned path to a tree node.
func (s *Service) Create(ctx context.Context, folder, title, user string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", ErrTitleRequired
	}

	// Serialize against all namespace mutations so uniquePath's existence check is
	// atomic with the commit: two creates of the same title (or a create racing a
	// rename/restore onto the same path) resolve to distinct paths (foo.md, foo-2.md)
	// instead of both picking foo.md and one silently clobbering the other.
	defer s.lockMutation()()

	path, err := s.uniquePath(folder, title)
	if err != nil {
		return "", err
	}

	// Scaffold a fresh doc with all required frontmatter present (D-13) so a new
	// page passes validation with no repair churn. Repair on an empty doc adds
	// every required field; we then set the generated title explicitly.
	doc := &okf.Doc{}
	okf.Repair(doc, s.now())
	okf.SetField(doc, okf.FieldTitle, title)

	bytesOut, err := doc.Emit()
	if err != nil {
		return "", fmt.Errorf("pages: emit new page: %w", err)
	}

	if err := s.enqueueWrite(ctx, path, bytesOut, "create", user); err != nil {
		return "", err
	}
	s.enqueueIndexUpsert(ctx, path)
	return path, nil
}

// Get reads the page at path, returning its frontmatter text, body, and current
// committed revision. ErrPageNotFound is returned when the .md does not exist.
func (s *Service) Get(ctx context.Context, path string) (Page, error) {
	exists, err := s.repo.Exists(path)
	if err != nil {
		return Page{}, err
	}
	if !exists {
		return Page{}, ErrPageNotFound
	}
	raw, err := s.repo.Read(path)
	if err != nil {
		return Page{}, fmt.Errorf("pages: read %q: %w", path, err)
	}
	doc, err := okf.Parse(raw)
	if err != nil {
		return Page{}, fmt.Errorf("pages: parse %q: %w", path, err)
	}
	rev, err := s.Revision(ctx, path)
	if err != nil {
		return Page{}, err
	}
	return Page{
		Frontmatter: string(doc.RawFront),
		Body:        string(doc.Body),
		Revision:    rev,
	}, nil
}

// Revision returns the current committed blob revision of path (the optimistic-
// concurrency token). Delegates to gitstore (git rev-parse HEAD:<path>).
func (s *Service) Revision(ctx context.Context, path string) (string, error) {
	return s.git.BlobRevision(ctx, path)
}

// Save writes a new version of path. It enforces the optimistic-concurrency
// floor FIRST: if baseRevision does not equal the current committed revision it
// returns ErrStaleRevision and enqueues NOTHING (so a stale save can never
// silently overwrite a concurrent edit — the 409 floor). Otherwise it parses the
// incoming bytes, repairs any missing required frontmatter (PAGE-09, byte-safe
// via okf.Repair), and enqueues a single edit CommitJob.
func (s *Service) Save(ctx context.Context, path, body, frontmatter, baseRevision, user string) error {
	// Serialize concurrent saves to the SAME page so the revision check is atomic
	// with the commit: a second writer cannot read the stale revision until the
	// first writer's commit has landed, at which point its base_revision no longer
	// matches and it correctly 409s instead of silently overwriting (the COLL-04
	// no-lost-update floor under true concurrency).
	unlock := s.keyed.lock("page:" + path)
	defer unlock()

	exists, err := s.repo.Exists(path)
	if err != nil {
		return err
	}
	if !exists {
		return ErrPageNotFound
	}

	// 409 floor — checked BEFORE any write is enqueued.
	current, err := s.Revision(ctx, path)
	if err != nil {
		return err
	}
	if current != baseRevision {
		return ErrStaleRevision
	}

	// Reassemble the incoming page (frontmatter region + body), parse, repair
	// missing required fields, and re-emit byte-stably.
	doc, err := okf.Parse(assemble(frontmatter, body))
	if err != nil {
		return fmt.Errorf("pages: parse incoming %q: %w", path, err)
	}
	okf.Repair(doc, s.now())
	out, err := doc.Emit()
	if err != nil {
		return fmt.Errorf("pages: emit %q: %w", path, err)
	}
	if err := s.enqueueWrite(ctx, path, out, "edit", user); err != nil {
		return err
	}
	s.enqueueIndexUpsert(ctx, path)
	return nil
}

// CreateFolder creates a folder under parent by seeding a blank index.md (NAV-03)
// through a CommitJob, so an otherwise-empty folder is representable in Git (an
// empty directory cannot be committed). name is slugified for the directory.
func (s *Service) CreateFolder(ctx context.Context, parent, name string, user string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrTitleRequired
	}
	dir := slugify(name)
	// A non-empty but punctuation/CJK-only name (e.g. "!!!", "##") slugs to "".
	// Without this guard dir would be "" → indexPath "/index.md" (absolute), which
	// the resolver rejects as a generic 500. Return the clean 400 instead, mirroring
	// the empty-title contract (WR-07; parallels uniquePath's "untitled" fallback).
	if dir == "" {
		return ErrTitleRequired
	}
	if parent != "" {
		dir = strings.TrimSuffix(parent, "/") + "/" + dir
	}
	indexPath := dir + "/index.md"

	// Serialize against all namespace mutations so a concurrent folder create cannot
	// clobber a page (or another folder's index.md) at a colliding path.
	defer s.lockMutation()()

	doc := &okf.Doc{}
	okf.Repair(doc, s.now())
	okf.SetField(doc, okf.FieldTitle, name)
	out, err := doc.Emit()
	if err != nil {
		return fmt.Errorf("pages: emit folder index: %w", err)
	}
	if err := s.enqueueWrite(ctx, indexPath, out, "create", user); err != nil {
		return err
	}
	s.enqueueIndexUpsert(ctx, indexPath)
	return nil
}

// enqueueWrite builds a single-file commit payload and enqueues it through the
// single-writer worker, threading the configured Push flag (Plan 05 activation).
func (s *Service) enqueueWrite(ctx context.Context, path string, data []byte, action, user string) error {
	// Validate the path through the resolver before it is staged (the CommitJob
	// re-resolves as a backstop, but failing here gives a clean error).
	if _, err := s.repo.Resolve(path); err != nil {
		return err
	}
	p := commitPayload{
		Writes: []fileWrite{{Path: path, Bytes: data}},
		Spec: gitstore.CommitSpec{
			Paths:   []string{path},
			Message: commitSubject(action, path),
			User:    user,
			Action:  action,
			Source:  "web-ui",
		},
		Push: s.pushOnCommit,
	}
	return EnqueueCommit(ctx, s.worker, p)
}

// enqueueIndexUpsert fires a FIRE-AND-FORGET search.KindIndex upsert for a page
// path so the live index tracks the change without a restart (CONTEXT Area 4
// reindex triggers). It is called from the HTTP-handler goroutine AFTER the
// commit has been enqueued/landed (so the index reflects committed state). It
// uses worker.Enqueue (never EnqueueAndWait) so a search-freshness job never adds
// latency to a save; correctness is guaranteed by the idempotent rebuild +
// startup drift check, so a dropped enqueue is logged at Warn and swallowed
// (T-03-20 accept).
func (s *Service) enqueueIndexUpsert(ctx context.Context, pagePath string) {
	if err := s.worker.Enqueue(ctx, search.KindIndex, search.UpsertPagePayload(pagePath)); err != nil {
		slog.WarnContext(ctx, "pages: failed to enqueue search index upsert (rebuild backstop reconciles)",
			slog.String("path", pagePath), slog.String("error", err.Error()))
	}
}

// enqueueIndexDelete fires a FIRE-AND-FORGET search.KindIndex delete for a page
// path (removing the page doc AND its heading sub-docs via the handler's delete
// branch). Same context/contract as enqueueIndexUpsert.
func (s *Service) enqueueIndexDelete(ctx context.Context, pagePath string) {
	if err := s.worker.Enqueue(ctx, search.KindIndex, search.DeletePagePayload(pagePath)); err != nil {
		slog.WarnContext(ctx, "pages: failed to enqueue search index delete (rebuild backstop reconciles)",
			slog.String("path", pagePath), slog.String("error", err.Error()))
	}
}

// commitSubject renders a short human-readable subject for the hidden commit.
func commitSubject(action, path string) string {
	switch action {
	case "create":
		return "Create " + path
	case "edit":
		return "Edit " + path
	default:
		return action + " " + path
	}
}

// uniquePath slugifies title into a .md filename inside folder, appending a
// numeric suffix (-2, -3, …) until the path is free (D-12 collision handling).
func (s *Service) uniquePath(folder, title string) (string, error) {
	base := slugify(title)
	if base == "" {
		base = "untitled"
	}
	dir := strings.Trim(strings.TrimSpace(folder), "/")
	build := func(name string) string {
		if dir == "" {
			return name + ".md"
		}
		return dir + "/" + name + ".md"
	}

	candidate := build(base)
	if _, err := s.repo.Resolve(candidate); err != nil {
		return "", err
	}
	for n := 2; ; n++ {
		exists, err := s.repo.Exists(candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
		candidate = build(fmt.Sprintf("%s-%d", base, n))
	}
}

// slugify lowercases title, replaces whitespace runs with a single hyphen, and
// strips characters outside [a-z0-9-]. Path-unsafe inputs (.. / absolute / NUL)
// are neutralized because every disallowed rune is dropped; the result is then
// re-validated through repo.Resolve by the caller.
func slugify(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	var b strings.Builder
	prevHyphen := false
	for _, r := range title {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		case unicode.IsSpace(r), r == '-', r == '_', r == '/', r == '.':
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		default:
			// Drop everything else (punctuation, NUL, control chars).
		}
	}
	return strings.Trim(b.String(), "-")
}

// assemble reconstructs an OKF source from a frontmatter region and a body. It is
// the AUTHORITATIVE assembler — the exact bytes Save parses/repairs/writes — and the
// only such function in the codebase (the agent service and HTTP layer previously
// carried near-duplicate currentSource/assembleSource variants that drifted on
// trim/newline rules; those were removed and any caller that needs "the bytes Save
// writes" must route through AssembleSource so the invariant is single-sourced,
// WR-05). When frontmatter is non-empty it is wrapped in --- fences; otherwise the
// body is returned as-is (Repair promotes it to a frontmatter region).
func assemble(frontmatter, body string) []byte {
	fm := strings.TrimSpace(frontmatter)
	if fm == "" {
		return []byte(body)
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fm)
	if !strings.HasSuffix(fm, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("---\n")
	b.WriteString(body)
	return []byte(b.String())
}

// AssembleSource is the exported canonical assembler (WR-05): it returns the exact
// OKF source bytes Save assembles from a frontmatter region + body, so any caller
// outside this package ("the bytes Save writes") shares ONE implementation and can
// never drift from the writer. Delegates to the authoritative assemble.
func AssembleSource(frontmatter, body string) []byte {
	return assemble(frontmatter, body)
}
