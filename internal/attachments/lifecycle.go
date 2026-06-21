package attachments

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gabriel-vasile/mimetype"

	"github.com/postfix/okworkspace/internal/gitstore"
)

// Replace swaps an attachment's bytes IN PLACE, reusing the SAME opaque id
// (ATT-05). It validates the new bytes exactly like Upload (size cap →
// ErrTooLarge, magic-byte MIME sniff vs the allow-list → ErrTypeForbidden; the
// filename is never trusted, SEC-02/ATT-09), recomputes sha256/size, and writes
// the new binary + an updated meta sidecar through the EXISTING single-writer
// CommitJob in ONE commit (ATT-10). Because the id is unchanged, every page link
// (DownloadRefPath(id)) keeps pointing at the same attachment — no page edits are
// needed. The prior bytes are retained automatically in history and can be
// restored (hidden-Git: the commit message carries no Git vocabulary). If the new
// sniffed extension differs from the stored one, the stale binary path is added to
// the SAME commit's Removes so exactly one binary survives. Finally it re-enqueues
// a KindExtract job (fire-and-forget) so the extracted text is refreshed and the
// card's chip transitions live again. ErrAttachmentNotFound when no attachment
// with id exists.
func (s *Service) Replace(ctx context.Context, id, filename string, data []byte, user string) (AttachmentMeta, error) {
	if id == "" || strings.ContainsAny(id, "/\x00") {
		return AttachmentMeta{}, ErrAttachmentNotFound
	}

	// The attachment must already exist; load the current meta to learn its
	// stored extension (so a changed type can delete the old binary path) and
	// its page (so the row/meta stay consistent).
	prev, err := readMeta(s.repo, id)
	if err != nil {
		return AttachmentMeta{}, err
	}

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

	binPath := BinPath(id, ext)
	metaPath := MetaPath(id)

	// Resolver backstop (SEC-01) before anything is staged.
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
		PagePath:     prev.PagePath,
		Sha256:       hex.EncodeToString(sum[:]),
		Ext:          ext,
	}
	metaJSON, err := marshalMeta(meta)
	if err != nil {
		return AttachmentMeta{}, err
	}

	// One commit for the new binary + updated meta through the single-writer spine
	// (ATT-10). If the new sniffed extension differs from the stored one, the stale
	// binary path is removed in the SAME commit so exactly one binary survives.
	paths := []string{binPath, metaPath}
	var removes []string
	if !strings.EqualFold(prev.Ext, ext) {
		oldBin := BinPath(id, prev.Ext)
		removes = append(removes, oldBin)
		paths = append(paths, oldBin)
	}
	p := commitPayload{
		Writes: []fileWrite{
			{Path: binPath, Bytes: data},
			{Path: metaPath, Bytes: metaJSON},
		},
		Removes: removes,
		Spec: gitstore.CommitSpec{
			Paths:   paths,
			Message: "Replace attachment",
			User:    user,
			Action:  "attach-replace",
			Source:  "web-ui",
		},
		Push: s.pushOnCommit,
	}
	// Refresh the operational row BEFORE enqueuing the commit (WR-01), mirroring
	// Upload's ordering. The row already exists (Replace is in place under the same
	// id), so insertRow upserts the new size/sha/mime/name. If the commit then
	// fails for real (non-timeout), the row would be ahead of the on-disk bytes;
	// we re-sync the row back to the previous meta so List never advertises a
	// replace that did not durably land. The prior binary is untouched on disk in
	// that case (the new write never committed), so reverting the row keeps the
	// row and disk consistent.
	if err := s.insertRow(ctx, meta); err != nil {
		return AttachmentMeta{}, err
	}
	if err := s.enqueueCommit(ctx, p); err != nil {
		// Real (non-timeout) commit failure: the new bytes never landed. Restore
		// the row to the previous meta so it does not claim a size/sha that is not
		// on disk. Best-effort — log if the revert itself fails.
		if rerr := s.insertRow(ctx, prev); rerr != nil {
			slog.WarnContext(ctx, "attachments: failed to revert row after replace commit-enqueue error",
				slog.String("attachment_id", id), slog.String("error", rerr.Error()))
		}
		return AttachmentMeta{}, err
	}

	// Reset extract status to pending so the chip transitions live again on
	// re-extraction (insertRow's upsert does not touch extract_status for an
	// existing row).
	if err := setExtractStatus(ctx, s.db, id, ExtractionPending, ""); err != nil {
		// Non-fatal: the row update above already wrote pending via the upsert path
		// for new rows; this is a defensive reset for the in-place replace.
		slog.WarnContext(ctx, "attachments: failed to reset extract status on replace",
			slog.String("attachment_id", id), slog.String("error", err.Error()))
	}

	// Re-extract the new bytes (ATT-05): fire-and-forget so Replace returns
	// immediately and the card's chip tracks the refreshed lifecycle over SSE. A
	// non-extractable new type enqueues NOTHING (no .txt, no chip).
	if s.extractText && isExtractable(ext) {
		if err := s.enqueueExtract(ctx, meta); err != nil {
			slog.WarnContext(ctx, "attachments: failed to enqueue re-extraction (replace still succeeded)",
				slog.String("attachment_id", id), slog.String("error", err.Error()))
		}
	}
	return meta, nil
}

