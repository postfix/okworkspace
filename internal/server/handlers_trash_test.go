package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/audit"
)

// listTrash GETs the trash listing and decodes it.
func listTrash(t *testing.T, f *pageFixture, cookies []*http.Cookie) []map[string]any {
	t.Helper()
	rec := doGet(t, f.handler, "/api/v1/trash", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /trash = %d, body=%s", rec.Code, rec.Body.String())
	}
	var entries []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode trash: %v (body=%s)", err, rec.Body.String())
	}
	return entries
}

// waitForTrashEntry polls GET /trash until at least one entry appears (the
// delete commit has drained and the row is recorded).
func waitForTrashEntry(t *testing.T, f *pageFixture, cookies []*http.Cookie) []map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		entries := listTrash(t, f, cookies)
		if len(entries) > 0 {
			return entries
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("trash entry never appeared (delete did not drain)")
	return nil
}

// waitForDeleteDrained polls until the page is gone from its original path (the
// delete commit has fully drained — the trashed bytes are on disk to restore).
func waitForDeleteDrained(t *testing.T, f *pageFixture, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ok, _ := f.repo.Exists(path); !ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("page %q never left its original path (delete commit did not drain)", path)
}

// TestDeletePageRBAC: reader DELETE -> 403; editor DELETE -> 204, page gone from
// the tree and present in /trash.
func TestDeletePageRBAC(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	target := createPageAs(t, f, editor, "runbooks", "Deploy") // runbooks/deploy.md

	// Reader cannot delete.
	reader := loginReader(t, f)
	rec := doMutate(t, f.handler, http.MethodDelete, "/api/v1/pages/"+target, nil, reader)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("reader delete = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}

	// Editor deletes -> 204.
	rec = doMutate(t, f.handler, http.MethodDelete, "/api/v1/pages/"+target, nil, editor)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("editor delete = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}

	// Wait for the trash row, then assert the page is in /trash and gone from /tree.
	entries := waitForTrashEntry(t, f, editor)
	found := false
	for _, e := range entries {
		if e["original_path"] == target {
			found = true
		}
	}
	if !found {
		t.Fatalf("deleted page %q not present in /trash: %v", target, entries)
	}

	// Wait for the delete commit to drain (the original leaves the working tree)
	// before asserting it is gone from the tree listing.
	waitForDeleteDrained(t, f, target)
	treeRec := doGet(t, f.handler, "/api/v1/tree", editor)
	if containsPath(t, treeRec.Body.Bytes(), target) {
		t.Fatalf("deleted page %q still present in /tree", target)
	}
}

// TestRestoreHandler: editor restores -> 200 with the restored path; the page
// reappears in /tree.
func TestRestoreHandler(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	target := createPageAs(t, f, editor, "runbooks", "Deploy")

	rec := doMutate(t, f.handler, http.MethodDelete, "/api/v1/pages/"+target, nil, editor)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", rec.Code)
	}
	entries := waitForTrashEntry(t, f, editor)
	id := int64(entries[0]["id"].(float64))
	waitForDeleteDrained(t, f, target)

	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/trash/"+itoa(id)+"/restore", nil, editor)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Path != target {
		t.Fatalf("restored path = %q, want %q", resp.Path, target)
	}
	waitForPath(t, f, resp.Path)

	treeRec := doGet(t, f.handler, "/api/v1/tree", editor)
	if !containsPath(t, treeRec.Body.Bytes(), target) {
		t.Fatalf("restored page %q not back in /tree", target)
	}
}

// TestDeleteAudits: a successful delete records an audit event (Action trash).
func TestDeleteAudits(t *testing.T) {
	st := newPageServerStore(t)
	f := st.fixture
	editor := loginEditor(t, f)
	target := createPageAs(t, f, editor, "runbooks", "Deploy")

	rec := doMutate(t, f.handler, http.MethodDelete, "/api/v1/pages/"+target, nil, editor)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", rec.Code)
	}
	if n := countAudit(t, st.store, "trash"); n < 1 {
		t.Fatalf("expected a trash audit row, got %d", n)
	}
}

// TestRestoreHandlerNotFound: restoring an unknown trash id -> 404.
func TestRestoreHandlerNotFound(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/trash/9999/restore", nil, editor)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("restore unknown id = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestRestoreAudits: a successful restore records an audit event (Action restore).
func TestRestoreAudits(t *testing.T) {
	st := newPageServerStore(t)
	f := st.fixture
	editor := loginEditor(t, f)
	target := createPageAs(t, f, editor, "runbooks", "Deploy")

	if rec := doMutate(t, f.handler, http.MethodDelete, "/api/v1/pages/"+target, nil, editor); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	entries := waitForTrashEntry(t, f, editor)
	id := int64(entries[0]["id"].(float64))
	// Wait for the delete COMMIT to drain (the original is gone from the tree)
	// before restoring, so the trashed bytes are on disk to read back.
	waitForDeleteDrained(t, f, target)

	if rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/trash/"+itoa(id)+"/restore", nil, editor); rec.Code != http.StatusOK {
		t.Fatalf("restore = %d", rec.Code)
	}
	if n := countAudit(t, st.store, audit.ActionPageRestore); n < 1 {
		// Fall back to the literal action string if the constant differs.
		if n2 := countAudit(t, st.store, "restore"); n2 < 1 {
			t.Fatalf("expected a restore audit row, got %d", n)
		}
	}
}
