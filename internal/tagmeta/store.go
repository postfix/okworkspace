package tagmeta

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/postfix/okworkspace/internal/okf"
	"github.com/postfix/okworkspace/internal/pages"
	"github.com/postfix/okworkspace/internal/repo"
)

// tagsDir is the single top-level folder whose flat *.md files are the
// per-tag metadata source of truth (D-01). Mirrors pages.templatesDir.
const tagsDir = "Tags"

// Store owns the on-disk tag-metadata store + its rebuildable SQLite cache.
// It holds a *repo.Repo (the SEC-01 resolver path for file I/O), a *sql.DB
// (the derived cache), and a single-writer commit enqueue (the SAME path
// every page save uses — no second commit path).
type Store struct {
	repo      *repo.Repo
	db        *sql.DB
	commit    commitEnqueuer
	push      bool
	commitUsr string // actor threaded onto the commit spec (the authenticated user)
}

// SetUser threads the authenticated actor onto the next commit spec. Called by
// the admin handler before SetMeta so the git commit records the real user.
func (s *Store) SetUser(user string) { s.commitUsr = user }

// commitEnqueuer is the subset of *jobs.Worker the store needs to enqueue a
// single-writer commit. It reuses the exported pages.CommitEnqueuer interface
// (*jobs.Worker satisfies it) so the tag-meta write path IS the page-save commit
// path — never a second commit kind.
type commitEnqueuer = pages.CommitEnqueuer

// NewStore constructs the tag-metadata store. db may be nil in a no-cache harness
// (the on-disk files are still truth; cache reads return empty). commit may be
// nil when no write path is exercised. push threads config.Git.PushOnCommit.
func NewStore(r *repo.Repo, db *sql.DB, commit commitEnqueuer, push bool) *Store {
	return &Store{repo: r, db: db, commit: commit, push: push}
}

// SetRepo wires the content repo (used by main.go after the store is opened).
func (s *Store) SetRepo(r *repo.Repo) { s.repo = r }

// GetMeta reads one tag's metadata from the SQLite cache. Returns (Meta{}, false)
// when there is no row (graceful absent-store behavior). A nil db returns false.
func (s *Store) GetMeta(ctx context.Context, tag string) (Meta, bool) {
	if s.db == nil {
		return Meta{}, false
	}
	var m Meta
	var color, desc, parent string
	err := s.db.QueryRowContext(ctx,
		`SELECT tag, color, description, parent FROM tag_meta WHERE tag = ?`, tag).
		Scan(&m.Tag, &color, &desc, &parent)
	if err != nil {
		return Meta{}, false
	}
	m.Color = color
	m.Description = desc
	m.Parent = parent
	return m, true
}

