/**
 * DiffReviewDialog — the load-bearing TRUST CONTRACT under test (AGNT-10;
 * AI-SPEC §1 #1/#3, §6; T-04-22). These four assertions are the gate against a
 * future refactor silently weakening the human approval step:
 *
 *   1. The dialog renders a REAL diff for differing old/new — never prose-only.
 *   2. Approve is NOT the initially-focused element (Reject / the diff is).
 *   3. The stale state BLOCKS approve (no Approve button at all).
 *   4. A no-op (old === new) shows "No changes were proposed." + disabled Approve.
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";

import DiffReviewDialog from "./DiffReviewDialog";

const OLD = "# Setup\n\nRun the old deploy steps.\n";
const NEW = "# Setup\n\nRun the new deploy steps with the updated flags.\n";

describe("DiffReviewDialog trust contract", () => {
  it("renders a REAL diff for differing old/new — never prose-only", () => {
    render(
      <DiffReviewDialog
        open
        title="Review this change"
        oldText={OLD}
        newText={NEW}
        summary="Updated the deploy steps."
        onApprove={() => {}}
        onReject={() => {}}
      />,
    );

    // The "No changes" prose path must NOT be present when there is a change.
    expect(screen.queryByText("No changes were proposed.")).toBeNull();

    // The labeled diff region (the scrollable real-diff frame) is present and is
    // a REAL diff: react-diff-viewer-continued renders a <table> grid with the
    // left/right diff columns (structural proof it is the diff COMPONENT, not a
    // prose summary). We assert on structure rather than line text because the
    // diff library virtualizes/folds unchanged lines, so individual line text is
    // not reliably in the DOM under jsdom — but the diff table always is.
    const region = screen.getByLabelText("Proposed changes");
    // react-diff-viewer-continued renders a <table> diff grid — its presence is
    // the structural proof that the diff COMPONENT mounted (the prose `.diff-empty`
    // path renders a <p>, never a <table>). The diff library virtualizes/folds
    // row content (no layout under jsdom), so we assert on the diff table shell
    // (always present) rather than individual line text (not reliably in the DOM).
    expect(region.querySelector("table")).toBeTruthy();
    // The caller's summary is a CAPTION above the diff — never a REPLACEMENT for
    // it. Both the summary AND the diff table are present (prose never stands in
    // for the diff).
    expect(screen.getByText("Updated the deploy steps.")).toBeTruthy();
  });

  it("does NOT auto-focus Approve — Reject is the initial focus", () => {
    render(
      <DiffReviewDialog
        open
        title="Review this change"
        oldText={OLD}
        newText={NEW}
        onApprove={() => {}}
        onReject={() => {}}
      />,
    );

    const approve = screen.getByRole("button", { name: /approve & save/i });
    const reject = screen.getByRole("button", { name: /^reject$/i });

    // The deliberate inversion: the safe action holds initial focus so a
    // reflexive Enter cannot apply a consequential write.
    expect(document.activeElement).toBe(reject);
    expect(document.activeElement).not.toBe(approve);
  });

  it("BLOCKS approve in the stale state (no Approve path)", () => {
    const onApprove = vi.fn();
    render(
      <DiffReviewDialog
        open
        title="Review this change"
        oldText={OLD}
        newText={NEW}
        stale
        onApprove={onApprove}
        onReject={() => {}}
        onRerun={() => {}}
      />,
    );

    // Stale removes the Approve button entirely — there is no path to apply a
    // stale proposal.
    expect(screen.queryByRole("button", { name: /approve & save/i })).toBeNull();
    expect(onApprove).not.toHaveBeenCalled();

    // The blocking condition is surfaced as a warning (not an accent CTA).
    const alert = screen.getByRole("alert");
    expect(
      within(alert).getByText(/changed since the assistant read it/i),
    ).toBeTruthy();
    // Re-run / Close are the only paths offered.
    expect(screen.getByRole("button", { name: /re-run/i })).toBeTruthy();
    expect(screen.getByRole("button", { name: /close/i })).toBeTruthy();
  });

  it("disables Approve for a no-op and shows the no-change message", () => {
    render(
      <DiffReviewDialog
        open
        title="Review this change"
        oldText={OLD}
        newText={OLD}
        onApprove={() => {}}
        onReject={() => {}}
      />,
    );

    expect(screen.getByText("No changes were proposed.")).toBeTruthy();
    // The no-op path renders the message INSIDE the same diff region (the surface
    // is never replaced by prose elsewhere) and renders NO diff table — but it
    // never fabricates a diff either.
    const region = screen.getByLabelText("Proposed changes");
    expect(region.querySelector("table")).toBeNull();
    const approve = screen.getByRole("button", {
      name: /approve & save/i,
    }) as HTMLButtonElement;
    expect(approve.disabled).toBe(true);
  });
});
