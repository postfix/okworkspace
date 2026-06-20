package attachments

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ExtractionStatusFor reads an attachment's current extraction status from the
// operational row's extract_status column — the single source of truth the SSE
// endpoint streams to the card chip (interface contract). Reading the column
// (rather than the jobs table) means the status survives job-row pruning and keeps
// the SSE handler trivially simple. ErrAttachmentNotFound is returned when no row
// exists so the handler can map it to a clean 404.
//
// NOTE: extract_error is deliberately NOT returned to callers that feed the client
// — the raw parser error stays server-side (T-02-12 information disclosure); the
// failed chip shows the fixed copy "Couldn't extract text".
func ExtractionStatusFor(ctx context.Context, db *sql.DB, id string) (ExtractionStatus, error) {
	if db == nil {
		return "", fmt.Errorf("attachments: extraction status requires a database")
	}
	var status string
	row := db.QueryRowContext(ctx, `SELECT extract_status FROM attachments WHERE id = ?`, id)
	if err := row.Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrAttachmentNotFound
		}
		return "", fmt.Errorf("attachments: read extract status %q: %w", id, err)
	}
	return ExtractionStatus(status), nil
}

// IsTerminalStatus reports whether an extraction status is final (the SSE stream
// closes once a terminal status is reached). "extracting"/pending are in-flight;
// done/empty/failed are terminal.
func IsTerminalStatus(s ExtractionStatus) bool {
	switch s {
	case ExtractionDone, ExtractionEmpty, ExtractionFailed:
		return true
	default:
		return false
	}
}

// setExtractStatus updates an attachment row's extract_status (and extract_error
// for a failure; cleared otherwise). It is best-effort from the ExtractJob's
// perspective — a status write failure is surfaced so the handler can decide, but
// the durable artifact is the committed <id>.txt, not this mirror row.
func setExtractStatus(ctx context.Context, db *sql.DB, id string, status ExtractionStatus, errMsg string) error {
	if db == nil {
		return nil
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE attachments SET extract_status = ?, extract_error = ? WHERE id = ?`,
		string(status), errMsg, id); err != nil {
		return fmt.Errorf("attachments: set extract status %q=%q: %w", id, status, err)
	}
	return nil
}