// AllMeta reads all metadata rows from the SQLite cache, sorted by tag. A nil db
func (s *Store) AllMeta(ctx context.Context) ([]Meta, error) {
	if s.db == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT tag, color, description, parent FROM tag_meta ORDER BY tag`)
	if err != nil {
		return nil, fmt.Errorf("tagmeta: read all: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Meta
	for rows.Next() {
		var m Meta
		if err := rows.Scan(&m.Tag, &m.Color, &m.Description, &m.Parent); err != nil {
			return nil, fmt.Errorf("tagmeta: scan: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tagmeta: rows: %w", err)
	}
	return out, nil
}

// metaFile renders the on-disk Markdown shape for a tag's metadata: YAML
// frontmatter (tag, color, description, parent) + a `# {tag}` heading body for
// human-readability. The body is free-form documentation (ignored by the cache).
// The parent line is always written (empty = root) so rebuild reproduces it.
func metaFile(tag, color, description, parent string) []byte {
	return []byte("---\n" +
		"tag: " + tag + "\n" +
		"color: \"" + color + "\"\n" +
		"description: \"" + description + "\"\n" +
		"parent: " + parent + "\n" +
		"---\n\n" +
		"# " + tag + "\n")
}

// tagPath returns the repo-relative path for a tag's metadata file.
func tagPath(tag string) string {
	return tagsDir + "/" + tag + ".md"
}
// SetMeta validates inputs, writes Tags/{tag}.md via repo.Write + the single-writer
// commit path (EnqueueCommit — the SAME path page saves use), then upserts the
// cache row. When color and description are BOTH empty, the file is DELETED (absent
// = graceful: a tag with no metadata renders as today). The parent field forms a
// cycle-safe hierarchy: checkCycle rejects a tag being its own ancestor BEFORE
// the write (D-02a — a cycle-producing parent is never written to disk).
func (s *Store) SetMeta(ctx context.Context, tag, color, description, parent string) error {
	if s.repo == nil {
		return fmt.Errorf("tagmeta: no content repo wired")
	}
	if err := ValidateColor(color); err != nil {
		return err
	}
	if err := ValidateDescription(description); err != nil {
		return err
	}
	if err := ValidateParent(parent); err != nil {
		return err
	}
	// Cycle check BEFORE the write (D-02a): a cycle-producing parent is rejected,
	// never written to disk (which would create an un-rebuildable on-disk cycle).
	if err := s.CheckCycle(ctx, tag, parent); err != nil {
		return err
	}
	rel := tagPath(tag)

	// When all three metadata fields are empty, DELETE the file so absent = graceful.
	if color == "" && description == "" && parent == "" {
		return s.clearFileAndCache(ctx, rel, tag)
	}

	data := metaFile(tag, color, description, parent)
	if err := s.writeAndCommit(ctx, rel, data, "tag-meta"); err != nil {
		return err
	}
	// Upsert the cache row so an immediate read sees the new metadata.
	return s.upsertCache(ctx, Meta{Tag: tag, Color: color, Description: description, Parent: parent})
}

// ClearMeta removes a tag's metadata (deletes the Tags/{tag}.md file + the cache
// row). Equivalent to SetMeta with empty color + description + parent (root).
func (s *Store) ClearMeta(ctx context.Context, tag string) error {
	if s.repo == nil {
		return fmt.Errorf("tagmeta: no content repo wired")
	}
	return s.SetMeta(ctx, tag, "", "", "")
}

// clearFileAndCommit deletes the on-disk file via a commit-removes payload and
// deletes the cache row.
func (s *Store) clearFileAndCache(ctx context.Context, rel, tag string) error {
	// If the file doesn't exist, just drop the cache row (idempotent clear).
	exists, err := s.repo.Exists(rel)
	if err != nil {
		return fmt.Errorf("tagmeta: stat %q: %w", rel, err)
	}
	if exists {
		if err := s.removeAndCommit(ctx, rel, "tag-meta-clear"); err != nil {
			return err
		}
	}
	return s.deleteCache(ctx, tag)
}

// writeAndCommit writes one file through the single-writer commit path (the SAME
// pages.EnqueueFileCommit path every page save uses — no second commit path).
func (s *Store) writeAndCommit(ctx context.Context, rel string, data []byte, action string) error {
	if s.commit == nil {
		// No commit worker (e.g. a no-commit harness): write directly so the
		// file lands on disk and the cache stays honest. Tests that exercise the
		// commit path inject a real worker.
		if err := s.repo.Write(rel, data); err != nil {
			return fmt.Errorf("tagmeta: write %q: %w", rel, err)
		}
		return nil
	}
	if err := pages.EnqueueFileCommit(ctx, s.commit, rel, data, action, s.commitUsr, s.push); err != nil {
		return fmt.Errorf("tagmeta: commit %q: %w", rel, err)
	}
	return nil
}

// removeAndCommit deletes one file through the single-writer commit path.
func (s *Store) removeAndCommit(ctx context.Context, rel, action string) error {
	if s.commit == nil {
		if err := s.repo.Remove(rel); err != nil {
			return fmt.Errorf("tagmeta: remove %q: %w", rel, err)
		}
		return nil
	}
	if err := pages.EnqueueRemoveCommit(ctx, s.commit, rel, action, s.commitUsr, s.push); err != nil {
		return fmt.Errorf("tagmeta: commit remove %q: %w", rel, err)
	}
	return nil
}

// upsertCache inserts or replaces one tag_meta row (including parent).
func (s *Store) upsertCache(ctx context.Context, m Meta) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tag_meta (tag, color, description, parent) VALUES (?, ?, ?, ?)
		 ON CONFLICT(tag) DO UPDATE SET color=excluded.color, description=excluded.description, parent=excluded.parent`,
		m.Tag, m.Color, m.Description, m.Parent)
	if err != nil {
		return fmt.Errorf("tagmeta: upsert cache: %w", err)
	}
	return nil
}

// deleteCache removes one tag_meta row.
func (s *Store) deleteCache(ctx context.Context, tag string) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM tag_meta WHERE tag = ?`, tag)
	if err != nil {
		return fmt.Errorf("tagmeta: delete cache: %w", err)
	}
	return nil
}

