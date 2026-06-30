package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestListTemplatesEndpoint: GET /templates returns 200 + the flat Templates/*.md
// files (name without extension), available to a reader (any-authed read model),
// and empty when there is no Templates/ folder.
func TestListTemplatesEndpoint(t *testing.T) {
	f := newPageServer(t)
	readerCookies := loginReader(t, f)

	// No Templates/ folder yet → empty list, 200.
	rec := doGet(t, f.handler, "/api/v1/templates", readerCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET templates (empty) = %d, body=%s", rec.Code, rec.Body.String())
	}
	var empty struct {
		Templates []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"templates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &empty); err != nil {
		t.Fatalf("decode empty templates: %v", err)
	}
	if len(empty.Templates) != 0 {
		t.Fatalf("want empty templates, got %v", empty.Templates)
	}

	// Drop two templates + noise directly on disk.
	if err := f.repo.Write("Templates/Meeting.md", []byte("---\ntype: Meeting\n---\nBody\n")); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := f.repo.Write("Templates/Daily.md", []byte("Body only\n")); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := f.repo.Write("Templates/nested/Deep.md", []byte("x\n")); err != nil {
		t.Fatalf("write nested: %v", err)
	}

	rec2 := doGet(t, f.handler, "/api/v1/templates", readerCookies)
	if rec2.Code != http.StatusOK {
		t.Fatalf("GET templates = %d, body=%s", rec2.Code, rec2.Body.String())
	}
	var got struct {
		Templates []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"templates"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode templates: %v", err)
	}
	names := map[string]string{}
	for _, ti := range got.Templates {
		names[ti.Name] = ti.Path
	}
	if len(got.Templates) != 2 {
		t.Fatalf("want 2 flat templates, got %d: %v", len(got.Templates), got.Templates)
	}
	if names["Meeting"] != "Templates/Meeting.md" || names["Daily"] != "Templates/Daily.md" {
		t.Fatalf("unexpected template list: %v", names)
	}
	if _, ok := names["Deep"]; ok {
		t.Fatal("nested template leaked into the list")
	}
}

// TestCreatePageFromTemplate: POST /pages with Frontmatter+Body seeds the new
// page from the (substituted) template content — the created page has the merged
// single-block frontmatter (user title wins, template fields carried, required
// fields repaired) + the seeded body.
func TestCreatePageFromTemplate(t *testing.T) {
	f := newPageServer(t)
	editorCookies := loginEditor(t, f)

	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/pages",
		map[string]string{
			"folder":      "runbooks",
			"title":       "Sprint Review",
			"frontmatter": "title: Template Title\ntype: Meeting\ntags: [planning]",
			"body":        "# Sprint Review\n\nAgenda here.\n",
		}, editorCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create from template = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if resp.Path != "runbooks/sprint-review.md" {
		t.Fatalf("path = %q, want runbooks/sprint-review.md (derived from user title)", resp.Path)
	}
	waitForPath(t, f, resp.Path)

	raw, err := f.repo.Read(resp.Path)
	if err != nil {
		t.Fatalf("read created page: %v", err)
	}
	page := string(raw)
	for _, want := range []string{"title: Sprint Review", "type: Meeting", "planning", "description:", "timestamp:", "Agenda here."} {
		if !strings.Contains(page, want) {
			t.Fatalf("created page missing %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "Template Title") {
		t.Fatalf("template title leaked into the page:\n%s", page)
	}
}

// TestCreatePageBlankUnchanged: POST /pages WITHOUT a seed is the existing
// title-only blank create (no template path taken).
func TestCreatePageBlankUnchanged(t *testing.T) {
	f := newPageServer(t)
	editorCookies := loginEditor(t, f)
	path := createPageAs(t, f, editorCookies, "", "Plain Page")
	raw, err := f.repo.Read(path)
	if err != nil {
		t.Fatalf("read created page: %v", err)
	}
	if !strings.Contains(string(raw), "title: Plain Page") || !strings.Contains(string(raw), "type: Page") {
		t.Fatalf("blank create produced unexpected page:\n%s", raw)
	}
}

// TestCreatePageFromTemplateRBAC: a reader is forbidden (editor gate) and a
// missing CSRF token is rejected — the seeded create rides the SAME editor+CSRF
// gate as the blank create.
func TestCreatePageFromTemplateRBAC(t *testing.T) {
	f := newPageServer(t)
	readerCookies := loginReader(t, f)
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/pages",
		map[string]string{"folder": "", "title": "Nope", "body": "seeded\n"}, readerCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("reader create-from-template = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}

	// Missing CSRF: editor cookies but no X-CSRF-Token header → rejected by the
	// nosurf gate (the seeded create rides the same CSRF gate as the blank create).
	editorCookies := loginEditor(t, f)
	b, _ := json.Marshal(map[string]string{"folder": "", "title": "NoCSRF", "body": "seeded\n"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pages", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range editorCookies {
		req.AddCookie(c)
	}
	rec2 := httptest.NewRecorder()
	f.handler.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusForbidden && rec2.Code != http.StatusBadRequest {
		t.Fatalf("missing-CSRF create = %d, want 403/400 (body=%s)", rec2.Code, rec2.Body.String())
	}
}

// TestCreatePageFromTemplateMalformedRejected: a duplicate-key / malformed
// template seed fails closed with a clean 400 (never a corrupt page).
func TestCreatePageFromTemplateMalformedRejected(t *testing.T) {
	f := newPageServer(t)
	editorCookies := loginEditor(t, f)
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/pages",
		map[string]string{
			"folder":      "",
			"title":       "Dup Page",
			"frontmatter": "title: A\ntitle: B\ntags: [x]\ntags: [y]",
			"body":        "Body\n",
		}, editorCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("duplicate-key template = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}
