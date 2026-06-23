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
import { render, screen, within, fireEvent } from "@testing-library/react";

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

/**
 * DiffReviewDialog — conflict mode (Phase 5, COLL-04). The conflict footer encodes
 * the safety hierarchy under test:
 *
 *   1. Exactly three risk-ranked footer buttons (Overwrite / Manual merge / Save
 *      as copy) with the verbatim UI-SPEC accessible names.
 *   2. Initial focus is NEVER on Overwrite (the data-losing action) — a reflexive
 *      Enter cannot discard the server's change.
 *   3. The REAL diff is always rendered (old = their version, new = mine).
 *   4. old === new shows the identical-versions message + a single safe Save —
 *      never a fabricated diff.
 *   5. Esc cancels (calls the cancel handler) and applies NONE of overwrite/
 *      merge/copy.
 *
 * Review-mode assertions above stay green — conflict is an additive branch.
 */
const SERVER = "# Plan\n\nTheir saved version.\n";
const MINE = "# Plan\n\nMy unsaved version with more detail.\n";

describe("DiffReviewDialog conflict mode trust contract", () => {
  it("renders exactly three risk-ranked footer buttons with the verbatim names", () => {
    render(
      <DiffReviewDialog
        open
        mode="conflict"
        title="This page was changed somewhere else"
        oldText={SERVER}
        newText={MINE}
        summary="Someone saved a different version while you were editing."
        columnCaption="Left: the saved version · Right: your unsaved version"
        onReject={() => {}}
        onOverwrite={() => {}}
        onManualMerge={() => {}}
        onSaveAsCopy={() => {}}
      />,
    );

    expect(
      screen.getByRole("button", {
        name: /overwrite with my version, replacing their changes/i,
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: /merge manually/i,
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: /save my version as a new page/i,
      }),
    ).toBeTruthy();

    // The review-mode Approve/Reject must NOT be present in conflict mode.
    expect(screen.queryByRole("button", { name: /approve & save/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /^reject$/i })).toBeNull();

    // The risk is stated in WORDS (color is never the sole signal).
    expect(
      screen.getByText(/this replaces the other person.s changes/i),
    ).toBeTruthy();
  });

  it("does NOT auto-focus Overwrite — a safe choice holds initial focus", () => {
    render(
      <DiffReviewDialog
        open
        mode="conflict"
        title="This page was changed somewhere else"
        oldText={SERVER}
        newText={MINE}
        onReject={() => {}}
        onOverwrite={() => {}}
        onManualMerge={() => {}}
        onSaveAsCopy={() => {}}
      />,
    );

    const overwrite = screen.getByRole("button", {
      name: /overwrite with my version/i,
    });
    const saveAsCopy = screen.getByRole("button", {
      name: /save my version as a new page/i,
    });

    // The deliberate inversion: a reflexive Enter on open must NOT discard the
    // server's change. Focus lands on the SAFE control, never Overwrite.
    expect(document.activeElement).not.toBe(overwrite);
    expect(document.activeElement).toBe(saveAsCopy);
  });

  it("always renders a REAL diff (old = server, new = mine) — never prose-only", () => {
    render(
      <DiffReviewDialog
        open
        mode="conflict"
        title="This page was changed somewhere else"
        oldText={SERVER}
        newText={MINE}
        onReject={() => {}}
        onOverwrite={() => {}}
        onManualMerge={() => {}}
        onSaveAsCopy={() => {}}
      />,
    );

    const region = screen.getByLabelText("Proposed changes");
    expect(region.querySelector("table")).toBeTruthy();
  });

  it("shows the identical-versions message + a single Save when old === new", () => {
    const onOverwrite = vi.fn();
    render(
      <DiffReviewDialog
        open
        mode="conflict"
        title="This page was changed somewhere else"
        oldText={MINE}
        newText={MINE}
        onReject={() => {}}
        onOverwrite={onOverwrite}
        onManualMerge={() => {}}
        onSaveAsCopy={() => {}}
      />,
    );

    expect(
      screen.getByText(/these versions are identical/i),
    ).toBeTruthy();
    // No fabricated diff for identical versions.
    const region = screen.getByLabelText("Proposed changes");
    expect(region.querySelector("table")).toBeNull();
    // A single safe Save (the 3-button risk footer is NOT shown).
    expect(screen.queryByRole("button", { name: /overwrite/i })).toBeNull();
    expect(
      screen.queryByRole("button", { name: /save as copy/i }),
    ).toBeNull();
    const save = screen.getByRole("button", { name: /^save$/i });
    expect(save).toBeTruthy();
  });

  it("Esc cancels and applies NONE of overwrite/merge/copy", () => {
    const onReject = vi.fn();
    const onOverwrite = vi.fn();
    const onManualMerge = vi.fn();
    const onSaveAsCopy = vi.fn();
    render(
      <DiffReviewDialog
        open
        mode="conflict"
        title="This page was changed somewhere else"
        oldText={SERVER}
        newText={MINE}
        onReject={onReject}
        onOverwrite={onOverwrite}
        onManualMerge={onManualMerge}
        onSaveAsCopy={onSaveAsCopy}
      />,
    );

    fireEvent.keyDown(document, { key: "Escape" });

    expect(onReject).toHaveBeenCalledTimes(1);
    expect(onOverwrite).not.toHaveBeenCalled();
    expect(onManualMerge).not.toHaveBeenCalled();
    expect(onSaveAsCopy).not.toHaveBeenCalled();
  });
});
