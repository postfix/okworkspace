// tools_test.go holds the LOAD-BEARING structural write-boundary gate (D5 /
// AGNT-11). TestToolSetIsExactlyReadOnlyAllowList asserts that the set of tool
// names registered by readTools EQUALS exactly the five read-only tools and
// nothing else. It is the build gate for the agent's read-only boundary:
//
//	*** DO NOT relax this test to "expect" a new tool. ***
//
// If a sixth tool name ever appears (especially a write/apply/commit/shell
// tool), this test MUST fail — that failure is the whole point. A write path is
// a non-tool HTTP endpoint, never an Eino tool, so the allow-list never grows.
//
// The test runs OFFLINE with NO API key: it only inspects the registered tool
// name set and never calls the model or any backing service (nil Deps).
package agent

import (
	"reflect"
	"sort"
	"testing"
)

func TestToolSetIsExactlyReadOnlyAllowList(t *testing.T) {
	// The canonical, never-growing read-only allow-list.
	want := map[string]bool{
		"list_tree":            true,
		"read_page":            true,
		"search_pages":         true,
		"search_attachments":   true,
		"read_attachment_text": true,
	}

	// nil Deps: construction must not touch any service (key-free, offline).
	tools, names, err := readTools(Deps{})
	if err != nil {
		t.Fatalf("readTools returned an unexpected error: %v", err)
	}

	// The tool slice and the name slice must be the same length — they are
	// derived in the same function and must never drift.
	if len(tools) != len(names) {
		t.Fatalf("tool slice (%d) and name slice (%d) lengths drifted", len(tools), len(names))
	}

	// The names reported by readTools must match what each tool actually
	// registered with Eino (Info().Name) — guards against the parallel name
	// list lying about the real tool surface.
	for i, tl := range tools {
		info, ierr := tl.Info(t.Context())
		if ierr != nil {
			t.Fatalf("tool %d Info() error: %v", i, ierr)
		}
		if info.Name != names[i] {
			t.Fatalf("tool %d registered name %q but name list says %q", i, info.Name, names[i])
		}
	}

	got := map[string]bool{}
	for _, n := range names {
		got[n] = true
	}

	if !reflect.DeepEqual(want, got) {
		// Any extra/missing tool fails the build — the structural gate.
		gotNames := make([]string, 0, len(got))
		for n := range got {
			gotNames = append(gotNames, n)
		}
		sort.Strings(gotNames)
		t.Fatalf("tool set drifted from the read-only allow-list.\n  want: list_tree, read_page, search_pages, search_attachments, read_attachment_text\n  got:  %v\n(if you added a write/apply tool, REMOVE it — apply is a non-tool HTTP endpoint, not an Eino tool)", gotNames)
	}
}

// TestReadToolNamesMatchesConstant pins the package-level readToolNames constant
// to the same five names, so a future edit that changes one place but not the
// other is caught here too.
func TestReadToolNamesMatchesConstant(t *testing.T) {
	want := map[string]bool{
		"list_tree":            true,
		"read_page":            true,
		"search_pages":         true,
		"search_attachments":   true,
		"read_attachment_text": true,
	}
	got := map[string]bool{}
	for _, n := range readToolNames {
		got[n] = true
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("readToolNames drifted from the read-only allow-list: %v", readToolNames)
	}
}
