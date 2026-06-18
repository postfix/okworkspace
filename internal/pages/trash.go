package pages

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/okf"
)

// trashDir is the repository location deleted pages move into (SPEC §9). A
// delete is a real git mv into this directory (D-08), so history is continuous
// across the move and nothing is ever destroyed by a delete in MVP (D-09).
const trashDir = ".okf-workspace/trash"

// ErrTrashNotFound is returned by Restore when no trash row matches the given id.
var ErrTrashNotFound = errors.New("trash entry not found")

// TrashEntry is one row of the trash listing (the GET /trash response shape). The
// user sees the title, where the page came from, who deleted it, and when — never
// any Git vocabulary. DeletedAt is the stored timestamp; the UI renders it as a
// relative time.
type TrashEntry struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	OriginalPath string `json:"original_path"`
	DeletedBy    string `json:"deleted_by"`
	DeletedAt    string `json:"deleted_at"`
}

// Delete moves the page at path into the trash directory as a recoverable commit
// (D-08) and records its provenance (original path + who + when, D-10) so Restore
// can return it to its original folder. The move is modeled as the old path
// removed + a new path written in ONE commit (Action "trash") through the
// single-writer CommitJob — exactly the rename/move write path (no second write
// path). The trash directory is created defensively on first delete (RESEARCH
// A1). ErrPageNotFound is returned when the page does not exist.
func (s *Service) Delete(ctx context.Context, pagePath, user string) error {
	exists, err := s.repo.Exists(pagePath)
	if err != nil {
		return err
	}
	if !exists {
		return ErrPageNotFound
	}

	// Defensive: ensure the trash directory exists before the move (A1). The
	// directory is created through the resolver-gated MkdirAll (never os.*).
	if err := s.repo.MkdirAll(trashDir); err != nil {
		return fmt.Errorf("pages: ensure trash dir: %w", err)
	}

	srcBytes, err := s.repo.Read(pagePath)
	if err != nil {
		return fmt.Errorf("pages: read %q for delete: %w", pagePath, err)
	}

	// Recover the page title from its frontmatter for the trash listing; fall
	// back to the base filename (without .md) when absent.
	title := s.titleOf(srcBytes, pagePath)

	// Compute a unique trash path: <timestamp>-<basename>. The timestamp keeps
	// repeated deletes of the same-named page from colliding; uniqueExactPath is a
	// belt-and-suspenders suffix if two deletes land in the same second.
	base := path.Base(pagePath)
	trashPath := fmt.Sprintf("%s/%s-%s", trashDir, s.now().UTC().Format("20060102T150405"), base)
	trashPath, err = s.uniqueExactPath(trashPath)
	if err != nil {
		return err
	}

	// Model the git mv as old-removed + new-written in one commit so git records
	// the move as a rename and `git log --follow` stays continuous (D-08).
	p := commitPayload{
		Writes:  []fileWrite{{Path: trashPath, Bytes: srcBytes}},
		Removes: []string{pagePath},
		Spec: gitstore.CommitSpec{
			Paths:   []string{trashPath, pagePath},
			Message: "Move " + pagePath + " to trash",
			User:    user,
			Action:  "trash",
			Source:  "web-ui",
		},
		Push: s.pushOnCommit,
	}
	if err := EnqueueCommit(ctx, s.worker, p); err != nil {
		return err
	}

	// Record provenance (D-10). The page content is NOT stored here — only the
	// metadata Restore needs (original path, trash path, title, who, when).
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO trash (original_path, trash_path, title, deleted_by, deleted_at)
		 VALUES (?, ?, ?, ?, ?)`,
		pagePath, trashPath, title, user, s.now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("pages: record trash row: %w", err)
	}
	return nil
}

// ListTrash returns every trashed page, most recently deleted first, for the
// trash view. It reports the title, original path, who deleted it, and when —
// the SQLite-stored provenance, not page content.
func (s *Service) ListTrash(ctx context.Context) ([]TrashEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, original_path, deleted_by, deleted_at
		   FROM trash
		  ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("pages: list trash: %w", err)
	}
	defer rows.Close()

	entries := []TrashEntry{}
	for rows.Next() {
		var e TrashEntry
		if err := rows.Scan(&e.ID, &e.Title, &e.OriginalPath, &e.DeletedBy, &e.DeletedAt); err != nil {
			return nil, fmt.Errorf("pages: scan trash row: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pages: iterate trash rows: %w", err)
	}
	return entries, nil
}

// Restore moves a trashed page back to its original folder as a forward commit
// (Action "restore") through the single-writer CommitJob and removes the trash
// row. If a LIVE page already occupies the original path, Restore auto-suffixes
// so the live page is NEVER clobbered (D-10): the restored page's title is
// suffixed with " (restored)" and its filename re-slugged, and the suffixed path
// is returned. Returns ErrTrashNotFound when no row matches id.
func (s *Service) Restore(ctx context.Context, id int64, user string) (string, error) {
	var originalPath, trashPath string
	err := s.db.QueryRowContext(ctx,
		`SELECT original_path, trash_path FROM trash WHERE id = ?`, id).
		Scan(&originalPath, &trashPath)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrTrashNotFound
	}
	if err != nil {
		return "", fmt.Errorf("pages: load trash row %d: %w", id, err)
	}

	srcBytes, err := s.repo.Read(trashPath)
	if err != nil {
		return "", fmt.Errorf("pages: read trashed %q: %w", trashPath, err)
	}

	target := originalPath
	collision, err := s.repo.Exists(originalPath)
	if err != nil {
		return "", err
	}
	if collision {
		// A live page occupies the original path. Re-slug the filename from a
		// "(restored)"-suffixed title and write that instead, so the live page is
		// never overwritten (D-10). The page's own title frontmatter is also
		// updated to match.
		target, srcBytes = s.restoredAlternative(originalPath, srcBytes)
		target, err = s.uniqueExactPath(target)
		if err != nil {
			return "", err
		}
	}

	// Move the page back: write the restored path AND remove the trash copy in one
	// commit so git records the move as a rename (history continuous).
	p := commitPayload{
		Writes:  []fileWrite{{Path: target, Bytes: srcBytes}},
		Removes: []string{trashPath},
		Spec: gitstore.CommitSpec{
			Paths:   []string{target, trashPath},
			Message: "Restore " + target + " from trash",
			User:    user,
			Action:  "restore",
			Source:  "web-ui",
		},
		Push: s.pushOnCommit,
	}
	if err := EnqueueCommit(ctx, s.worker, p); err != nil {
		return "", err
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM trash WHERE id = ?`, id); err != nil {
		return "", fmt.Errorf("pages: delete trash row %d: %w", id, err)
	}
	return target, nil
}

