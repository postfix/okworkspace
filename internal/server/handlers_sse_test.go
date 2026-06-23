package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// seedAttachmentRow inserts an attachments row at the given extraction status so
// the SSE stream emits a single terminal event and closes deterministically
// (without racing the async ExtractJob).
func seedAttachmentRow(t *testing.T, f *attachFixture, id, status string) {
	t.Helper()
	_, err := f.db.ExecContext(context.Background(),
		`INSERT INTO attachments (id, page_path, original_name, mime_type, size_bytes, uploader_name, uploaded_at, extract_status, extract_error)
		 VALUES (?, 'runbooks/deploy.md', ?, 'application/pdf', 1, 'ed', ?, ?, '')`,
		id, id+".pdf", time.Now().UTC().Format(time.RFC3339Nano), status)
	if err != nil {
		t.Fatalf("seed attachment row: %v", err)
	}
}

// TestExtractionSSEStream: GET /attachments/{id}/status returns text/event-stream,
// emits at least one data event reflecting the row, and closes on a terminal
// status (the handler returns, so the recorder's body is complete).
func TestExtractionSSEStream(t *testing.T) {
	f := newAttachServer(t)
	cookies := loginEditorAttach(t, f)
	seedAttachmentRow(t, f, "01SSEDONE", "done")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/attachments/01SSEDONE/status", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	// Bound the request so a regression that fails to close the stream cannot hang
	// the test forever.
	ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "data: ") {
		t.Fatalf("body has no SSE data event; got %q", body)
	}
	if !strings.Contains(body, `"status":"done"`) {
		t.Fatalf("body does not reflect the terminal status; got %q", body)
	}
}

// TestExtractionSSEAuthRequired: an UNauthenticated request to the status stream is
// rejected (the route is under the authed group).
func TestExtractionSSEAuthRequired(t *testing.T) {
	f := newAttachServer(t)
	seedAttachmentRow(t, f, "01SSEAUTH", "done")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/attachments/01SSEAUTH/status", nil)
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("unauthenticated status request returned 200, want a 401/403 rejection; body=%s", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("unauthenticated request opened an event-stream (%q), want rejection before streaming", ct)
	}
}
