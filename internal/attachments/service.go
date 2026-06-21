package attachments

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"

	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/search"
)

// commitWaitTimeout bounds how long an upload blocks waiting for its commit job
// to land on disk before falling back to async semantics (mirrors the pages
// policy: the local commit completes well before the optional remote push, so a
// hung push must not break the upload — on timeout we log and return success).
const commitWaitTimeout = 5 * time.Second

// kindCommit is the job kind registered on the worker for commits. It MUST equal
// pages.KindCommit ("commit") because attachments reuses the ONE registered
// KindCommit handler (RESEARCH Open Question 2, recommendation c). The literal is
// duplicated rather than imported to keep the packages decoupled; a guard test
// (TestCommitPayloadShape) asserts the JSON the pages handler unmarshals matches.
const kindCommit = "commit"

// enqueuer is the subset of *jobs.Worker the service needs, defined as an
// interface so a test can inject a fake worker that captures the enqueued payload
// (mirrors internal/pages/service.go enqueuer). EnqueueAndWait blocks until the
// job is terminal: nil on done, a non-nil error on failed, jobs.ErrJobTimeout on
// timeout.
type enqueuer interface {
	Enqueue(ctx context.Context, kind, payload string) error
	EnqueueAndWait(ctx context.Context, kind, payload string, timeout time.Duration) error
}

// extractableExts is the set of extensions the ExtractJob can read text from. A
// type not in this set gets NO ExtractJob enqueued (and so no <id>.txt and no
// extraction chip on the card). Mirrors the dispatch in Extract.
var extractableExts = map[string]bool{"pdf": true, "docx": true, "txt": true}

// isExtractable reports whether an upload's sniffed extension is one the ExtractJob
// can process.
func isExtractable(ext string) bool { return extractableExts[strings.ToLower(ext)] }

// fileWrite mirrors internal/pages.fileWrite EXACTLY (same JSON tags) so the
// payload this service marshals is byte-identical to what the registered
// KindCommit handler unmarshals.
type fileWrite struct {
	Path  string `json:"path"`
	Bytes []byte `json:"bytes"`
}

// commitPayload mirrors internal/pages.commitPayload EXACTLY (same JSON tags) so
// attachments can reuse the ONE registered KindCommit handler without a second
// write path (ATT-10). Keeping it a local mirror (rather than exporting the pages
// type) keeps the two packages decoupled; TestCommitPayloadShape guards drift.
type commitPayload struct {
	Writes  []fileWrite         `json:"writes"`
	Removes []string            `json:"removes,omitempty"`
	Spec    gitstore.CommitSpec `json:"spec"`
	Push    bool                `json:"push"`
}

// Service is the attachment lifecycle service. Like pages.Service, every write
// flows through the single-writer CommitJob — the service NEVER writes the
// filesystem or shells out to git directly (ATT-10). All path I/O routes through
// repo.* (SEC-01).
type Service struct {
	repo              *repo.Repo
	worker            enqueuer
	db                *sql.DB
	allowedExtensions []string
	maxUploadMB       int
	pushOnCommit      bool
	extractText       bool
	now               func() time.Time
}

// NewService constructs the attachment service. cfg supplies the MIME-sniff
// allow-list (config.AttachmentsConfig.AllowedExtensions); maxUploadMB is the
// server-side size cap (read from config.Storage.MaxUploadMB by the caller, NOT
// duplicated on AttachmentsConfig); pushOnCommit threads config.Git.PushOnCommit
// onto every commit payload (mirrors pages.NewService). worker is held as the
// enqueuer interface so tests can inject a fake.
func NewService(r *repo.Repo, w *jobs.Worker, db *sql.DB, cfg config.AttachmentsConfig, maxUploadMB int, pushOnCommit bool) *Service {
	s := newService(r, w, db, cfg.AllowedExtensions, maxUploadMB, pushOnCommit)
	s.extractText = cfg.ExtractText
	return s
}

// newService is the concrete constructor used by NewService and by tests that
// inject a fake enqueuer. extractText defaults to true so tests exercise the
// ExtractJob enqueue path by default (NewService threads the real config flag).
func newService(r *repo.Repo, w enqueuer, db *sql.DB, allowed []string, maxUploadMB int, pushOnCommit bool) *Service {
	return &Service{
		repo:              r,
		worker:            w,
		db:                db,
		allowedExtensions: allowed,
		maxUploadMB:       maxUploadMB,
		pushOnCommit:      pushOnCommit,
		extractText:       true,
		now:               time.Now,
	}
}