// RebuildTagMeta walks Tags/*.md via repo.Tree() + the Tags/ prefix filter +
// repo.Read + okf.Parse, parses each file's frontmatter, and repopulates the
// SQLite cache in ONE transaction (DELETE all + re-insert). This is the parity
// harness: delete-cache → rebuild = identical rows. A repo with no Tags/ folder
// yields an empty cache (graceful — not an error). Mirrors graph.RebuildGraph.
func (s *Store) RebuildTagMeta(ctx context.Context) error {
	if s.repo == nil {
		return fmt.Errorf("tagmeta: rebuild requires a content repo")
	}
	items, err := s.repo.Tree()
	if err != nil {
		return fmt.Errorf("tagmeta: walk repo: %w", err)
	}

	// Collect parsed metadata from every Tags/*.md file.
	var metas []Meta
	for _, it := range items {
		if it.IsDir || !strings.HasSuffix(it.Path, ".md") {
			continue
		}
		if !isTagMetaFile(it.Path) {
			continue
		}
		raw, rerr := s.repo.Read(it.Path)
		if rerr != nil {
			// A transient/IO read error on ONE file must not abort the whole rebuild
			// (mirrors graph/search rebuild resilience — WR-06): skip it.
			slog.WarnContext(ctx, "tagmeta: skipping unreadable file during rebuild",
				slog.String("path", it.Path), slog.String("error", rerr.Error()))
			continue
		}
		m, perr := parseMetaFile(raw)
		if perr != nil {
			slog.WarnContext(ctx, "tagmeta: skipping unparseable file during rebuild",
				slog.String("path", it.Path), slog.String("error", perr.Error()))
			continue
		}
		metas = append(metas, m)
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].Tag < metas[j].Tag })

	if s.db == nil {
		return nil // no-cache harness: the walk above is still the backstop
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("tagmeta: begin rebuild tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM tag_meta`); err != nil {
		return fmt.Errorf("tagmeta: clear cache: %w", err)
	}
	for _, m := range metas {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tag_meta (tag, color, description, parent) VALUES (?, ?, ?, ?)`,
			m.Tag, m.Color, m.Description, m.Parent); err != nil {
			return fmt.Errorf("tagmeta: rebuild insert %q: %w", m.Tag, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("tagmeta: commit rebuild: %w", err)
	}
	return nil
}

// parseMetaFile parses the frontmatter of a Tags/{tag}.md file into a Meta
// record. The tag key is authoritative; color/description/parent are optional
// scalars (absent parent = root, "" = root).
func parseMetaFile(raw []byte) (Meta, error) {
	doc, err := okf.Parse(raw)
	if err != nil {
		return Meta{}, fmt.Errorf("parse: %w", err)
	}
	tag := okf.Field(doc, "tag")
	if tag == "" {
		return Meta{}, fmt.Errorf("missing tag field")
	}
	return Meta{
		Tag:         tag,
		Color:       okf.Field(doc, "color"),
		Description: okf.Field(doc, "description"),
		Parent:      okf.Field(doc, "parent"),
	}, nil
}

// isTagMetaFile reports whether a repo-relative path lives under the Tags/
// folder (the tag-metadata store). Mirrors graph.isTrashed / search.isTrashed.
func isTagMetaFile(path string) bool {
	return path == tagsDir || strings.HasPrefix(path, tagsDir+"/")
}

// maxCycleDepth is the safety valve against a corrupted store with a pre-existing
// cycle. The hierarchy is shallow for a ~5-user wiki, so a walk exceeding this is
// treated as corruption (an un-rebuildable cycle), not a legitimate chain.
const maxCycleDepth = 100

// CheckCycle rejects a cycle-producing parent BEFORE the write (D-02a). It walks
// the parent chain from `parent` upward via the cache (in sync after a rebuild):
//   - parent == tag → self-parenting, rejected directly.
//   - parent == "" → root, valid (no cycle).
//   - walk visits parent's parent, etc. If `tag` is revisited → cycle (reject).
//   - if the chain exceeds maxCycleDepth → corrupted store (reject).
//   - reaching an empty parent (root) or a tag with no metadata → valid (no cycle;
//     a non-existent parent is treated as root per D-02b).
// The check is O(depth) — the hierarchy is shallow, so a simple walk suffices.
// Exported so the server handler can call it before the write (D-02a).
func (s *Store) CheckCycle(ctx context.Context, tag, parent string) error {
	if parent == "" {
		return nil // root: no cycle possible
	}
	if parent == tag {
		return fmt.Errorf("tagmeta: cycle — %q cannot be its own parent", tag)
	}
	// Walk the parent chain from `parent` upward. We revisit `tag` → cycle.
	cur := parent
	for range maxCycleDepth {
		m, ok := s.GetMeta(ctx, cur)
		if !ok {
			return nil // parent not in store → treated as root (D-02b): no cycle
		}
		if m.Parent == "" {
			return nil // reached a root: no cycle
		}
		if m.Parent == tag {
			return fmt.Errorf("tagmeta: cycle — %q is already an ancestor of %q", tag, parent)
		}
		cur = m.Parent
	}
	return fmt.Errorf("tagmeta: cycle — parent chain exceeds %d (corrupted store)", maxCycleDepth)
}