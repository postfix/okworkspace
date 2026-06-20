package attachments

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/store"
)

// fakeEnqueuer captures the enqueued commit payload and applies its Writes to the
// repo so List/Meta/download can observe the committed state WITHOUT standing up
// the real drain goroutine (mirrors the pages fake-worker test pattern). It
// unmarshals the payload into the LOCAL commitPayload shape — exercising the same
// JSON the pages KindCommit handler unmarshals.
type fakeEnqueuer struct {
	r        *repo.Repo
	payloads []commitPayload
	rawJSON  []string
}

func (f *fakeEnqueuer) Enqueue(ctx context.Context, kind, payload string) error {
	return f.apply(payload)
}

func (f *fakeEnqueuer) EnqueueAndWait(ctx context.Context, kind, payload string, _ time.Duration) error {
	return f.apply(payload)
}

func (f *fakeEnqueuer) apply(payload string) error {
	f.rawJSON = append(f.rawJSON, payload)
	var p commitPayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return err
	}
	f.payloads = append(f.payloads, p)
	for _, w := range p.Writes {
		if err := f.r.Write(w.Path, w.Bytes); err != nil {
			return err
		}
	}
	for _, rm := range p.Removes {
		if err := f.r.Remove(rm); err != nil {
			return err
		}
	}
	return nil
}

// newTestService builds a Service backed by a real repo + DB and a fake enqueuer.
func newTestService(t *testing.T, allowed []string, maxMB int) (*Service, *fakeEnqueuer, *sql.DB) {
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

	fe := &fakeEnqueuer{r: r}
	svc := newService(r, fe, st.DB(), allowed, maxMB, false)
	svc.now = func() time.Time { return time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC) }
	return svc, fe, st.DB()
}

// TestMetaSidecar (ATT-03 foundation): writing then reading a <id>.json round-
// trips an AttachmentMeta with all fields intact.
func TestMetaSidecar(t *testing.T) {
	svc, _, _ := newTestService(t, []string{"txt"}, 100)
	want := AttachmentMeta{
		ID:           "01ABCDEF",
		OriginalName: "Quarterly Report.txt",
		MimeType:     "text/plain; charset=utf-8",
		SizeBytes:    42,
		UploaderName: "alice",
		UploadedAt:   time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC),
		PagePath:     "runbooks/deploy.md",
		Sha256:       "deadbeef",
		Ext:          "txt",
	}
	b, err := marshalMeta(want)
	if err != nil {
		t.Fatalf("marshalMeta: %v", err)
	}
	if err := svc.repo.Write(MetaPath(want.ID), b); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	got, err := readMeta(svc.repo, want.ID)
	if err != nil {
		t.Fatalf("readMeta: %v", err)
	}
	if got.ID != want.ID || got.OriginalName != want.OriginalName ||
		got.MimeType != want.MimeType || got.SizeBytes != want.SizeBytes ||
		got.UploaderName != want.UploaderName || got.PagePath != want.PagePath ||
		got.Sha256 != want.Sha256 || got.Ext != want.Ext {
		t.Fatalf("meta round-trip mismatch:\n got %+v\nwant %+v", got, want)
	}
	if !got.UploadedAt.Equal(want.UploadedAt) {
		t.Fatalf("uploaded_at = %v, want %v", got.UploadedAt, want.UploadedAt)
	}
}

