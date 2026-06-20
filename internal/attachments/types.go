// Package attachments implements the attachment lifecycle (upload, byte-exact
// download, list, and — in later slices — replace/remove/extract) on top of the
// Phase-0/1 spines. Like internal/pages, every write/delete flows through the
// single-writer CommitJob (ATT-10): the service NEVER calls os.WriteFile or shells
// out to git directly. All path I/O routes through repo.* (SEC-01 chokepoint).
//
// The three-part on-disk model (SPEC §11) is attachments/<id>.<ext> (the
// byte-exact original), attachments/<id>.json (the meta sidecar — the ONLY place
// the original filename lives, never the on-disk path, SEC-02), and
// attachments/<id>.txt (extracted text, written by the ExtractJob in slice 02-03).
package attachments

import (
	"errors"
	"time"
)

// Sentinel errors mapped to HTTP status codes by handlers_attachments.go via
// errors.Is (mirrors the pages package pattern).
var (
	// ErrAttachmentNotFound is returned when no attachment with the given id exists.
	ErrAttachmentNotFound = errors.New("attachment not found")
	// ErrPageNotFound is returned when an operation targets a page that does not exist.
	ErrPageNotFound = errors.New("page not found")
	// ErrTypeForbidden is returned by Upload when the sniffed MIME type is not on
	// the configured allow-list (ATT-09 — the type is sniffed from magic bytes, the
	// filename is never trusted).
	ErrTypeForbidden = errors.New("file type not allowed")
	// ErrTooLarge is returned by Upload when the supplied bytes exceed
	// maxUploadMB (ATT-09). The HTTP layer also caps the body with MaxBytesReader.
	ErrTooLarge = errors.New("file too large")
)

// ExtractionStatus is the text-extraction state surfaced to the card chip. The
// four values are mutually exclusive (RESEARCH error-handling contract). In slice
// 02-01 every upload starts ExtractionPending; the ExtractJob (02-03) advances it.
type ExtractionStatus string

const (
	// ExtractionPending — extraction has not yet run (or is queued/running).
	ExtractionPending ExtractionStatus = "pending"
	// ExtractionDone — text was extracted.
	ExtractionDone ExtractionStatus = "done"
	// ExtractionEmpty — extraction succeeded but the document has no text layer
	// (e.g. a scanned PDF). NOT a failure.
	ExtractionEmpty ExtractionStatus = "empty"
	// ExtractionFailed — extraction failed after the worker's retry/backoff.
	ExtractionFailed ExtractionStatus = "failed"
)

// AttachmentMeta is the <id>.json sidecar (encoding/json) AND the upload/list
// response shape. The original filename lives ONLY here, never in the on-disk path
// (SEC-02). Sha256 is stored for cheap integrity / future dedup; Ext is the
// sniffed extension (no leading dot) used to build the on-disk binary path.
type AttachmentMeta struct {
	ID           string    `json:"id"`
	OriginalName string    `json:"original_name"`
	MimeType     string    `json:"mime_type"`
	SizeBytes    int64     `json:"size_bytes"`
	UploaderName string    `json:"uploader_name"`
	UploadedAt   time.Time `json:"uploaded_at"`
	PagePath     string    `json:"page_path"`
	Sha256       string    `json:"sha256"`
	Ext          string    `json:"ext"`
}

// AttachmentListItem is the GET /attachments/{pagePath} response shape: the meta
// plus the current extraction status (the chip driver). It embeds the meta so the
// card has every field it needs in one payload (ATT-03 foundation).
type AttachmentListItem struct {
	AttachmentMeta
	ExtractionStatus ExtractionStatus `json:"extraction_status"`
}
