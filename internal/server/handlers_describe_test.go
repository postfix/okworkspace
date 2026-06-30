// handlers_describe_test.go drives the per-page description endpoints (ED-01b)
// through the real HTTP seam (a full pages.Service backed by a real git repo, the
// pageFixture). It proves the load-bearing apply-description safety properties:
//   - a STALE base_revision 409s and writes NOTHING (the concurrent-edit floor),
//   - the current base_revision writes exactly once (the gate blocks stale, not all),
//   - the write is byte-stable: the body + every other frontmatter key are unchanged
//     after apply, only `description:` changes,
//   - a server-invalid approved description (newline / over-cap) is rejected 422
//     server-side (the client value is never trusted verbatim),
//   - apply is editor-gated (reader → 403) and CSRF-protected (no token → rejected),
//   - the generate endpoint fails closed when the agent is unconfigured.
//
// It is deterministic and KEY-FREE: apply-description is a non-tool endpoint that
// never touches the model, so the 409 / re-validation / byte-stable assertions need
// no API key (the generate happy path's model logic is proven at the agent seam in
// internal/agent/describe_test.go). It reuses getPage / countTagLines from
// handlers_tags_test.go (same server_test package).
package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApplyDescriptionStaleRevision(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	path := createPageAs(t, f, editor, "notes", "Release Notes")

	fmBefore, bodyBefore, rev := getPage(t, f, editor, path)
	const desc = "A concise note about the 2.0 release process."

	// Apply with a STALE base_revision (the page has NOT moved to this token) — must
	// 409 and write nothing.
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/agent/apply-description",
		map[string]any{"page_path": path, "description": desc, "base_revision": "rev-does-not-match"}, editor)
	if rec.Code != http.StatusConflict {
		t.Fatalf("stale apply = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
	}

	// The page must be unchanged: same revision, same body, no description written.
	fmAfter, bodyAfter, revAfter := getPage(t, f, editor, path)
	if revAfter != rev {
		t.Fatalf("stale apply must write nothing, but revision moved %q → %q", rev, revAfter)
	}
	if bodyAfter != bodyBefore {
		t.Fatalf("stale apply changed the body")
	}
	if strings.Contains(fmAfter, desc) {
		t.Fatalf("stale apply wrote the description into frontmatter: %q", fmAfter)
	}

	// Control: applying with the CURRENT revision DOES write exactly once, byte-stably.
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/agent/apply-description",
		map[string]any{"page_path": path, "description": desc, "base_revision": rev}, editor)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("fresh apply = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
	waitForRevisionChange(t, f, editor, path, rev)

	fmFinal, bodyFinal, newRev := getPage(t, f, editor, path)
	if !strings.Contains(fmFinal, desc) {
		t.Fatalf("fresh apply did not write the description; frontmatter=%q", fmFinal)
	}
	if bodyFinal != bodyBefore {
		t.Fatalf("apply must be byte-stable on the body; body changed:\n before=%q\n after =%q", bodyBefore, bodyFinal)
	}
	if newRev == rev {
		t.Fatalf("fresh apply should have advanced the revision")
	}
	// Byte-stability of the OTHER keys: the title is untouched and the tag count is
	// unchanged (description is a scalar — it never disturbs the tags region).
	if !strings.Contains(fmFinal, "title: Release Notes") {
		t.Fatalf("apply disturbed the title key; frontmatter=%q", fmFinal)
	}
	if before, after := countTagLines(fmBefore), countTagLines(fmFinal); before != after {
		t.Fatalf("apply changed the tags region (%d → %d lines); frontmatter=%q", before, after, fmFinal)
	}
}

