package attachments

import (
	"encoding/json"
	"fmt"

	"github.com/postfix/okworkspace/internal/repo"
)

// marshalMeta renders an AttachmentMeta to the <id>.json sidecar bytes. Indented
// so the on-disk sidecar stays human-readable (files-as-truth: a teammate can
// open it and see what the attachment is). The bytes are written ONLY through the
// commit path — never os.WriteFile (the single-writer invariant, ATT-10).
func marshalMeta(m AttachmentMeta) ([]byte, error) {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("attachments: marshal meta %q: %w", m.ID, err)
	}
	// Trailing newline so the file is a well-formed text line on disk.
	return append(b, '\n'), nil
}

// readMeta reads and parses an attachment's <id>.json sidecar through the
// resolver (SEC-01). ErrAttachmentNotFound is returned when the sidecar does not
// exist, so callers can map it to a clean 404.
func readMeta(r *repo.Repo, id string) (AttachmentMeta, error) {
	path := MetaPath(id)
	exists, err := r.Exists(path)
	if err != nil {
		return AttachmentMeta{}, err
	}
	if !exists {
		return AttachmentMeta{}, ErrAttachmentNotFound
	}
	raw, err := r.Read(path)
	if err != nil {
		return AttachmentMeta{}, fmt.Errorf("attachments: read meta %q: %w", path, err)
	}
	var m AttachmentMeta
	if err := json.Unmarshal(raw, &m); err != nil {
		return AttachmentMeta{}, fmt.Errorf("attachments: parse meta %q: %w", path, err)
	}
	return m, nil
}
