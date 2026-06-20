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
	now               func() time.Time
}

// NewService constructs the attachment service. cfg supplies the MIME-sniff
// allow-list (config.AttachmentsConfig.AllowedExtensions); maxUploadMB is the
// server-side size cap (read from config.Storage.MaxUploadMB by the caller, NOT
// duplicated on AttachmentsConfig); pushOnCommit threads config.Git.PushOnCommit
// onto every commit payload (mirrors pages.NewService). worker is held as the
// enqueuer interface so tests can inject a fake.
func NewService(r *repo.Repo, w *jobs.Worker, db *sql.DB, cfg config.AttachmentsConfig, maxUploadMB int, pushOnCommit bool) *Service {
	return newService(r, w, db, cfg.AllowedExtensions, maxUploadMB, pushOnCommit)
}

// newService is the concrete constructor used by NewService and by tests that
// inject a fake enqueuer.
func newService(r *repo.Repo, w enqueuer, db *sql.DB, allowed []string, maxUploadMB int, pushOnCommit bool) *Service {
	return &Service{
		repo:              r,
		worker:            w,
		db:                db,
		allowedExtensions: allowed,
		maxUploadMB:       maxUploadMB,
		pushOnCommit:      pushOnCommit,
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
	if err := s.enqueueCommit(ctx, p); err != nil {
		return AttachmentMeta{}, err
	}

	// Record the operational row (best-effort mirror of the on-disk meta). A DB
	// error must not undo a durable commit, but it is surfaced so the handler can
	// 500 if the row is essential to the response — here the meta is already on
	// disk, so we treat a row failure as fatal to keep the list consistent.
	if err := s.insertRow(ctx, meta); err != nil {
		return AttachmentMeta{}, err
	}
	return meta, nil
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

// Meta reads an attachment's meta sidecar (the source of truth for the original
// filename + sniffed type used by the download handler). ErrAttachmentNotFound
// when the sidecar does not exist.
func (s *Service) Meta(ctx context.Context, id string) (AttachmentMeta, error) {
	return readMeta(s.repo, id)
}

// Open resolves and reads the byte-exact binary for an attachment. The download
// handler uses the resolved path with http.ServeContent; this returns the raw
// bytes for callers that need them directly. ErrAttachmentNotFound when missing.
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
		if strings.EqualFold(strings.TrimPrefix(a, "."), ext) {
			return ext, true
		}
		// Also allow when the sniffed MIME advertises an alias the config lists by
		// MIME string (defensive; allow-list is normally extensions).
		if mt.Is(a) {
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
