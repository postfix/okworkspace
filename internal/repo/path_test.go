package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestRepo creates a Repo rooted at a fresh temp dir. The root is resolved
// through EvalSymlinks so the canonical-root comparison in tests is stable on
// platforms (e.g. macOS) where /tmp is itself a symlink.
func newTestRepo(t *testing.T) (*Repo, string) {
	t.Helper()
	dir := t.TempDir()
	r, err := New(dir)
	if err != nil {
		t.Fatalf("New(%q): %v", dir, err)
	}
	canonRoot, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", dir, err)
	}
	return r, canonRoot
}

func TestResolveValidPathInsideRoot(t *testing.T) {
	r, canonRoot := newTestRepo(t)

	abs, err := r.Resolve("runbooks/deploy.md")
	if err != nil {
		t.Fatalf("Resolve valid path: unexpected error %v", err)
	}
	if !filepath.IsAbs(abs) {
		t.Fatalf("Resolve returned non-absolute path %q", abs)
	}
	// The resolved path must be strictly inside the canonical root.
	if abs != canonRoot && !strings.HasPrefix(abs, canonRoot+string(os.PathSeparator)) {
		t.Fatalf("resolved path %q escaped root %q", abs, canonRoot)
	}
}

func TestReadWriteRoundTrip(t *testing.T) {
	r, _ := newTestRepo(t)

	const rel = "runbooks/deploy.md"
	want := []byte("# Deploy\n")
	if err := r.Write(rel, want); err != nil {
		t.Fatalf("Write(%q): %v", rel, err)
	}
	got, err := r.Read(rel)
	if err != nil {
		t.Fatalf("Read(%q): %v", rel, err)
	}
	if string(got) != string(want) {
		t.Fatalf("round-trip mismatch: got %q want %q", got, want)
	}
}

func TestResolveRejectsMaliciousInputs(t *testing.T) {
	r, _ := newTestRepo(t)

	cases := map[string]string{
		"empty":                 "",
		"parent traversal":      "../etc/passwd",
		"nested traversal":      "runbooks/../../etc/passwd",
		"leading slash absolute": "/etc/passwd",
		"url-encoded traversal":  "%2e%2e%2f%2e%2e%2fetc%2fpasswd",
		"windows backslash":      `..\windows\system32`,
		"nul byte":               "runbooks/deploy\x00.md",
		"bare dotdot":            "..",
		"dotdot segment":         "a/../../b",
		"mixed-case encoded":     "%2E%2E/secret",
		"trailing dot segment":   "runbooks/../..",
	}

	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := r.Resolve(in); err == nil {
				t.Fatalf("Resolve(%q) = nil error, want rejection", in)
			}
		})
	}
}

func TestResolveRejectsSymlinkEscape(t *testing.T) {
	r, canonRoot := newTestRepo(t)

	// Create a target OUTSIDE the repo root.
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("top secret"), 0o600); err != nil {
		t.Fatalf("write outside target: %v", err)
	}

	// Create a symlink INSIDE the repo that points to the outside target.
	link := filepath.Join(canonRoot, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}

	// Resolving a path THROUGH the symlink must be rejected because the
	// evaluated real path is outside the canonical root.
	if _, err := r.Resolve("escape/secret.txt"); err == nil {
		t.Fatalf("Resolve through symlink escape returned nil error, want rejection")
	}
}

// FuzzResolve asserts the core invariant: for ANY input, either Resolve errors
// OR the returned absolute path is strictly within the canonical repo root
// (after EvalSymlinks). No input may ever yield an escaping path.
func FuzzResolve(f *testing.F) {
	seeds := []string{
		"runbooks/deploy.md",
		"",
		"../etc/passwd",
		"/etc/passwd",
		"%2e%2e%2f",
		`..\windows`,
		"a/../../b",
		"runbooks/deploy\x00.md",
		"..",
		".",
		"a/b/c/../../../../d",
		"%2E%2E/secret",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	dir := f.TempDir()
	r, err := New(dir)
	if err != nil {
		f.Fatalf("New(%q): %v", dir, err)
	}
	canonRoot, err := filepath.EvalSymlinks(dir)
	if err != nil {
		f.Fatalf("EvalSymlinks(%q): %v", dir, err)
	}

	f.Fuzz(func(t *testing.T, in string) {
		abs, err := r.Resolve(in)
		if err != nil {
			return // rejection is always acceptable
		}
		if !filepath.IsAbs(abs) {
			t.Fatalf("Resolve(%q) returned non-absolute path %q with nil error", in, abs)
		}
		if abs != canonRoot && !strings.HasPrefix(abs, canonRoot+string(os.PathSeparator)) {
			t.Fatalf("Resolve(%q) = %q escaped canonical root %q", in, abs, canonRoot)
		}
	})
}
