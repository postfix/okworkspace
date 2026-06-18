package okf_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/okf"
)

var fixedNow = time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)

func TestRepairAddsOnlyMissingFields(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "repair", "missing-fields.md"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	doc, err := okf.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	okf.Repair(doc, fixedNow)

	if !doc.FrontDirty {
		t.Fatal("FrontDirty = false, want true (fields were missing)")
	}
	out, err := doc.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	emitted := string(out)

	// The two missing fields are now present.
	for _, want := range []string{"description:", "tags:"} {
		if !strings.Contains(emitted, want) {
			t.Fatalf("repaired output missing %q:\n%s", want, emitted)
		}
	}
	// Existing keys/values and the comment are preserved.
	for _, want := range []string{"# keep this comment", "type: Reference", "title: Partially Filled", "custom: keepme", "timestamp: 2026-01-01T00:00:00Z"} {
		if !strings.Contains(emitted, want) {
			t.Fatalf("repaired output dropped existing content %q:\n%s", want, emitted)
		}
	}
	// The opaque body is untouched byte-for-byte.
	if !bytes.Contains(out, []byte("# Body stays byte-identical\n\ncontent\n")) {
		t.Fatalf("body was altered during repair:\n%s", emitted)
	}
	// type was already present, so the default "Page" must NOT have been applied.
	if strings.Contains(emitted, "type: Page") {
		t.Fatalf("repair overwrote existing type with default:\n%s", emitted)
	}
}

func TestRepairCompleteIsByteIdentical(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "repair", "complete.md"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	doc, err := okf.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	okf.Repair(doc, fixedNow)

	if doc.FrontDirty {
		t.Fatal("FrontDirty = true, want false (all required fields already present)")
	}
	out, err := doc.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if !bytes.Equal(src, out) {
		t.Fatalf("complete page not byte-identical after repair\nin:  %q\nout: %q", src, out)
	}
}

func TestRepairDefaultsAndTimestamp(t *testing.T) {
	// A body-only file gets a fresh frontmatter region with all five fields.
	doc, err := okf.Parse([]byte("# Just a body\n\nno frontmatter here\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	okf.Repair(doc, fixedNow)
	if !doc.FrontDirty {
		t.Fatal("FrontDirty = false, want true (frontmatter was created)")
	}
	out, err := doc.Emit()
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	emitted := string(out)

	if !strings.Contains(emitted, "type: Page") {
		t.Fatalf("default type not applied:\n%s", emitted)
	}
	if !strings.Contains(emitted, fixedNow.Format(time.RFC3339)) {
		t.Fatalf("timestamp %q not present:\n%s", fixedNow.Format(time.RFC3339), emitted)
	}
	// Original body preserved verbatim.
	if !strings.Contains(emitted, "# Just a body\n\nno frontmatter here\n") {
		t.Fatalf("body lost when promoting body-only file:\n%s", emitted)
	}
	// Re-parsing the repaired output yields all five required fields parseable.
	redoc, err := okf.Parse(out)
	if err != nil {
		t.Fatalf("re-parse repaired output: %v", err)
	}
	if !redoc.HasFrontmatter {
		t.Fatal("repaired body-only file did not gain a frontmatter region")
	}
}
