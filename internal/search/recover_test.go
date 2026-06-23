package search

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestOpenOrRecover_FreshDir: a missing dir is created fresh, not flagged as a
// recovery (the normal first-run path).
func TestOpenOrRecover_FreshDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "index")
	idx, recovered, err := OpenOrRecover(dir)
	if err != nil {
		t.Fatalf("OpenOrRecover(fresh): %v", err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	if recovered {
		t.Fatal("recovered=true for a fresh dir; want false")
	}
	if _, err := idx.DocCount(); err != nil {
		t.Fatalf("fresh index not usable: %v", err)
	}
}

// TestOpenOrRecover_CorruptIndex is the WR-02 regression: a corrupt existing
// index must NOT error (which would take the server down). OpenOrRecover wipes it,
// recreates a fresh empty index, and reports recovered=true so the caller can
// enqueue a rebuild.
func TestOpenOrRecover_CorruptIndex(t *testing.T) {
	if _, err := exec.LookPath("git"); err == nil {
		_ = err // git not required here; kept for harness-parity readers
	}
	dir := filepath.Join(t.TempDir(), "index")

	// Create a valid index, then corrupt it on disk so bleve.Open fails with a
	// non-"does-not-exist" error.
	idx, _, err := OpenOrRecover(dir)
	if err != nil {
		t.Fatalf("OpenOrRecover(initial): %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("close initial: %v", err)
	}
	corruptIndexDir(t, dir)

	idx2, recovered, err := OpenOrRecover(dir)
	if err != nil {
		t.Fatalf("OpenOrRecover(corrupt) must not error, got: %v", err)
	}
	t.Cleanup(func() { _ = idx2.Close() })
	if !recovered {
		t.Fatal("recovered=false for a corrupt index; want true so a rebuild is enqueued")
	}
	// The recovered index must be a usable, empty index.
	n, err := idx2.DocCount()
	if err != nil {
		t.Fatalf("recovered index not usable: %v", err)
	}
	if n != 0 {
		t.Fatalf("recovered index doc count = %d, want 0 (fresh empty)", n)
	}
}

// corruptIndexDir overwrites the scorch store files with garbage so bleve.Open
// fails with a corruption error (not ErrorIndexPathDoesNotExist).
func corruptIndexDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read index dir: %v", err)
	}
	corrupted := false
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if err := os.WriteFile(p, []byte("not a valid bleve file"), 0o644); err != nil {
			t.Fatalf("corrupt %q: %v", p, err)
		}
		corrupted = true
	}
	if !corrupted {
		// No top-level files (store may nest); clobber the whole dir with a single
		// junk file in place of the expected metadata so Open fails.
		if err := os.WriteFile(filepath.Join(dir, "index_meta.json"), []byte("garbage"), 0o644); err != nil {
			t.Fatalf("write junk meta: %v", err)
		}
	}
}
