package pages

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/okf"
)

// ErrRenameCollision is returned when a computed rename/move target already
// exists and the service could not auto-suffix a free path. (Collisions are
// normally auto-suffixed per D-12; this sentinel exists for the exhausted case.)
var ErrRenameCollision = errors.New("rename target collision")

// Rename changes the page at oldPath to a new title within the SAME folder. The
// new filename is slugged from newTitle (collision-suffixed, D-12). Every inbound
// link across the whole workspace is eagerly recomputed and rewritten to the new
// path (D-07), and the move plus all rewrites land in ONE commit so links are
// never momentarily broken and history is continuous (git rename detection).
// Returns the new repo-relative path. The rewrite goes through the round-trip-safe
// okf path (okf.RewriteLinks on the opaque body, re-emitted via okf.Emit) so no
// unrelated Markdown bytes are ever corrupted.
func (s *Service) Rename(ctx context.Context, oldPath, newTitle, user string) (string, error) {
	newTitle = strings.TrimSpace(newTitle)
	if newTitle == "" {
		return "", ErrTitleRequired
	}
	exists, err := s.repo.Exists(oldPath)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", ErrPageNotFound
	}

	folder := path.Dir(oldPath)
	if folder == "." {
		folder = ""
	}
	newPath, err := s.uniquePath(folder, newTitle)
	if err != nil {
		return "", err
	}
	return s.relocate(ctx, oldPath, newPath, "rename", user)
}

// Move relocates the page at oldPath into newParentDir (a repo-relative folder,
// "" = root), keeping its slugged filename. Inbound links are recomputed and
// rewritten across the workspace and committed atomically with the move (D-07);
// history stays continuous via git rename detection. Returns the new path.
func (s *Service) Move(ctx context.Context, oldPath, newParentDir, user string) (string, error) {
	exists, err := s.repo.Exists(oldPath)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", ErrPageNotFound
	}

	base := path.Base(oldPath)
	newParentDir = strings.Trim(strings.TrimSpace(newParentDir), "/")
	newPath := base
	if newParentDir != "" {
		newPath = newParentDir + "/" + base
	}
	newPath = path.Clean(newPath)

	if newPath == oldPath {
		// No-op move (same location): nothing to do.
		return oldPath, nil
	}

	// Collision-suffix the destination filename (D-12) if something already lives
	// there.
	newPath, err = s.uniqueExactPath(newPath)
	if err != nil {
		return "", err
	}
	return s.relocate(ctx, oldPath, newPath, "move", user)
}

// relocate is the shared rename/move body: read the source bytes, scan the whole
// repo for inbound links, rewrite each to the new path (round-trip-safe), and
// enqueue ONE commit that writes the new file, rewrites every linking file, and
// removes the old file. action is "rename" or "move".
func (s *Service) relocate(ctx context.Context, oldPath, newPath, action, user string) (string, error) {
	srcBytes, err := s.repo.Read(oldPath)
	if err != nil {
		return "", fmt.Errorf("pages: read source %q: %w", oldPath, err)
	}

	writes := []fileWrite{{Path: newPath, Bytes: srcBytes}}
	// Stage the new path AND the old path (the deletion) in one commit so git
	// detects the rename and `--follow` traces history across it.
	stagePaths := []string{newPath, oldPath}

	// Walk every .md in the repo and rewrite inbound links to oldPath -> newPath.
	rewrites, rewritePaths, err := s.rewriteInboundLinks(oldPath, newPath)
	if err != nil {
		return "", err
	}
	writes = append(writes, rewrites...)
	stagePaths = append(stagePaths, rewritePaths...)

	p := commitPayload{
		Writes:  writes,
		Removes: []string{oldPath},
		Spec: gitstore.CommitSpec{
			Paths:   stagePaths,
			Message: relocateSubject(action, oldPath, newPath),
			User:    user,
			Action:  action,
			Source:  "web-ui",
		},
		Push: s.pushOnCommit,
	}
	if err := EnqueueCommit(ctx, s.worker, p); err != nil {
		return "", err
	}
	// A rename/move is an index MOVE: remove the doc (and its headings) at the OLD
	// path, then upsert the NEW path. Fire-and-forget (HTTP-handler context); the
	// rebuild backstop reconciles a dropped enqueue (T-03-20).
	s.enqueueIndexDelete(ctx, oldPath)
	s.enqueueIndexUpsert(ctx, newPath)
	// Inbound-link rewrites edited OTHER pages in the same commit; re-index each so
	// their (changed) body bytes stay searchable without a restart.
	for _, w := range writes {
		if w.Path == newPath {
			continue
		}
		s.enqueueIndexUpsert(ctx, w.Path)
	}
	return newPath, nil
}