// Upload validates, stores, and records one attachment for a page. It (1) caps
// the size (ErrTooLarge), (2) sniffs the REAL MIME type from magic bytes and
// rejects a type not on the allow-list (ErrTypeForbidden — the filename is never
// trusted, SEC-02/ATT-09), (3) generates an opaque ULID id, (4) writes the
// byte-exact binary + the JSON meta sidecar through the EXISTING single-writer
// CommitJob in ONE commit (ATT-10), and (5) records the operational row. It
// returns the stored AttachmentMeta.
func (s *Service) Upload(ctx context.Context, pagePath, filename string, data []byte, user string) (AttachmentMeta, error) {
	if s.maxUploadMB > 0 && int64(len(data)) > int64(s.maxUploadMB)<<20 {
		return AttachmentMeta{}, ErrTooLarge
	}

	// Sniff the REAL type from magic bytes; never trust the upload filename's
	// extension (SEC-02). Reject anything not on the configured allow-list (ATT-09).
	mt := mimetype.Detect(data)
	ext, ok := s.allowedExt(mt)
	if !ok {
		return AttachmentMeta{}, ErrTypeForbidden
	}

	id := NewID()
	binPath := BinPath(id, ext)
	metaPath := MetaPath(id)

	// Resolver backstop (SEC-01) before anything is staged. The CommitJob
	// re-resolves, but failing here gives a clean error.
	if _, err := s.repo.Resolve(binPath); err != nil {
		return AttachmentMeta{}, err
	}
	if _, err := s.repo.Resolve(metaPath); err != nil {
		return AttachmentMeta{}, err
	}

	sum := sha256.Sum256(data)
	meta := AttachmentMeta{
		ID:           id,
		OriginalName: filename,
		MimeType:     mt.String(),
		SizeBytes:    int64(len(data)),
		UploaderName: user,
		UploadedAt:   s.now().UTC(),
		PagePath:     pagePath,
		Sha256:       hex.EncodeToString(sum[:]),
		Ext:          ext,
	}
	metaJSON, err := marshalMeta(meta)
	if err != nil {
		return AttachmentMeta{}, err
	}

	// One commit for the binary + meta through the single-writer spine (ATT-10).
	// Message uses no Git vocabulary that surfaces to the user (hidden-Git rule).
	p := commitPayload{
		Writes: []fileWrite{
			{Path: binPath, Bytes: data},
			{Path: metaPath, Bytes: metaJSON},
		},
		Spec: gitstore.CommitSpec{
			Paths:   []string{binPath, metaPath},
			Message: "Add attachment",
			User:    user,
			Action:  "attach",
			Source:  "web-ui",
		},
		Push: s.pushOnCommit,
	}

	// Record the operational row BEFORE enqueuing the commit (WR-01). Ordering
	// matters because List reads only the DB: if the commit landed first and the
	// row insert then failed, the bytes would be committed-but-unlisted — an
	// orphan invisible to List, un-downloadable, never extracted, never
	// orphan-deletable. By inserting the row first we get a clean abort with
	// nothing committed on a DB failure; the only residual window is a row that
	// briefly exists without the file, which is reconcilable (and the commit
	// enqueue below rolls the row back on a real failure).
	if err := s.insertRow(ctx, meta); err != nil {
		return AttachmentMeta{}, err
	}
	if err := s.enqueueCommit(ctx, p); err != nil {
		// The commit enqueue returned a real (non-timeout) error — enqueueCommit
		// swallows ErrJobTimeout as success, so reaching here means the commit
		// will never land. Roll the row back so we never leave a listed row whose
		// file does not exist.
		if derr := s.deleteRow(ctx, meta.ID); derr != nil {
			slog.WarnContext(ctx, "attachments: failed to roll back row after commit-enqueue error",
				slog.String("attachment_id", meta.ID), slog.String("error", derr.Error()))
		}
		return AttachmentMeta{}, err
	}

	// Index the attachment now (filename → searchable immediately, SRCH-04). The
	// extracted text follows when the KindExtract handler completes and re-indexes
	// (SRCH-05). Fire-and-forget; does not block the upload.
	s.enqueueIndexUpsert(ctx, meta.ID)

	// Fire-and-forget text extraction (ATT-08): for an extractable type, enqueue a
	// KindExtract job that reads the just-committed binary, extracts text, and
	// commits the <id>.txt sidecar. This is Enqueue (NOT EnqueueAndWait) so the
	// upload returns immediately — the card's chip then tracks the lifecycle live
	// over SSE. A non-extractable type (image/other) enqueues NOTHING, so no .txt
	// is written and the card shows no extraction chip. An enqueue error is logged
	// but never fails the upload (the binary is already durably committed).
	if s.extractText && isExtractable(ext) {
		if err := s.enqueueExtract(ctx, meta); err != nil {
			slog.WarnContext(ctx, "attachments: failed to enqueue text extraction (upload still succeeded)",
				slog.String("attachment_id", meta.ID), slog.String("error", err.Error()))
		}
	}
	return meta, nil
}

