package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/attachments"
	"github.com/postfix/okworkspace/internal/audit"
	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/pages"
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/server"
	"github.com/postfix/okworkspace/internal/store"
	"github.com/postfix/okworkspace/internal/users"
)

// attachFixture wires a full server with a real attachments.Service (repo + git
// + worker) so the upload/download/list routes are exercised end to end (the
// same harness shape as newPageServer, plus the attachments service).
type attachFixture struct {
	handler http.Handler
	repo    *repo.Repo
	repoo   *users.Repository
}

func newAttachServer(t *testing.T) *attachFixture {
	t.Helper()
	if _, err := lookGit(); err != nil {
		t.Skip("git binary not available")
	}

	st, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	contentRepo, err := repo.New(filepath.Join(t.TempDir(), "repo"))
	if err != nil {
		t.Fatalf("repo.New: %v", err)
	}
	t.Cleanup(func() { _ = contentRepo.Close() })
	gs := gitstore.New(contentRepo, config.GitConfig{Enabled: true, Branch: "main"})
	if err := gs.Init(context.Background()); err != nil {
		t.Fatalf("gitstore.Init: %v", err)
	}

	w := jobs.New(st.DB(), jobs.Config{PollInterval: 5 * time.Millisecond})
	w.Register(pages.KindCommit, pages.CommitHandler(contentRepo, gs))
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	t.Cleanup(func() { w.Stop(); cancel() })

	pagesSvc := pages.NewService(contentRepo, gs, w, st.DB(), false)

	var cfg config.Config
	cfg.Auth.SessionCookieName = config.DefaultSessionCookieName
	cfg.Auth.SessionTTLHours = config.DefaultSessionTTLHours
	cfg.Admin.Username = "admin"
	cfg.Storage.MaxUploadMB = 1 // 1 MB cap so the oversize test stays small
	cfg.Attachments.AllowedExtensions = []string{"pdf", "docx", "txt", "png", "jpg", "svg"}

	attachSvc := attachments.NewService(contentRepo, w, st.DB(), cfg.Attachments, cfg.Storage.MaxUploadMB, false)

	userRepo := users.NewRepository(st.DB())
	if _, _, _, err := users.BootstrapAdmin(context.Background(), userRepo, cfg); err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	h, err := server.New(server.Deps{
		Store:       st,
		Config:      cfg,
		UserRepo:    userRepo,
		Audit:       audit.New(st.DB(), nil),
		Pages:       pagesSvc,
		Attachments: attachSvc,
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return &attachFixture{handler: h, repo: contentRepo, repoo: userRepo}
}

func lookGit() (string, error) { return exec.LookPath("git") }

// decodeJSON unmarshals a recorder body into v.
func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("decode JSON: %v; body=%s", err, rec.Body.String())
	}
}

// contains reports whether s contains substr (thin strings.Contains alias for
// readable assertions).
func contains(s, substr string) bool { return strings.Contains(s, substr) }

// loginEditorAttach creates an editor user and logs in.
func loginEditorAttach(t *testing.T, f *attachFixture) []*http.Cookie {
	t.Helper()
	_, otp, err := users.Create(context.Background(), f.repoo, users.NewUser{Username: "ed", DisplayName: "Ed", Role: users.RoleEditor})
	if err != nil {
		t.Fatalf("Create editor: %v", err)
	}
	ed, _ := f.repoo.GetByUsername(context.Background(), "ed")
	if err := users.ChangeOwnPassword(context.Background(), f.repoo, ed.ID, otp, "editor-long-password"); err != nil {
		t.Fatalf("set editor password: %v", err)
	}
	return loginAs(t, f.handler, "ed", "editor-long-password")
}

// uploadFile POSTs a multipart upload of the given bytes under the "file" field
// (with page_path) and returns the response recorder.
func uploadFile(t *testing.T, f *attachFixture, cookies []*http.Cookie, pagePath, filename string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	token, csrfCookies := fetchCSRF(t, f.handler)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("page_path", pagePath)
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

	req := httptest.NewRequest(http.MethodPost, "/api/v1/attachments", &body)
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

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "attachments", name))
	if err != nil {
		t.Fatalf("read fixture %q: %v", name, err)
	}
	return data
}

// uploadedMeta is the subset of the upload/list response the tests assert on.
type uploadedMeta struct {
	ID           string `json:"id"`
	OriginalName string `json:"original_name"`
	MimeType     string `json:"mime_type"`
	SizeBytes    int64  `json:"size_bytes"`
}

