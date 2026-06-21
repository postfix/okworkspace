package attachments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
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
	kinds    []string
	// failCommit, when set, makes EnqueueAndWait return this error for kindCommit
	// jobs WITHOUT applying the payload to the repo — simulating a real
	// (non-timeout) commit-enqueue failure so the WR-01 row-rollback path is
	// exercised.
	failCommit error
	// recordOnlyMessages: commit payloads whose Spec.Message is in this set are
	// RECORDED but NOT applied to the repo — simulating a commit that was accepted
	// (soft-success / queued) but has not landed on disk yet. Used to exercise the
	// WR-02 stale-working-tree hazard (the unlink edit not yet on disk).
	recordOnlyMessages map[string]bool
}

func (f *fakeEnqueuer) Enqueue(ctx context.Context, kind, payload string) error {
	f.kinds = append(f.kinds, kind)
	// Only commit-kind payloads carry a commitPayload to apply to the repo; other
	// kinds (e.g. KindExtract) are fire-and-forget — just record the kind so a test
	// can assert what was enqueued WITHOUT running the real drain goroutine.
	if kind != kindCommit {
		return nil
	}
	if f.failCommit != nil {
		return f.failCommit
	}
	return f.apply(payload)
}

func (f *fakeEnqueuer) EnqueueAndWait(ctx context.Context, kind, payload string, _ time.Duration) error {
	f.kinds = append(f.kinds, kind)
	if kind == kindCommit && f.failCommit != nil {
		return f.failCommit
	}
	return f.apply(payload)
}

// countKind reports how many jobs of the given kind were enqueued on the fake.
func countKind(f *fakeEnqueuer, kind string) int {
	n := 0
	for _, k := range f.kinds {
		if k == kind {
			n++
		}
	}
	return n
}

func (f *fakeEnqueuer) apply(payload string) error {
	f.rawJSON = append(f.rawJSON, payload)
	var p commitPayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return err
	}
	f.payloads = append(f.payloads, p)
	if f.recordOnlyMessages != nil && f.recordOnlyMessages[p.Spec.Message] {
		// Accepted but not landed on disk yet (simulated soft-success/queued).
		return nil
	}
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

// TestReplaceKeepsID (ATT-05): Replace produces a commit writing the SAME id's
// binary + an updated meta (new size/sha256) and re-enqueues a KindExtract job; the
// id is unchanged and the prior bytes are retained in history (modeled here by the
// commit being captured, not destroyed).
func TestReplaceKeepsID(t *testing.T) {
	svc, fe, db := newTestService(t, []string{"txt"}, 100)

	orig := []byte("the original attachment body")
	up, err := svc.Upload(context.Background(), "runbooks/deploy.md", "notes.txt", orig, "alice")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	uploadCommits := len(fe.payloads)
	uploadExtracts := countKind(fe, KindExtract)

	newBytes := []byte("the REPLACED attachment body, now longer than before")
	rep, err := svc.Replace(context.Background(), up.ID, "notes-v2.txt", newBytes, "bob")
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}

	// Same id (ATT-05): every page link keeps resolving without a page edit.
	if rep.ID != up.ID {
		t.Fatalf("Replace changed id: got %q, want %q", rep.ID, up.ID)
	}
	// New size + new sha256 (the bytes really changed).
	if rep.SizeBytes != int64(len(newBytes)) {
		t.Fatalf("replaced size = %d, want %d", rep.SizeBytes, len(newBytes))
	}
	if rep.Sha256 == up.Sha256 {
		t.Fatalf("replaced sha256 unchanged (%q) — bytes did not change", rep.Sha256)
	}

	// Exactly one NEW commit (binary + meta) landed on top of the upload commit.
	if len(fe.payloads) != uploadCommits+1 {
		t.Fatalf("commit payloads = %d, want %d (one replace commit)", len(fe.payloads), uploadCommits+1)
	}
	p := fe.payloads[len(fe.payloads)-1]
	if len(p.Writes) != 2 {
		t.Fatalf("replace writes = %d, want 2 (binary + meta)", len(p.Writes))
	}
	if len(p.Removes) != 0 {
		t.Fatalf("replace removes = %d, want 0 (same ext, same path)", len(p.Removes))
	}
	wrote := map[string]bool{p.Writes[0].Path: true, p.Writes[1].Path: true}
	if !wrote[BinPath(up.ID, "txt")] || !wrote[MetaPath(up.ID)] {
		t.Fatalf("replace did not write the same id's bin+meta: %v", wrote)
	}
	if p.Spec.Message != "Replace attachment" {
		t.Fatalf("replace commit message = %q, want %q (hidden-Git)", p.Spec.Message, "Replace attachment")
	}

	// Re-extraction was re-enqueued (one MORE KindExtract than after upload).
	if got := countKind(fe, KindExtract); got != uploadExtracts+1 {
		t.Fatalf("KindExtract enqueues = %d, want %d (one re-extract on replace)", got, uploadExtracts+1)
	}

	// The operational row still exists under the SAME id with the new size.
	st, err := ExtractionStatusFor(context.Background(), db, up.ID)
	if err != nil {
		t.Fatalf("status after replace: %v", err)
	}
	if st != ExtractionPending {
		t.Fatalf("extract status after replace = %q, want pending (reset for re-extract)", st)
	}
}

