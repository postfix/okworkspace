package gitstore_test

import (
	"context"
	"testing"

	"github.com/postfix/okworkspace/internal/gitstore"
)

// TestHeadSHA verifies the drift-bookkeeping accessor: an empty repo (no HEAD)
// returns "" with nil error; after a commit it returns the full HEAD SHA, which
// matches `git rev-parse HEAD` as seen by the test's own git inspection helper.
func TestHeadSHA(t *testing.T) {
	gs, r, root := newGitStore(t)
	ctx := context.Background()
	if err := gs.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// No commit yet => empty string, nil error (mirrors IsEmpty's no-HEAD case).
	sha, err := gs.HeadSHA(ctx)
	if err != nil {
		t.Fatalf("HeadSHA (no commit): %v", err)
	}
	if sha != "" {
		t.Fatalf("HeadSHA before any commit = %q, want empty", sha)
	}

	// Commit a file, then HEAD becomes resolvable.
	if err := r.Write("index.md", []byte("# Home\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := gs.Commit(ctx, gitstore.CommitSpec{
		Paths: []string{"index.md"}, Message: "seed", User: "admin", Action: "seed", Source: "test",
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	sha, err = gs.HeadSHA(ctx)
	if err != nil {
		t.Fatalf("HeadSHA (after commit): %v", err)
	}
	if sha == "" {
		t.Fatal("HeadSHA after a commit returned empty")
	}
	if want := runGit(t, root, "rev-parse", "HEAD"); sha != want {
		t.Fatalf("HeadSHA = %q, want %q (git rev-parse HEAD)", sha, want)
	}
}