// TestDownloadByteExact (ATT-02): an uploaded attachment downloads byte-for-byte
// identical to the uploaded bytes.
func TestDownloadByteExact(t *testing.T) {
	f := newAttachServer(t)
	cookies := loginEditorAttach(t, f)

	original := readFixture(t, "sample.txt")
	rec := uploadFile(t, f, cookies, "runbooks/deploy.md", "sample.txt", original)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var meta uploadedMeta
	decodeJSON(t, rec, &meta)
	if meta.ID == "" {
		t.Fatal("upload returned empty id")
	}

	dreq := httptest.NewRequest(http.MethodGet, "/api/v1/attachments/"+meta.ID+"/download", nil)
	for _, c := range cookies {
		dreq.AddCookie(c)
	}
	drec := httptest.NewRecorder()
	f.handler.ServeHTTP(drec, dreq)
	if drec.Code != http.StatusOK {
		t.Fatalf("download status = %d, want 200; body=%s", drec.Code, drec.Body.String())
	}
	if !bytes.Equal(drec.Body.Bytes(), original) {
		t.Fatalf("download bytes != upload bytes (len %d vs %d)", drec.Body.Len(), len(original))
	}
}

// TestUploadValidation (ATT-09): oversize → 413; disallowed sniffed type → 415/422.
func TestUploadValidation(t *testing.T) {
	f := newAttachServer(t)
	cookies := loginEditorAttach(t, f)

	// Oversize: 2 MB > the 1 MB cap.
	big := bytes.Repeat([]byte("A"), 2<<20)
	rec := uploadFile(t, f, cookies, "runbooks/deploy.md", "big.txt", big)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize upload status = %d, want 413; body=%s", rec.Code, rec.Body.String())
	}

	// Disallowed type: a Windows PE/ELF-like blob sniffs to a type not on the
	// allow-list. Use an ELF magic header.
	elf := append([]byte{0x7f, 'E', 'L', 'F'}, bytes.Repeat([]byte{0}, 64)...)
	rec2 := uploadFile(t, f, cookies, "runbooks/deploy.md", "evil.txt", elf)
	if rec2.Code != http.StatusUnsupportedMediaType && rec2.Code != http.StatusUnprocessableEntity {
		t.Fatalf("disallowed-type upload status = %d, want 415 or 422; body=%s", rec2.Code, rec2.Body.String())
	}
}

// TestDownloadDisposition (SEC-02): non-image → attachment + nosniff; image → inline.
func TestDownloadDisposition(t *testing.T) {
	f := newAttachServer(t)
	cookies := loginEditorAttach(t, f)

	// Non-image (txt): attachment + nosniff.
	txt := readFixture(t, "sample.txt")
	rec := uploadFile(t, f, cookies, "runbooks/deploy.md", "sample.txt", txt)
	if rec.Code != http.StatusCreated {
		t.Fatalf("txt upload status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var tmeta uploadedMeta
	decodeJSON(t, rec, &tmeta)
	drec := download(t, f, cookies, tmeta.ID)
	if cd := drec.Header().Get("Content-Disposition"); cd == "" || !contains(cd, "attachment") {
		t.Fatalf("txt Content-Disposition = %q, want attachment", cd)
	}
	if drec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("txt X-Content-Type-Options = %q, want nosniff", drec.Header().Get("X-Content-Type-Options"))
	}

	// Image (png): inline.
	png := readFixture(t, "pixel.png")
	prec := uploadFile(t, f, cookies, "runbooks/deploy.md", "pixel.png", png)
	if prec.Code != http.StatusCreated {
		t.Fatalf("png upload status = %d, want 201; body=%s", prec.Code, prec.Body.String())
	}
	var pmeta uploadedMeta
	decodeJSON(t, prec, &pmeta)
	pdrec := download(t, f, cookies, pmeta.ID)
	if cd := pdrec.Header().Get("Content-Disposition"); !contains(cd, "inline") {
		t.Fatalf("png Content-Disposition = %q, want inline", cd)
	}
	if pdrec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("png X-Content-Type-Options = %q, want nosniff", pdrec.Header().Get("X-Content-Type-Options"))
	}
}

// TestListAttachments: GET returns the page's uploaded attachment meta as JSON.
func TestListAttachments(t *testing.T) {
	f := newAttachServer(t)
	cookies := loginEditorAttach(t, f)

	txt := readFixture(t, "sample.txt")
	if rec := uploadFile(t, f, cookies, "runbooks/deploy.md", "sample.txt", txt); rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}

	lreq := httptest.NewRequest(http.MethodGet, "/api/v1/attachments/runbooks/deploy.md", nil)
	for _, c := range cookies {
		lreq.AddCookie(c)
	}
	lrec := httptest.NewRecorder()
	f.handler.ServeHTTP(lrec, lreq)
	if lrec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body=%s", lrec.Code, lrec.Body.String())
	}
	var items []uploadedMeta
	decodeJSON(t, lrec, &items)
	if len(items) != 1 {
		t.Fatalf("list len = %d, want 1; body=%s", len(items), lrec.Body.String())
	}
	if items[0].OriginalName != "sample.txt" {
		t.Fatalf("list[0].original_name = %q, want sample.txt", items[0].OriginalName)
	}
}

func download(t *testing.T, f *attachFixture, cookies []*http.Cookie, id string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/attachments/"+id+"/download", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}
