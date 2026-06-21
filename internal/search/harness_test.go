package search

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/store"
)

// testHarness wires a real on-disk Bleve index over a real content repo + SQLite
// store + gitstore, mirroring the extractjob_test / service_test t.TempDir()
// harness convention. The index dir lives OUTSIDE the content repo (a sibling
// temp dir) per the lifecycle invariant.
type testHarness struct {
	idx  *Index
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

	idx, err := OpenOrCreate(filepath.Join(base, "index"))
	if err != nil {
		t.Fatalf("OpenOrCreate: %v", err)
	}
	idx.SetRepo(r)
	idx.SetDB(st.DB())
	idx.SetGit(gs)
	t.Cleanup(func() { _ = idx.Close() })

	return &testHarness{idx: idx, repo: r, gs: gs}
}

// writePage renders an OKF page with frontmatter (title + tags) and a body and
// writes it to the content repo at path.
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

// writeAttachment writes an attachment's meta sidecar (attachments/<id>.json)
// and its extracted-text sidecar (attachments/<id>.txt) to the content repo,
// mirroring the on-disk three-part model the attachments package owns. The meta
// JSON carries PagePath (the owning page) so indexing links to it with no scan.
// Pass extracted="" to simulate a pending/empty extraction (no .txt written).
func (h *testHarness) writeAttachment(t *testing.T, id, originalName, pagePath, extracted string) {
	t.Helper()
	meta := fmt.Sprintf(`{
  "id": %q,
  "original_name": %q,
  "mime_type": "application/pdf",
  "size_bytes": 123,
  "uploader_name": "tester",
  "uploaded_at": "2026-06-21T00:00:00Z",
  "page_path": %q,
  "sha256": "deadbeef",
  "ext": "pdf"
}
`, id, originalName, pagePath)
	if err := h.repo.Write("attachments/"+id+".json", []byte(meta)); err != nil {
		t.Fatalf("write attachment meta %q: %v", id, err)
	}
	if extracted != "" {
		if err := h.repo.Write("attachments/"+id+".txt", []byte(extracted)); err != nil {
			t.Fatalf("write attachment txt %q: %v", id, err)
		}
	}
}

// commitAll stages and commits the working tree so HEAD advances (drift tests).
func (h *testHarness) commit(t *testing.T, msg string, paths ...string) {
	t.Helper()
	if err := h.gs.Commit(context.Background(), gitstore.CommitSpec{
		Paths: paths, Message: msg, User: "tester", Action: "edit", Source: "test",
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// rebuild rebuilds the index from the current files.
func (h *testHarness) rebuild(t *testing.T) {
	t.Helper()
	if err := h.idx.RebuildIndex(context.Background()); err != nil {
		t.Fatalf("RebuildIndex: %v", err)
	}
}
