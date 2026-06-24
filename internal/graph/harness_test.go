package graph

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/store"
)

// testHarness wires a real graph Store over a real content repo + migrated SQLite
// store + gitstore, mirroring the search package's t.TempDir() harness convention.
type testHarness struct {
	st   *Store
	db   *store.Store
	repo *repo.Repo
	gs   *gitstore.GitStore
}

func newHarness(t *testing.T) *testHarness {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	base := t.TempDir()

	r, err := repo.New(filepath.Join(base, "repo"))
	if err != nil {
		t.Fatalf("repo.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	st, err := store.Open(filepath.Join(base, "app.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	gs := gitstore.New(r, config.GitConfig{Enabled: true, Branch: "main"})
	if err := gs.Init(context.Background()); err != nil {
		t.Fatalf("gitstore.Init: %v", err)
	}

	gstore := OpenStore(st.DB())
	gstore.SetRepo(r)
	gstore.SetGit(gs)

	return &testHarness{st: gstore, db: st, repo: r, gs: gs}
}

// writePage renders an OKF page with frontmatter (title + tags) and a body and
// writes it to the content repo at path. Mirrors the search harness writer.
func (h *testHarness) writePage(t *testing.T, path, title string, tags []string, body string) {
	t.Helper()
	tagLines := ""
	for _, tg := range tags {
		tagLines += fmt.Sprintf("  - %s\n", tg)
	}
	if tagLines == "" {
		tagLines = "  []\n"
	}
	content := fmt.Sprintf("---\ntype: page\ntitle: %s\ndescription: \ntags:\n%stimestamp: 2026-06-21T00:00:00Z\n---\n\n%s\n", title, tagLines, body)
	if err := h.repo.Write(path, []byte(content)); err != nil {
		t.Fatalf("write page %q: %v", path, err)
	}
}

// commitSpec builds a gitstore.CommitSpec for the harness (advances HEAD so
// drift tests have a real HEAD to compare against).
func commitSpec(msg string, paths ...string) gitstore.CommitSpec {
	return gitstore.CommitSpec{
		Paths: paths, Message: msg, User: "tester", Action: "edit", Source: "test",
	}
}

// snapshotLinks returns all page_links rows sorted "src|dst" for byte-stable
// equality assertions across rebuilds.
func (h *testHarness) snapshotLinks(t *testing.T) []string {
	t.Helper()
	rows, err := h.db.DB().Query(`SELECT src_path, dst_path FROM page_links`)
	if err != nil {
		t.Fatalf("query page_links: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var src, dst string
		if err := rows.Scan(&src, &dst); err != nil {
			t.Fatalf("scan page_links: %v", err)
		}
		out = append(out, src+"|"+dst)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err page_links: %v", err)
	}
	sort.Strings(out)
	return out
}

// snapshotTags returns all page_tags rows sorted "page|tag".
func (h *testHarness) snapshotTags(t *testing.T) []string {
	t.Helper()
	rows, err := h.db.DB().Query(`SELECT page_path, tag FROM page_tags`)
	if err != nil {
		t.Fatalf("query page_tags: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var page, tag string
		if err := rows.Scan(&page, &tag); err != nil {
			t.Fatalf("scan page_tags: %v", err)
		}
		out = append(out, page+"|"+tag)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err page_tags: %v", err)
	}
	sort.Strings(out)
	return out
}
