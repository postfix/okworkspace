package pages

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/okf"
	"github.com/postfix/okworkspace/internal/search"
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
	// DeleteGroupID groups every trash row produced by ONE folder-delete operation
	// (TREE-04/05) so the client can render them as a single restorable unit. Empty
	// for a solo per-page delete (the stored column is NULL).
	DeleteGroupID string `json:"delete_group_id"`
}

// Delete moves the page at path into the trash directory as a recoverable commit
// (D-08) and records its provenance (original path + who + when, D-10) so Restore
// can return it to its original folder. The move is modeled as the old path
// removed + a new path written in ONE commit (Action "trash") through the
// single-writer CommitJob — exactly the rename/move write path (no second write
// path). The trash directory is created defensively on first delete (RESEARCH
// A1). ErrPageNotFound is returned when the page does not exist.
func (s *Service) Delete(ctx context.Context, pagePath, user string) error {
	// A solo per-page delete stores delete_group_id = NULL (empty group id binds as
	// SQL NULL via deleteWithGroup), so the per-page row is byte-identical to before
	// the grouped-delete refactor.
	return s.deleteWithGroup(ctx, pagePath, user, "")
}

// deleteWithGroup is the shared delete body for both a solo per-page Delete (empty
// groupID -> SQL NULL) and a grouped folder delete (a shared opaque groupID bound
// on every member row). The groupID is bound as a PARAMETER (sql.NullString) — never
// string-interpolated — so an attacker-influenced value can never be SQL-injected
// (T-07-05). Everything else mirrors the original single-page delete (git mv into
// trash + provenance row + live-index eviction) through the single-writer worker.
func (s *Service) deleteWithGroup(ctx context.Context, pagePath, user, groupID string) error {
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
	// Remove the page (and its heading sub-docs) from the live index immediately so
	// a trashed page leaves search results without a restart (T-03-17). The doc id
	// is the ORIGINAL page path; the page now lives under trashDir, which the index
	// excludes, so only the delete is enqueued. Fire-and-forget.
	s.enqueueIndexDelete(ctx, pagePath)
	// Also drop the trashed page's attachment docs (WR-04): an attachment's indexed
	// page_path stays the original LIVE path (not the trash path), so the query-time
	// trash filter never matches it and the attachment would remain searchable,
	// deep-linking to a page that no longer exists at that path. Enqueue a delete for
	// each owned attachment, consistent with the page exclusion. Fire-and-forget.
	s.enqueueTrashedAttachmentDeletes(ctx, pagePath)

	// Record provenance (D-10). The page content is NOT stored here — only the
	// metadata Restore needs (original path, trash path, title, who, when, and the
	// folder-delete group id). An empty groupID stores SQL NULL (solo delete) via
	// sql.NullString; a non-empty one tags this row as part of a folder delete. The
	// value is BOUND (?), never interpolated (T-07-05 SQLi guard).
	group := sql.NullString{String: groupID, Valid: groupID != ""}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO trash (original_path, trash_path, title, deleted_by, deleted_at, delete_group_id)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		pagePath, trashPath, title, user, s.now().UTC().Format(time.RFC3339), group)
	if err != nil {
		return fmt.Errorf("pages: record trash row: %w", err)
	}
	return nil
}

// DeleteFolder moves an entire folder (its index.md plus every descendant page) to
// trash under ONE shared opaque delete_group_id so the whole folder delete can be
// restored as a unit (TREE-04). It enumerates the folder with descendantPages
// (added in 07-01) and loops the existing per-page deleteWithGroup over each member
// under the shared group id. Per the RESOLVED atomicity decision (CONTEXT/RESEARCH
// Pitfall 3) the loop is per-page — each page keeps its own restorable trash row and
// its own commit, NOT one mega-commit — so partial progress is accepted and
// recoverable by the group id (mirrors ReconcileTrash WR-01). Returns
// ErrPageNotFound when the folder has no pages.
func (s *Service) DeleteFolder(ctx context.Context, dir, user string) error {
	dir = strings.Trim(strings.TrimSpace(dir), "/")
	pages, err := s.descendantPages(dir)
	if err != nil {
		return err
	}
	if len(pages) == 0 {
		return ErrPageNotFound
	}

	groupID, err := newDeleteGroupID()
	if err != nil {
		return err
	}

	for _, p := range pages {
		if err := s.deleteWithGroup(ctx, p, user, groupID); err != nil {
			// Per-page-looped: a mid-loop failure leaves the already-trashed rows
			// coherent under the shared group id (still restorable as a group). The
			// error is surfaced so the caller can report partial progress; the
			// reconcile backstop converges any phantom row (WR-01).
			return err
		}
	}
	return nil
}

