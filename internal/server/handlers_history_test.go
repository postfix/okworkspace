package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/audit"
)

// getHistory GETs the version history list for a page.
func getHistory(t *testing.T, f *pageFixture, cookies []*http.Cookie, path string) []map[string]any {
	t.Helper()
	rec := doGet(t, f.handler, "/api/v1/pages/"+path+"/history", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET history = %d, body=%s", rec.Code, rec.Body.String())
	}
	var entries []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode history: %v (body=%s)", err, rec.Body.String())
	}
	return entries
}

// waitForHistoryLen polls the history endpoint until it has at least n entries.
func waitForHistoryLen(t *testing.T, f *pageFixture, cookies []*http.Cookie, path string, n int) []map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last []map[string]any
	for time.Now().Before(deadline) {
		last = getHistory(t, f, cookies, path)
		if len(last) >= n {
			return last
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("history of %q never reached %d entries (got %d)", path, n, len(last))
	return nil
}

// TestHistoryHandler: GET /pages/{path}/history returns the version list as JSON
// to any authenticated user, with NO SHA/hash/commit field in the payload.
func TestHistoryHandler(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	path := createPageAs(t, f, editor, "notes", "My Page")

	saveBody(t, f, editor, path, "# edited\n")
	entries := waitForHistoryLen(t, f, editor, path, 2)

	// Each entry has version/action/who/when; NO sha/hash/commit_id key.
	for _, e := range entries {
		for _, banned := range []string{"sha", "hash", "commit_id", "commit"} {
			if _, ok := e[banned]; ok {
				t.Fatalf("history entry leaks Git key %q: %v", banned, e)
			}
		}
		if _, ok := e["version"]; !ok {
			t.Fatalf("history entry missing opaque version token: %v", e)
		}
		if _, ok := e["who"]; !ok {
			t.Fatalf("history entry missing who: %v", e)
		}
	}

	// A reader can also view history (it is a read, open to any auth user).
	reader := loginReader(t, f)
	rec := doGet(t, f.handler, "/api/v1/pages/"+path+"/history", reader)
	if rec.Code != http.StatusOK {
		t.Fatalf("reader GET history = %d, want 200", rec.Code)
	}
}

// TestRestoreVersionRBAC: POST /pages/{path}/restore as reader -> 403; as editor
// -> 200, and a subsequent GET returns the restored (original) content.
func TestRestoreVersionRBAC(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	path := createPageAs(t, f, editor, "notes", "Page")

	// Capture the ORIGINAL body, then save a different version.
	origRec := doGet(t, f.handler, "/api/v1/pages/"+path, editor)
	var orig struct {
		Body     string `json:"body"`
		Revision string `json:"revision"`
	}
	_ = json.Unmarshal(origRec.Body.Bytes(), &orig)

	saveBody(t, f, editor, path, "# brand new content\n")
	entries := waitForHistoryLen(t, f, editor, path, 2)

	// The oldest version's opaque token.
	oldest := entries[len(entries)-1]
	version, _ := oldest["version"].(string)
	if version == "" {
		t.Fatal("no version token in history")
	}

	// Reader cannot restore.
	reader := loginReader(t, f)
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/pages/"+path+"/restore",
		map[string]string{"version": version}, reader)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("reader restore = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}

	// Editor restores -> 200.
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/pages/"+path+"/restore",
		map[string]string{"version": version}, editor)
	if rec.Code != http.StatusOK {
		t.Fatalf("editor restore = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	// Poll the page until the restored (original) body comes back.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		getRec := doGet(t, f.handler, "/api/v1/pages/"+path, editor)
		var cur struct {
			Body string `json:"body"`
		}
		_ = json.Unmarshal(getRec.Body.Bytes(), &cur)
		if strings.TrimSpace(cur.Body) == strings.TrimSpace(orig.Body) {
			return // restored content is live — success
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("restored page never returned the original body")
}

// TestRestoreVersionAudits: a successful restore records an audit event (Action
// restore, actor = the session user).
func TestRestoreVersionAudits(t *testing.T) {
	st := newPageServerStore(t)
	f := st.fixture
	editor := loginEditor(t, f)
	path := createPageAs(t, f, editor, "notes", "Page")

	saveBody(t, f, editor, path, "# v2\n")
	entries := waitForHistoryLen(t, f, editor, path, 2)
	version, _ := entries[len(entries)-1]["version"].(string)

	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/pages/"+path+"/restore",
		map[string]string{"version": version}, editor)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if n := countAudit(t, st.store, audit.ActionPageRestore); n < 1 {
		if n2 := countAudit(t, st.store, "restore"); n2 < 1 {
			t.Fatalf("expected a restore audit row, got %d", n)
		}
	}
}