// Remove drops an attachment's canonical link from one page and, when that was the
// LAST reference across ALL pages, deletes the attachment's three artifacts in ONE
// commit (ATT-06 + ATT-07). It (1) loads pagePath's Markdown and strips every
// occurrence of the canonical DownloadRefPath(id) link, committing the edited page
// through the single-writer CommitJob (ATT-06); (2) scans EVERY page for remaining
// references via PageReferences (the single canonical match — Pitfall 6); and (3)
// if zero pages still reference the id, builds ONE commitPayload whose Removes
// carries the binary + JSON meta + (when present) the TXT sidecar, commits the
// delete, and removes the operational row (ATT-07). If any other page still
// references the id, the files are KEPT — only the link on pagePath is dropped.
// Returns deletedOrphan = true iff the files were deleted. ErrAttachmentNotFound
// when no attachment with id exists; ErrPageNotFound when pagePath does not exist.
func (s *Service) Remove(ctx context.Context, id, pagePath, user string) (bool, error) {
	if id == "" || strings.ContainsAny(id, "/\x00") {
		return false, ErrAttachmentNotFound
	}

	// The attachment must exist (so a stray delete on a bad id is a clean 404 and
	// we know the stored extension for the orphan-delete binary path).
	meta, err := readMeta(s.repo, id)
	if err != nil {
		return false, err
	}

	// (1) Unlink: strip the canonical link from the target page's Markdown and
	// commit the edited page through the existing single-writer path (ATT-06).
	if err := s.unlinkPage(ctx, id, pagePath, user); err != nil {
		return false, err
	}

	// (2) Ref-count across ALL pages using the SINGLE canonical match (Pitfall 6).
	refs, err := PageReferences(ctx, s.repo, id)
	if err != nil {
		return false, err
	}
	if refs > 0 {
		// Still referenced elsewhere — KEEP the files; only the link was dropped.
		return false, nil
	}

	// (3) Orphan: delete the binary + meta + (if present) txt in ONE commit (ATT-07).
	binPath := BinPath(id, meta.Ext)
	metaPath := MetaPath(id)
	txtPath := TxtPath(id)

	removes := []string{binPath, metaPath}
	paths := []string{binPath, metaPath}
	if exists, err := s.repo.Exists(txtPath); err != nil {
		return false, err
	} else if exists {
		removes = append(removes, txtPath)
		paths = append(paths, txtPath)
	}

	p := commitPayload{
		Removes: removes,
		Spec: gitstore.CommitSpec{
			Paths:   paths,
			Message: "Delete attachment",
			User:    user,
			Action:  "attach-delete",
			Source:  "web-ui",
		},
		Push: s.pushOnCommit,
	}
	if err := s.enqueueCommit(ctx, p); err != nil {
		return false, err
	}

	if err := s.deleteRow(ctx, id); err != nil {
		return false, err
	}
	return true, nil
}