// TestOrphanDelete (ATT-07): when the only referencing page's link is removed,
// Remove emits ONE commitPayload whose Removes carries [bin, meta, txt] and deletes
// the operational row.
func TestOrphanDelete(t *testing.T) {
	svc, fe, db := newTestService(t, []string{"txt"}, 100)

	data := []byte("orphan-bound attachment")
	up, err := svc.Upload(context.Background(), "notes.md", "doc.txt", data, "alice")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	// Seed the single referencing page with the canonical link, and a .txt sidecar
	// (extraction is fire-and-forget in tests, so write it directly to assert it is
	// part of the delete payload).
	ref := DownloadRefPath(up.ID)
	mustWrite(t, svc, "notes.md", "Body. See [doc]("+ref+").\n")
	mustWrite(t, svc, TxtPath(up.ID), "extracted text")

	before := len(fe.payloads)
	orphan, err := svc.Remove(context.Background(), up.ID, "notes.md", "bob")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !orphan {
		t.Fatalf("Remove deletedOrphan = false, want true (last reference removed)")
	}

	// Two commits: (1) the unlink edit of notes.md, (2) the orphan delete.
	if len(fe.payloads) != before+2 {
		t.Fatalf("commit payloads = %d, want %d (unlink + delete)", len(fe.payloads), before+2)
	}
	del := fe.payloads[len(fe.payloads)-1]
	if del.Spec.Message != "Delete attachment" {
		t.Fatalf("delete message = %q, want %q", del.Spec.Message, "Delete attachment")
	}
	gotRemoves := map[string]bool{}
	for _, rm := range del.Removes {
		gotRemoves[rm] = true
	}
	for _, want := range []string{BinPath(up.ID, "txt"), MetaPath(up.ID), TxtPath(up.ID)} {
		if !gotRemoves[want] {
			t.Fatalf("orphan delete Removes missing %q; got %v", want, del.Removes)
		}
	}
	if len(del.Removes) != 3 {
		t.Fatalf("orphan delete Removes = %d, want 3 (bin+meta+txt in ONE commit)", len(del.Removes))
	}

	// The operational row is gone.
	if _, err := ExtractionStatusFor(context.Background(), db, up.ID); err != ErrAttachmentNotFound {
		t.Fatalf("row after orphan delete: err = %v, want ErrAttachmentNotFound", err)
	}

	// The files are gone on disk (the fake applies Removes to the repo).
	if exists, _ := svc.repo.Exists(BinPath(up.ID, "txt")); exists {
		t.Fatalf("binary still on disk after orphan delete")
	}
	if exists, _ := svc.repo.Exists(MetaPath(up.ID)); exists {
		t.Fatalf("meta still on disk after orphan delete")
	}
}

// TestRemoveOrphansEvenIfUnlinkCommitNotLanded (WR-02): when the unlink edit has
// NOT yet landed on disk (its commit soft-succeeded / is still queued), the
// orphan recount must still treat pagePath as unlinked and delete the now-orphaned
// files, instead of re-reading the stale page (which still shows the link) and
// keeping a silent orphan.
func TestRemoveOrphansEvenIfUnlinkCommitNotLanded(t *testing.T) {
	svc, fe, db := newTestService(t, []string{"txt"}, 100)

	data := []byte("attachment whose only ref is about to go")
	up, err := svc.Upload(context.Background(), "only.md", "doc.txt", data, "alice")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	ref := DownloadRefPath(up.ID)
	mustWrite(t, svc, "only.md", "Sole ref: [doc]("+ref+").\n")
	mustWrite(t, svc, TxtPath(up.ID), "extracted text")

	// Simulate the unlink commit being accepted but NOT landing on disk: the
	// "Remove attachment link" commit is recorded but not applied, so only.md on
	// disk STILL contains the canonical link when the recount runs.
	fe.recordOnlyMessages = map[string]bool{"Remove attachment link": true}

	before := len(fe.payloads)
	orphan, err := svc.Remove(context.Background(), up.ID, "only.md", "bob")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !orphan {
		t.Fatalf("Remove deletedOrphan = false, want true (sole ref removed even though unlink not landed)")
	}

	// only.md on disk STILL has the link (the unlink commit was not applied) —
	// proving the recount did NOT rely on a stale working-tree read of pagePath.
	body, _ := svc.repo.Read("only.md")
	if !contains(string(body), ref) {
		t.Fatalf("test precondition broken: only.md unexpectedly had its link applied")
	}

	// A delete commit landed and the row is gone.
	if len(fe.payloads) != before+2 {
		t.Fatalf("commit payloads = %d, want %d (unlink recorded + delete)", len(fe.payloads), before+2)
	}
	if _, err := ExtractionStatusFor(context.Background(), db, up.ID); err != ErrAttachmentNotFound {
		t.Fatalf("row after orphan delete: err = %v, want ErrAttachmentNotFound", err)
	}
	if exists, _ := svc.repo.Exists(BinPath(up.ID, "txt")); exists {
		t.Fatalf("binary still on disk after orphan delete")
	}
}