// restoredAlternative computes the collision-safe restore target and the
// re-titled page bytes when a live page already occupies originalPath. It
// suffixes the title with " (restored)", re-slugs a filename from that title in
// the same folder, and re-emits the page (byte-stably, through okf) with the new
// title so the restored copy reads as "<title> (restored)" (D-10).
func (s *Service) restoredAlternative(originalPath string, srcBytes []byte) (string, []byte) {
	dir := path.Dir(originalPath)
	if dir == "." {
		dir = ""
	}

	doc, err := okf.Parse(srcBytes)
	if err != nil {
		// Cannot parse: fall back to a slugged filename derived from the base name
		// and leave the bytes unchanged.
		base := strings.TrimSuffix(path.Base(originalPath), ".md")
		alt := slugify(base+" restored") + ".md"
		if dir != "" {
			alt = dir + "/" + alt
		}
		return alt, srcBytes
	}

	title := okf.Field(doc, okf.FieldTitle)
	if strings.TrimSpace(title) == "" {
		title = strings.TrimSuffix(path.Base(originalPath), ".md")
	}
	newTitle := title + " (restored)"
	okf.SetField(doc, okf.FieldTitle, newTitle)
	out, err := doc.Emit()
	if err != nil {
		out = srcBytes
	}

	alt := slugify(newTitle) + ".md"
	if dir != "" {
		alt = dir + "/" + alt
	}
	return alt, out
}

// titleOf recovers a page's display title from its frontmatter, falling back to
// the base filename (without .md) when there is no title field or the bytes do
// not parse.
func (s *Service) titleOf(raw []byte, pagePath string) string {
	fallback := strings.TrimSuffix(path.Base(pagePath), ".md")
	doc, err := okf.Parse(raw)
	if err != nil {
		return fallback
	}
	if t := strings.TrimSpace(okf.Field(doc, okf.FieldTitle)); t != "" {
		return t
	}
	return fallback
}