// TestUploadCommits (ATT-10): Upload captures exactly ONE commit payload
// containing Writes for the binary + meta sidecar (no Removes).
func TestUploadCommits(t *testing.T) {
	svc, fe, _ := newTestService(t, []string{"txt"}, 100)
	data := []byte("hello attachment world")
	meta, err := svc.Upload(context.Background(), "runbooks/deploy.md", "notes.txt", data, "alice")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if len(fe.payloads) != 1 {
		t.Fatalf("commit payloads = %d, want exactly 1", len(fe.payloads))
	}
	p := fe.payloads[0]
	if len(p.Writes) != 2 {
		t.Fatalf("writes = %d, want 2 (binary + meta)", len(p.Writes))
	}
	if len(p.Removes) != 0 {
		t.Fatalf("removes = %d, want 0", len(p.Removes))
	}
	gotPaths := map[string]bool{p.Writes[0].Path: true, p.Writes[1].Path: true}
	if !gotPaths[BinPath(meta.ID, "txt")] {
		t.Fatalf("missing binary write %q in %v", BinPath(meta.ID, "txt"), gotPaths)
	}
	if !gotPaths[MetaPath(meta.ID)] {
		t.Fatalf("missing meta write %q in %v", MetaPath(meta.ID), gotPaths)
	}
	// The binary write must be byte-exact (ATT-02 at the commit layer).
	for _, w := range p.Writes {
		if w.Path == BinPath(meta.ID, "txt") && string(w.Bytes) != string(data) {
			t.Fatalf("binary write bytes mutated")
		}
	}
	// The list endpoint must now see exactly one item for the page.
	items, err := svc.List(context.Background(), "runbooks/deploy.md")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].OriginalName != "notes.txt" {
		t.Fatalf("List = %+v, want one item named notes.txt", items)
	}
	if items[0].ExtractionStatus != ExtractionPending {
		t.Fatalf("extraction_status = %q, want pending", items[0].ExtractionStatus)
	}
}

// TestCommitPayloadShape (Open Question 2 guard): the attachments-local
// commitPayload marshals to JSON whose field names match the pages KindCommit
// handler's expected shape (writes[].path, writes[].bytes, removes, spec, push).
func TestCommitPayloadShape(t *testing.T) {
	p := commitPayload{
		Writes:  []fileWrite{{Path: "attachments/x.txt", Bytes: []byte("ab")}},
		Removes: []string{"attachments/y.txt"},
		Push:    true,
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("unmarshal generic: %v", err)
	}
	for _, key := range []string{"writes", "removes", "spec", "push"} {
		if _, ok := generic[key]; !ok {
			t.Fatalf("payload missing top-level key %q; got %s", key, raw)
		}
	}
	var writes []map[string]json.RawMessage
	if err := json.Unmarshal(generic["writes"], &writes); err != nil {
		t.Fatalf("unmarshal writes: %v", err)
	}
	if len(writes) != 1 {
		t.Fatalf("writes len = %d, want 1", len(writes))
	}
	for _, key := range []string{"path", "bytes"} {
		if _, ok := writes[0][key]; !ok {
			t.Fatalf("write missing key %q; got %s", key, generic["writes"])
		}
	}
}

// TestUploadRejectsBadType (ATT-09): a sniffed type not on the allow-list returns
// ErrTypeForbidden; oversize returns ErrTooLarge.
func TestUploadRejectsBadType(t *testing.T) {
	svc, fe, _ := newTestService(t, []string{"txt", "pdf"}, 1)

	// ELF magic → sniffs to application/x-elf, not on the allow-list.
	elf := append([]byte{0x7f, 'E', 'L', 'F'}, make([]byte, 64)...)
	if _, err := svc.Upload(context.Background(), "p.md", "evil.txt", elf, "alice"); err != ErrTypeForbidden {
		t.Fatalf("bad-type Upload err = %v, want ErrTypeForbidden", err)
	}

	// Oversize: 2 MB > the 1 MB cap.
	big := make([]byte, 2<<20)
	copy(big, []byte("plain text payload")) // sniffs as text/plain (allowed) so size is the only reason
	if _, err := svc.Upload(context.Background(), "p.md", "big.txt", big, "alice"); err != ErrTooLarge {
		t.Fatalf("oversize Upload err = %v, want ErrTooLarge", err)
	}

	// Neither rejected upload should have produced a commit.
	if len(fe.payloads) != 0 {
		t.Fatalf("rejected uploads produced %d commits, want 0", len(fe.payloads))
	}
}
