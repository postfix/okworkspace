package attachments

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/jobs"
)

// KindExtract is the job kind registered on the EXISTING single jobs worker for
// text extraction. It is a SEPARATE kind from KindCommit ("commit"): the extract
// handler does the extraction work, then commits its <id>.txt sidecar by enqueuing
// a normal KindCommit job — there is no second commit/write path (ATT-10). The
// binary is only ever READ by extraction; only the .txt is written (T-02-11).
const KindExtract = "extract"

// extractPayload is the JSON enqueued for a KindExtract job. It carries everything
// the handler needs without re-reading the meta sidecar: the attachment id, the
// on-disk binary path to read, the .txt sidecar path to write, the extension that
// selects the extractor, the owning page path (for the commit provenance), and the
// acting user (Git author identity).
type extractPayload struct {
	AttachmentID string `json:"attachment_id"`
	BinPath      string `json:"bin_path"`
	TxtPath      string `json:"txt_path"`
	Ext          string `json:"ext"`
	PagePath     string `json:"page_path"`
	User         string `json:"user"`
}

// binaryReader is the subset of *repo.Repo the extract handler needs: read the
// committed binary through the safe-path resolver (SEC-01). Defined as an
// interface so a test can inject a fake without standing up a real repo.
type binaryReader interface {
	Read(rel string) ([]byte, error)
}

// ExtractHandler returns the jobs.Handler registered for KindExtract (mirrors the
// pages.CommitHandler constructor shape). For each job it:
//
//  1. unmarshals the payload and (best-effort) marks the row "extracting"
//     (kept as pending in the DB column; the chip renders pending as "Extracting…"),
//  2. READS the committed binary through the resolver (the original is never
//     written by extraction — T-02-11),
//  3. extracts text via Extract (which recovers any parser panic — Pitfall 5),
//  4. on a parse ERROR: records status=failed + the raw error server-side and
//     RETURNS the error so the worker retries then terminally fails ("Couldn't
//     extract text"); NO <id>.txt is written,
//  5. on SUCCESS: commits the (possibly EMPTY) <id>.txt through the SAME KindCommit
//     path and sets status=done (non-empty) or status=empty (empty result — the
//     "No text extracted" success case, ATT-08).
//
// The whole body runs under a defer recover() (defense-in-depth beyond the
// per-extractor guard) so a panic anywhere becomes a returned error and the single
// drain goroutine survives (T-02-09).
func ExtractHandler(r binaryReader, w enqueuer, db *sql.DB, pushOnCommit bool) jobs.Handler {
	return func(ctx context.Context, payload string) (err error) {
		defer func() {
			if rec := recover(); rec != nil {
				err = fmt.Errorf("attachments: extract handler panic: %v", rec)
			}
		}()

		var p extractPayload
		if uerr := json.Unmarshal([]byte(payload), &p); uerr != nil {
			return fmt.Errorf("attachments: extract payload: %w", uerr)
		}
		if p.AttachmentID == "" || p.BinPath == "" || p.TxtPath == "" {
			return fmt.Errorf("attachments: extract payload missing required fields")
		}

		// Mark in-flight (best-effort): the chip seeds from the list item and the SSE
		// stream renders pending/extracting identically, so a failure to write this
		// transient state never blocks extraction.
		_ = setExtractStatus(ctx, db, p.AttachmentID, ExtractionPending, "")

		data, rerr := r.Read(p.BinPath)
		if rerr != nil {
			// Reading the committed binary failed (resolver/IO). Treat as a job error
			// so the worker retries; keep the row pending (a transient read race can
			// resolve on retry). Do NOT mark failed here — failed is reserved for a
			// genuine parse failure after the bytes are in hand.
			return fmt.Errorf("attachments: read binary for extract %q: %w", p.AttachmentID, rerr)
		}

		text, eerr := Extract(p.Ext, data)
		if eerr != nil {
			// Genuine parse error (corrupt/encrypted). Record the raw error
			// server-side (NEVER surfaced to the client — T-02-12) and return the
			// error so the worker retries with backoff; terminal failure leaves the
			// row at status=failed → the "Couldn't extract text" chip. No .txt is
			// written, so the card can still tell failed apart from empty-succeeded.
			_ = setExtractStatus(ctx, db, p.AttachmentID, ExtractionFailed, eerr.Error())
			return eerr
		}

		// SUCCESS — commit the (possibly EMPTY) <id>.txt through the single-writer
		// KindCommit spine (ATT-10). Writing an empty file on empty extraction lets
		// the card distinguish "extracted nothing" (No text extracted) from "not yet
		// run". Only TxtPath is in Writes — the binary is never re-written (T-02-11).
		cp := commitPayload{
			Writes: []fileWrite{{Path: p.TxtPath, Bytes: []byte(text)}},
			Spec: gitstore.CommitSpec{
				Paths:   []string{p.TxtPath},
				Message: "Update extracted text",
				User:    p.User,
				Action:  "attach-extract",
				Source:  "web-ui",
			},
			Push: pushOnCommit,
		}
		raw, merr := json.Marshal(cp)
		if merr != nil {
			return fmt.Errorf("attachments: marshal extract commit payload: %w", merr)
		}
		if cerr := w.EnqueueAndWait(ctx, kindCommit, string(raw), commitWaitTimeout); cerr != nil {
			// A commit failure must surface so the worker retries — the .txt is the
			// durable artifact. (A timeout is treated as a soft success by the upload
			// path, but here we want the retry to ensure the sidecar lands.)
			if cerr == jobs.ErrJobTimeout {
				// The commit job is queued and will still complete; do not fail the
				// extract over a slow drain — mirror the upload-path policy.
				cerr = nil
			}
			if cerr != nil {
				return fmt.Errorf("attachments: commit extracted text %q: %w", p.AttachmentID, cerr)
			}
		}

		status := ExtractionDone
		if text == "" {
			status = ExtractionEmpty
		}
		if serr := setExtractStatus(ctx, db, p.AttachmentID, status, ""); serr != nil {
			return serr
		}
		return nil
	}
}
