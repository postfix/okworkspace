// Package tagsweep is the backend fan-out + staging spine for the bulk tagging
// sweep (Phase 12, TAG-05). An admin starts a sweep that enqueues one
// KindTagSuggest job PER target page on the EXISTING single-drain jobs worker;
// each job calls the Phase-11 agent.SuggestTags mode for ONE page and STAGES the
// result into the tag_suggestions table with status 'pending'. The sweep WRITES
// NO frontmatter and triggers NO commit — a tag is written to a file ONLY via an
// explicit human-approved apply (Wave 2). The serial single drain is the natural
// LLM rate-limiter (PITFALLS Pitfall 6) — there is NO parallel LLM caller here.
//
// tag_suggestions is operational/derived STAGING data (migration 0010), NOT page
// content and NEVER the source of truth: re-running a sweep reproduces the rows.
// This package owns the staging persistence + the untagged/all target
// enumeration; it mirrors graph.Store's "Store over the shared *sql.DB + SetRepo"
// shape and graph.livePages's exact live-page skip rules so the two agree.
package tagsweep

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/postfix/okworkspace/internal/repo"
)

// trashPrefix is the working-tree location deleted pages move into; pages under
// it are excluded from sweep targets (live pages only). Mirrors graph.trashPrefix
// / search.trashPrefix (kept local to avoid importing those packages).
const trashPrefix = ".okf-workspace/trash"

// isTrashed reports whether a workspace-relative path lives under the trash dir.
func isTrashed(path string) bool {
	return path == trashPrefix || strings.HasPrefix(path, trashPrefix+"/")
}

// Suggestion is one staged tag proposal: the normalized tag plus whether it
// already exists in the workspace vocabulary (the existing-vs-new flag Phase-11
// SuggestTags computed). Stored verbatim in the suggestions JSON column.
type Suggestion struct {
	Tag      string `json:"tag"`
	Existing bool   `json:"existing"`
}

// PendingEntry is one page's pending suggestion as returned by ListPending: the
// page path, its staged suggestions, and the base revision captured at suggest
// time (the optimistic-concurrency token the later apply re-checks).
type PendingEntry struct {
	PagePath     string       `json:"page_path"`
	Suggestions  []Suggestion `json:"suggestions"`
	BaseRevision string       `json:"base_revision"`
}

// Store is the tag-sweep staging store over the shared *sql.DB (where the
// tag_suggestions table lives) plus the content repo (live-page enumeration for
// target selection). repo is attached after open via SetRepo, mirroring
// graph.Store's shape.
type Store struct {
	db   *sql.DB
	repo *repo.Repo
}

// OpenStore constructs a Store over the shared *sql.DB. The content repo is
// attached afterwards via SetRepo (mirrors graph.OpenStore), keeping the
// constructor a simple single-arg call for the startup wiring path.
func OpenStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// SetRepo attaches the content repo used by Targets for live-page enumeration.
func (s *Store) SetRepo(r *repo.Repo) { s.repo = r }

// StagePending stages a pending suggestion row for pagePath, superseding any
// prior pending row for the same page (last suggestion wins) in one transaction:
// DELETE the existing pending row, then INSERT the new one with suggestions =
// json.Marshal(sugg). The partial-unique index on (page_path) WHERE
// status='pending' enforces the one-pending-row-per-page invariant.
//
// The store TRUSTS its caller's pagePath: the only caller is the KindTagSuggest
// handler, whose path originates from Targets' repo.Tree() enumeration (or, on
// the HTTP layer, is validated there). The store does not re-validate identifiers.
func (s *Store) StagePending(ctx context.Context, pagePath string, sugg []Suggestion, baseRevision string) error {
	if s.db == nil {
		return fmt.Errorf("tagsweep: StagePending requires a database")
	}
	raw, err := json.Marshal(sugg)
	if err != nil {
		return fmt.Errorf("tagsweep: marshal suggestions for %q: %w", pagePath, err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM tag_suggestions WHERE page_path=? AND status='pending'`, pagePath); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO tag_suggestions (page_path, suggestions, base_revision, status)
		 VALUES (?, ?, ?, 'pending')`, pagePath, string(raw), baseRevision); err != nil {
		return err
	}
	return tx.Commit()
}

// ListPending returns one entry per page with a pending row, carrying the staged
// suggestions + base_revision, ordered deterministically by page_path. It drives
// the admin review-queue read endpoint.
func (s *Store) ListPending(ctx context.Context) ([]PendingEntry, error) {
	if s.db == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT page_path, suggestions, base_revision FROM tag_suggestions
		 WHERE status='pending' ORDER BY page_path`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []PendingEntry{}
	for rows.Next() {
		var pagePath, raw, baseRev string
		if err := rows.Scan(&pagePath, &raw, &baseRev); err != nil {
			return nil, err
		}
		var sugg []Suggestion
		if err := json.Unmarshal([]byte(raw), &sugg); err != nil {
			return nil, fmt.Errorf("tagsweep: unmarshal suggestions for %q: %w", pagePath, err)
		}
		out = append(out, PendingEntry{PagePath: pagePath, Suggestions: sugg, BaseRevision: baseRev})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// GetPending returns the single PENDING staged entry for pagePath (suggestions +
// the STAGED base_revision the suggestion was captured against). The found bool is
// false (with a nil error and a zero PendingEntry) when the page has no pending
// row. The approve handler reads the staged base_revision from HERE — it is the
// server's record of which revision a suggestion was made against, so the apply
// re-checks against that staged value and NEVER trusts a client-supplied revision
// (T-12-07). pagePath is bound as a parameter (no SQLi).
func (s *Store) GetPending(ctx context.Context, pagePath string) (PendingEntry, bool, error) {
	if s.db == nil {
		return PendingEntry{}, false, nil
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT page_path, suggestions, base_revision FROM tag_suggestions
		 WHERE page_path=? AND status='pending'`, pagePath)
	var gotPath, raw, baseRev string
	if err := row.Scan(&gotPath, &raw, &baseRev); err != nil {
		if err == sql.ErrNoRows {
			return PendingEntry{}, false, nil
		}
		return PendingEntry{}, false, err
	}
	var sugg []Suggestion
	if err := json.Unmarshal([]byte(raw), &sugg); err != nil {
		return PendingEntry{}, false, fmt.Errorf("tagsweep: unmarshal suggestions for %q: %w", gotPath, err)
	}
	return PendingEntry{PagePath: gotPath, Suggestions: sugg, BaseRevision: baseRev}, true, nil
}

