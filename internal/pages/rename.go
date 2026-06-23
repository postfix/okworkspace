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

// ErrFolderExists is returned when a folder rename/move targets a directory that
// already exists. Folders NEVER auto-suffix or silently merge (TREE-06): the
// operation rejects cleanly (surfaced as HTTP 409) before any disk write so two
// folders are never merged into one.
var ErrFolderExists = errors.New("folder target already exists")

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

// RenameFolder renames the folder at dir to a sibling folder named slugify(newName),
// relocating the folder's index.md AND every descendant page under the dir/ prefix
// AND rewriting every inbound link to every moved page in ONE commit (TREE-02).
// Returns the new folder dir. Rejects with ErrFolderExists if the target dir already
// exists (folders never auto-suffix or merge — TREE-06). dir not existing returns
// ErrPageNotFound.
func (s *Service) RenameFolder(ctx context.Context, dir, newName, user string) (string, error) {
	dir = strings.Trim(strings.TrimSpace(dir), "/")
	exists, err := s.repo.Exists(dir + "/index.md")
	if err != nil {
		return "", err
	}
	if !exists {
		return "", ErrPageNotFound
	}
	slug := slugify(newName)
	if slug == "" {
		return "", ErrTitleRequired
	}
	parent := path.Dir(dir)
	if parent == "." {
		parent = ""
	}
	newDir := slug
	if parent != "" {
		newDir = parent + "/" + slug
	}
	if newDir == dir {
		// No-op rename (same slug): nothing to do.
		return dir, nil
	}
	if err := s.relocateFolder(ctx, dir, newDir, "rename", user); err != nil {
		return "", err
	}
	return newDir, nil
}

// MoveFolder relocates the folder at dir into newParentDir (repo-relative, "" =
// root), keeping the folder's own base name, and atomically relocates every
// descendant + rewrites all inbound links in ONE commit (TREE-02). Returns the new
// folder dir. Rejects with ErrFolderExists if the destination dir already exists
// (TREE-06). dir not existing returns ErrPageNotFound.
func (s *Service) MoveFolder(ctx context.Context, dir, newParentDir, user string) (string, error) {
	dir = strings.Trim(strings.TrimSpace(dir), "/")
	exists, err := s.repo.Exists(dir + "/index.md")
	if err != nil {
		return "", err
	}
	if !exists {
		return "", ErrPageNotFound
	}
	base := path.Base(dir)
	newParentDir = strings.Trim(strings.TrimSpace(newParentDir), "/")
	newDir := base
	if newParentDir != "" {
		newDir = newParentDir + "/" + base
	}
	newDir = path.Clean(newDir)
	if newDir == dir {
		// No-op move (same location): nothing to do.
		return dir, nil
	}
	if err := s.relocateFolder(ctx, dir, newDir, "move", user); err != nil {
		return "", err
	}
	return newDir, nil
}

// descendantPages returns every .md page that belongs to the folder dir: the
// folder's own index.md plus every page under the dir/ prefix (recursively). Paths
// are repo-relative, slash-separated. It mirrors the rewriteInboundLinks walk
// (skip .git/.okf-workspace, .md files only).
func (s *Service) descendantPages(dir string) ([]string, error) {
	root := s.repo.Root()
	prefix := dir + "/"
	var pages []string
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
		// prefix is dir+"/", so slashRel == prefix+"index.md" is always subsumed
		// by HasPrefix(slashRel, prefix) — the explicit index.md clause was dead.
		if strings.HasPrefix(slashRel, prefix) {
			pages = append(pages, slashRel)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("pages: enumerate descendants of %q: %w", dir, err)
	}
	return pages, nil
}

