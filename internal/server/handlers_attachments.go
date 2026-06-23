package server

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/postfix/okworkspace/internal/attachments"
	"github.com/postfix/okworkspace/internal/audit"
)

// inlineImageTypes are the ONLY MIME types served inline (SEC-02). Everything else
// is forced to download with Content-Disposition: attachment.
//
// SVG is deliberately NOT inline: although an <img>-loaded SVG cannot execute
// script (the SPA preview uses <img src> and browsers ignore Content-Disposition
// for <img>, so inline preview still works), serving image/svg+xml with
// Content-Disposition: inline lets a user NAVIGATE DIRECTLY to the attachment URL,
// which renders the SVG as a top-level document on the APP ORIGIN — any embedded
// <script> then runs with the app's session (stored XSS). So SVG is served as a
// forced download like any other risky type; the <img> thumbnail/preview is
// unaffected. (Defense-in-depth CSP sandbox is also set on every response below.)
var inlineImageTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
}

// isInlineImage reports whether the stored MIME type may be served inline. The
// stored mime_type may carry parameters (e.g. "image/svg+xml; charset=utf-8"), so
// the media type is compared on its prefix before the first ";".
func isInlineImage(mimeType string) bool {
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}
	return inlineImageTypes[strings.ToLower(mimeType)]
}

// handleUploadAttachment accepts a multipart upload and stores it for a page
// (editor only). The body is hard-capped with MaxBytesReader BEFORE multipart
// parsing so an oversize upload is rejected before any bytes are buffered/spooled
// (ATT-09/Pitfall 1). The real type is sniffed server-side from magic bytes by the
// service (the filename is never trusted, SEC-02).
func (h *authHandlers) handleUploadAttachment(w http.ResponseWriter, r *http.Request) {
	if h.attachments == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}

	// HARD server-side cap before parsing (the real DoS guard — Pitfall 1). A
	// little slack is added for the multipart form overhead so a file at exactly
	// the limit is not rejected by the envelope bytes.
	maxBytes := int64(h.config.Storage.MaxUploadMB) << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "That file is too large.")
		return
	}

	pagePath, ok := cleanPathString(w, r.FormValue("page_path"))
	if !ok {
		return
	}

	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Choose a file to upload.")
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		// A read error past the MaxBytesReader limit surfaces here as the body cap.
		writeError(w, http.StatusRequestEntityTooLarge, "That file is too large.")
		return
	}

	meta, err := h.attachments.Upload(r.Context(), pagePath, hdr.Filename, data, h.actorUsername(r.Context()))
	if err != nil {
		writeAttachmentError(w, err)
		return
	}

	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionAttachUpload,
		Actor:  h.actorUsername(r.Context()),
		Target: pagePath + "/" + meta.ID,
		Source: auditSourceWeb,
	})
	writeJSON(w, http.StatusCreated, meta)
}

// handleReplaceAttachment swaps an attachment's bytes in place, reusing the SAME id
// (ATT-05, editor only). Like upload, the body is hard-capped with MaxBytesReader
// BEFORE multipart parsing (Pitfall 1) and the real type is re-sniffed server-side
// (the filename is never trusted, SEC-02/ATT-09). The id is taken from the URL
// param; the new file from the "file" form field. On success it records an
// ActionAttachReplace audit event and returns the updated meta. The route is
// editor-gated from the SESSION (RequireRole), never client input (T-02-14).
func (h *authHandlers) handleReplaceAttachment(w http.ResponseWriter, r *http.Request) {
	if h.attachments == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}

	id := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	if id == "" || strings.ContainsAny(id, "/\x00") {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}

	// HARD server-side cap before parsing (the real DoS guard — Pitfall 1 / T-02-16),
	// identical to upload.
	maxBytes := int64(h.config.Storage.MaxUploadMB) << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "That file is too large.")
		return
	}

	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Choose a file to upload.")
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "That file is too large.")
		return
	}

	meta, err := h.attachments.Replace(r.Context(), id, hdr.Filename, data, h.actorUsername(r.Context()))
	if err != nil {
		writeAttachmentError(w, err)
		return
	}

	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionAttachReplace,
		Actor:  h.actorUsername(r.Context()),
		Target: meta.PagePath + "/" + meta.ID,
		Source: auditSourceWeb,
	})
	writeJSON(w, http.StatusOK, meta)
}

// handleDeleteAttachment removes an attachment's link from a page and, when that was
// the LAST reference across all pages, deletes the binary + meta + txt in ONE commit
// (ATT-06/ATT-07, editor only). The id is the URL param; the page to unlink is read
// from the "page_path" query parameter (the canonical link to drop lives on that
// page). On success it records an ActionAttachDelete audit event and returns
// {deleted_orphan: bool}. Editor-gated from the SESSION (T-02-14).
func (h *authHandlers) handleDeleteAttachment(w http.ResponseWriter, r *http.Request) {
	if h.attachments == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}

	id := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	if id == "" || strings.ContainsAny(id, "/\x00") {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}

	pagePath, ok := cleanPathString(w, r.URL.Query().Get("page_path"))
	if !ok {
		return
	}

	deletedOrphan, err := h.attachments.Remove(r.Context(), id, pagePath, h.actorUsername(r.Context()))
	if err != nil {
		writeAttachmentError(w, err)
		return
	}

	_ = h.audit.Record(r.Context(), audit.Event{
		Action: audit.ActionAttachDelete,
		Actor:  h.actorUsername(r.Context()),
		Target: pagePath + "/" + id,
		Source: auditSourceWeb,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"deleted_orphan": deletedOrphan})
}

