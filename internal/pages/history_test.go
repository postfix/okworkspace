package pages

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/gitstore"
)

// TestHistoryNoSHA verifies the page-level History returns human-readable
// entries (action/who/when + an opaque version token) and that the exported
// HistoryEntry struct exposes NO Git-named (sha/hash/commit) field (VER-02).
func TestHistoryNoSHA(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	path, err := svc.Create(ctx, "notes", "My Page", "Sam")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitForFile(t, r, path)
	rev1, _ := svc.Revision(ctx, path)

	// Cut a second version.
	if err := svc.Save(ctx, path, "# edited\n", "", rev1, "Sam"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	waitForRevisionChange(t, svc, path, rev1)

	hist, err := svc.History(ctx, path)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("history len = %d, want 2", len(hist))
	}
	// Newest first; actions recovered from the trailer; who = author.
	if hist[0].Action != "edit" {
		t.Fatalf("newest action = %q, want edit", hist[0].Action)
	}
	if hist[len(hist)-1].Action != "create" {
		t.Fatalf("oldest action = %q, want create", hist[len(hist)-1].Action)
	}
	for i, e := range hist {
		if e.Who != "Sam" {
			t.Fatalf("entry %d who = %q, want Sam", i, e.Who)
		}
		if strings.TrimSpace(e.When) == "" {
			t.Fatalf("entry %d when is empty", i)
		}
		if strings.TrimSpace(e.Version) == "" {
			t.Fatalf("entry %d version token is empty", i)
		}
	}

	// Structural assertion: the exported HistoryEntry type must NOT have any
	// Git-named field (sha/hash/commit). The SHA lives only inside Version.
	typ := reflect.TypeOf(HistoryEntry{})
	for i := 0; i < typ.NumField(); i++ {
		name := strings.ToLower(typ.Field(i).Name)
		tag := strings.ToLower(typ.Field(i).Tag.Get("json"))
		for _, banned := range []string{"sha", "hash", "commit"} {
			if strings.Contains(name, banned) || strings.Contains(tag, banned) {
				t.Fatalf("HistoryEntry field %q (tag %q) leaks Git vocabulary %q (VER-02)",
					typ.Field(i).Name, tag, banned)
			}
		}
	}
}