// enqueueExtract marshals and fire-and-forget enqueues a KindExtract job for an
// uploaded attachment. The payload carries the binary path to READ and the .txt
// path to WRITE (the only path extraction ever writes — T-02-11).
func (s *Service) enqueueExtract(ctx context.Context, m AttachmentMeta) error {
	p := extractPayload{
		AttachmentID: m.ID,
		BinPath:      BinPath(m.ID, m.Ext),
		TxtPath:      TxtPath(m.ID),
		Ext:          m.Ext,
		PagePath:     m.PagePath,
		User:         m.UploaderName,
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("attachments: marshal extract payload: %w", err)
	}
	return s.worker.Enqueue(ctx, KindExtract, string(raw))
}

// enqueueIndexUpsert fires a FIRE-AND-FORGET search.KindIndex upsert for an
// attachment id so its filename (and, once extraction completes, its extracted
// text) is searchable without a restart. Called from the HTTP-handler goroutine
// (Upload/Replace) AFTER the commit lands; worker.Enqueue (never EnqueueAndWait)
// keeps search freshness off the upload latency path. A dropped enqueue is logged
// at Warn and swallowed — the rebuild backstop reconciles (T-03-20 accept).
func (s *Service) enqueueIndexUpsert(ctx context.Context, id string) {
	if err := s.worker.Enqueue(ctx, search.KindIndex, search.UpsertAttachmentPayload(id)); err != nil {
		slog.WarnContext(ctx, "attachments: failed to enqueue search index upsert (rebuild backstop reconciles)",
			slog.String("attachment_id", id), slog.String("error", err.Error()))
	}
}

// enqueueIndexDelete fires a FIRE-AND-FORGET search.KindIndex delete for an
// attachment id (removing its doc from the live index). Same context/contract as
// enqueueIndexUpsert.
func (s *Service) enqueueIndexDelete(ctx context.Context, id string) {
	if err := s.worker.Enqueue(ctx, search.KindIndex, search.DeleteAttachmentPayload(id)); err != nil {
		slog.WarnContext(ctx, "attachments: failed to enqueue search index delete (rebuild backstop reconciles)",
			slog.String("attachment_id", id), slog.String("error", err.Error()))
	}
}

// List returns the attachments recorded for a page, newest first, each with its
// current extraction status (ATT-03 foundation). The meta is read from the
// operational table (mirrored from the on-disk sidecar at upload time).
func (s *Service) List(ctx context.Context, pagePath string) ([]AttachmentListItem, error) {
	if s.db == nil {
		return nil, fmt.Errorf("attachments: list requires a database")
	}
	const q = `SELECT id, page_path, original_name, mime_type, size_bytes,
	                  uploader_name, uploaded_at, extract_status
	             FROM attachments WHERE page_path = ? ORDER BY uploaded_at DESC, id DESC`
	rows, err := s.db.QueryContext(ctx, q, pagePath)
	if err != nil {
		return nil, fmt.Errorf("attachments: list %q: %w", pagePath, err)
	}
	defer func() { _ = rows.Close() }()

	var items []AttachmentListItem
	for rows.Next() {
		var (
			it       AttachmentListItem
			uploaded string
			status   string
		)
		if err := rows.Scan(&it.ID, &it.PagePath, &it.OriginalName, &it.MimeType,
			&it.SizeBytes, &it.UploaderName, &uploaded, &status); err != nil {
			return nil, fmt.Errorf("attachments: scan list row: %w", err)
		}
		if t, perr := time.Parse(time.RFC3339Nano, uploaded); perr == nil {
			it.UploadedAt = t
		} else if t, perr := time.Parse("2006-01-02 15:04:05", uploaded); perr == nil {
			it.UploadedAt = t
		}
		it.ExtractionStatus = ExtractionStatus(status)
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("attachments: iterate list rows: %w", err)
	}
	return items, nil
}

