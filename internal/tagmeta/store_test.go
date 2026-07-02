package tagmeta

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/postfix/okworkspace/internal/config"
	"github.com/postfix/okworkspace/internal/gitstore"
	"github.com/postfix/okworkspace/internal/jobs"
	"github.com/postfix/okworkspace/internal/pages"
	"github.com/postfix/okworkspace/internal/repo"
	"github.com/postfix/okworkspace/internal/store"
)

// testHarness wires a real tagmeta.Store over a temp content repo + migrated
// SQLite store + a real jobs.Worker with the KindCommit handler (mirrors the
// graph package's harness convention). The commit worker is the SAME
// single-writer path page saves use — SetMeta commits through it.
type testHarness struct {
	st   *Store
	db   *store.Store
	repo *repo.Repo
	gs   *gitstore.GitStore
}

func newHarness(t *testing.T) *testHarness {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	base := t.TempDir()

	r, err := repo.New(filepath.Join(base, "repo"))
	if err != nil {
		t.Fatalf("repo.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	st, err := store.Open(filepath.Join(base, "app.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	gs := gitstore.New(r, config.GitConfig{Enabled: true, Branch: "main"})
	if err := gs.Init(context.Background()); err != nil {
		t.Fatalf("gitstore.Init: %v", err)
	}

	w := jobs.New(st.DB(), jobs.Config{PollInterval: 5 * time.Millisecond})
	w.Register(pages.KindCommit, pages.CommitHandler(r, gs))
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	t.Cleanup(func() { w.Stop(); cancel() })

	tm := NewStore(r, st.DB(), w, false)
	return &testHarness{st: tm, db: st, repo: r, gs: gs}
}

// snapshotMeta returns all tag_meta rows sorted by tag for byte-stable
// equality assertions across rebuilds.
func (h *testHarness) snapshotMeta(t *testing.T) []Meta {
	t.Helper()
	rows, err := h.db.DB().Query(`SELECT tag, color, description, parent FROM tag_meta ORDER BY tag`)
	if err != nil {
		t.Fatalf("snapshot tag_meta: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Meta
	for rows.Next() {
		var m Meta
		if err := rows.Scan(&m.Tag, &m.Color, &m.Description, &m.Parent); err != nil {
			t.Fatalf("scan tag_meta: %v", err)
		}
		out = append(out, m)
	}
	return out
}

func TestSetGetMeta(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if err := h.st.SetMeta(ctx, "deploy", "#4caf50", "Deploy ops", ""); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	m, ok := h.st.GetMeta(ctx, "deploy")
	if !ok {
		t.Fatal("GetMeta: tag not found after set")
	}
	if m.Color != "#4caf50" {
		t.Errorf("color = %q, want #4caf50", m.Color)
	}
	if m.Description != "Deploy ops" {
		t.Errorf("description = %q, want Deploy ops", m.Description)
	}
}

func TestRebuildTagMeta(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// Seed two Tags/*.md files via SetMeta (which writes + commits + caches).
	if err := h.st.SetMeta(ctx, "deploy", "#4caf50", "Deploy ops", ""); err != nil {
		t.Fatalf("SetMeta deploy: %v", err)
	}
	if err := h.st.SetMeta(ctx, "ops", "red", "Operations", ""); err != nil {
		t.Fatalf("SetMeta ops: %v", err)
	}

	if err := h.st.RebuildTagMeta(ctx); err != nil {
		t.Fatalf("RebuildTagMeta: %v", err)
	}
	got := h.snapshotMeta(t)
	want := []Meta{
		{Tag: "deploy", Color: "#4caf50", Description: "Deploy ops"},
		{Tag: "ops", Color: "red", Description: "Operations"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("after rebuild: got %v, want %v", got, want)
	}
}

func TestRebuildParity(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if err := h.st.SetMeta(ctx, "deploy", "#4caf50", "Deploy ops", ""); err != nil {
		t.Fatalf("SetMeta deploy: %v", err)
	}
	if err := h.st.SetMeta(ctx, "ops", "blue", "", ""); err != nil {
		t.Fatalf("SetMeta ops: %v", err)
	}
	if err := h.st.RebuildTagMeta(ctx); err != nil {
		t.Fatalf("RebuildTagMeta first: %v", err)
	}
	before := h.snapshotMeta(t)

	// Nuke the cache and rebuild from files — must reproduce identical rows.
	if _, err := h.db.DB().Exec(`DELETE FROM tag_meta`); err != nil {
		t.Fatalf("delete tag_meta: %v", err)
	}
	if err := h.st.RebuildTagMeta(ctx); err != nil {
		t.Fatalf("RebuildTagMeta second: %v", err)
	}
	after := h.snapshotMeta(t)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("tag_meta not reproduced: after=%v before=%v", after, before)
	}
}

func TestEmptyStore(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// No Tags/ folder → AllMeta returns empty, GetMeta returns false.
	all, err := h.st.AllMeta(ctx)
	if err != nil {
		t.Fatalf("AllMeta on empty store: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("AllMeta on empty store = %v, want empty", all)
	}
	_, ok := h.st.GetMeta(ctx, "nope")
	if ok {
		t.Error("GetMeta on empty store returned true, want false")
	}

	// Rebuild on an empty store is a no-op (graceful — not an error).
	if err := h.st.RebuildTagMeta(ctx); err != nil {
		t.Fatalf("RebuildTagMeta on empty store: %v", err)
	}
	if got := h.snapshotMeta(t); len(got) != 0 {
		t.Errorf("snapshot after rebuild on empty store = %v, want empty", got)
	}
}

func TestValidateColor(t *testing.T) {
	for _, tc := range []struct {
		color string
		want  bool
	}{
		{"#4caf50", true},
		{"#fff", true},
		{"#FFF", true},
		{"#AbCdEf", true},
		{"red", true},
		{"blue", true},
		{"green", true},
		{"amber", true},
		{"purple", true},
		{"gray", true},
		{"", true}, // empty = clear
		{"url(x)", false},
		{"var(--x)", false},
		{"<script>", false},
		{"#1234", false},   // wrong length
		{"#12345", false}, // wrong length
		{"#gggggg", false},
		{"red;", false},
		{"orange", false}, // not in palette
	} {
		err := ValidateColor(tc.color)
		if tc.want && err != nil {
			t.Errorf("ValidateColor(%q): got error %v, want nil", tc.color, err)
		}
		if !tc.want && err == nil {
			t.Errorf("ValidateColor(%q): got nil, want error", tc.color)
		}
	}
}

func TestValidateDescription(t *testing.T) {
	valid := []string{"", "Deploy ops", "A short description"}
	for _, desc := range valid {
		if err := ValidateDescription(desc); err != nil {
			t.Errorf("ValidateDescription(%q): got error %v, want nil", desc, err)
		}
	}
	invalid := []struct {
		desc string
		msg  string
	}{
		{"line1\nline2", "newline"},
		{"line1\rline2", "CR"},
		{"<script>alert(1)</script>", "angle brackets"},
		{"a&b", "ampersand"},
		{`a"b`, "double quote"},
		{`a'b`, "single quote"},
	}
	for _, tc := range invalid {
		if err := ValidateDescription(tc.desc); err == nil {
			t.Errorf("ValidateDescription(%q) [%s]: got nil, want error", tc.desc, tc.msg)
		}
	}
	// Exactly 200 chars is valid; 201 is not.
	var sb strings.Builder
	for range 200 {
		sb.WriteByte('a')
	}
	if err := ValidateDescription(sb.String()); err != nil {
		t.Errorf("ValidateDescription(200 chars): got error %v, want nil", err)
	}
	sb.WriteByte('a')
	if err := ValidateDescription(sb.String()); err == nil {
		t.Error("ValidateDescription(201 chars): got nil, want error")
	}
}

func TestSetMetaWritesFile(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if err := h.st.SetMeta(ctx, "deploy", "#4caf50", "Deploy ops", ""); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	// The file Tags/deploy.md must exist on disk with the correct frontmatter.
	raw, err := h.repo.Read("Tags/deploy.md")
	if err != nil {
		t.Fatalf("Read Tags/deploy.md: %v", err)
	}
	m, err := parseMetaFile(raw)
	if err != nil {
		t.Fatalf("parseMetaFile: %v", err)
	}
	if m.Tag != "deploy" {
		t.Errorf("tag = %q, want deploy", m.Tag)
	}
	if m.Color != "#4caf50" {
		t.Errorf("color = %q, want #4caf50", m.Color)
	}
	if m.Description != "Deploy ops" {
		t.Errorf("description = %q, want Deploy ops", m.Description)
	}
}

func TestClearMetaDeletesFile(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// Set metadata, then clear it (empty color + description → file deleted).
	if err := h.st.SetMeta(ctx, "deploy", "#4caf50", "Deploy ops", ""); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	if err := h.st.ClearMeta(ctx, "deploy"); err != nil {
		t.Fatalf("ClearMeta: %v", err)
	}
	// The file should be gone.
	exists, err := h.repo.Exists("Tags/deploy.md")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Error("Tags/deploy.md still exists after ClearMeta, want deleted")
	}
	// Cache row should be gone too.
	_, ok := h.st.GetMeta(ctx, "deploy")
	if ok {
		t.Error("GetMeta returned true after ClearMeta, want false")
	}
}

func TestRebuildSortsByTag(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// Seed in non-sorted order.
	if err := h.st.SetMeta(ctx, "zeta", "red", "", ""); err != nil {
		t.Fatalf("SetMeta zeta: %v", err)
	}
	if err := h.st.SetMeta(ctx, "alpha", "blue", "", ""); err != nil {
		t.Fatalf("SetMeta alpha: %v", err)
	}
	if err := h.st.RebuildTagMeta(ctx); err != nil {
		t.Fatalf("RebuildTagMeta: %v", err)
	}
	got := h.snapshotMeta(t)
	tags := make([]string, len(got))
	for i, m := range got {
		tags[i] = m.Tag
	}
	if !sort.StringsAreSorted(tags) {
		t.Errorf("rebuild did not sort by tag: %v", tags)
	}
}

// --- Tag hierarchy tests (Phase 33 / TF-03) ---

// TestSetMetaWithParent: set parent on "deploy" → "ops"; GetMeta returns
// parent="ops"; Tags/deploy.md frontmatter has parent: ops.
func TestSetMetaWithParent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// Seed the parent tag so the store has it.
	if err := h.st.SetMeta(ctx, "ops", "red", "Operations", ""); err != nil {
		t.Fatalf("SetMeta ops: %v", err)
	}
	if err := h.st.SetMeta(ctx, "deploy", "#4caf50", "Deploy ops", "ops"); err != nil {
		t.Fatalf("SetMeta deploy: %v", err)
	}
	m, ok := h.st.GetMeta(ctx, "deploy")
	if !ok {
		t.Fatal("GetMeta: deploy not found")
	}
	if m.Parent != "ops" {
		t.Errorf("parent = %q, want ops", m.Parent)
	}
	// Tags/deploy.md frontmatter must contain parent: ops.
	raw, err := h.repo.Read("Tags/deploy.md")
	if err != nil {
		t.Fatalf("Read Tags/deploy.md: %v", err)
	}
	if !strings.Contains(string(raw), "parent: ops") {
		t.Errorf("Tags/deploy.md missing parent frontmatter: %s", raw)
	}
}

// TestCycleDetection_SelfParent: set parent="deploy" on "deploy" → rejected
// with cycle error.
func TestCycleDetection_SelfParent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if err := h.st.SetMeta(ctx, "deploy", "#4caf50", "Deploy ops", ""); err != nil {
		t.Fatalf("SetMeta deploy: %v", err)
	}
	err := h.st.SetMeta(ctx, "deploy", "#4caf50", "Deploy ops", "deploy")
	if err == nil {
		t.Fatal("self-parent should be rejected with cycle error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error should mention cycle, got: %v", err)
	}
}

// TestCycleDetection_Indirect: build a chain ops→infra→deploy, then try to
// set parent="deploy" on "ops" → rejected (cycle).
func TestCycleDetection_Indirect(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// Build chain: deploy's parent = infra, infra's parent = ops.
	if err := h.st.SetMeta(ctx, "ops", "red", "Operations", ""); err != nil {
		t.Fatalf("SetMeta ops: %v", err)
	}
	if err := h.st.SetMeta(ctx, "infra", "blue", "Infrastructure", "ops"); err != nil {
		t.Fatalf("SetMeta infra: %v", err)
	}
	if err := h.st.SetMeta(ctx, "deploy", "#4caf50", "Deploy ops", "infra"); err != nil {
		t.Fatalf("SetMeta deploy: %v", err)
	}

	// Try to close the loop: set deploy as parent of ops → cycle.
	err := h.st.SetMeta(ctx, "ops", "red", "Operations", "deploy")
	if err == nil {
		t.Fatal("indirect cycle should be rejected")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error should mention cycle, got: %v", err)
	}
}

// TestCycleDetection_DeepChain: build a 50-tag chain, set parent at the end →
// valid (no cycle); try to close the loop → rejected.
func TestCycleDetection_DeepChain(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// Build a 50-tag chain: tag0 is root, tag1's parent is tag0, ..., tag49's
	// parent is tag48.
	for i := range 50 {
		parent := ""
		if i > 0 {
			parent = fmt.Sprintf("tag%d", i-1)
		}
		if err := h.st.SetMeta(ctx, fmt.Sprintf("tag%d", i), "red", "", parent); err != nil {
			t.Fatalf("SetMeta tag%d: %v", i, err)
		}
	}

	// Set tag50's parent to tag49 → valid (no cycle, depth 51).
	if err := h.st.SetMeta(ctx, "tag50", "blue", "", "tag49"); err != nil {
		t.Fatalf("SetMeta tag50 (deep chain): %v", err)
	}

	// Try to close the loop: set tag0's parent to tag50 → cycle.
	err := h.st.SetMeta(ctx, "tag0", "red", "", "tag50")
	if err == nil {
		t.Fatal("deep chain cycle should be rejected")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error should mention cycle, got: %v", err)
	}
}

// TestNonExistentParent: set parent="ghost" (doesn't exist) → accepted (treated
// as root per D-02b); GetMeta returns parent="ghost" (the value is stored; the
// UI treats non-existent as root).
func TestNonExistentParent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if err := h.st.SetMeta(ctx, "deploy", "#4caf50", "Deploy ops", "ghost"); err != nil {
		t.Fatalf("SetMeta with non-existent parent: %v", err)
	}
	m, ok := h.st.GetMeta(ctx, "deploy")
	if !ok {
		t.Fatal("GetMeta: deploy not found")
	}
	if m.Parent != "ghost" {
		t.Errorf("parent = %q, want ghost (stored as-is)", m.Parent)
	}
}

// TestRebuildParityWithParent: seed Tags/*.md with parent fields, rebuild,
// verify cache; delete cache, rebuild, verify identical.
func TestRebuildParityWithParent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// Seed with parent fields forming a hierarchy.
	if err := h.st.SetMeta(ctx, "ops", "red", "Operations", ""); err != nil {
		t.Fatalf("SetMeta ops: %v", err)
	}
	if err := h.st.SetMeta(ctx, "infra", "blue", "Infrastructure", "ops"); err != nil {
		t.Fatalf("SetMeta infra: %v", err)
	}
	if err := h.st.SetMeta(ctx, "deploy", "#4caf50", "Deploy ops", "infra"); err != nil {
		t.Fatalf("SetMeta deploy: %v", err)
	}

	if err := h.st.RebuildTagMeta(ctx); err != nil {
		t.Fatalf("RebuildTagMeta first: %v", err)
	}
	before := h.snapshotMeta(t)

	// Nuke the cache and rebuild from files — must reproduce identical rows.
	if _, err := h.db.DB().Exec(`DELETE FROM tag_meta`); err != nil {
		t.Fatalf("delete tag_meta: %v", err)
	}
	if err := h.st.RebuildTagMeta(ctx); err != nil {
		t.Fatalf("RebuildTagMeta second: %v", err)
	}
	after := h.snapshotMeta(t)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("tag_meta with parent not reproduced: after=%v before=%v", after, before)
	}

	// Verify the parent values are correct.
	want := []Meta{
		{Tag: "deploy", Color: "#4caf50", Description: "Deploy ops", Parent: "infra"},
		{Tag: "infra", Color: "blue", Description: "Infrastructure", Parent: "ops"},
		{Tag: "ops", Color: "red", Description: "Operations", Parent: ""},
	}
	if !reflect.DeepEqual(after, want) {
		t.Fatalf("rebuild with parent: got %v, want %v", after, want)
	}
}