// TestRemoveKeepsSharedFile (Pitfall 6): when a SECOND page still references the
// id, Remove drops the link from the target page but does NOT delete the files.
func TestRemoveKeepsSharedFile(t *testing.T) {
	svc, fe, db := newTestService(t, []string{"txt"}, 100)

	data := []byte("shared attachment")
	up, err := svc.Upload(context.Background(), "page-one.md", "shared.txt", data, "alice")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	ref := DownloadRefPath(up.ID)
	// Two pages reference the same id.
	mustWrite(t, svc, "page-one.md", "One: [shared]("+ref+").\n")
	mustWrite(t, svc, "page-two.md", "Two: [shared]("+ref+") too.\n")

	before := len(fe.payloads)
	orphan, err := svc.Remove(context.Background(), up.ID, "page-one.md", "bob")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if orphan {
		t.Fatalf("Remove deletedOrphan = true, want false (page-two still references it)")
	}

	// Exactly ONE commit: the unlink edit of page-one (no delete commit).
	if len(fe.payloads) != before+1 {
		t.Fatalf("commit payloads = %d, want %d (unlink only, NO delete)", len(fe.payloads), before+1)
	}
	last := fe.payloads[len(fe.payloads)-1]
	if len(last.Removes) != 0 {
		t.Fatalf("shared-file Remove produced Removes %v, want none", last.Removes)
	}

	// The files are STILL on disk and the row STILL exists.
	if exists, _ := svc.repo.Exists(BinPath(up.ID, "txt")); !exists {
		t.Fatalf("shared binary was deleted — must be kept")
	}
	if _, err := ExtractionStatusFor(context.Background(), db, up.ID); err != nil {
		t.Fatalf("shared row missing after Remove: %v", err)
	}

	// page-one no longer references it; page-two still does.
	one, _ := svc.repo.Read("page-one.md")
	if contains(string(one), ref) {
		t.Fatalf("page-one still contains the link after unlink: %q", string(one))
	}
	two, _ := svc.repo.Read("page-two.md")
	if !contains(string(two), ref) {
		t.Fatalf("page-two lost the link it should keep: %q", string(two))
	}
}

// contains is a thin strings.Contains alias for readable assertions.
func contains(s, sub string) bool { return strings.Contains(s, sub) }

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

// TestUploadRollsBackRowOnCommitError (WR-01): when the commit enqueue returns a
// real (non-timeout) error AFTER the operational row was inserted, Upload must
// delete the row again so List never advertises a row whose file does not exist.
func TestUploadRollsBackRowOnCommitError(t *testing.T) {
	svc, fe, db := newTestService(t, []string{"txt"}, 100)
	fe.failCommit = errors.New("commit drain exploded")

	data := []byte("payload that will fail to commit")
	if _, err := svc.Upload(context.Background(), "p.md", "doc.txt", data, "alice"); err == nil {
		t.Fatalf("Upload succeeded, want a commit error surfaced")
	}

	// No commit should have applied (the fake fails before apply).
	if len(fe.payloads) != 0 {
		t.Fatalf("commit payloads = %d, want 0 (commit failed)", len(fe.payloads))
	}
	// The row must NOT linger: List sees nothing and the page has zero rows.
	items, err := svc.List(context.Background(), "p.md")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("List = %+v, want 0 rows (row rolled back after commit failure)", items)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM attachments`).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if n != 0 {
		t.Fatalf("attachments rows = %d, want 0 (orphan row rolled back)", n)
	}
}

// TestReplaceRevertsRowOnCommitError (WR-01): a real (non-timeout) commit failure
// on Replace must restore the operational row to the PREVIOUS meta so List never
// advertises a size/sha that did not durably land.
func TestReplaceRevertsRowOnCommitError(t *testing.T) {
	svc, fe, _ := newTestService(t, []string{"txt"}, 100)

	orig := []byte("the original body")
	up, err := svc.Upload(context.Background(), "p.md", "notes.txt", orig, "alice")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	// Now make the replace commit fail and attempt a replace with new bytes.
	fe.failCommit = errors.New("replace commit drain exploded")
	newBytes := []byte("the replaced body, longer than before")
	if _, err := svc.Replace(context.Background(), up.ID, "notes-v2.txt", newBytes, "bob"); err == nil {
		t.Fatalf("Replace succeeded, want a commit error surfaced")
	}

	// The row must still describe the ORIGINAL upload, not the failed replace.
	items, err := svc.List(context.Background(), "p.md")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("List = %d rows, want 1", len(items))
	}
	if items[0].OriginalName != "notes.txt" {
		t.Fatalf("row name = %q, want %q (reverted to pre-replace meta)", items[0].OriginalName, "notes.txt")
	}
	if items[0].SizeBytes != int64(len(orig)) {
		t.Fatalf("row size = %d, want %d (reverted to original size)", items[0].SizeBytes, len(orig))
	}
}
