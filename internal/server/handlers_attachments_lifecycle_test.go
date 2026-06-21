package server_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/postfix/okworkspace/internal/attachments"
	"github.com/postfix/okworkspace/internal/users"
)

// loginReaderAttach creates a reader user on the attach fixture and logs in, so the
// editor-gate rejection tests can use a non-editor session (RBAC from the session,
// never client input — T-02-14).
func loginReaderAttach(t *testing.T, f *attachFixture) []*http.Cookie {
	t.Helper()
	_, otp, err := users.Create(context.Background(), f.repoo, users.NewUser{Username: "rdr", DisplayName: "Reader", Role: users.RoleReader})
	if err != nil {
		t.Fatalf("Create reader: %v", err)
	}
	rd, _ := f.repoo.GetByUsername(context.Background(), "rdr")
	if err := users.ChangeOwnPassword(context.Background(), f.repoo, rd.ID, otp, "reader-long-password"); err != nil {
		t.Fatalf("set reader password: %v", err)
	}
	return loginAs(t, f.handler, "rdr", "reader-long-password")
}

// seedPage writes a Markdown page directly through the repo so a Remove has a page
// whose link it can strip (the upload path does not auto-insert the link in MVP).
func seedPage(t *testing.T, f *attachFixture, path, body string) {
	t.Helper()
	if err := f.repo.Write(path, []byte(body)); err != nil {
		t.Fatalf("seed page %q: %v", path, err)
	}
}

// replaceFile PUTs a multipart replacement of an attachment's bytes under the
// "file" field and returns the recorder.
func replaceFile(t *testing.T, f *attachFixture, cookies []*http.Cookie, id, filename string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	token, csrfCookies := fetchCSRF(t, f.handler)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/attachments/"+id, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-CSRF-Token", token)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	for _, c := range csrfCookies {
		req.AddCookie(c)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

// deleteAttachment DELETEs an attachment, passing the page to unlink via the
// page_path query parameter, and returns the recorder.
func deleteAttachment(t *testing.T, f *attachFixture, cookies []*http.Cookie, id, pagePath string) *httptest.ResponseRecorder {
	t.Helper()
	token, csrfCookies := fetchCSRF(t, f.handler)

	u := "/api/v1/attachments/" + id + "?page_path=" + url.QueryEscape(pagePath)
	req := httptest.NewRequest(http.MethodDelete, u, nil)
	req.Header.Set("X-CSRF-Token", token)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	for _, c := range csrfCookies {
		req.AddCookie(c)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

// TestReplaceAttachment (ATT-05): PUT /attachments/{id} with a new file → 200, the
// SAME id, and the new size in the returned meta. Bad input still re-validates
// (413 oversize / 415 disallowed type).
func TestReplaceAttachment(t *testing.T) {
	f := newAttachServer(t)
	cookies := loginEditorAttach(t, f)

	orig := readFixture(t, "sample.txt")
	urec := uploadFile(t, f, cookies, "runbooks/deploy.md", "sample.txt", orig)
	if urec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201; body=%s", urec.Code, urec.Body.String())
	}
	var up uploadedMeta
	decodeJSON(t, urec, &up)

	newBytes := append([]byte("REPLACED: "), orig...)
	rec := replaceFile(t, f, cookies, up.ID, "sample-v2.txt", newBytes)
	if rec.Code != http.StatusOK {
		t.Fatalf("replace status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var rep uploadedMeta
	decodeJSON(t, rec, &rep)
	if rep.ID != up.ID {
		t.Fatalf("replace changed id: got %q, want %q", rep.ID, up.ID)
	}
	if rep.SizeBytes != int64(len(newBytes)) {
		t.Fatalf("replace size = %d, want %d", rep.SizeBytes, len(newBytes))
	}

	// The download now returns the NEW bytes under the same id (ATT-05).
	drec := download(t, f, cookies, up.ID)
	if !bytes.Equal(drec.Body.Bytes(), newBytes) {
		t.Fatalf("download after replace != replaced bytes")
	}

	// Re-validation on bad input.
	big := bytes.Repeat([]byte("A"), 2<<20)
	if rrec := replaceFile(t, f, cookies, up.ID, "big.txt", big); rrec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize replace status = %d, want 413; body=%s", rrec.Code, rrec.Body.String())
	}
	elf := append([]byte{0x7f, 'E', 'L', 'F'}, bytes.Repeat([]byte{0}, 64)...)
	if erec := replaceFile(t, f, cookies, up.ID, "evil.txt", elf); erec.Code != http.StatusUnsupportedMediaType && erec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("disallowed-type replace status = %d, want 415/422; body=%s", erec.Code, erec.Body.String())
	}
}

// TestRemoveAttachment (ATT-06/07): DELETE /attachments/{id} with the page_path →
// 200; when it was the last reference, the files are gone (download → 404). When a
// second page still references the id, the files remain (download → 200).
func TestRemoveAttachment(t *testing.T) {
	f := newAttachServer(t)
	cookies := loginEditorAttach(t, f)

	data := readFixture(t, "sample.txt")
	urec := uploadFile(t, f, cookies, "notes.md", "doc.txt", data)
	if urec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201; body=%s", urec.Code, urec.Body.String())
	}
	var up uploadedMeta
	decodeJSON(t, urec, &up)
	ref := attachments.DownloadRefPath(up.ID)

	// Shared case first: two pages reference the id; removing from one keeps files.
	seedPage(t, f, "notes.md", "Notes [doc]("+ref+").\n")
	seedPage(t, f, "other.md", "Other [doc]("+ref+") too.\n")

	srec := deleteAttachment(t, f, cookies, up.ID, "notes.md")
	if srec.Code != http.StatusOK {
		t.Fatalf("shared delete status = %d, want 200; body=%s", srec.Code, srec.Body.String())
	}
	if dl := download(t, f, cookies, up.ID); dl.Code != http.StatusOK {
		t.Fatalf("shared download after remove = %d, want 200 (file kept)", dl.Code)
	}

	// Now remove the LAST reference (other.md) → orphan delete; download 404s.
	orec := deleteAttachment(t, f, cookies, up.ID, "other.md")
	if orec.Code != http.StatusOK {
		t.Fatalf("orphan delete status = %d, want 200; body=%s", orec.Code, orec.Body.String())
	}
	if dl := download(t, f, cookies, up.ID); dl.Code != http.StatusNotFound {
		t.Fatalf("download after orphan delete = %d, want 404 (file gone); body=%s", dl.Code, dl.Body.String())
	}
}

// TestReplaceRemoveEditorOnly (T-02-14): a reader (non-editor) session is rejected
// on both the PUT replace and DELETE remove routes (RBAC from the session).
func TestReplaceRemoveEditorOnly(t *testing.T) {
	f := newAttachServer(t)
	editor := loginEditorAttach(t, f)

	// Editor uploads a real attachment to target.
	data := readFixture(t, "sample.txt")
	urec := uploadFile(t, f, editor, "notes.md", "doc.txt", data)
	if urec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201; body=%s", urec.Code, urec.Body.String())
	}
	var up uploadedMeta
	decodeJSON(t, urec, &up)

	reader := loginReaderAttach(t, f)

	if rec := replaceFile(t, f, reader, up.ID, "x.txt", []byte("nope")); rec.Code != http.StatusForbidden {
		t.Fatalf("reader replace status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if rec := deleteAttachment(t, f, reader, up.ID, "notes.md"); rec.Code != http.StatusForbidden {
		t.Fatalf("reader delete status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}
