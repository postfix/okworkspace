package pages

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/postfix/okworkspace/internal/okf"
)

// Node is one entry in the nested navigation tree (SPEC §17.2). A folder carries
// Children; a page is a leaf. Title comes from the page's frontmatter `title`
// (falling back to the base filename) for pages, and the directory name for
// folders. Path is repo-relative and slash-separated.
type Node struct {
	Type     string `json:"type"` // "folder" | "page" | "attachment"
	Path     string `json:"path"`
	Title    string `json:"title"`
	Children []Node `json:"children,omitempty"`
	// Attachments lists a page's uploaded files as leaf nodes (type "attachment",
	// Path = attachment id, Title = original filename). Populated by the tree
	// handler, not the walk. Empty/omitted for folders.
	Attachments []Node `json:"attachments,omitempty"`
}

// Tree walks the repo and returns the nested page/folder tree. It skips both the
// hidden .git directory and the .okf-workspace operational directory (trash,
// locks, manifest) so neither leaks into navigation. Only .md files become page
// nodes; their titles are read from the frontmatter region via okf.Parse (the
// body is never parsed). Folders that contain no pages still appear (they carry
// a seeded index.md from CreateFolder).
func (s *Service) Tree(ctx context.Context) ([]Node, error) {
	root := s.repo.Root()
	// folders maps a repo-relative dir path to its accumulating children; "" is
	// the repo root. Built in a first pass, then assembled into the nested shape.
	children := map[string][]Node{}

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

		// Skip hidden Git metadata and the operational .okf-workspace dir entirely.
		if isSkippedDir(slashRel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		parent := path.Dir(slashRel)
		if parent == "." {
			parent = ""
		}

		if d.IsDir() {
			children[parent] = append(children[parent], Node{
				Type:  "folder",
				Path:  slashRel,
				Title: path.Base(slashRel),
			})
			return nil
		}

		// Pages are .md files only.
		if !strings.HasSuffix(slashRel, ".md") {
			return nil
		}
		children[parent] = append(children[parent], Node{
			Type:  "page",
			Path:  slashRel,
			Title: s.pageTitle(slashRel),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("pages: walk tree: %w", err)
	}

	// Guarantee a non-nil top-level slice so the tree endpoint serializes to a
	// JSON array ([]) rather than null when the repo root has no direct entries.
	// A null body would make the SPA's nodes.map crash (UAT blocker).
	nodes := assembleTree("", children)
	if nodes == nil {
		nodes = []Node{}
	}
	return nodes, nil
}

// assembleTree recursively builds the nested node list for dir, attaching each
// folder's children. Nodes are sorted folders-first then by title (stable,
// human-friendly ordering).
func assembleTree(dir string, children map[string][]Node) []Node {
	nodes := children[dir]
	for i := range nodes {
		if nodes[i].Type == "folder" {
			nodes[i].Children = assembleTree(nodes[i].Path, children)
		}
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		if (nodes[i].Type == "folder") != (nodes[j].Type == "folder") {
			return nodes[i].Type == "folder"
		}
		return strings.ToLower(nodes[i].Title) < strings.ToLower(nodes[j].Title)
	})
	return nodes
}

// isSkippedDir reports whether a repo-relative path is inside .git or
// .okf-workspace (the two directories the navigation tree never exposes).
func isSkippedDir(slashRel string) bool {
	if slashRel == ".git" || strings.HasPrefix(slashRel, ".git/") {
		return true
	}
	if slashRel == ".okf-workspace" || strings.HasPrefix(slashRel, ".okf-workspace/") {
		return true
	}
	// The reserved attachment store ("attachments/" at the repo root) holds
	// ULID-named blobs/sidecars, not pages — it must not surface as a navigable
	// folder. Attachments reach the tree as leaves under their owning page.
	if slashRel == "attachments" || strings.HasPrefix(slashRel, "attachments/") {
		return true
	}
	return false
}

// pageTitle reads ONLY the frontmatter region of a page to recover its title,
// falling back to the base filename (without .md) when there is no title field
// or the file cannot be read/parsed. The body is never parsed as Markdown.
func (s *Service) pageTitle(slashRel string) string {
	fallback := strings.TrimSuffix(path.Base(slashRel), ".md")
	raw, err := s.repo.Read(slashRel)
	if err != nil {
		return fallback
	}
	doc, err := okf.Parse(raw)
	if err != nil || !doc.HasFrontmatter {
		return fallback
	}
	if title := okf.Field(doc, okf.FieldTitle); strings.TrimSpace(title) != "" {
		return title
	}
	return fallback
}