// unlinkPage loads pagePath's Markdown, removes every occurrence of the canonical
// DownloadRefPath(id) link (ATT-06), and commits the edited page through the
// existing single-writer CommitJob. When the page no longer changes (the link was
// already gone), no commit is made. ErrPageNotFound when pagePath does not exist.
//
// The unlink is a conservative string operation against the ONE canonical link
// form (Pitfall 6): it removes the surrounding Markdown link/image syntax that
// targets DownloadRefPath(id), falling back to dropping just the URL if the
// surrounding syntax is unrecognised, so a still-referenced id elsewhere is never
// affected (the scan and the edit share DownloadRefPath).
func (s *Service) unlinkPage(ctx context.Context, id, pagePath, user string) error {
	exists, err := s.repo.Exists(pagePath)
	if err != nil {
		return err
	}
	if !exists {
		return ErrPageNotFound
	}
	raw, err := s.repo.Read(pagePath)
	if err != nil {
		return fmt.Errorf("attachments: read %q for unlink: %w", pagePath, err)
	}

	edited := stripAttachmentLinks(string(raw), DownloadRefPath(id))
	if edited == string(raw) {
		// Nothing to change on this page (the link was already absent) — do not
		// create an empty commit.
		return nil
	}

	p := commitPayload{
		Writes: []fileWrite{{Path: pagePath, Bytes: []byte(edited)}},
		Spec: gitstore.CommitSpec{
			Paths:   []string{pagePath},
			Message: "Remove attachment link",
			User:    user,
			Action:  "attach-unlink",
			Source:  "web-ui",
		},
		Push: s.pushOnCommit,
	}
	return s.enqueueCommit(ctx, p)
}

// stripAttachmentLinks removes every Markdown link/image whose target is ref (the
// canonical DownloadRefPath) from body, returning the edited body. It handles the
// two shapes the upload-time insert can produce — an inline link `[text](ref)` and
// an image `![alt](ref)` — and as a final fallback drops a bare `ref` occurrence.
// Matching on the SINGLE canonical ref string (not a heuristic) guarantees a
// still-referenced id elsewhere is untouched (Pitfall 6). Whitespace-only lines
// left behind by removing a link that occupied its own line are collapsed so the
// page does not accrue blank lines on repeated edits.
func stripAttachmentLinks(body, ref string) string {
	if !strings.Contains(body, ref) {
		return body
	}

	out := body
	// Remove `![alt](ref ...)` and `[text](ref ...)` inline forms. The closing
	// paren may be preceded by an optional title, so scan to the next ")".
	for _, prefixOpen := range []string{"![", "["} {
		for {
			idx := indexLinkWithTarget(out, prefixOpen, ref)
			if idx < 0 {
				break
			}
			end := strings.IndexByte(out[idx:], ')')
			if end < 0 {
				break
			}
			out = out[:idx] + out[idx+end+1:]
		}
	}

	// Fallback: drop any remaining bare occurrences of the canonical URL.
	out = strings.ReplaceAll(out, ref, "")

	// Collapse lines that are now blank-or-whitespace where a link used to live,
	// without disturbing intentional blank lines elsewhere: trim trailing spaces
	// on each line.
	lines := strings.Split(out, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(ln, " \t")
	}
	return strings.Join(lines, "\n")
}

// indexLinkWithTarget returns the byte index of the FIRST Markdown link/image that
// opens with prefixOpen ("[" or "![") and whose target part contains ref, or -1.
// It finds each prefixOpen, then checks the matching "](" sits before a "(" that
// introduces ref — a conservative scan that avoids a full Markdown parse while only
// ever matching links pointed at the canonical attachment URL.
func indexLinkWithTarget(s, prefixOpen, ref string) int {
	from := 0
	for {
		i := strings.Index(s[from:], prefixOpen)
		if i < 0 {
			return -1
		}
		i += from
		// Find the "](" that closes the text part for this prefix.
		close := strings.Index(s[i:], "](")
		if close < 0 {
			return -1
		}
		targetStart := i + close + 2
		paren := strings.IndexByte(s[targetStart:], ')')
		if paren < 0 {
			return -1
		}
		target := s[targetStart : targetStart+paren]
		if strings.Contains(target, ref) {
			return i
		}
		from = i + len(prefixOpen)
	}
}

// deleteRow removes the operational attachment row after an orphan delete. A nil DB
// is a no-op (tests that do not exercise the row path).
func (s *Service) deleteRow(ctx context.Context, id string) error {
	if s.db == nil {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM attachments WHERE id = ?`, id); err != nil {
		return fmt.Errorf("attachments: delete row %q: %w", id, err)
	}
	return nil
}
