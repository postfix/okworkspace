package users_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/users"
)

func newSeedFixture(t *testing.T) (*gitstore.GitStore, *repo.Repo, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	root := t.TempDir()
	r, err := repo.New(root)
	if err != nil {
		t.Fatalf("repo.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	gs := gitstore.New(r, config.GitConfig{Enabled: true, Branch: "main"})
	if err := gs.Init(context.Background()); err != nil {
		t.Fatalf("gitstore.Init: %v", err)
	}
	return gs, r, r.Root()
}

// okfFrontmatter holds the SPEC §10 required fields for validation.
type okfFrontmatter struct {
	Type        string   `yaml:"type"`
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
	Timestamp   string   `yaml:"timestamp"`
}

// parseFrontmatter extracts and parses the YAML frontmatter block, asserting all
// SPEC §10 required fields are present and non-empty.
func parseFrontmatter(t *testing.T, content []byte) {
	t.Helper()
	s := string(content)
	if !strings.HasPrefix(s, "---\n") {
		t.Fatalf("content does not start with a frontmatter fence:\n%s", s)
	}
	rest := s[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		t.Fatalf("no closing frontmatter fence:\n%s", s)
	}
	var fm okfFrontmatter
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		t.Fatalf("frontmatter is not valid YAML: %v", err)
	}
	if fm.Type == "" {
		t.Errorf("required field 'type' missing")
	}
	if fm.Title == "" {
		t.Errorf("required field 'title' missing")
	}
	if fm.Description == "" {
		t.Errorf("required field 'description' missing")
	}
	if fm.Tags == nil {
		t.Errorf("required field 'tags' missing")
	}
	if fm.Timestamp == "" {
		t.Errorf("required field 'timestamp' missing")
	}
}

func TestSeedStarterRepoOnEmpty(t *testing.T) {
	gs, r, root := newSeedFixture(t)
	ctx := context.Background()

	seeded, err := users.SeedStarterRepo(ctx, gs, r, "admin")
	if err != nil {
		t.Fatalf("SeedStarterRepo: %v", err)
	}
	if !seeded {
		t.Fatal("SeedStarterRepo on empty repo returned seeded=false, want true")
	}

	// Exactly one commit.
	if got := commitCount(t, root); got != 1 {
		t.Fatalf("commit count = %d, want exactly 1", got)
	}

	// All starter index pages exist and carry valid OKF frontmatter.
	for _, rel := range []string{
		"index.md",
		"runbooks/index.md",
		"architecture/index.md",
		"decisions/index.md",
	} {
		data, err := r.Read(rel)
		if err != nil {
			t.Fatalf("read seeded %q: %v", rel, err)
		}
		parseFrontmatter(t, data)
	}

	// The .okf-workspace scaffold exists.
	if ok, err := r.Exists(".okf-workspace"); err != nil || !ok {
		t.Fatalf(".okf-workspace scaffold missing (ok=%v err=%v)", ok, err)
	}

	// The seed commit is authored with the admin identity.
	author := commitAuthor(t, root)
	if !strings.Contains(author, "admin") {
		t.Fatalf("seed commit author %q does not reflect admin identity", author)
	}
}

func TestSeedStarterRepoNoSeedOnNonEmpty(t *testing.T) {
	gs, r, root := newSeedFixture(t)
	ctx := context.Background()

	// Pre-populate the repo with existing content and commit it (simulating a
	// pulled, non-empty repo).
	if err := r.Write("existing.md", []byte("# Existing\n")); err != nil {
		t.Fatalf("write existing: %v", err)
	}
	if err := gs.Commit(ctx, gitstore.CommitSpec{
		Paths: []string{"existing.md"}, Message: "pre-existing", User: "someone", Action: "import", Source: "test",
	}); err != nil {
		t.Fatalf("commit pre-existing: %v", err)
	}
	before := commitCount(t, root)

	seeded, err := users.SeedStarterRepo(ctx, gs, r, "admin")
	if err != nil {
		t.Fatalf("SeedStarterRepo: %v", err)
	}
	if seeded {
		t.Fatal("SeedStarterRepo on non-empty repo returned seeded=true, want false")
	}
	if after := commitCount(t, root); after != before {
		t.Fatalf("commit count changed from %d to %d on a non-empty repo", before, after)
	}
}

func commitCount(t *testing.T, root string) int {
	t.Helper()
	cmd := exec.Command("git", "rev-list", "--count", "HEAD")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0 // no commits yet
	}
	n := 0
	for _, c := range strings.TrimSpace(string(out)) {
		n = n*10 + int(c-'0')
	}
	return n
}

func commitAuthor(t *testing.T, root string) string {
	t.Helper()
	cmd := exec.Command("git", "show", "-s", "--format=%an", "HEAD")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git show author: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}