// rewriteInboundLinks scans every .md page in the repo (skipping .git and
// .okf-workspace, and the source page itself) and, for each page that links to
// oldRel, produces a rewritten fileWrite whose links now point at newRel. The
// rewrite is computed relative to EACH linking page's own directory and goes
// through the round-trip-safe okf path: okf.Parse -> RewriteLinks(body) ->
// okf.Emit, so a page whose links did not match is never touched and unrelated
// bytes are never corrupted.
func (s *Service) rewriteInboundLinks(oldRel, newRel string) ([]fileWrite, []string, error) {
	root := s.repo.Root()
	var writes []fileWrite
	var paths []string

	err := filepath.WalkDir(root, func(abs string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if abs == root {
			return nil
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return err
		}
		slashRel := filepath.ToSlash(rel)

		if isSkippedDir(slashRel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(slashRel, ".md") {
			return nil
		}
		// The source page is being moved; its own (relative) links are unchanged
		// by a move of itself, so skip it here.
		if slashRel == oldRel {
			return nil
		}

		raw, err := s.repo.Read(slashRel)
		if err != nil {
			return fmt.Errorf("pages: read %q for link rewrite: %w", slashRel, err)
		}
		doc, err := okf.Parse(raw)
		if err != nil {
			return fmt.Errorf("pages: parse %q for link rewrite: %w", slashRel, err)
		}
		fromDir := path.Dir(slashRel)
		if fromDir == "." {
			fromDir = ""
		}
		newBody, changed := okf.RewriteLinks(doc.Body, fromDir, oldRel, newRel)
		if !changed {
			return nil
		}
		doc.Body = newBody
		out, err := doc.Emit()
		if err != nil {
			return fmt.Errorf("pages: emit %q after link rewrite: %w", slashRel, err)
		}
		writes = append(writes, fileWrite{Path: slashRel, Bytes: out})
		paths = append(paths, slashRel)
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("pages: scan inbound links: %w", err)
	}
	return writes, paths, nil
}

// uniqueExactPath returns candidate unchanged if it is free, otherwise it
// inserts a numeric suffix before the extension (deploy.md -> deploy-2.md) until
// a free path is found (D-12 collision handling for move).
func (s *Service) uniqueExactPath(candidate string) (string, error) {
	if _, err := s.repo.Resolve(candidate); err != nil {
		return "", err
	}
	exists, err := s.repo.Exists(candidate)
	if err != nil {
		return "", err
	}
	if !exists {
		return candidate, nil
	}
	dir := path.Dir(candidate)
	base := path.Base(candidate)
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	build := func(n int) string {
		name := fmt.Sprintf("%s-%d%s", stem, n, ext)
		if dir == "." || dir == "" {
			return name
		}
		return dir + "/" + name
	}
	for n := 2; n < 10000; n++ {
		c := build(n)
		exists, err := s.repo.Exists(c)
		if err != nil {
			return "", err
		}
		if !exists {
			return c, nil
		}
	}
	return "", ErrRenameCollision
}

// relocateSubject renders a hidden-commit subject for a rename/move. The user
// never reads these; the history view derives display text from the Action
// trailer, not this subject.
func relocateSubject(action, oldPath, newPath string) string {
	switch action {
	case "rename":
		return "Rename " + oldPath + " to " + newPath
	case "move":
		return "Move " + oldPath + " to " + newPath
	default:
		return action + " " + oldPath + " to " + newPath
	}
}
