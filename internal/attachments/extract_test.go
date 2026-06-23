package attachments

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture reads a testdata/attachments/ file (created in 02-01) for the extractor
// fidelity tests. The fixtures are the spike's repeatable deliverable (RESEARCH
// Fidelity test plan).
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "attachments", name))
	if err != nil {
		t.Fatalf("read fixture %q: %v", name, err)
	}
	return data
}

// TestExtractPDFTextLayer: a text-layer PDF extracts a non-empty string containing
// the known sentinel (the happy path → "Text extracted").
func TestExtractPDFTextLayer(t *testing.T) {
	got, err := Extract("pdf", fixture(t, "text-layer.pdf"))
	if err != nil {
		t.Fatalf("Extract(pdf text-layer) err = %v, want nil", err)
	}
	if got == "" {
		t.Fatalf("text-layer PDF extracted empty, want sentinel text")
	}
	if !strings.Contains(got, "OKFTEXTLAYER") {
		t.Fatalf("text-layer PDF missing sentinel; got %q", got)
	}
}

// TestExtractScannedPDFEmpty is the ATT-08 empty guarantee: a scanned/image-only
// PDF (no text layer) extracts an EMPTY string with NO error — the
// empty-but-succeeded case ("No text extracted"), NOT a failure.
func TestExtractScannedPDFEmpty(t *testing.T) {
	got, err := Extract("pdf", fixture(t, "scanned-image.pdf"))
	if err != nil {
		t.Fatalf("scanned PDF err = %v, want nil (empty is SUCCESS, not failure)", err)
	}
	if got != "" {
		t.Fatalf("scanned PDF extracted %q, want empty string", got)
	}
}

// TestExtractDOCX: a DOCX extracts both paragraph text and table cell text.
func TestExtractDOCX(t *testing.T) {
	got, err := Extract("docx", fixture(t, "sample.docx"))
	if err != nil {
		t.Fatalf("Extract(docx) err = %v, want nil", err)
	}
	if !strings.Contains(got, "OKF sample document paragraph sentinel") {
		t.Fatalf("docx missing paragraph text; got %q", got)
	}
	if !strings.Contains(got, "CellSentinel42") {
		t.Fatalf("docx missing table cell text; got %q", got)
	}
}

// TestExtractTXT: a TXT with a UTF-8 BOM and CRLF endings is BOM-stripped,
// CRLF→LF normalized, TrimSpace'd, and contains the sentinel.
func TestExtractTXT(t *testing.T) {
	got, err := Extract("txt", fixture(t, "sample.txt"))
	if err != nil {
		t.Fatalf("Extract(txt) err = %v, want nil", err)
	}
	if strings.HasPrefix(got, "\ufeff") {
		t.Fatalf("txt retained a UTF-8 BOM; got %q", got)
	}
	if strings.Contains(got, "\r") {
		t.Fatalf("txt retained CR (CRLF not normalized); got %q", got)
	}
	if got != strings.TrimSpace(got) {
		t.Fatalf("txt not TrimSpace'd; got %q", got)
	}
	if !strings.Contains(got, "TXTSENTINEL") {
		t.Fatalf("txt missing sentinel; got %q", got)
	}
}

// TestExtractCorruptErrors: a corrupt PDF returns a non-nil error (drives
// "Couldn't extract text" after the worker's retries).
func TestExtractCorruptErrors(t *testing.T) {
	got, err := Extract("pdf", fixture(t, "corrupt.pdf"))
	if err == nil {
		t.Fatalf("corrupt PDF err = nil, want a non-nil parse error")
	}
	if got != "" {
		t.Fatalf("corrupt PDF returned text %q, want empty on error", got)
	}
}

// TestExtractPanicRecovered proves the Pitfall-5 guarantee deterministically: a
// parser that panics on an adversarial file is RECOVERED by guardExtract and
// returned as an error, never propagated to crash the single drain goroutine.
// It exercises the guard chokepoint directly (every extractor flows through it),
// so the guarantee holds regardless of whether any particular fixture happens to
// panic on a given parser version.
func TestExtractPanicRecovered(t *testing.T) {
	got, err := guardExtract("pdf", func() (string, error) {
		panic("adversarial file blew up the parser")
	})
	if err == nil {
		t.Fatalf("a panicking extractor returned err = nil, want a recovered error")
	}
	if got != "" {
		t.Fatalf("a panicking extractor returned text %q, want empty", got)
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Fatalf("recovered error %q does not mention the panic", err.Error())
	}
}

// TestExtractNonExtractableType: an unknown/non-extractable extension (e.g. png)
// returns ("", nil) so the caller writes no sidecar and shows no chip.
func TestExtractNonExtractableType(t *testing.T) {
	got, err := Extract("png", fixture(t, "pixel.png"))
	if err != nil {
		t.Fatalf("Extract(png) err = %v, want nil", err)
	}
	if got != "" {
		t.Fatalf("non-extractable type returned %q, want empty", got)
	}
}