// handleGetAttachment dispatches the GET /attachments/* catch-all: a download iff
// the wildcard is "{id}/download", otherwise a per-page attachment list keyed on
// the wildcard as a page path. chi cannot host a `{id}/download` route next to the
// slash-bearing list wildcard (the sibling-wildcard conflict the page routes also
// hit), so both reads are dispatched here on the same catch-all.
func (h *authHandlers) handleGetAttachment(w http.ResponseWriter, r *http.Request) {
	wild := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	if id, ok := strings.CutSuffix(wild, "/download"); ok {
		h.handleDownloadAttachment(w, r, id)
		return
	}
	// SSE extraction-status stream is dispatched on the SAME catch-all (a sibling
	// {id}/status route would hit the chi sibling-wildcard conflict the list
	// wildcard already triggers — see the router comment).
	if id, ok := strings.CutSuffix(wild, "/status"); ok {
		h.handleExtractionStatus(w, r, id)
		return
	}
	h.handleListAttachments(w, r, wild)
}

// handleListAttachments returns a page's attachment meta list as JSON (any
// authenticated user). The list is read from the operational mirror.
func (h *authHandlers) handleListAttachments(w http.ResponseWriter, r *http.Request, pagePath string) {
	if h.attachments == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	pagePath, ok := cleanPathString(w, pagePath)
	if !ok {
		return
	}
	items, err := h.attachments.List(r.Context(), pagePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't load attachments — try again.")
		return
	}
	if items == nil {
		items = []attachments.AttachmentListItem{}
	}
	writeJSON(w, http.StatusOK, items)
}

// handleDownloadAttachment streams an attachment's byte-exact original (ATT-02).
// The disposition is decided by the STORED sniffed type (SEC-02), never the
// request: png/jpeg are served inline with their real Content-Type; SVG and every
// other type are forced to download as application/octet-stream with the original
// filename (SVG inline on the app origin is a stored-XSS vector — see
// inlineImageTypes). X-Content-Type-Options: nosniff and a CSP sandbox are ALWAYS
// set. http.ServeContent streams the bytes unchanged (never transcodes) and handles
// Range for <img> preview (Pitfall 4).
func (h *authHandlers) handleDownloadAttachment(w http.ResponseWriter, r *http.Request, id string) {
	if h.attachments == nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}
	if id == "" || strings.ContainsAny(id, "/\x00") {
		writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}

	meta, err := h.attachments.Meta(r.Context(), id)
	if err != nil {
		if errors.Is(err, attachments.ErrAttachmentNotFound) {
			writeError(w, http.StatusNotFound, "That attachment no longer exists.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}

	abs, err := h.attachments.ResolveBin(id, meta.Ext)
	if err != nil {
		writeError(w, http.StatusNotFound, "That attachment no longer exists.")
		return
	}
	f, err := os.Open(abs)
	if err != nil {
		writeError(w, http.StatusNotFound, "That attachment no longer exists.")
		return
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
		return
	}

	// Harden against content-type confusion on EVERY branch (SEC-02).
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Defense-in-depth: even if a renderable/active type is ever served, this
	// sandbox + null default-src neuters scripts, plugins, and same-origin access
	// for any attachment loaded as a top-level document.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	if isInlineImage(meta.MimeType) {
		w.Header().Set("Content-Type", meta.MimeType)
		w.Header().Set("Content-Disposition", "inline")
	} else {
		// Risky types are download-only; quote the ORIGINAL filename for the user
		// via RFC 5987 encoding (the on-disk name is the opaque id).
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition",
			"attachment; filename*=UTF-8''"+url.PathEscape(meta.OriginalName))
	}

	// ServeContent uses the name only for a Content-Type fallback (we already set
	// one) and for Range handling; it streams the bytes byte-exact (ATT-02).
	http.ServeContent(w, r, meta.OriginalName, fi.ModTime(), f)
}

// writeAttachmentError maps attachment service sentinel errors to HTTP statuses
// (mirrors the pages error-mapping block).
func writeAttachmentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, attachments.ErrAttachmentNotFound):
		writeError(w, http.StatusNotFound, "That attachment no longer exists.")
	case errors.Is(err, attachments.ErrTypeForbidden):
		writeError(w, http.StatusUnsupportedMediaType, "That file type isn't allowed.")
	case errors.Is(err, attachments.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "That file is too large.")
	case errors.Is(err, attachments.ErrPageNotFound):
		writeError(w, http.StatusNotFound, "This page no longer exists. It may have been moved or deleted.")
	default:
		writeError(w, http.StatusInternalServerError, "Something went wrong. Check your connection and try again.")
	}
}
