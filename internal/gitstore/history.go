package gitstore

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// hexObjectName matches a Git object name: 7–64 lowercase hex characters. Every
// legitimate version token History() emits is built from %H (a full 40-char
// lowercase hex commit hash), so anchoring on hex lets ShowAt reject any token
// that could be parsed by git as a flag (e.g. "--output=...", "-O...") in the
// `<ref>:<path>` argument position — closing the argv flag-smuggling vector
// (MEDIUM) with zero impact on real traffic. Compiled once at package scope.
var hexObjectName = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

// isHexObjectName reports whether ref is a Git object name (7–64 lowercase hex).
// The version token is CLIENT-supplied (it round-trips through the URL/body),
// so callers MUST validate its FORMAT before it reaches git as an object spec.
func isHexObjectName(ref string) bool {
	return hexObjectName.MatchString(ref)
}

// Commit is one entry of a page's version history (VER-02). It carries ONLY
// human-readable provenance — when the version was cut, who cut it, and what
// action it was (recovered from the Action: trailer buildMessage wrote). The
// underlying Git SHA is deliberately NOT exposed to callers in a serialized
// field: callers that need to read an old version use the opaque Token, which is
// the SHA but is never surfaced to the UI (T-05-02). History rows in the UI show
// "Edited by Sam · 2 hours ago" with no Git vocabulary or hashes.
type Commit struct {
	// When is the commit (author) time.
	When time.Time
	// Who is the commit author's display name (the acting user, set on Commit).
	Who string
	// Action is recovered from the "Action:" trailer (e.g. "edit", "create",
	// "rename", "restore-version"). Empty when no trailer was present.
	Action string
	// Token is the commit SHA used ONLY server-side to read an old blob via
	// ShowAt. It is the opaque version token; it must NEVER be serialized to the
	// UI. The pages layer maps it to a non-Git "version" identifier.
	Token string
}

// historyFieldSep is an unlikely byte sequence used to delimit the log fields so
// author names containing spaces or other characters parse unambiguously.
const historyFieldSep = "\x1f" // ASCII unit separator

// historyRecordSep separates whole log records.
const historyRecordSep = "\x1e" // ASCII record separator

// History returns the version history of the page at path, newest-first, using
// `git log --follow` so versions before a rename/move are still returned (A5).
// Each entry carries the action/who/when the UI renders; the SHA is kept in the
// opaque Token and never serialized to the user (VER-02). The path is validated
// through the resolver before it is handed to git so no path bypasses the SEC-01
// chokepoint. An empty history (no commits touch the path) returns an empty
// slice with a nil error.
func (g *GitStore) History(ctx context.Context, path string) ([]Commit, error) {
	if _, err := g.repo.Resolve(path); err != nil {
		return nil, fmt.Errorf("gitstore: unsafe history path %q: %w", path, err)
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// Format: <sha>US<author-name>US<author-ISO-date>US<full-body>RS
	// %H = commit hash (opaque token, never surfaced), %an = author name,
	// %aI = author date (strict ISO-8601), %B = raw body (carries the Action
	// trailer). --follow traces history across renames/moves (A5).
	format := "--format=%H" + historyFieldSep + "%an" + historyFieldSep +
		"%aI" + historyFieldSep + "%B" + historyRecordSep
	out, err := g.git(ctx, "log", "--follow", format, "--", path)
	if err != nil {
		// `git log` on a path with no history (or no HEAD yet) exits non-zero or
		// empty. Treat a missing history as an empty list, not an error, so a
		// freshly created (but not-yet-committed) page renders an empty panel.
		return []Commit{}, nil
	}

	var commits []Commit
	for _, rec := range strings.Split(out, historyRecordSep) {
		rec = strings.Trim(rec, "\n")
		if strings.TrimSpace(rec) == "" {
			continue
		}
		fields := strings.SplitN(rec, historyFieldSep, 4)
		if len(fields) < 4 {
			continue
		}
		sha := strings.TrimSpace(fields[0])
		who := strings.TrimSpace(fields[1])
		dateStr := strings.TrimSpace(fields[2])
		body := fields[3]

		when, _ := time.Parse(time.RFC3339, dateStr)
		commits = append(commits, Commit{
			When:   when,
			Who:    who,
			Action: actionFromTrailer(body),
			Token:  sha,
		})
	}
	if commits == nil {
		commits = []Commit{}
	}
	return commits, nil
}

// actionFromTrailer recovers the Action value from a commit body's
// "Action: <value>" trailer line (written by buildMessage). Returns "" when no
// such trailer is present.
func actionFromTrailer(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "Action:"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// ShowAt returns the bytes of path as they existed at the given ref (the opaque
// version token), via `git show <ref>:<path>`. The ref is CLIENT-supplied — it
// round-trips to the UI and back through the URL/request body — so it is
// validated here as a Git hex object name (7–64 lowercase hex) before it reaches
// git: any non-hex ref (e.g. one beginning with "-") could be parsed by git as a
// flag in the `<ref>:<path>` argument position (argv flag smuggling), so it is
// rejected outright. The path is validated through the resolver, and neither is
// interpolated into a shell (T-05-01). Used by the pages layer to read an old
// version for view/restore.
func (g *GitStore) ShowAt(ctx context.Context, ref, path string) ([]byte, error) {
	if _, err := g.repo.Resolve(path); err != nil {
		return nil, fmt.Errorf("gitstore: unsafe show path %q: %w", path, err)
	}
	if strings.TrimSpace(ref) == "" {
		return nil, fmt.Errorf("gitstore: show requires a version reference")
	}
	if !isHexObjectName(ref) {
		return nil, fmt.Errorf("gitstore: invalid version reference")
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	out, err := g.git(ctx, "show", ref+":"+path)
	if err != nil {
		return nil, fmt.Errorf("gitstore: show %s:%s: %w", ref, path, err)
	}
	return []byte(out), nil
}