// TestViewVersion reads an old version back by its opaque token and gets the old
// content (not the current content).
func TestViewVersion(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	path, err := svc.Create(ctx, "notes", "Page", "Sam")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitForFile(t, r, path)
	rev1, _ := svc.Revision(ctx, path)

	if err := svc.Save(ctx, path, "# the new body\n", "", rev1, "Sam"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	waitForRevisionChange(t, svc, path, rev1)

	hist, err := svc.History(ctx, path)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	oldest := hist[len(hist)-1]
	page, err := svc.ViewVersion(ctx, path, oldest.Version)
	if err != nil {
		t.Fatalf("ViewVersion: %v", err)
	}
	if strings.Contains(page.Body, "the new body") {
		t.Fatalf("ViewVersion returned the CURRENT body, want the original: %q", page.Body)
	}
}

// TestRestoreForwardCommit proves restore writes the chosen old version as a NEW
// forward commit: HEAD advances (commit count increases), the old commits still
// exist, and the page content equals the old version (VER-03).
func TestRestoreForwardCommit(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()
	root := r.Root()

	path, err := svc.Create(ctx, "notes", "Page", "Sam")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitForFile(t, r, path)
	waitForCommitCount(t, root, 1)
	rev1, _ := svc.Revision(ctx, path)

	// Capture the original body bytes for the equality check.
	orig, err := svc.Get(ctx, path)
	if err != nil {
		t.Fatalf("Get original: %v", err)
	}

	// Cut a second, different version.
	if err := svc.Save(ctx, path, "# completely different\n", "", rev1, "Sam"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	waitForRevisionChange(t, svc, path, rev1)
	countBeforeRestore := waitForCommitCount(t, root, 2)
	if countBeforeRestore != 2 {
		t.Fatalf("commit count before restore = %d, want 2", countBeforeRestore)
	}

	// Find the oldest version token and restore it.
	hist, err := svc.History(ctx, path)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	oldest := hist[len(hist)-1]

	if err := svc.RestoreVersion(ctx, path, oldest.Version, "Sam"); err != nil {
		t.Fatalf("RestoreVersion: %v", err)
	}
	// HEAD advanced: count increased by one (a NEW forward commit, not a reset).
	countAfter := waitForCommitCount(t, root, 3)
	if countAfter != 3 {
		t.Fatalf("commit count after restore = %d, want 3 (forward commit, history preserved)", countAfter)
	}

	// The old commits still exist: the oldest token is still resolvable.
	if _, err := svc.git.ShowAt(ctx, oldest.Version, path); err != nil {
		t.Fatalf("oldest version token no longer resolves after restore (history rewritten?): %v", err)
	}

	// The live page content now equals the old version's body.
	restored, err := svc.Get(ctx, path)
	if err != nil {
		t.Fatalf("Get restored: %v", err)
	}
	if strings.TrimSpace(restored.Body) != strings.TrimSpace(orig.Body) {
		t.Fatalf("restored body = %q, want original %q", restored.Body, orig.Body)
	}
}

// spyReviser records whether ShowAt was reached. A flag-shaped/invalid version
// token must be rejected by the pages layer BEFORE ShowAt is called, so its
// ShowAtCalled flag must stay false for those inputs (defense in depth ahead of
// the gitstore chokepoint).
type spyReviser struct{ ShowAtCalled bool }

func (s *spyReviser) BlobRevision(context.Context, string) (string, error) { return "", nil }
func (s *spyReviser) History(context.Context, string) ([]gitstore.Commit, error) {
	return nil, nil
}
func (s *spyReviser) ShowAt(context.Context, string, string) ([]byte, error) {
	s.ShowAtCalled = true
	return nil, nil
}

// TestVersionTokenValidationRejectsFlagShaped proves ViewVersion and
// RestoreVersion reject a non-hex (flag-shaped) version token with
// ErrInvalidVersion and NEVER reach ShowAt — closing the argv flag-smuggling
// vector at the pages layer too (MEDIUM, defense in depth). A real committed
// token still flows through to ShowAt.
func TestVersionTokenValidationRejectsFlagShaped(t *testing.T) {
	badTokens := []string{
		"--output=/tmp/pwned",
		"-O",
		"..",
		"deadbeefzz",
		"DEADBEEF",
		"abc",
		"",
		"deadbeef/notes",
	}

	t.Run("ViewVersion", func(t *testing.T) {
		for _, tok := range badTokens {
			spy := &spyReviser{}
			svc := &Service{git: spy}
			_, err := svc.ViewVersion(context.Background(), "notes/page.md", tok)
			if !errors.Is(err, ErrInvalidVersion) {
				t.Fatalf("ViewVersion(%q) err = %v, want ErrInvalidVersion", tok, err)
			}
			if spy.ShowAtCalled {
				t.Fatalf("ViewVersion(%q) reached ShowAt — bad token must be rejected first", tok)
			}
		}
	})

	t.Run("RestoreVersion", func(t *testing.T) {
		for _, tok := range badTokens {
			spy := &spyReviser{}
			svc := &Service{git: spy}
			err := svc.RestoreVersion(context.Background(), "notes/page.md", tok, "Sam")
			if !errors.Is(err, ErrInvalidVersion) {
				t.Fatalf("RestoreVersion(%q) err = %v, want ErrInvalidVersion", tok, err)
			}
			if spy.ShowAtCalled {
				t.Fatalf("RestoreVersion(%q) reached ShowAt — bad token must be rejected first", tok)
			}
		}
	})

	// A genuine token must still reach git: ViewVersion with a real committed
	// token flows past the format check into ShowAt.
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()
	path, err := svc.Create(ctx, "notes", "Page", "Sam")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitForFile(t, r, path)
	hist, err := svc.History(ctx, path)
	if err != nil || len(hist) == 0 {
		t.Fatalf("History: %v (len=%d)", err, len(hist))
	}
	if !validVersionToken(hist[0].Version) {
		t.Fatalf("real token %q is not a valid hex token — premise broken", hist[0].Version)
	}
	if _, err := svc.ViewVersion(ctx, path, hist[0].Version); err != nil {
		t.Fatalf("ViewVersion with a real token failed: %v", err)
	}
}

// TestPushFlagReachesPayload proves config.Git.PushOnCommit reaches the
// commitPayload.Push on EVERY mutation path (Create, Save, RestoreVersion, and —
// via the shared enqueue path — rename/move/trash). With pushOnCommit=true every
// payload has Push=true; with false every payload has Push=false. This is the
// assertion that VER-04 is config-driven, not dead.
func TestPushFlagReachesPayload(t *testing.T) {
	for _, push := range []bool{true, false} {
		r, gs, _ := newTestRepoAndGit(t)
		fake := &capturingAllWorker{}
		svc := &Service{repo: r, git: gs, worker: fake, pushOnCommit: push, now: func() time.Time {
			return time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
		}}

		// Seed a committed page directly so Save/Restore have a HEAD to read.
		commitFileForSvc(t, gs, r, "notes/page.md", "# seed\n", "Sam")

		mutations := []struct {
			name string
			run  func() error
		}{
			{"Create", func() error { _, err := svc.Create(context.Background(), "", "T", "Sam"); return err }},
			{"Save", func() error {
				rev, _ := svc.Revision(context.Background(), "notes/page.md")
				return svc.Save(context.Background(), "notes/page.md", "# edit\n", "", rev, "Sam")
			}},
			{"CreateFolder", func() error { return svc.CreateFolder(context.Background(), "", "arch", "Sam") }},
			{"RestoreVersion", func() error {
				hist, err := gs.History(context.Background(), "notes/page.md")
				if err != nil || len(hist) == 0 {
					t.Fatalf("history for restore: %v", err)
				}
				return svc.RestoreVersion(context.Background(), "notes/page.md", hist[0].Token, "Sam")
			}},
		}
		for _, m := range mutations {
			fake.payloads = nil
			if err := m.run(); err != nil {
				t.Fatalf("%s (push=%v): %v", m.name, push, err)
			}
			if len(fake.payloads) == 0 {
				t.Fatalf("%s (push=%v): no payload enqueued", m.name, push)
			}
			for _, raw := range fake.payloads {
				var p commitPayload
				if err := json.Unmarshal([]byte(raw), &p); err != nil {
					t.Fatalf("%s: unmarshal payload: %v", m.name, err)
				}
				if p.Push != push {
					t.Fatalf("%s (push=%v): payload Push = %v, want %v", m.name, push, p.Push, push)
				}
			}
		}
	}
}

// TestCommitJobPushBranch verifies the CommitJob calls g.Push exactly when the
// payload Push flag is set AND a remote is configured, and never when Push=false.
func TestCommitJobPushBranch(t *testing.T) {
	// Push=true + remote configured → the commit lands in the bare remote.
	remoteDir := t.TempDir()
	runGitCmd(t, remoteDir, "init", "--bare", "-b", "main")

	r, gsPush := newRepoWithRemote(t, remoteDir)
	h := CommitHandler(r, gsPush)
	payload := commitPayload{
		Writes: []fileWrite{{Path: "a.md", Bytes: []byte("# a\n")}},
		Spec: gitstore.CommitSpec{
			Paths: []string{"a.md"}, Message: "create a", User: "Sam",
			Action: "create", Source: "web-ui",
		},
		Push: true,
	}
	raw := mustMarshal(t, payload)
	if err := h(context.Background(), raw); err != nil {
		t.Fatalf("handler (push=true): %v", err)
	}
	// The remote must have received the commit (proves g.Push ran).
	if got := remoteCommitCount(t, remoteDir); got != 1 {
		t.Fatalf("remote commit count = %d, want 1 (push branch did not run)", got)
	}

	// Push=false → the remote must stay empty even though it is configured.
	payload2 := commitPayload{
		Writes: []fileWrite{{Path: "b.md", Bytes: []byte("# b\n")}},
		Spec: gitstore.CommitSpec{
			Paths: []string{"b.md"}, Message: "create b", User: "Sam",
			Action: "create", Source: "web-ui",
		},
		Push: false,
	}
	if err := h(context.Background(), mustMarshal(t, payload2)); err != nil {
		t.Fatalf("handler (push=false): %v", err)
	}
	if got := remoteCommitCount(t, remoteDir); got != 1 {
		t.Fatalf("remote commit count = %d after a Push=false commit, want still 1 (g.Push must NOT run)", got)
	}
}
