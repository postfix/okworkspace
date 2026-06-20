package attachments

import (
	"github.com/oklog/ulid/v2"
)

// attachmentDir is the flat tree at the repo root that holds every attachment's
// three artifacts (binary + meta + extracted text). Kept as one place so the path
// helpers below can never drift.
const attachmentDir = "attachments/"

// NewID generates a fresh opaque attachment id. A ULID is sortable, fixed-length,
// collision-resistant, and — crucially — carries NO information from the
// attacker-controlled filename, so it is safe to use as the on-disk name (SEC-02).
// Replace (slice 02-04) reuses the SAME id, which is why a ULID (not a content
// hash) is the id: a content hash would change when the bytes change.
func NewID() string { return ulid.Make().String() }

// BinPath returns the repo-relative path of an attachment's byte-exact binary,
// attachments/<id>.<ext>. ext must NOT include a leading dot.
func BinPath(id, ext string) string { return attachmentDir + id + "." + ext }

// MetaPath returns the repo-relative path of an attachment's JSON meta sidecar,
// attachments/<id>.json.
func MetaPath(id string) string { return attachmentDir + id + ".json" }

// TxtPath returns the repo-relative path of an attachment's extracted-text
// sidecar, attachments/<id>.txt (written by the ExtractJob in slice 02-03).
func TxtPath(id string) string { return attachmentDir + id + ".txt" }

// DownloadRefPath is the SINGLE source of truth for the canonical page-attachment
// link target: /api/v1/attachments/<id>/download. The upload-time link insert
// (slice 02-04) builds page Markdown around this exact string, and the orphan
// reference scan (02-04) matches on this exact substring — defining it once here
// guarantees insert and scan can never drift (RESEARCH Open Question 3, Pitfall 6).
// NOTE: slice 02-01 does NOT yet edit the page body on upload; the helper is fixed
// now so the contract is stable for the later slices.
func DownloadRefPath(id string) string { return "/api/v1/attachments/" + id + "/download" }
