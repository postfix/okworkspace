package repo

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// File permissions for repo content. Files-as-truth: plain files on disk,
// readable/copyable off the server (CLAUDE.md core value).
const (
	fileMode = 0o640
	dirMode  = 0o750
)

// TreeItem is one entry in a minimal repo listing (used this phase for seed
// verification; the full tree API lands in Phase 1).
type TreeItem struct {
	Path  string // repo-relative, slash-separated
	IsDir bool
}

// Read returns the bytes of the repo-relative path, routing through Resolve.
func (r *Repo) Read(rel string) ([]byte, error) {
	abs, err := r.Resolve(rel)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(abs)
}

// Write writes data to the repo-relative path (creating parent dirs), routing
// through Resolve. The parent directory is created via MkdirAll, which itself
// resolves safely.
func (r *Repo) Write(rel string, data []byte) error {
	abs, err := r.Resolve(rel)
	if err != nil {
		return err
	}
	parent := filepath.Dir(rel)
	if parent != "." && parent != "" {
		if err := r.MkdirAll(parent); err != nil {
			return err
		}
	}
	return os.WriteFile(abs, data, fileMode)
}

// MkdirAll creates the repo-relative directory (and parents), routing through
// Resolve.
func (r *Repo) MkdirAll(rel string) error {
	abs, err := r.Resolve(rel)
	if err != nil {
		return err
	}
	return os.MkdirAll(abs, dirMode)
}

// Exists reports whether the repo-relative path exists, routing through
// Resolve. A rejected (unsafe) path returns the resolver error.
func (r *Repo) Exists(rel string) (bool, error) {
	abs, err := r.Resolve(rel)
	if err != nil {
		return false, err
	}
	_, statErr := os.Stat(abs)
	if statErr == nil {
		return true, nil
	}
	if os.IsNotExist(statErr) {
		return false, nil
	}
	return false, statErr
}

// Tree returns a minimal recursive listing of the repo, repo-relative and
// slash-separated, skipping the .git directory. Used for seed verification.
func (r *Repo) Tree() ([]TreeItem, error) {
	var items []TreeItem
	err := filepath.WalkDir(r.root, func(abs string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if abs == r.root {
			return nil
		}
		relPath, err := filepath.Rel(r.root, abs)
		if err != nil {
			return err
		}
		slashRel := filepath.ToSlash(relPath)
		// Skip the hidden Git metadata directory entirely.
		if slashRel == ".git" || strings.HasPrefix(slashRel, ".git/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		items = append(items, TreeItem{Path: slashRel, IsDir: d.IsDir()})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("repo: walk tree: %w", err)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	return items, nil
}
