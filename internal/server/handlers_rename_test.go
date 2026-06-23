package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/audit"
)

func TestRenameDispatch_Title(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	target := createPageAs(t, f, editor, "runbooks", "Deploy")    // runbooks/deploy.md
	linker := createPageAs(t, f, editor, "architecture", "Overview")

	// Give the linker an inbound link to the target.
	saveBody(t, f, editor, linker, "See [Deploy](../runbooks/deploy.md).\n")

	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/pages/"+target+"/rename",
		map[string]string{"new_title": "Deploy Prod"}, editor)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode rename resp: %v", err)
	}
	if resp.Path != "runbooks/deploy-prod.md" {
		t.Fatalf("new path = %q, want runbooks/deploy-prod.md", resp.Path)
	}
	waitForPath(t, f, resp.Path)

	// The linking page, fetched after the rename, has the rewritten link.
	waitForLinkRewrite(t, f, editor, linker, "../runbooks/deploy-prod.md")
}

func TestRenameDispatch_Move(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	target := createPageAs(t, f, editor, "runbooks", "Deploy") // runbooks/deploy.md
	linker := createPageAs(t, f, editor, "architecture", "Overview")
	saveBody(t, f, editor, linker, "See [Deploy](../runbooks/deploy.md).\n")

	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/pages/"+target+"/rename",
		map[string]string{"new_parent": "architecture"}, editor)
	if rec.Code != http.StatusOK {
		t.Fatalf("move = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Path != "architecture/deploy.md" {
		t.Fatalf("moved path = %q, want architecture/deploy.md", resp.Path)
	}
	waitForPath(t, f, resp.Path)
	// Linker is now a sibling; the link recomputes to deploy.md.
	waitForLinkRewrite(t, f, editor, linker, "[Deploy](deploy.md)")
}

func TestRenameDispatch_BadBody(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	target := createPageAs(t, f, editor, "runbooks", "Deploy")

	// Both fields set -> 400, no mutation.
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/pages/"+target+"/rename",
		map[string]string{"new_title": "X", "new_parent": "architecture"}, editor)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("both fields = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
	// Neither field set -> 400.
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/pages/"+target+"/rename",
		map[string]string{}, editor)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("neither field = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
	// The page is untouched.
	time.Sleep(30 * time.Millisecond)
	if ok, _ := f.repo.Exists(target); !ok {
		t.Fatal("original page was mutated by a rejected rename request")
	}
}

func TestRenameHandlerRBAC(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	target := createPageAs(t, f, editor, "runbooks", "Deploy")

	// Reader -> 403.
	reader := loginReader(t, f)
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/pages/"+target+"/rename",
		map[string]string{"new_title": "Nope"}, reader)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("reader rename = %d, want 403", rec.Code)
	}

	// Editor -> 200.
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/pages/"+target+"/rename",
		map[string]string{"new_title": "Deploy Prod"}, editor)
	if rec.Code != http.StatusOK {
		t.Fatalf("editor rename = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestRenameAuditsSeparately(t *testing.T) {
	st := newPageServerStore(t)
	f := st.fixture
	editor := loginEditor(t, f)

	t1 := createPageAs(t, f, editor, "runbooks", "Deploy")
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/pages/"+t1+"/rename",
		map[string]string{"new_title": "Deploy Prod"}, editor)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename = %d", rec.Code)
	}
	if n := countAudit(t, st.store, audit.ActionPageRename); n < 1 {
		t.Fatalf("expected a rename audit row, got %d", n)
	}

	t2 := createPageAs(t, f, editor, "runbooks", "Restart")
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/pages/"+t2+"/rename",
		map[string]string{"new_parent": "architecture"}, editor)
	if rec.Code != http.StatusOK {
		t.Fatalf("move = %d", rec.Code)
	}
	if n := countAudit(t, st.store, audit.ActionPageMove); n < 1 {
		t.Fatalf("expected a move audit row, got %d", n)
	}
}

// --- helpers ---

// saveBody saves a body to a page (reading its current revision first).
func saveBody(t *testing.T, f *pageFixture, cookies []*http.Cookie, path, body string) {
	t.Helper()
	getRec := doGet(t, f.handler, "/api/v1/pages/"+path, cookies)
	var page struct {
		Frontmatter string `json:"frontmatter"`
		Revision    string `json:"revision"`
	}
	_ = json.Unmarshal(getRec.Body.Bytes(), &page)
	rec := doMutate(t, f.handler, http.MethodPut, "/api/v1/pages/"+path,
		map[string]string{"body": body, "frontmatter": page.Frontmatter, "base_revision": page.Revision}, cookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("saveBody %q = %d (body=%s)", path, rec.Code, rec.Body.String())
	}
	// Wait for the save to commit (revision changes).
	waitForRevisionChange(t, f, cookies, path, page.Revision)
}

func waitForRevisionChange(t *testing.T, f *pageFixture, cookies []*http.Cookie, path, prev string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		getRec := doGet(t, f.handler, "/api/v1/pages/"+path, cookies)
		var page struct {
			Revision string `json:"revision"`
		}
		_ = json.Unmarshal(getRec.Body.Bytes(), &page)
		if page.Revision != "" && page.Revision != prev {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("revision of %q never changed (commit did not drain)", path)
}

func waitForLinkRewrite(t *testing.T, f *pageFixture, cookies []*http.Cookie, path, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		getRec := doGet(t, f.handler, "/api/v1/pages/"+path, cookies)
		var page struct {
			Body string `json:"body"`
		}
		_ = json.Unmarshal(getRec.Body.Bytes(), &page)
		if containsStr(page.Body, want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("link in %q was never rewritten to contain %q", path, want)
}

func containsStr(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
