package pages

import (
	"context"
	"encoding/json"
	"testing"
)

// TestTreeEmptyRepoSerializesToArray guards the UAT blocker: an empty repo (no
// pages at the root) must serialize to a JSON array `[]`, never `null`. A null
// body makes the SPA's nodes.map crash with a white screen.
func TestTreeEmptyRepoSerializesToArray(t *testing.T) {
	svc, _, _ := newServiceFixture(t, false)
	ctx := context.Background()

	nodes, err := svc.Tree(ctx)
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	if nodes == nil {
		t.Fatalf("Tree returned a nil slice; it must be non-nil so JSON is [] not null")
	}

	b, err := json.Marshal(nodes)
	if err != nil {
		t.Fatalf("Marshal tree: %v", err)
	}
	if got := string(b); got != "[]" {
		t.Fatalf("empty repo tree serialized to %q, want %q", got, "[]")
	}
}

func TestTree(t *testing.T) {
	svc, r, _ := newServiceFixture(t, false)
	ctx := context.Background()

	// Seed a small repo through the service so commits/files are real.
	idx, err := svc.Create(ctx, "", "Home", "alice")
	if err != nil {
		t.Fatalf("Create Home: %v", err)
	}
	waitForFile(t, r, idx)
	dep, err := svc.Create(ctx, "runbooks", "Deploy Staging", "alice")
	if err != nil {
		t.Fatalf("Create Deploy: %v", err)
	}
	waitForFile(t, r, dep)

	// A .okf-workspace artifact and a .git path must NOT appear in the tree.
	if err := r.Write(".okf-workspace/trash/old.md", []byte("x")); err != nil {
		t.Fatalf("write trash: %v", err)
	}

	nodes, err := svc.Tree(ctx)
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}

	var sawHome, sawRunbooks bool
	var runbooks *Node
	for i := range nodes {
		if nodes[i].Type == "page" && nodes[i].Title == "Home" {
			sawHome = true
		}
		if nodes[i].Type == "folder" && nodes[i].Path == "runbooks" {
			sawRunbooks = true
			runbooks = &nodes[i]
		}
		if nodes[i].Path == ".okf-workspace" || nodes[i].Path == ".git" {
			t.Fatalf("tree leaked a skipped dir: %q", nodes[i].Path)
		}
	}
	if !sawHome {
		t.Fatalf("Home page missing from tree: %+v", nodes)
	}
	if !sawRunbooks || runbooks == nil {
		t.Fatalf("runbooks folder missing from tree: %+v", nodes)
	}
	// The page inside the folder is titled from frontmatter.
	var sawDeploy bool
	for _, c := range runbooks.Children {
		if c.Type == "page" && c.Title == "Deploy Staging" {
			sawDeploy = true
		}
	}
	if !sawDeploy {
		t.Fatalf("Deploy Staging page missing under runbooks: %+v", runbooks.Children)
	}
}
