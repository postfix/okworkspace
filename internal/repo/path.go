// Package repo is the single filesystem chokepoint for OKF Workspace wiki
// content (SEC-01, SPEC §21.1). Every read/write of a repo-relative path MUST
// route through Repo.Resolve; no other package constructs absolute repo paths.
//
// Resolve enforces, in order: reject empty / NUL-byte input; reject absolute
// paths and any ".." segment AFTER lexical cleaning; join to the canonical
// root; EvalSymlinks the result and assert it is within the canonical root
// (boundary-prefix, not raw string prefix); and finally open through os.Root
// as OS-enforced defense-in-depth (Go 1.24+). Lexical cleaning ALONE is
// insufficient — a symlink inside the tree can still escape, so the evaluated
// real path is the authority (research/PITFALLS.md Pitfall 5).
package repo

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ErrUnsafePath is returned when a relative path is rejected by Resolve. It is
// the typed sentinel callers (and handlers) match on to return a 400.
var ErrUnsafePath = errors.New("repo: unsafe path")

// Repo is rooted at the configured storage.repo_dir. root is the canonical
// (EvalSymlinks-resolved) absolute path; osRoot is the OS-enforced traversal
// guard opened once at construction.
type Repo struct {
	root   string
	osRoot *os.Root
}

// New roots a Repo at dir, creating it if necessary, then canonicalizing it via
// EvalSymlinks so all later boundary checks compare against the real path. It
// also opens an os.Root on the canonical root for OS-enforced escape refusal.
func New(dir string) (*Repo, error) {
	if dir == "" {
		return nil, fmt.Errorf("%w: empty root", ErrUnsafePath)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("repo: abs root %q: %w", dir, err)
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("repo: create root %q: %w", abs, err)
	}
	canon, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("repo: canonicalize root %q: %w", abs, err)
	}
	osRoot, err := os.OpenRoot(canon)
	if err != nil {
		return nil, fmt.Errorf("repo: open os.Root %q: %w", canon, err)
	}
	return &Repo{root: canon, osRoot: osRoot}, nil
}

// Root returns the canonical absolute root path.
func (r *Repo) Root() string { return r.root }

// GitMetaPath returns an absolute path to a file under the repo's .git
// directory (e.g. "index.lock"). Segments are application constants, never
// user input — this is Git metadata, not resolver-gated wiki content — so it
// keeps all path joining confined to the repo package.
func (r *Repo) GitMetaPath(segments ...string) string {
	parts := append([]string{r.root, ".git"}, segments...)
	return filepath.Join(parts...)
}

// Close releases the OS-level traversal guard.
func (r *Repo) Close() error {
	if r.osRoot == nil {
		return nil
	}
	return r.osRoot.Close()
}

// Resolve validates a repo-relative path and returns the canonical absolute
// path inside the root, or ErrUnsafePath. The handler is expected to pass the
// URL-DECODED path; Resolve re-validates lexically and then via EvalSymlinks.
func (r *Repo) Resolve(rel string) (string, error) {
	cleaned, err := r.validateLexical(rel)
	if err != nil {
		return "", err
	}

	abs := filepath.Join(r.root, cleaned)

	// Defense-in-depth: ask the OS to open the path within the root. os.Root
	// refuses traversal/symlink escape at the syscall layer. A non-existent
	// leaf is fine (we may be resolving for a Write); only escape is fatal.
	if err := r.osRootCheck(cleaned); err != nil {
		return "", err
	}

	// Authoritative check: evaluate symlinks on the longest existing prefix and
	// assert the real path stays within the canonical root. EvalSymlinks fails
	// on non-existent paths, so walk up to the nearest existing ancestor.
	evaluated, err := evalExistingPrefix(abs)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnsafePath, err)
	}
	if !withinRoot(r.root, evaluated) {
		return "", fmt.Errorf("%w: %q escapes repo root", ErrUnsafePath, rel)
	}
	return abs, nil
}

// validateLexical rejects empty input, NUL bytes, absolute paths, and any ".."
// segment after filepath.Clean, returning the cleaned, slash-normalized
// relative path.
func (r *Repo) validateLexical(rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("%w: empty path", ErrUnsafePath)
	}
	if strings.ContainsRune(rel, 0x00) {
		return "", fmt.Errorf("%w: NUL byte in path", ErrUnsafePath)
	}
	// Defense-in-depth: the handler is expected to pass an already-decoded
	// path, but if a percent-encoded sequence reaches the resolver, decode it
	// and re-validate so an encoded "%2e%2e%2f" can never bypass the lexical
	// traversal scan below. A decoded value containing a NUL is also rejected.
	if strings.Contains(rel, "%") {
		if decoded, err := url.PathUnescape(rel); err == nil && decoded != rel {
			if strings.ContainsRune(decoded, 0x00) {
				return "", fmt.Errorf("%w: NUL byte in encoded path", ErrUnsafePath)
			}
			rel = decoded
		}
	}
	// Normalize Windows-style separators so a "..\\" segment cannot slip past a
	// forward-slash-only scan on any platform.
	norm := strings.ReplaceAll(rel, `\`, "/")
	if filepath.IsAbs(rel) || strings.HasPrefix(norm, "/") {
		return "", fmt.Errorf("%w: absolute path %q", ErrUnsafePath, rel)
	}
	cleaned := filepath.Clean(norm)
	if cleaned == ".." || cleaned == "." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("%w: traversal in %q", ErrUnsafePath, rel)
	}
	for _, seg := range strings.Split(filepath.ToSlash(cleaned), "/") {
		if seg == ".." {
			return "", fmt.Errorf("%w: traversal segment in %q", ErrUnsafePath, rel)
		}
	}
	return cleaned, nil
}

// osRootCheck uses os.Root to confirm the OS will not let cleaned escape the
// root. A missing leaf (ENOENT) is acceptable; an escape or other refusal is
// fatal. We Stat through the Root, which resolves symlinks relative to it and
// refuses any component that would leave the root.
func (r *Repo) osRootCheck(cleaned string) error {
	_, err := r.osRoot.Stat(cleaned)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	// os.Root returns a path error wrapping the escape/refusal; treat anything
	// other than "not exist" as unsafe.
	return fmt.Errorf("%w: %v", ErrUnsafePath, err)
}

// evalExistingPrefix EvalSymlinks the longest existing ancestor of abs and
// re-appends the not-yet-existing tail, so resolution works for paths we are
// about to create (Write) as well as existing ones.
func evalExistingPrefix(abs string) (string, error) {
	tail := ""
	cur := abs
	for {
		if _, err := os.Lstat(cur); err == nil {
			real, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return "", err
			}
			return filepath.Join(real, tail), nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			// Reached the filesystem root without finding an existing ancestor.
			return abs, nil
		}
		tail = filepath.Join(filepath.Base(cur), tail)
		cur = parent
	}
}

// withinRoot reports whether candidate is the root itself or a descendant,
// using a separator-terminated boundary prefix (so "/root2" is not a child of
// "/root").
func withinRoot(root, candidate string) bool {
	if candidate == root {
		return true
	}
	return strings.HasPrefix(candidate, root+string(os.PathSeparator))
}