func TestApplyDescriptionRevalidatesServerSide(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	path := createPageAs(t, f, editor, "notes", "Revalidate Desc")
	_, _, rev := getPage(t, f, editor, path)

	cases := []struct {
		name string
		desc string
	}{
		{"newline (multi-line)", "Line one.\nLine two."},
		{"carriage return", "Has\rcr"},
		{"over the length cap", strings.Repeat("word ", 60)},
		{"empty after trim", "   "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/agent/apply-description",
				map[string]any{"page_path": path, "description": tc.desc, "base_revision": rev}, editor)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("invalid description apply = %d, want 422 (body=%s)", rec.Code, rec.Body.String())
			}
		})
	}

	// The page is untouched after all the rejected applies (nothing was written).
	fmAfter, _, revAfter := getPage(t, f, editor, path)
	if revAfter != rev {
		t.Fatalf("a rejected (422) apply must write nothing; revision moved %q → %q", rev, revAfter)
	}
	if strings.Contains(fmAfter, "Line one") || strings.Contains(fmAfter, "word word") {
		t.Fatalf("a rejected description leaked into frontmatter: %q", fmAfter)
	}
}

func TestApplyDescriptionRBAC(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	path := createPageAs(t, f, editor, "notes", "RBAC Desc")
	_, _, rev := getPage(t, f, editor, path)

	// A reader cannot apply a description — the editor gate blocks the write (403).
	reader := loginReader(t, f)
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/agent/apply-description",
		map[string]any{"page_path": path, "description": "x", "base_revision": rev}, reader)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("reader apply-description = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestApplyDescriptionBadRequest(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	path := createPageAs(t, f, editor, "notes", "Bad Desc Request")
	_, _, rev := getPage(t, f, editor, path)

	// Missing page_path → 400.
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/agent/apply-description",
		map[string]any{"page_path": "", "description": "x", "base_revision": rev}, editor)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing page_path apply = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestApplyDescriptionMissingCSRF proves apply-description sits inside the global
// nosurf-protected group: a mutating POST with NO CSRF token is rejected and writes
// nothing (the client list / value can never reach pages.Save without a valid token).
func TestApplyDescriptionMissingCSRF(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	path := createPageAs(t, f, editor, "notes", "CSRF Desc")
	_, _, rev := getPage(t, f, editor, path)

	body, _ := json.Marshal(map[string]any{
		"page_path": path, "description": "A description.", "base_revision": rev,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/apply-description", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	for _, c := range editor { // session + csrf COOKIE, but NO X-CSRF-Token header
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusNoContent {
		t.Fatalf("apply-description without a CSRF token must be rejected, got 204")
	}

	// And nothing was written.
	fmAfter, _, revAfter := getPage(t, f, editor, path)
	if revAfter != rev || strings.Contains(fmAfter, "A description.") {
		t.Fatalf("a CSRF-rejected apply wrote to the page; rev %q→%q frontmatter=%q", rev, revAfter, fmAfter)
	}
}

// TestGenerateDescriptionEndpoint covers the read endpoint's fail-closed behaviour
// and its any-authed (editor-or-reader) gating. The full happy path (a validated
// one-sentence description from a fake model + the base-revision capture) is proven
// key-free at the agent seam (TestGenerateDescription). The server.New fixture wires
// no agent (h.agent == nil), so every generate request fails CLOSED (500) — never a
// hang — which is the load-bearing safety property at the HTTP layer.
func TestGenerateDescriptionEndpoint(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	path := createPageAs(t, f, editor, "notes", "Generate Endpoint")

	// A valid page with NO agent configured → fails closed (500), never a hang.
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/agent/generate-description",
		map[string]any{"page_path": path}, editor)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("generate with no agent = %d, want 500 fail-closed (body=%s)", rec.Code, rec.Body.String())
	}

	// generate-description is any-authed (read): a reader reaches the handler too (and
	// also fails closed at 500 with no agent — never a 403, since it is not editor-gated).
	reader := loginReader(t, f)
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/agent/generate-description",
		map[string]any{"page_path": path}, reader)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("reader generate with no agent = %d, want 500 (any-authed, not editor-gated) (body=%s)", rec.Code, rec.Body.String())
	}

	// Missing page_path with an agent absent still fails closed at the agent-nil guard
	// (500) — the handler checks h.agent first; this asserts no panic / no hang.
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/agent/generate-description",
		map[string]any{"page_path": ""}, editor)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("missing page_path generate (no agent) = %d, want 500 (body=%s)", rec.Code, rec.Body.String())
	}
}