// relocateFolder relocates every descendant page of oldDir to the same path under
// newDir and rewrites every inbound link to every moved page, all in ONE commit
// (TREE-02). It REJECTS with ErrFolderExists if newDir already exists, BEFORE any
// disk write (TREE-06). Moved pages are written with their EXISTING bytes verbatim
// (never re-emitted through okf.Emit — byte-stability invariant); only genuine
// inbound links are rewritten through the round-trip-safe okf path. A page that is
// BOTH moved and an inbound-link target is staged exactly once with the rewritten
// bytes (Pitfall 1, last-write-wins merge-by-path).
func (s *Service) relocateFolder(ctx context.Context, oldDir, newDir, action, user string) error {
	// (1) Collision precheck FIRST — before building any payload or touching disk.
	exists, err := s.repo.Exists(newDir)
	if err != nil {
		return err
	}
	if exists {
		return ErrFolderExists
	}

	// (2) Enumerate every descendant page of the folder.
	oldPaths, err := s.descendantPages(oldDir)
	if err != nil {
		return err
	}
	if len(oldPaths) == 0 {
		return ErrPageNotFound
	}

	// (3) Build the new-path write set (existing bytes verbatim) and the per-page
	// old->new mapping. byPath is keyed by NEW (or unchanged) path so a later
	// inbound-link rewrite of a moved page (Pitfall 1) overwrites the verbatim copy
	// with the rewritten bytes (last-write-wins, one stage per path).
	byPath := map[string][]byte{}
	var removes []string
	var stagePaths []string
	moves := make(map[string]string, len(oldPaths)) // oldPath -> newPath
	for _, oldPath := range oldPaths {
		newPath := newDir + strings.TrimPrefix(oldPath, oldDir)
		srcBytes, err := s.repo.Read(oldPath)
		if err != nil {
			return fmt.Errorf("pages: read source %q: %w", oldPath, err)
		}
		byPath[newPath] = srcBytes
		moves[oldPath] = newPath
		removes = append(removes, oldPath)
		// Stage both the new path and the old path so git detects each per-descendant
		// rename and `git log --follow` traces history across the move.
		stagePaths = append(stagePaths, newPath, oldPath)
	}

	// (4) Rewrite inbound links to EVERY moved page in ONE unified per-page pass.
	// For each page in the repo (moved or not), resolve links from its CURRENT dir
	// and emit relative paths from its FINAL dir (its new dir if it is itself moving,
	// Pitfall 1 — a moved sibling that links to another moved sibling must recompute
	// from the new location, not produce a ../ back-reference). A moved page's
	// rewrite lands on its NEW path key, superseding the verbatim copy exactly once.
	rewritten, rewriteStage, err := s.rewriteFolderInboundLinks(moves)
	if err != nil {
		return err
	}
	for p, b := range rewritten {
		byPath[p] = b
	}
	stagePaths = append(stagePaths, rewriteStage...)

	// (5) Convert the merged map to []fileWrite and enqueue ONE commit.
	writes := make([]fileWrite, 0, len(byPath))
	for p, b := range byPath {
		writes = append(writes, fileWrite{Path: p, Bytes: b})
	}
	cp := commitPayload{
		Writes:  writes,
		Removes: removes,
		Spec: gitstore.CommitSpec{
			Paths:   stagePaths,
			Message: relocateSubject(action, oldDir, newDir),
			User:    user,
			Action:  action,
			Source:  "web-ui",
		},
		Push: s.pushOnCommit,
	}
	if err := EnqueueCommit(ctx, s.worker, cp); err != nil {
		return err
	}

	// (6) Index maintenance: per moved page delete the old doc and upsert the new
	// one; re-upsert any OTHER page whose body was rewritten. Fire-and-forget; the
	// rebuild backstop reconciles a dropped enqueue (T-03-20).
	for oldPath, newPath := range moves {
		s.enqueueIndexDelete(ctx, oldPath)
		s.enqueueIndexUpsert(ctx, newPath)
	}
	for p := range byPath {
		if _, moved := movedDestinations(moves)[p]; moved {
			continue // already upserted above
		}
		s.enqueueIndexUpsert(ctx, p)
	}
	return nil
}

// rewriteFolderInboundLinks scans every .md page in the repo once and rewrites
// every inbound link that points at ANY page in the moves set (oldPath -> newPath).
// It returns the rewritten bytes keyed by each page's FINAL path (a moving page's
// own new path; a stationary linker's unchanged path) plus the stage paths to add
// to the commit spec. The rewrite goes through the round-trip-safe okf path:
// okf.Parse -> RewriteLinksMoved(body, resolveDir, emitDir, oldRel, newRel) ->
// okf.Emit. resolveDir is the page's CURRENT directory (so a link resolves to its
// real target); emitDir is the page's FINAL directory (so a moving page's links are
// recomputed from where it lands — Pitfall 1, no stale ../ back-reference and no
// double-staging). A page is staged at most once: its FINAL-path key is unique, and
// a moving page's stage paths are already added by the caller, so only stationary
// linkers contribute new stage paths here.
func (s *Service) rewriteFolderInboundLinks(moves map[string]string) (map[string][]byte, []string, error) {
	root := s.repo.Root()
	out := map[string][]byte{}
	var stage []string

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

		// finalPath is where this page will live after the relocate (its new path if
		// it is itself moving, else unchanged).
		finalPath, moving := moves[slashRel]
		if !moving {
			finalPath = slashRel
		}
		resolveDir := path.Dir(slashRel)
		if resolveDir == "." {
			resolveDir = ""
		}
		emitDir := path.Dir(finalPath)
		if emitDir == "." {
			emitDir = ""
		}

		raw, err := s.repo.Read(slashRel)
		if err != nil {
			return fmt.Errorf("pages: read %q for link rewrite: %w", slashRel, err)
		}
		doc, err := okf.Parse(raw)
		if err != nil {
			return fmt.Errorf("pages: parse %q for link rewrite: %w", slashRel, err)
		}

		body := doc.Body
		changed := false
		for oldRel, newRel := range moves {
			// A page's own move does not change a link that points at ITSELF; and a
			// page never links to its own old path in a way that needs rewriting here
			// (RewriteLinksMoved is a no-op when nothing resolves to oldRel).
			nb, ch := okf.RewriteLinksMoved(body, resolveDir, emitDir, oldRel, newRel)
			if ch {
				body = nb
				changed = true
			}
		}

		if !changed {
			// A stationary page with no inbound link is untouched. A MOVING page with
			// no rewritten link is still relocated by the caller's verbatim copy, so we
			// must not emit it here (that would re-encode bytes and break byte-stability).
			return nil
		}

		doc.Body = body
		emitted, err := doc.Emit()
		if err != nil {
			return fmt.Errorf("pages: emit %q after link rewrite: %w", slashRel, err)
		}
		out[finalPath] = emitted
		if !moving {
			// Stationary linker: stage its (unchanged) path. Moving pages are already
			// staged by the caller (new+old), so they are not added again here.
			stage = append(stage, slashRel)
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("pages: scan folder inbound links: %w", err)
	}
	return out, stage, nil
}

// movedDestinations returns the set of NEW paths from a moves map (oldPath ->
// newPath), used to skip double-upserting a page that was already index-upserted as
// a relocation target.
func movedDestinations(moves map[string]string) map[string]struct{} {
	dests := make(map[string]struct{}, len(moves))
	for _, newPath := range moves {
		dests[newPath] = struct{}{}
	}
	return dests
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
