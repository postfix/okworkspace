package okf_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/postfix/okworkspace/internal/okf"
)

// TestGoldenRoundTrip is the Phase 1 exit gate: for every fixture in
// testdata/corpus, Parse(bytes) followed by Emit() with NO edit must return
// bytes byte-identical to the input. This pins the byte-stability invariant
// that protects users from silent Markdown mangling (RESEARCH Pitfall 1).
func TestGoldenRoundTrip(t *testing.T) {
	dir := filepath.Join("testdata", "corpus")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read corpus dir: %v", err)
	}
	var seen int
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			doc, err := okf.Parse(src)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			out, err := doc.Emit()
			if err != nil {
				t.Fatalf("Emit: %v", err)
			}
			if !bytes.Equal(src, out) {
				t.Fatalf("round-trip not byte-identical for %s\n--- in (%d bytes) ---\n%q\n--- out (%d bytes) ---\n%q",
					name, len(src), src, len(out), out)
			}
		})
		seen++
	}
	if seen == 0 {
		t.Fatal("no corpus fixtures found")
	}
}

// TestCorpusHasCRLFFixture asserts the corpus includes at least one CRLF file,
// guarding the A4 line-ending-preservation requirement at the fixture level.
func TestCorpusHasCRLFFixture(t *testing.T) {
	dir := filepath.Join("testdata", "corpus")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read corpus dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if bytes.Contains(b, []byte("\r\n")) {
			return // found one
		}
	}
	t.Fatal("corpus contains no CRLF fixture (A4 line-ending preservation untested)")
}
