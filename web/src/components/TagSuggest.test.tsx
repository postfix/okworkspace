/**
 * TagSuggest — the per-page tag-suggestion TRUST SURFACE under test (TAG-01/TAG-02).
 * These assertions are the gate against a future refactor silently weakening the
 * locked interaction contract:
 *
 *   1. new (invented) tags carry the "new" badge AND default UNCHECKED; existing
 *      tags default checked. The "{n} selected" count is live.
 *   2. only the CHECKED tags + the captured base_revision are sent to applyTags —
 *      never an unchecked tag, never the full list.
 *   3. Apply is NOT the initially-focused element (Cancel is); Esc and a backdrop
 *      click invoke Cancel and call applyTags ZERO times.
 *   4. a 409 from applyTags switches to the stale state (Apply removed; Re-run
 *      offered) — no clobbering retry.
 *   5. loading / empty / suggest-error / apply-error render the exact quiet copy.
 *   6. the trigger is editor-gated (no trigger for a reader).
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return {
    ...actual,
    me: vi.fn(),
    suggestTags: vi.fn(),
    applyTags: vi.fn(),
  };
});

import * as client from "../api/client";
import TagSuggest from "./TagSuggest";

function renderSuggest(
  role = "editor",
  opts: {
    body?: string;
    frontmatter?: string;
    onApplied?: (appliedTags: string[]) => void;
  } = {},
) {
  vi.mocked(client.me).mockResolvedValue({
    username: "u",
    display_name: "U",
    role,
    must_change_password: false,
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  // Pre-seed the cached ["me"] query so the role is available synchronously for
  // the editor gate (mirrors App.tsx seeding the session).
  qc.setQueryData(["me"], {
    username: "u",
    display_name: "U",
    role,
    must_change_password: false,
  });
  return render(
    <QueryClientProvider client={qc}>
      <TagSuggest
        pagePath="notes/a.md"
        body={opts.body ?? ""}
        frontmatter={opts.frontmatter ?? ""}
        onApplied={opts.onApplied}
      />
    </QueryClientProvider>,
  );
}

const SUGGESTIONS = {
  suggestions: [
    { tag: "release", existing: true },
    { tag: "q3-launch", existing: false },
  ],
  base_revision: "rev-1",
};

async function openSurface(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: /suggest tags/i }));
  await screen.findByRole("dialog", { name: /suggested tags/i });
}

describe("TagSuggest trust surface", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("does not render the trigger for a reader (editor-gated)", () => {
    renderSuggest("reader");
    expect(screen.queryByRole("button", { name: /suggest tags/i })).toBeNull();
  });

  it("renders new tags with the 'new' badge AND unchecked; existing checked; live count", async () => {
    vi.mocked(client.suggestTags).mockResolvedValue(SUGGESTIONS);
    const user = userEvent.setup();
    renderSuggest();
    await openSurface(user);

    const releaseRow = screen.getByText("release").closest("label")!;
    const newRow = screen.getByText("q3-launch").closest("label")!;

    // The invented tag carries the "new" badge; the existing tag does not.
    expect(within(newRow).getByText("new")).toBeTruthy();
    expect(within(releaseRow).queryByText("new")).toBeNull();

    // New defaults UNCHECKED; existing defaults checked.
    const releaseBox = within(releaseRow).getByRole("checkbox") as HTMLInputElement;
    const newBox = within(newRow).getByRole("checkbox") as HTMLInputElement;
    expect(releaseBox.checked).toBe(true);
    expect(newBox.checked).toBe(false);

    // The count reflects the initially-checked set and updates live.
    expect(screen.getByText("1 selected")).toBeTruthy();
    await user.click(newBox);
    expect(screen.getByText("2 selected")).toBeTruthy();
  });

  it("applies EXACTLY the checked tags + the captured base_revision", async () => {
    vi.mocked(client.suggestTags).mockResolvedValue(SUGGESTIONS);
    vi.mocked(client.applyTags).mockResolvedValue(undefined);
    const user = userEvent.setup();
    renderSuggest();
    await openSurface(user);

    // Default state: release checked, q3-launch unchecked. Apply now → only release.
    await user.click(screen.getByRole("button", { name: /apply tags/i }));

    await waitFor(() => expect(client.applyTags).toHaveBeenCalledTimes(1));
    expect(client.applyTags).toHaveBeenCalledWith({
      page_path: "notes/a.md",
      tags: ["release"],
      base_revision: "rev-1",
    });
  });

  it("does NOT auto-focus Apply — Cancel is the initial focus", async () => {
    vi.mocked(client.suggestTags).mockResolvedValue(SUGGESTIONS);
    const user = userEvent.setup();
    renderSuggest();
    await openSurface(user);

    const apply = screen.getByRole("button", { name: /apply tags/i });
    const cancel = screen.getByRole("button", { name: /^cancel$/i });
    expect(document.activeElement).toBe(cancel);
    expect(document.activeElement).not.toBe(apply);
  });

  it("Esc cancels and calls applyTags ZERO times (nothing written until Apply)", async () => {
    vi.mocked(client.suggestTags).mockResolvedValue(SUGGESTIONS);
    vi.mocked(client.applyTags).mockResolvedValue(undefined);
    const user = userEvent.setup();
    renderSuggest();
    await openSurface(user);

    await user.keyboard("{Escape}");
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: /suggested tags/i })).toBeNull(),
    );
    expect(client.applyTags).not.toHaveBeenCalled();
  });

  it("a 409 switches to the stale state (Apply removed; Re-run offered) — no clobber retry", async () => {
    vi.mocked(client.suggestTags).mockResolvedValue(SUGGESTIONS);
    const stale = new Error("This page changed since the tags were suggested.") as Error & {
      status?: number;
    };
    stale.status = 409;
    vi.mocked(client.applyTags).mockRejectedValue(stale);
    const user = userEvent.setup();
    renderSuggest();
    await openSurface(user);

    await user.click(screen.getByRole("button", { name: /apply tags/i }));

    // The stale state removes the Apply path and surfaces a warning + Re-run/Close.
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: /apply tags/i })).toBeNull(),
    );
    expect(client.applyTags).toHaveBeenCalledTimes(1); // no clobbering retry
    const alert = screen.getByRole("alert");
    expect(
      within(alert).getByText(/changed since the tags were suggested/i),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: /re-run/i })).toBeTruthy();
    expect(screen.getByRole("button", { name: /close/i })).toBeTruthy();
  });

  it("Re-run re-fires suggestTags from the stale state", async () => {
    vi.mocked(client.suggestTags).mockResolvedValue(SUGGESTIONS);
    const stale = new Error("stale") as Error & { status?: number };
    stale.status = 409;
    vi.mocked(client.applyTags).mockRejectedValue(stale);
    const user = userEvent.setup();
    renderSuggest();
    await openSurface(user);
    expect(client.suggestTags).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole("button", { name: /apply tags/i }));
    await screen.findByRole("button", { name: /re-run/i });

    await user.click(screen.getByRole("button", { name: /re-run/i }));
    await waitFor(() => expect(client.suggestTags).toHaveBeenCalledTimes(2));
  });

  it("shows the empty state when the model returns no tags", async () => {
    vi.mocked(client.suggestTags).mockResolvedValue({
      suggestions: [],
      base_revision: "rev-1",
    });
    const user = userEvent.setup();
    renderSuggest();
    await openSurface(user);
    expect(screen.getByText("No tag suggestions for this page.")).toBeTruthy();
    // No Apply path for an empty set.
    expect(screen.queryByRole("button", { name: /apply tags/i })).toBeNull();
  });

  it("shows the in-flight loading label on the trigger while suggesting", async () => {
    let resolve!: (v: typeof SUGGESTIONS) => void;
    vi.mocked(client.suggestTags).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }),
    );
    const user = userEvent.setup();
    renderSuggest();
    await user.click(screen.getByRole("button", { name: /suggest tags/i }));
    expect(await screen.findByText("Suggesting tags…")).toBeTruthy();
    resolve(SUGGESTIONS);
  });

  it("shows the suggest-error line when suggestTags rejects", async () => {
    vi.mocked(client.suggestTags).mockRejectedValue(new Error("boom"));
    const user = userEvent.setup();
    renderSuggest();
    await user.click(screen.getByRole("button", { name: /suggest tags/i }));
    expect(
      await screen.findByText("Couldn’t suggest tags. Try again."),
    ).toBeTruthy();
  });

  it("shows the apply-error line on a non-stale apply failure", async () => {
    vi.mocked(client.suggestTags).mockResolvedValue(SUGGESTIONS);
    vi.mocked(client.applyTags).mockRejectedValue(new Error("boom")); // no status → not stale
    const user = userEvent.setup();
    renderSuggest();
    await openSurface(user);

    await user.click(screen.getByRole("button", { name: /apply tags/i }));
    expect(
      await screen.findByText("Couldn’t apply the tags. Try again."),
    ).toBeTruthy();
    // The surface stays in the review state (Apply still present), not stale.
    expect(screen.getByRole("button", { name: /apply tags/i })).toBeTruthy();
  });
});

// Phase 23 (ED-03a) — the union de-dup, the onApplied handoff, and the server-as-
// authority pin. These are the criterion-3/criterion-4/reconcile-handoff gates.
describe("TagSuggest — union de-dup + onApplied (Phase 23)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("filters suggestions already on the page (union over LIVE body inline #tags + frontmatter tags, case-insensitive)", async () => {
    vi.mocked(client.suggestTags).mockResolvedValue({
      // The model re-offers an inline tag (different case), a frontmatter tag, a
      // nested inline tag, and one genuinely fresh tag.
      suggestions: [
        { tag: "Inline", existing: true }, // == body #inline (case-insensitive)
        { tag: "fmtag", existing: true }, // == frontmatter tag
        { tag: "a/b", existing: true }, // == body #a/b (nested)
        { tag: "fresh", existing: true }, // the only NEW tag
      ],
      base_revision: "rev-1",
    });
    const user = userEvent.setup();
    renderSuggest("editor", {
      body: "see #inline and #a/b here",
      frontmatter: "tags:\n  - fmtag\n",
    });
    await openSurface(user);

    // Only the not-on-page tag is offered; the inline, nested, and frontmatter tags
    // are filtered out BEFORE the surface opens.
    expect(screen.getByText("fresh")).toBeTruthy();
    expect(screen.queryByText("Inline")).toBeNull();
    expect(screen.queryByText("fmtag")).toBeNull();
    expect(screen.queryByText("a/b")).toBeNull();
  });

  it("approving calls onApplied with EXACTLY the approved (checked) tags", async () => {
    vi.mocked(client.suggestTags).mockResolvedValue({
      suggestions: [
        { tag: "fmtag", existing: true }, // filtered (on page)
        { tag: "fresh", existing: true }, // remains, checked by default
      ],
      base_revision: "rev-1",
    });
    vi.mocked(client.applyTags).mockResolvedValue(undefined);
    const onApplied = vi.fn();
    const user = userEvent.setup();
    renderSuggest("editor", { frontmatter: "tags:\n  - fmtag\n", onApplied });
    await openSurface(user);

    await user.click(screen.getByRole("button", { name: /apply tags/i }));

    // The CHECKED set goes to the server AND to onApplied (the editor reconciles).
    await waitFor(() =>
      expect(client.applyTags).toHaveBeenCalledWith({
        page_path: "notes/a.md",
        tags: ["fresh"],
        base_revision: "rev-1",
      }),
    );
    await waitFor(() => expect(onApplied).toHaveBeenCalledWith(["fresh"]));
  });

  it("shows the quiet no-new-tags state when every suggestion is already on the page (never writes)", async () => {
    vi.mocked(client.suggestTags).mockResolvedValue({
      suggestions: [
        { tag: "inline", existing: true },
        { tag: "fmtag", existing: true },
      ],
      base_revision: "rev-1",
    });
    vi.mocked(client.applyTags).mockResolvedValue(undefined);
    const onApplied = vi.fn();
    const user = userEvent.setup();
    renderSuggest("editor", {
      body: "#inline",
      frontmatter: "tags:\n  - fmtag\n",
      onApplied,
    });
    await openSurface(user);

    expect(screen.getByText(/no new tags to suggest/i)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /apply tags/i })).toBeNull();
    expect(client.applyTags).not.toHaveBeenCalled();
    expect(onApplied).not.toHaveBeenCalled();
  });

  // Criterion 4 (D-05): the client never trusts its own list — a server rejection of
  // an approved tag (422 from re-validation / 400 from charset) surfaces as the apply-
  // error line, the surface is NOT dismissed, and onApplied is NEVER called (nothing
  // is reconciled into the editor on a rejected write).
  it("surfaces a server rejection (422/400) of an approved tag and does NOT call onApplied", async () => {
    vi.mocked(client.suggestTags).mockResolvedValue({
      suggestions: [{ tag: "fresh", existing: true }],
      base_revision: "rev-1",
    });
    const rejected = new Error("There are no valid tags to apply.") as Error & {
      status?: number;
    };
    rejected.status = 422;
    vi.mocked(client.applyTags).mockRejectedValue(rejected);
    const onApplied = vi.fn();
    const user = userEvent.setup();
    renderSuggest("editor", { onApplied });
    await openSurface(user);

    await user.click(screen.getByRole("button", { name: /apply tags/i }));

    expect(
      await screen.findByText("Couldn’t apply the tags. Try again."),
    ).toBeTruthy();
    // Not the stale (409) path — the review surface stays, Apply still present.
    expect(screen.getByRole("button", { name: /apply tags/i })).toBeTruthy();
    expect(onApplied).not.toHaveBeenCalled();
  });
});
