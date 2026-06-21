package attachments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/store"
)

// newExtractHarness builds a real repo + DB and a fake enqueuer for exercising the
// KindExtract handler in isolation. The repo doubles as the binaryReader (the
// handler reads the committed binary through it) and the fake enqueuer applies the
// handler's .txt commit to the same repo so the test can observe the sidecar.
func newExtractHarness(t *testing.T) (*repo.Repo, *fakeEnqueuer, *sql.DB) {
	t.Helper()
	r, err := repo.New(filepath.Join(t.TempDir(), "repo"))
	if err != nil {
		t.Fatalf("repo.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	st, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	return r, &fakeEnqueuer{r: r}, st.DB()
}

// seedRow inserts a minimal attachments row at status=pending so the handler's
// status updates have a row to mutate (mirrors Upload's insertRow).
func seedRow(t *testing.T, db *sql.DB, id, ext string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO attachments (id, page_path, original_name, mime_type, size_bytes, uploader_name, uploaded_at, extract_status, extract_error)
		 VALUES (?, 'p.md', ?, 'application/octet-stream', 0, 'alice', ?, 'pending', '')`,
		id, id+"."+ext, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("seed row: %v", err)
	}
}

// runExtract drives the handler once with a payload for the given id/ext, having
// first written the binary into the repo so the handler can read it.
func runExtract(t *testing.T, r *repo.Repo, fe *fakeEnqueuer, db *sql.DB, id, ext string, binary []byte) error {
	t.Helper()
	if err := r.Write(BinPath(id, ext), binary); err != nil {
		t.Fatalf("seed binary: %v", err)
	}
	seedRow(t, db, id, ext)
	h := ExtractHandler(r, fe, db, false)
	p := extractPayload{
		AttachmentID: id,
		BinPath:      BinPath(id, ext),
		TxtPath:      TxtPath(id),
		Ext:          ext,
		PagePath:     "p.md",
		User:         "alice",
	}
	raw, _ := json.Marshal(p)
	return h(context.Background(), string(raw))
}

func statusOf(t *testing.T, db *sql.DB, id string) ExtractionStatus {
	t.Helper()
	s, err := ExtractionStatusFor(context.Background(), db, id)
	if err != nil {
		t.Fatalf("ExtractionStatusFor: %v", err)
	}
	return s
}

// TestExtractJobWritesTxt: a text-layer PDF commits <id>.txt with the extracted
// text and sets status=done.
func TestExtractJobWritesTxt(t *testing.T) {
	r, fe, db := newExtractHarness(t)
	pdf, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "attachments", "text-layer.pdf"))

	if err := runExtract(t, r, fe, db, "01TEXTPDF", "pdf", pdf); err != nil {
		t.Fatalf("ExtractHandler err = %v, want nil", err)
	}
	if got := statusOf(t, db, "01TEXTPDF"); got != ExtractionDone {
		t.Fatalf("status = %q, want done", got)
	}
	txt, err := r.Read(TxtPath("01TEXTPDF"))
	if err != nil {
		t.Fatalf("read committed .txt: %v", err)
	}
	if len(txt) == 0 {
		t.Fatalf("committed .txt is empty, want extracted text")
	}
	// The committed sidecar must be the .txt ONLY — the binary path is never in a
	// write (T-02-11: extraction never re-writes the original).
	for _, p := range fe.payloads {
		for _, w := range p.Writes {
			if w.Path == BinPath("01TEXTPDF", "pdf") {
				t.Fatalf("extraction wrote the binary path %q — must only write the .txt", w.Path)
			}
		}
	}
}

// TestExtractJobEmptyIsSuccess (the ATT-08 empty guarantee): a scanned PDF commits
// an EMPTY <id>.txt and sets status=empty (NOT failed).
func TestExtractJobEmptyIsSuccess(t *testing.T) {
	r, fe, db := newExtractHarness(t)
	pdf, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "attachments", "scanned-image.pdf"))

	if err := runExtract(t, r, fe, db, "01SCANPDF", "pdf", pdf); err != nil {
		t.Fatalf("ExtractHandler err = %v, want nil (empty is SUCCESS)", err)
	}
	if got := statusOf(t, db, "01SCANPDF"); got != ExtractionEmpty {
		t.Fatalf("status = %q, want empty", got)
	}
	txt, err := r.Read(TxtPath("01SCANPDF"))
	if err != nil {
		t.Fatalf("read committed .txt: %v", err)
	}
	if len(txt) != 0 {
		t.Fatalf("empty-extraction .txt = %q, want an empty file", txt)
	}
	if len(fe.payloads) != 1 {
		t.Fatalf("commit payloads = %d, want exactly 1 (the empty .txt)", len(fe.payloads))
	}
}

// TestExtractJobParseErrorFails: a corrupt file → handler returns an error,
// status=failed with extract_error set, and NO <id>.txt is written.
func TestExtractJobParseErrorFails(t *testing.T) {
	r, fe, db := newExtractHarness(t)
	corrupt, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "attachments", "corrupt.pdf"))

	err := runExtract(t, r, fe, db, "01CORRUPT", "pdf", corrupt)
	if err == nil {
		t.Fatalf("ExtractHandler err = nil, want a parse error (drives retry/failed)")
	}
	if got := statusOf(t, db, "01CORRUPT"); got != ExtractionFailed {
		t.Fatalf("status = %q, want failed", got)
	}
	// extract_error must be set server-side (never surfaced to the client).
	var extractErr string
	if serr := db.QueryRowContext(context.Background(),
		`SELECT extract_error FROM attachments WHERE id = ?`, "01CORRUPT").Scan(&extractErr); serr != nil {
		t.Fatalf("read extract_error: %v", serr)
	}
	if extractErr == "" {
		t.Fatalf("extract_error is empty, want the raw parse error recorded server-side")
	}
	// No .txt was committed on failure (the card tells failed apart from empty).
	if exists, _ := r.Exists(TxtPath("01CORRUPT")); exists {
		t.Fatalf("a .txt was written on parse failure, want none")
	}
	if len(fe.payloads) != 0 {
		t.Fatalf("commit payloads = %d on failure, want 0", len(fe.payloads))
	}
}

// TestExtractJobEnqueueFailureSetsTerminalStatus (WR-04): when the .txt commit
// enqueue itself fails (cannot persist the job), the handler records a terminal
// status BEFORE returning the error so the chip does not stick on "Extracting…".
func TestExtractJobEnqueueFailureSetsTerminalStatus(t *testing.T) {
	r, fe, db := newExtractHarness(t)
	fe.failCommit = errors.New("cannot persist commit job")

	err := runExtract(t, r, fe, db, "01ENQFAIL", "txt", []byte("some extractable text"))
	if err == nil {
		t.Fatalf("ExtractHandler err = nil, want the enqueue error surfaced for retry")
	}
	// The row must NOT be left at pending — that is the stuck-chip hazard.
	if got := statusOf(t, db, "01ENQFAIL"); got != ExtractionFailed {
		t.Fatalf("status after enqueue failure = %q, want failed (not stuck pending)", got)
	}
	// extract_error is recorded server-side.
	var extractErr string
	if serr := db.QueryRowContext(context.Background(),
		`SELECT extract_error FROM attachments WHERE id = ?`, "01ENQFAIL").Scan(&extractErr); serr != nil {
		t.Fatalf("read extract_error: %v", serr)
	}
	if extractErr == "" {
		t.Fatalf("extract_error empty, want the enqueue error recorded server-side")
	}
}

// TestUploadEnqueuesExtract: Upload of an extractable type fire-and-forget enqueues
// a KindExtract job; a non-extractable image type enqueues NONE.
func TestUploadEnqueuesExtract(t *testing.T) {
	svc, fe, _ := newTestService(t, []string{"txt", "png"}, 100)

	if _, err := svc.Upload(context.Background(), "p.md", "notes.txt", []byte("hello world text"), "alice"); err != nil {
		t.Fatalf("Upload(txt): %v", err)
	}
	if got := countKind(fe, KindExtract); got != 1 {
		t.Fatalf("extract jobs enqueued for txt = %d, want 1", got)
	}

	// A 1x1 PNG sniffs to image/png — not extractable, so NO extract job.
	png, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "attachments", "pixel.png"))
	before := countKind(fe, KindExtract)
	if _, err := svc.Upload(context.Background(), "p.md", "pixel.png", png, "alice"); err != nil {
		t.Fatalf("Upload(png): %v", err)
	}
	if got := countKind(fe, KindExtract) - before; got != 0 {
		t.Fatalf("extract jobs enqueued for png = %d, want 0 (non-extractable)", got)
	}
}