// ResolvePending flips the PENDING rows for the named pages to status='resolved'
// in one statement so ListPending no longer returns them (the queue shrinks). It
// is called by the approve handler AFTER a successful apply (the applied pages) or
// on an explicit reject. Only rows currently 'pending' are touched (a stale/
// notfound page left pending for a re-run is NOT resolved unless its path is
// passed). An empty pagePaths slice is a no-op (no error). The IN list is built
// from parameter placeholders bound positionally — the page paths are NEVER
// interpolated into the SQL text (no SQLi, T-12-06 boundary).
func (s *Store) ResolvePending(ctx context.Context, pagePaths []string) error {
	if s.db == nil {
		return fmt.Errorf("tagsweep: ResolvePending requires a database")
	}
	if len(pagePaths) == 0 {
		return nil
	}
	placeholders := make([]string, len(pagePaths))
	args := make([]any, 0, len(pagePaths))
	for i, p := range pagePaths {
		placeholders[i] = "?"
		args = append(args, p)
	}
	query := `UPDATE tag_suggestions SET status='resolved'
		 WHERE status='pending' AND page_path IN (` + strings.Join(placeholders, ",") + `)`
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	return nil
}

// Targets returns the pages a sweep should enqueue jobs for, as a sorted slice.
// When allPages is false it returns live pages absent from page_tags (the
// untagged backfill case — the default). When allPages is true it returns ALL
// live pages. Both intersect the live-page set: a page in page_tags but no longer
// on disk is NEVER targeted. A nil repo (no-repo harness) returns an empty slice
// (no panic). Zero targets returns an empty slice, not an error (drives the
// "every page already has tags" UX).
func (s *Store) Targets(ctx context.Context, allPages bool) ([]string, error) {
	live := s.livePages()
	if len(live) == 0 {
		return []string{}, nil
	}

	if allPages {
		out := make([]string, 0, len(live))
		for p := range live {
			out = append(out, p)
		}
		sort.Strings(out)
		return out, nil
	}

	// Untagged = live MINUS the distinct page_path set in page_tags (mirrors the
	// graph untagged-detection set-difference).
	tagged, err := s.taggedPages(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(live))
	for p := range live {
		if _, isTagged := tagged[p]; !isTagged {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

// taggedPages returns the distinct set of page paths present in page_tags. A nil
// db returns an empty set.
func (s *Store) taggedPages(ctx context.Context) (map[string]struct{}, error) {
	set := map[string]struct{}{}
	if s.db == nil {
		return set, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT page_path FROM page_tags`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		set[p] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return set, nil
}

// livePages enumerates every live page on disk from the repo Tree, applying the
// SAME skip rules graph.livePages uses (skip directories, non-.md files, and
// anything under the trash prefix). A nil repo or a Tree() error returns an empty
// set (no panic) — the server path wires the repo so production enumerates all
// live pages.
func (s *Store) livePages() map[string]struct{} {
	live := map[string]struct{}{}
	if s.repo == nil {
		return live
	}
	items, err := s.repo.Tree()
	if err != nil {
		return live
	}
	for _, it := range items {
		if it.IsDir || isTrashed(it.Path) || !strings.HasSuffix(it.Path, ".md") {
			continue
		}
		live[it.Path] = struct{}{}
	}
	return live
}
