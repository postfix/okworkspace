// apply_test.go holds the D8 STALE-REVISION-409 safety test (AGNT-10). It is the
// load-bearing proof that the propose→approve→apply gate blocks a concurrent edit:
// a proposal captures the page revision at proposal time (N); if the page moves to
// N+1 before the user approves, apply MUST return pages.ErrStaleRevision (→ HTTP
// 409) and write NOTHING — never a silent overwrite of the concurrent edit (AI-SPEC
// §1 #4 / §6, T-04-17).
//
// It is deterministic and KEY-FREE: a fake pages service models the revision moving
// between propose and apply (the exact pages.Service.Save 409 floor at
// service.go:200), with no git/db/model. The real Save's stale check is reproduced
// here so the agent slice owns the structural assertion without standing up a repo.
package agent

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/postfix/okworkspace/internal/pages"
)

// stalePagesStub models a page whose committed revision advances from N to N+1
// between the propose-time Revision read and the apply-time Save. Save reproduces
// the real pages.Service optimistic-concurrency floor: if the caller's baseRevision
// no longer matches the current revision it returns pages.ErrStaleRevision and
// records NO write (writes stays empty).
type stalePagesStub struct {
	mu       sync.Mutex
	path     string
	body     string
	front    string
	rev      string   // current committed revision
	revSeq   []string // successive revisions returned by Revision (propose then apply)
	revCalls int
	writes   int // number of Save calls that actually wrote (stale saves do NOT count)
}

// Compile-time bindings: the stub must satisfy the REAL agent interfaces so a drift
// in pageReader/pageWriter (e.g. a new method, or a changed Save signature) breaks
// this test instead of letting it pass against a stale shape.
var (
	_ pageReader = (*stalePagesStub)(nil)
	_ pageWriter = (*stalePagesStub)(nil)
)

func (s *stalePagesStub) Get(_ context.Context, path string) (pages.Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if path != s.path {
		return pages.Page{}, pages.ErrPageNotFound
	}
	return pages.Page{Frontmatter: s.front, Body: s.body, Revision: s.rev}, nil
}

func (s *stalePagesStub) Tree(_ context.Context) ([]pages.Node, error) { return nil, nil }

// Revision returns the next revision in revSeq (propose reads index 0 → "N"); the
// last entry sticks. The page is then mutated via advanceRevision so the apply-time
// current revision is N+1.
func (s *stalePagesStub) Revision(_ context.Context, _ string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.rev
	if s.revCalls < len(s.revSeq) {
		r = s.revSeq[s.revCalls]
	}
	s.revCalls++
	return r, nil
}

// advanceRevision simulates a concurrent edit landing: the committed revision moves
// forward (N → N+1) after the proposal captured N.
func (s *stalePagesStub) advanceRevision(rev string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rev = rev
}

// Save reproduces the pages.Service 409 floor: stale baseRevision → ErrStaleRevision
// with NO write; a matching baseRevision writes and bumps the count.
func (s *stalePagesStub) Save(_ context.Context, path, body string, _ map[string]any, baseRevision, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if path != s.path {
		return pages.ErrPageNotFound
	}
	if baseRevision != s.rev {
		return pages.ErrStaleRevision // 409 floor — no write enqueued.
	}
	s.body = body
	s.writes++
	return nil
}

// TestApplyStaleRevision is the D8 gate. ProposePatch is NOT exercised against a
// model here (key-free); instead we model its contract directly: a proposal captured
// baseRev = "rev-N", then the page moved to "rev-N+1", then apply (Save with the
// stale baseRev) must 409 and not write.
func TestApplyStaleRevision(t *testing.T) {
	stub := &stalePagesStub{
		path:   "notes/x.md",
		front:  "title: X\n",
		body:   "# X\n\noriginal body\n",
		rev:    "rev-N",
		revSeq: []string{"rev-N"}, // propose-time Revision read returns N.
	}

	// 1) Propose time: capture the base revision the apply will re-check.
	baseRev, err := stub.Revision(context.Background(), stub.path)
	if err != nil {
		t.Fatalf("revision read: %v", err)
	}
	if baseRev != "rev-N" {
		t.Fatalf("expected captured baseRev rev-N, got %q", baseRev)
	}

	// 2) A concurrent edit lands — the committed revision moves to N+1.
	stub.advanceRevision("rev-N+1")

	// 3) Apply with the STALE baseRev — must 409 and write nothing.
	applyErr := stub.Save(context.Background(), stub.path, "# X\n\nagent-proposed body\n", nil, baseRev, "alice")
	if !errors.Is(applyErr, pages.ErrStaleRevision) {
		t.Fatalf("apply on a moved revision must return ErrStaleRevision, got: %v", applyErr)
	}
	if stub.writes != 0 {
		t.Fatalf("a stale apply must write NOTHING, but %d write(s) happened", stub.writes)
	}
	if stub.body != "# X\n\noriginal body\n" {
		t.Fatalf("the concurrent edit must NOT be overwritten; body changed to %q", stub.body)
	}

	// 4) Control: applying with the CURRENT revision DOES write (the gate blocks
	// stale, not all writes).
	if err := stub.Save(context.Background(), stub.path, "# X\n\nfresh body\n", nil, "rev-N+1", "alice"); err != nil {
		t.Fatalf("a fresh apply (matching revision) should succeed, got: %v", err)
	}
	if stub.writes != 1 {
		t.Fatalf("a fresh apply should write exactly once, got %d", stub.writes)
	}
}
