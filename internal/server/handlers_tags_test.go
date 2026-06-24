// handlers_tags_test.go drives the per-page tag-suggestion endpoints (TAG-01)
// through the real HTTP seam (a full pages.Service backed by a real git repo, the
// pageFixture). It proves the load-bearing apply-tags safety properties:
//   - a STALE base_revision 409s and writes NOTHING (the concurrent-edit floor),
//   - the current base_revision writes exactly once (the gate blocks stale, not all),
//   - a tampered/un-normalized/over-cap client tag list is RE-normalized+capped+
//     deduped+filtered server-side before the write (the client list is never trusted),
//   - the write is byte-stable: the body is unchanged after apply, only the tags change,
//   - the suggest endpoint validates its input (400) and fails closed when the agent
//     is unconfigured.
//
// It is deterministic and KEY-FREE: apply-tags is a non-tool endpoint that never
// touches the model, so the 409/renormalize/byte-stable assertions need no API key.
package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// getPage reads a page via GET /pages/* and returns its frontmatter, body, and
// current revision (the optimistic-concurrency token apply-tags echoes back).
func getPage(t *testing.T, f *pageFixture, cookies []*http.Cookie, path string) (frontmatter, body, revision string) {
	t.Helper()
	rec := doGet(t, f.handler, "/api/v1/pages/"+path, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET page %q = %d, body=%s", path, rec.Code, rec.Body.String())
	}
	var resp struct {
		Frontmatter string `json:"frontmatter"`
		Body        string `json:"body"`
		Revision    string `json:"revision"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode page: %v", err)
	}
	return resp.Frontmatter, resp.Body, resp.Revision
}

func TestApplyTagsStaleRevision(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	path := createPageAs(t, f, editor, "notes", "Release Notes")

	_, bodyBefore, rev := getPage(t, f, editor, path)

	// Apply with a STALE base_revision (the page has NOT moved to this token) — must
	// 409 and write nothing.
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/agent/apply-tags",
		map[string]any{"page_path": path, "tags": []string{"release"}, "base_revision": "rev-does-not-match"}, editor)
	if rec.Code != http.StatusConflict {
		t.Fatalf("stale apply = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
	}

	// The page must be unchanged: same revision, same body, and no `release` tag.
	fmAfter, bodyAfter, revAfter := getPage(t, f, editor, path)
	if revAfter != rev {
		t.Fatalf("stale apply must write nothing, but revision moved %q → %q", rev, revAfter)
	}
	if bodyAfter != bodyBefore {
		t.Fatalf("stale apply changed the body")
	}
	if strings.Contains(fmAfter, "release") {
		t.Fatalf("stale apply wrote tags into frontmatter: %q", fmAfter)
	}

	// Control: applying with the CURRENT revision DOES write exactly once.
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/agent/apply-tags",
		map[string]any{"page_path": path, "tags": []string{"release"}, "base_revision": rev}, editor)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("fresh apply = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
	waitForRevisionChange(t, f, editor, path, rev)

	fmFinal, bodyFinal, newRev := getPage(t, f, editor, path)
	if !strings.Contains(fmFinal, "release") {
		t.Fatalf("fresh apply did not write the tag; frontmatter=%q", fmFinal)
	}
	if bodyFinal != bodyBefore {
		t.Fatalf("apply must be byte-stable on the body; body changed:\n before=%q\n after =%q", bodyBefore, bodyFinal)
	}
	if newRev == rev {
		t.Fatalf("fresh apply should have advanced the revision")
	}
}

func TestApplyTagsRenormalizesServerSide(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	path := createPageAs(t, f, editor, "notes", "Tamper Test")

	_, _, rev := getPage(t, f, editor, path)

	// A tampered payload: un-normalized casing/whitespace, duplicates, an over-cap
	// count (>MaxSuggestedTags), and garbage tokens (interior whitespace, over-length).
	long := strings.Repeat("z", 200)
	tampered := []string{
		"  Release ", // → "release"
		"release",    // duplicate of the above after normalization
		"RELEASE",    // duplicate
		"Notes",      // → "notes"
		"draft",
		"planning",
		"q2",
		"two words", // garbage: interior whitespace
		long,        // garbage: over-length
	}
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/agent/apply-tags",
		map[string]any{"page_path": path, "tags": tampered, "base_revision": rev}, editor)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("apply tampered = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
	waitForRevisionChange(t, f, editor, path, rev)

	fmFinal, _, _ := getPage(t, f, editor, path)

	// The written tags must be the cleaned/capped set — NOT the raw client list.
	// Garbage must be absent; the count must be capped to MaxSuggestedTags (5).
	if strings.Contains(fmFinal, "two words") || strings.Contains(fmFinal, long) {
		t.Fatalf("garbage tags reached the frontmatter (client list trusted): %q", fmFinal)
	}
	if strings.Contains(fmFinal, "Release") || strings.Contains(fmFinal, "RELEASE") || strings.Contains(fmFinal, "Notes") {
		t.Fatalf("un-normalized tags reached the frontmatter: %q", fmFinal)
	}
	// Exactly the first 5 surviving normalized tokens: release, notes, draft, planning, q2.
	for _, want := range []string{"release", "notes", "draft", "planning", "q2"} {
		if !strings.Contains(fmFinal, want) {
			t.Fatalf("expected normalized tag %q missing from frontmatter: %q", want, fmFinal)
		}
	}
	// The 6th-surviving real token (none here beyond q2) and dupes must not inflate
	// the count. Count the block-style `- ` entries under tags to prove the cap.
	if n := countTagLines(fmFinal); n != 5 {
		t.Fatalf("wrote %d tag lines, want exactly %d (cap + dedupe); frontmatter=%q", n, 5, fmFinal)
	}
}

func TestApplyTagsEmptyAfterNormalize(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	path := createPageAs(t, f, editor, "notes", "Empty Tags")
	_, _, rev := getPage(t, f, editor, path)

	// All entries are garbage → normalizes to empty → 422 (no clean tags to apply).
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/agent/apply-tags",
		map[string]any{"page_path": path, "tags": []string{"   ", "two words", strings.Repeat("z", 200)}, "base_revision": rev}, editor)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("all-garbage apply = %d, want 422 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestApplyTagsRBAC(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	path := createPageAs(t, f, editor, "notes", "RBAC Tags")
	_, _, rev := getPage(t, f, editor, path)

	// A reader cannot apply tags — the editor gate blocks the write (403).
	reader := loginReader(t, f)
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/agent/apply-tags",
		map[string]any{"page_path": path, "tags": []string{"release"}, "base_revision": rev}, reader)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("reader apply-tags = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestApplyTagsBadRequest(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	path := createPageAs(t, f, editor, "notes", "Bad Request")
	_, _, rev := getPage(t, f, editor, path)

	// Empty tags list → 400 (nothing to apply).
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/agent/apply-tags",
		map[string]any{"page_path": path, "tags": []string{}, "base_revision": rev}, editor)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty tags apply = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}

	// Missing page_path → 400.
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/agent/apply-tags",
		map[string]any{"page_path": "", "tags": []string{"release"}, "base_revision": rev}, editor)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing page_path apply = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestSuggestTagsEndpoint covers the read endpoint's fail-closed behaviour and its
// editor-or-reader read gating. The full happy path (a vocab-biased capped
// suggestion from a fake model) + the input-validation 400 paths are proven
// key-free at the agent seam (TestSuggestTags) and at the apply seam
// (TestApplyTagsBadRequest) respectively. The server.New fixture wires no agent
// (h.agent == nil), so every suggest request fails CLOSED (500) — never a hang —
// which is the load-bearing safety property at the HTTP layer.
func TestSuggestTagsEndpoint(t *testing.T) {
	f := newPageServer(t)
	editor := loginEditor(t, f)
	path := createPageAs(t, f, editor, "notes", "Suggest Endpoint")

	// A valid page with NO agent configured → fails closed (500), never a hang.
	rec := doMutate(t, f.handler, http.MethodPost, "/api/v1/agent/suggest-tags",
		map[string]any{"page_path": path}, editor)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("suggest with no agent = %d, want 500 fail-closed (body=%s)", rec.Code, rec.Body.String())
	}

	// suggest-tags is any-authed (read): a reader reaches the handler too (and also
	// fails closed at 500 with no agent — never a 403, since it is not editor-gated).
	reader := loginReader(t, f)
	rec = doMutate(t, f.handler, http.MethodPost, "/api/v1/agent/suggest-tags",
		map[string]any{"page_path": path}, reader)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("reader suggest with no agent = %d, want 500 (any-authed, not editor-gated) (body=%s)", rec.Code, rec.Body.String())
	}
}

// countTagLines counts the block-style `- ` sequence entries under the `tags:` key
// in a frontmatter region (proving the cap/dedupe held). It scans only the lines
// after `tags:` until the next top-level key or end.
func countTagLines(frontmatter string) int {
	lines := strings.Split(frontmatter, "\n")
	inTags := false
	n := 0
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if strings.HasPrefix(trimmed, "tags:") {
			inTags = true
			continue
		}
		if inTags {
			if strings.HasPrefix(trimmed, "- ") {
				n++
				continue
			}
			// A non-list, non-blank line that is a top-level key ends the tags block.
			if trimmed != "" && !strings.HasPrefix(ln, " ") && !strings.HasPrefix(ln, "\t") {
				break
			}
		}
	}
	return n
}