// newDeleteGroupID returns an opaque hex group id (NOT a secret — just a collision-
// resistant tag) for a folder-delete batch, sourced from crypto/rand (stdlib; no new
// dependency).
func newDeleteGroupID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("pages: generate delete group id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// enqueueTrashedAttachmentDeletes fires a FIRE-AND-FORGET search delete for every
// attachment owned by pagePath (WR-04), so trashing a page also evicts its
// attachment docs from the live index. The owning page is read from the
// operational attachments table (page_path, indexed). Best-effort: a query error
// or a dropped enqueue is logged and swallowed — the idempotent rebuild backstop
// reconciles, mirroring enqueueIndexDelete's contract.
func (s *Service) enqueueTrashedAttachmentDeletes(ctx context.Context, pagePath string) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM attachments WHERE page_path = ?`, pagePath)
	if err != nil {
		slog.WarnContext(ctx, "pages: failed to list attachments for trashed page (rebuild backstop reconciles)",
			slog.String("path", pagePath), slog.String("error", err.Error()))
		return
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			slog.WarnContext(ctx, "pages: failed to scan attachment id for trashed page (rebuild backstop reconciles)",
				slog.String("path", pagePath), slog.String("error", err.Error()))
			return
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		slog.WarnContext(ctx, "pages: failed to iterate attachments for trashed page (rebuild backstop reconciles)",
			slog.String("path", pagePath), slog.String("error", err.Error()))
		return
	}

	for _, id := range ids {
		if err := s.worker.Enqueue(ctx, search.KindIndex, search.DeleteAttachmentPayload(id)); err != nil {
			slog.WarnContext(ctx, "pages: failed to enqueue trashed-attachment search delete (rebuild backstop reconciles)",
				slog.String("path", pagePath), slog.String("attachment_id", id), slog.String("error", err.Error()))
		}
	}
}

// ReconcileTrash prunes trash rows whose trash_path no longer exists on disk and
// reports how many rows were removed. It exists because Delete/Restore write the
// SQLite trash row synchronously while the actual git commit runs LATER in the
// single-writer worker (commitjob.go): if that commit fails, a Delete leaves a
// phantom trash row pointing at a trash_path that was never written, and a Restore
// can leave a row deleted while the page is still physically in trash. Running
// this at startup (and after a worker drain) reconverges the two stores: a row
// whose backing file is absent is unrecoverable through the UI and is pruned;
// nothing on disk is touched, so a page still physically in trash with a surviving
// row is left intact.
//
// RESIDUAL RISK (WR-01): this does NOT make the SQLite write atomic with the
// commit — it converges AFTER the fact. A phantom row is visible in ListTrash
// until the next reconcile pass, and a Restore whose commit fails AFTER its row
// was deleted is not re-created here (the page is still on disk but no longer
// listed). A full fix would record/delete the trash row from inside the commit
// handler; that refactor is deferred to keep the single write path and existing
// trash tests intact.
func (s *Service) ReconcileTrash(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, trash_path FROM trash`)
	if err != nil {
		return 0, fmt.Errorf("pages: reconcile trash: %w", err)
	}
	type stale struct {
		id   int64
		path string
	}
	var orphans []stale
	for rows.Next() {
		var id int64
		var tp string
		if err := rows.Scan(&id, &tp); err != nil {
			rows.Close()
			return 0, fmt.Errorf("pages: scan trash row: %w", err)
		}
		exists, err := s.repo.Exists(tp)
		if err != nil {
			rows.Close()
			return 0, err
		}
		if !exists {
			orphans = append(orphans, stale{id: id, path: tp})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("pages: iterate trash rows: %w", err)
	}
	rows.Close()

	for _, o := range orphans {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM trash WHERE id = ?`, o.id); err != nil {
			return 0, fmt.Errorf("pages: prune phantom trash row %d (%s): %w", o.id, o.path, err)
		}
	}
	return len(orphans), nil
}

// ListTrash returns every trashed page, most recently deleted first, for the
// trash view. It reports the title, original path, who deleted it, and when —
// the SQLite-stored provenance, not page content.
func (s *Service) ListTrash(ctx context.Context) ([]TrashEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, original_path, deleted_by, deleted_at, delete_group_id
		   FROM trash
		  ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("pages: list trash: %w", err)
	}
	defer rows.Close()

	entries := []TrashEntry{}
	for rows.Next() {
		var e TrashEntry
		// delete_group_id is NULL for solo deletes; scan into a NullString and surface
		// it as the empty string so the client groups only genuine folder-delete rows.
		var group sql.NullString
		if err := rows.Scan(&e.ID, &e.Title, &e.OriginalPath, &e.DeletedBy, &e.DeletedAt, &group); err != nil {
			return nil, fmt.Errorf("pages: scan trash row: %w", err)
		}
		e.DeleteGroupID = group.String
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
	// The page is back out of trash at target — re-index it so it reappears in
	// search without a restart. Fire-and-forget; rebuild backstop reconciles.
	s.enqueueIndexUpsert(ctx, target)

	if _, err := s.db.ExecContext(ctx, `DELETE FROM trash WHERE id = ?`, id); err != nil {
		return "", fmt.Errorf("pages: delete trash row %d: %w", id, err)
	}
	return target, nil
}

// RestoreGroup restores every trash row sharing groupID as a unit (the undo of a
// folder delete, TREE-05). It loops the EXISTING per-page Restore over the group so
// the live-page collision auto-suffix (restoredAlternative) applies per page
// automatically and the per-page restore path stays unchanged. The folder's
// <dir>/index.md row is restored FIRST so the folder exists before its descendants
// land. The group id is bound as a PARAMETER (?) in the SELECT — never interpolated —
// so an attacker-supplied id cannot be SQL-injected (T-07-05). Returns the list of
// restored paths (so the handler can surface a collision notice) and
// ErrTrashNotFound when no row matches groupID.
func (s *Service) RestoreGroup(ctx context.Context, groupID, user string) ([]string, error) {
	if strings.TrimSpace(groupID) == "" {
		return nil, ErrTrashNotFound
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, original_path FROM trash WHERE delete_group_id = ?`, groupID)
	if err != nil {
		return nil, fmt.Errorf("pages: list trash group %q: %w", groupID, err)
	}
	type member struct {
		id   int64
		orig string
	}
	var members []member
	for rows.Next() {
		var m member
		if err := rows.Scan(&m.id, &m.orig); err != nil {
			rows.Close()
			return nil, fmt.Errorf("pages: scan trash group row: %w", err)
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("pages: iterate trash group rows: %w", err)
	}
	rows.Close()

	if len(members) == 0 {
		return nil, ErrTrashNotFound
	}

	// Order so the folder's index.md row(s) restore FIRST (the folder exists before
	// its descendants land), then the rest by ascending original path for a stable,
	// shallow-to-deep order.
	sort.SliceStable(members, func(i, j int) bool {
		ii := isFolderIndex(members[i].orig)
		ij := isFolderIndex(members[j].orig)
		if ii != ij {
			return ii // index.md sorts before non-index
		}
		return members[i].orig < members[j].orig
	})

	restored := make([]string, 0, len(members))
	for _, m := range members {
		// Reuse the EXISTING per-page Restore: it auto-suffixes on a live-page
		// collision (restoredAlternative) so a live page is never clobbered, per page.
		path, err := s.Restore(ctx, m.id, user)
		if err != nil {
			// A mid-group failure leaves the rest of the group still restorable (the
			// remaining rows survive); surface the error with what was restored so far.
			return restored, err
		}
		restored = append(restored, path)
	}
	return restored, nil
}

// isFolderIndex reports whether a repo-relative page path is a folder's index.md
// (the page that must be restored first so the folder exists for its descendants).
func isFolderIndex(p string) bool {
	return path.Base(p) == "index.md"
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
