package pages

import (
	"context"
	"fmt"

	"github.com/postfix/okworkspace/internal/okf"
)

// HistoryEntry is one row of a page's version history as the UI consumes it
// (VER-02). It carries ONLY human-readable fields — what action cut the version,
// who cut it, and when — plus an OPAQUE Version token the client passes back to
// view or restore that version. There is intentionally NO Git-identifier field:
// the underlying Git revision lives only inside Version as a server-side handle
// and is never rendered to the user (the UI shows "Edited by Sam · 2 hours
// ago"). Renaming this field to anything Git-flavored would violate the
// hidden-Git contract.
type HistoryEntry struct {
	// Version is the opaque server-issued token used to read this version back
	// (ViewVersion/Restore). It is never displayed to the user as text.
	Version string `json:"version"`
	// Action is the human action recovered from the commit trailer ("edit",
	// "create", "rename", "restore-version", …) — rendered as "Edited"/"Created"
	// by the frontend, never as a Git verb.
	Action string `json:"action"`
	// Who is the display name of the person who cut the version.
	Who string `json:"who"`
	// When is the version time in RFC3339; the frontend renders it as a friendly
	// relative time ("2 hours ago"). The raw timestamp is acceptable on the wire;
	// the UI never shows it verbatim.
	When string `json:"when"`
}

// History returns the version history of the page at path, newest-first, mapped
// from the gitstore commits into the UI-safe HistoryEntry form (VER-02). The
// Git revision from each gitstore.Commit is carried only in the opaque Version
// token — never in a Git-named field — so no raw revision is surfaced to the
// user.
func (s *Service) History(ctx context.Context, path string) ([]HistoryEntry, error) {
	exists, err := s.repo.Exists(path)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrPageNotFound
	}
	commits, err := s.git.History(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("pages: history %q: %w", path, err)
	}
	entries := make([]HistoryEntry, 0, len(commits))
	for _, c := range commits {
		when := ""
		if !c.When.IsZero() {
			when = c.When.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		entries = append(entries, HistoryEntry{
			Version: c.Token, // opaque server-side handle, never shown to the user
			Action:  c.Action,
			Who:     c.Who,
			When:    when,
		})
	}
	return entries, nil
}

// ViewVersion returns the page at path as it existed at the given opaque version
// token, parsed back into the Page response form (frontmatter + body). The
// revision returned is the CURRENT committed revision of the live path (so the
// editor would still write against HEAD) — the old version is read-only here.
// ErrPageNotFound is returned when the live page no longer exists.
func (s *Service) ViewVersion(ctx context.Context, path, version string) (Page, error) {
	raw, err := s.git.ShowAt(ctx, version, path)
	if err != nil {
		return Page{}, fmt.Errorf("pages: view version %q@%s: %w", path, version, err)
	}
	doc, err := okf.Parse(raw)
	if err != nil {
		return Page{}, fmt.Errorf("pages: parse version %q@%s: %w", path, version, err)
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

// RestoreVersion writes the page's chosen old version (identified by the opaque
// version token) back as a NEW forward commit through the single-writer
// CommitJob (VER-03). It NEVER rewinds or rewrites history: the old bytes are read
// via ShowAt, repaired so the restored file still satisfies required frontmatter
// (PAGE-09), re-emitted byte-stably, and enqueued as a "restore-version" commit
// that advances HEAD. The current version therefore remains in history — nothing
// is lost. Push is threaded from config like every other mutation (VER-04).
//
// (Named RestoreVersion to distinguish it from Service.Restore, which restores a
// page from Trash. Both flow through the SAME single-writer commit path — there
// is no second write path.)
func (s *Service) RestoreVersion(ctx context.Context, path, version, user string) error {
	exists, err := s.repo.Exists(path)
	if err != nil {
		return err
	}
	if !exists {
		return ErrPageNotFound
	}

	old, err := s.git.ShowAt(ctx, version, path)
	if err != nil {
		return fmt.Errorf("pages: read version %q@%s: %w", path, version, err)
	}

	// Repair missing required frontmatter so the restored file is still valid
	// (an old version predating a required-field addition would otherwise be
	// re-introduced incomplete). Repair is additive and byte-safe.
	doc, err := okf.Parse(old)
	if err != nil {
		return fmt.Errorf("pages: parse restored %q: %w", path, err)
	}
	okf.Repair(doc, s.now())
	out, err := doc.Emit()
	if err != nil {
		return fmt.Errorf("pages: emit restored %q: %w", path, err)
	}

	// Write the old bytes as a NEW forward commit (forward-only, no rewind). This reuses the
	// exact single-writer enqueue path every other mutation flows through.
	return s.enqueueWrite(ctx, path, out, "restore-version", user)
}
