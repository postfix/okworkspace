package attachments

import (
	"bytes"
	"fmt"
	"strings"

	docx "github.com/fumiama/go-docx"
	"github.com/ledongthuc/pdf"
)

// Extract dispatches text extraction on a lowercased extension. It returns the
// extracted plain text (which MAY legitimately be empty — a scanned/image-only
// PDF or a blank document has no text layer; an empty string with a nil error is
// the explicit "No text extracted" success case, NOT a failure). An unknown /
// non-extractable extension returns ("", nil) so the caller writes no sidecar and
// the card shows no extraction chip.
//
// Every extractor runs under guardExtract, which recovers a panic raised deep
// inside a pure-Go third-party parser and converts it into a returned error
// instead of crashing the single job-drain goroutine (RESEARCH Pitfall 5 /
// T-02-09). The ExtractJob handler adds a second recover() for defense in depth.
func Extract(ext string, data []byte) (string, error) {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "pdf":
		return guardExtract("pdf", func() (string, error) { return extractPDF(data) })
	case "docx":
		return guardExtract("docx", func() (string, error) { return extractDOCX(data) })
	case "txt":
		return guardExtract("txt", func() (string, error) { return extractTXT(data) })
	default:
		// Non-extractable type: no .txt is written and no chip is shown.
		return "", nil
	}
}

// guardExtract runs fn under a recover() so a panic inside a third-party parser
// becomes a returned error rather than killing the single worker goroutine
// (Pitfall 5 / T-02-09). It is the single chokepoint every extractor flows
// through; a deterministic unit test exercises it with a deliberately panicking
// fn so the guarantee is proven regardless of whether a given fixture happens to
// panic on a particular parser version.
func guardExtract(kind string, fn func() (string, error)) (out string, err error) {
	defer func() {
		if r := recover(); r != nil {
			out = ""
			err = fmt.Errorf("attachments: %s extract panic: %v", kind, r)
		}
	}()
	return fn()
}

// extractPDF extracts whole-document plain text from PDF bytes using the pure-Go
// ledongthuc/pdf reader (CGO_ENABLED=0). An EMPTY result is a valid success: a
// scanned/image-only PDF has no text layer, which is the legitimate "No text
// extracted" path — never an error. A genuine open/parse failure (corrupt or
// encrypted) returns a non-nil error so the worker can retry then terminally fail
// ("Couldn't extract text").
func extractPDF(data []byte) (string, error) {
	rdr, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("attachments: pdf open: %w", err)
	}
	tr, err := rdr.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("attachments: pdf text: %w", err)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(tr); err != nil {
		return "", fmt.Errorf("attachments: pdf read: %w", err)
	}
	// TrimSpace so a whitespace-only / no-text-layer PDF reads as empty (the
	// "No text extracted" success case).
	return strings.TrimSpace(buf.String()), nil
}

// extractDOCX extracts paragraph and table text from DOCX bytes using the pure-Go
// fumiama/go-docx parser. It iterates the document body items, type-switching on
// paragraphs and tables and rendering each via String(). A parse failure (bad zip
// / not a DOCX) returns a non-nil error. An empty result (a blank document) is a
// valid success.
func extractDOCX(data []byte) (string, error) {
	doc, err := docx.Parse(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("attachments: docx parse: %w", err)
	}
	var b strings.Builder
	for _, it := range doc.Document.Body.Items {
		switch v := it.(type) {
		case *docx.Paragraph:
			b.WriteString(v.String())
			b.WriteByte('\n')
		case *docx.Table:
			b.WriteString(v.String())
			b.WriteByte('\n')
		}
	}
	return strings.TrimSpace(b.String()), nil
}

// extractTXT decodes a plain-text upload: it strips a leading UTF-8 BOM, normalizes
// CRLF to LF, and TrimSpaces the result (so a whitespace-only file reads as empty).
// UTF-16 / Latin-1 are out of MVP scope — the byte-exact original is always
// downloadable regardless. This never errors (text is taken as-is).
func extractTXT(data []byte) (string, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	return strings.TrimSpace(s), nil
}