// ExtractionStatus returns an attachment's current text-extraction status from the
// operational mirror (the SSE endpoint streams this to the card chip). It is the
// service-level wrapper over ExtractionStatusFor so callers never touch the DB
// directly. ErrAttachmentNotFound when no row exists.
func (s *Service) ExtractionStatus(ctx context.Context, id string) (ExtractionStatus, error) {
	return ExtractionStatusFor(ctx, s.db, id)
}

// Meta reads an attachment's meta sidecar (the source of truth for the original
// filename + sniffed type used by the download handler). ErrAttachmentNotFound
// when the sidecar does not exist.
func (s *Service) Meta(ctx context.Context, id string) (AttachmentMeta, error) {
	return readMeta(s.repo, id)
}

// ResolveBin resolves an attachment's binary to a safe absolute path (SEC-01) and
// returns it. It reads NOTHING — the download handler passes the resolved path to
// http.ServeContent, which streams the byte-exact file itself. Returns an error
// when the path cannot be resolved (e.g. the attachment is missing).
func (s *Service) ResolveBin(id, ext string) (string, error) {
	abs, err := s.repo.Resolve(BinPath(id, ext))
	if err != nil {
		return "", err
	}
	return abs, nil
}

// allowedExt sniffs the extension for an allowed MIME type. The allow-list is
// matched by extension (no leading dot), comparing against the sniffed type's
// canonical extension. Returns the sniffed extension (no dot) and whether it is
// allowed.
func (s *Service) allowedExt(mt *mimetype.MIME) (string, bool) {
	ext := strings.TrimPrefix(mt.Extension(), ".")
	if ext == "" {
		return "", false
	}
	for _, a := range s.allowedExtensions {
		// Single allow-list semantics (WR-05): match by the sniffed canonical
		// extension only. The old `mt.Is(a)` branch silently widened the check to
		// MIME-string matching, which could accept a type whose extension the rest
		// of the system (extractable set, inline-image set, download disposition)
		// does not expect — a footgun that desyncs the allow-list from those.
		if strings.EqualFold(strings.TrimPrefix(a, "."), ext) {
			return ext, true
		}
	}
	return ext, false
}

// enqueueCommit marshals p and enqueues a commit job, blocking until it lands (or
// times out, in which case the async commit still completes — mirrors
// pages.EnqueueCommit). The payload is byte-identical to what the pages KindCommit
// handler unmarshals.
func (s *Service) enqueueCommit(ctx context.Context, p commitPayload) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("attachments: marshal commit payload: %w", err)
	}
	err = s.worker.EnqueueAndWait(ctx, kindCommit, string(raw), commitWaitTimeout)
	if errors.Is(err, jobs.ErrJobTimeout) {
		slog.WarnContext(ctx, "attachments: commit wait timed out; returning success, job stays queued",
			slog.Duration("timeout", commitWaitTimeout))
		return nil
	}
	return err
}

// insertRow records the operational attachment row (mirrors the on-disk meta).
// extract_status starts pending; the ExtractJob (02-03) advances it.
func (s *Service) insertRow(ctx context.Context, m AttachmentMeta) error {
	if s.db == nil {
		return nil
	}
	const q = `INSERT INTO attachments
	    (id, page_path, original_name, mime_type, size_bytes, uploader_name, uploaded_at, extract_status, extract_error)
	    VALUES (?, ?, ?, ?, ?, ?, ?, ?, '')
	    ON CONFLICT(id) DO UPDATE SET
	        page_path=excluded.page_path,
	        original_name=excluded.original_name,
	        mime_type=excluded.mime_type,
	        size_bytes=excluded.size_bytes,
	        uploader_name=excluded.uploader_name,
	        uploaded_at=excluded.uploaded_at`
	if _, err := s.db.ExecContext(ctx, q,
		m.ID, m.PagePath, m.OriginalName, m.MimeType, m.SizeBytes,
		m.UploaderName, m.UploadedAt.Format(time.RFC3339Nano), string(ExtractionPending)); err != nil {
		return fmt.Errorf("attachments: insert row %q: %w", m.ID, err)
	}
	return nil
}
