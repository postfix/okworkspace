// propose_test.go holds the D4 propose-patch STRUCTURAL safety tests (AGNT-09).
// These are deterministic and KEY-FREE: they assert the byte-stable round-trip,
// frontmatter-key-set preservation, and the diff-locality (churn) properties that
// the propose→approve→apply gate depends on, using small in-test fixtures — no
// live model, no git/db. A real provider can drift; the structural guarantees here
// cannot (AI-SPEC §5 D4).
//
// The two properties proven:
//   - A clean proposed body round-trips byte-stable through okf.Parse→Emit and
//     keeps the frontmatter key-set + order (validateProposedBody is the gate; a
//     one-line edit that preserves frontmatter passes, a reorder/drop fails).
//   - A one-line-instruction proposal changes only a small number of lines (churn
//     ratio under threshold); an over-eager whole-body reformat exceeds it.
package agent

import (
	"strings"
	"testing"

	"github.com/postfix/okworkspace/internal/okf"
)

// d4Fixture is a frontmatter + table + code-fence page — the kind of content that
// shakes out round-trip rot if the proposal pipeline ever re-serializes the body
// through an AST instead of treating it as opaque text.
const d4Fixture = `---
title: Release Notes
tags: [release, notes]
status: draft
---
# Release Notes

A short intro paragraph.

| Item | Status |
| ---- | ------ |
| API  | done   |
| Docs | todo   |

` + "```" + `go
func main() { println("hi") }
` + "```" + `

The end.
`

// TestProposePatchDiff is the D4 gate: a clean proposed body round-trips byte-
// stable, preserves the frontmatter key-set/order, and a local edit yields a small
// churn — while an over-eager whole-body reformat is caught by the churn threshold.
func TestProposePatchDiff(t *testing.T) {
	t.Run("byte-stable okf round-trip", func(t *testing.T) {
		doc, err := okf.Parse([]byte(d4Fixture))
		if err != nil {
			t.Fatalf("parse fixture: %v", err)
		}
		out, err := doc.Emit()
		if err != nil {
			t.Fatalf("emit fixture: %v", err)
		}
		if string(out) != d4Fixture {
			t.Fatalf("okf round-trip is not byte-stable:\n--- want ---\n%q\n--- got ---\n%q", d4Fixture, string(out))
		}
	})

	t.Run("frontmatter key-set preserved (clean local edit passes validation)", func(t *testing.T) {
		// A one-line body edit that leaves the frontmatter untouched — the exact
		// shape a well-behaved propose returns. validateProposedBody must accept it.
		proposed := strings.Replace(d4Fixture, "A short intro paragraph.", "A revised intro paragraph.", 1)
		if err := validateProposedBody(d4Fixture, proposed); err != nil {
			t.Fatalf("a clean local edit that preserves frontmatter should pass validation, got: %v", err)
		}
		// Confirm the proposal itself still round-trips byte-stable.
		bd, err := okf.Parse([]byte(proposed))
		if err != nil {
			t.Fatalf("parse proposed: %v", err)
		}
		out, err := bd.Emit()
		if err != nil {
			t.Fatalf("emit proposed: %v", err)
		}
		if string(out) != proposed {
			t.Fatalf("proposed body is not byte-stable through okf")
		}
		// And the frontmatter key set/order is identical to the source.
		srcDoc, _ := okf.Parse([]byte(d4Fixture))
		if !sameOrderedKeys(frontmatterKeys(srcDoc), frontmatterKeys(bd)) {
			t.Fatalf("frontmatter key-set/order drifted on a clean edit")
		}
	})

	t.Run("frontmatter reorder/drop is rejected", func(t *testing.T) {
		// Reordered keys (status before tags) — round-trip rot the gate must catch.
		reordered := `---
title: Release Notes
status: draft
tags: [release, notes]
---
# Release Notes

A short intro paragraph.
`
		if err := validateProposedBody(d4Fixture, reordered); err == nil {
			t.Fatal("a frontmatter key REORDER must be rejected by validateProposedBody")
		}
		// Dropped key (no tags).
		dropped := `---
title: Release Notes
status: draft
---
# Release Notes

A short intro paragraph.
`
		if err := validateProposedBody(d4Fixture, dropped); err == nil {
			t.Fatal("a DROPPED frontmatter key must be rejected by validateProposedBody")
		}
	})

	t.Run("diff-locality: a local edit has low churn, a reformat has high churn", func(t *testing.T) {
		// A single-line change: only that line (removed + added) over the whole doc.
		local := strings.Replace(d4Fixture, "A short intro paragraph.", "A revised intro paragraph.", 1)
		localChurn := churnRatio(d4Fixture, local)
		const threshold = 0.25
		if localChurn >= threshold {
			t.Fatalf("a one-line edit should have churn < %.2f, got %.3f", threshold, localChurn)
		}

		// An over-eager whole-body reformat: every line rewritten. Churn must exceed
		// the threshold so the audit/metric can flag it.
		var reformatted strings.Builder
		for _, line := range strings.Split(strings.TrimRight(d4Fixture, "\n"), "\n") {
			reformatted.WriteString(strings.TrimSpace(line) + "   \n") // trailing ws on every line
		}
		reformatChurn := churnRatio(d4Fixture, reformatted.String())
		if reformatChurn <= threshold {
			t.Fatalf("a whole-body reformat should have churn > %.2f, got %.3f", threshold, reformatChurn)
		}
		if reformatChurn <= localChurn {
			t.Fatalf("a whole-body reformat (%.3f) must churn more than a one-line edit (%.3f)", reformatChurn, localChurn)
		}
	})
}
