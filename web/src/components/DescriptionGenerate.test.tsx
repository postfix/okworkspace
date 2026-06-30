/**
 * DescriptionGenerate — the per-page description TRUST SURFACE under test (ED-01b).
 * These assertions are the gate against a future refactor silently weakening the
 * locked interaction contract:
 *
 *   1. NEVER auto-runs: mounting issues ZERO generate/apply calls; generate fires
 *      only on the explicit click, apply only on the explicit Apply.
 *   2. the generated draft opens in an EDITABLE textarea; editing it then Apply
 *      sends the EDITED text (never a stale closure of the original draft) + the
 *      captured base_revision.
 *   3. Apply is NOT the initially-focused element (Cancel is); Esc invokes Cancel
 *      and calls applyDescription ZERO times.
 *   4. Regenerate re-fetches generateDescription.
 *   5. a 409 from applyDescription switches to the stale state (Apply removed;
 *      Regenerate offered) and does NOT call onApplied — no clobbering retry.
 *   6. the trigger is editor-gated (no trigger for a reader).
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return {
    ...actual,
    me: vi.fn(),
    generateDescription: vi.fn(),
    applyDescription: vi.fn(),
  };
});

import * as client from "../api/client";
import DescriptionGenerate from "./DescriptionGenerate";

function renderGenerate(
  role = "editor",
  opts: { onApplied?: (text: string) => void } = {},
) {
  vi.mocked(client.me).mockResolvedValue({
    username: "u",
    display_name: "U",
    role,
    must_change_password: false,
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  // Pre-seed the cached ["me"] query so the role is available synchronously for the
  // editor gate (mirrors App.tsx seeding the session).
  qc.setQueryData(["me"], {
    username: "u",
    display_name: "U",
    role,
    must_change_password: false,
  });
  return render(
    <QueryClientProvider client={qc}>
      <DescriptionGenerate pagePath="notes/a.md" onApplied={opts.onApplied} />
    </QueryClientProvider>,
  );
}

const DRAFT = {
  page_path: "notes/a.md",
  description: "A concise note about the 2.0 release.",
  base_revision: "rev-1",
};

async function openSurface(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: /generate description/i }));
  await screen.findByRole("dialog", { name: /generated description/i });
}

describe("DescriptionGenerate trust surface", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("does not render the trigger for a reader (editor-gated)", () => {
    renderGenerate("reader");
    expect(
      screen.queryByRole("button", { name: /generate description/i }),
    ).toBeNull();
  });

  it("mounting issues NO generate or apply call (never auto-runs)", async () => {
    vi.mocked(client.generateDescription).mockResolvedValue(DRAFT);
    renderGenerate();
    // Let the editor gate resolve + any effects flush.
    await screen.findByRole("button", { name: /generate description/i });
    expect(client.generateDescription).not.toHaveBeenCalled();
    expect(client.applyDescription).not.toHaveBeenCalled();
  });

  it("clicking Generate opens the editable draft in a textarea", async () => {
    vi.mocked(client.generateDescription).mockResolvedValue(DRAFT);
    const user = userEvent.setup();
    renderGenerate();
    await openSurface(user);

    const textarea = screen.getByLabelText("Description draft") as HTMLTextAreaElement;
    expect(textarea.value).toBe(DRAFT.description);
    expect(client.generateDescription).toHaveBeenCalledTimes(1);
    expect(client.applyDescription).not.toHaveBeenCalled(); // nothing written yet
  });

  it("editing the textarea then Apply sends the EDITED text + captured base_revision", async () => {
    vi.mocked(client.generateDescription).mockResolvedValue(DRAFT);
    vi.mocked(client.applyDescription).mockResolvedValue(undefined);
    const onApplied = vi.fn();
    const user = userEvent.setup();
    renderGenerate("editor", { onApplied });
    await openSurface(user);

    const textarea = screen.getByLabelText("Description draft");
    await user.clear(textarea);
    await user.type(textarea, "My hand-edited description.");

    await user.click(screen.getByRole("button", { name: /^apply$/i }));

    await waitFor(() =>
      expect(client.applyDescription).toHaveBeenCalledWith({
        page_path: "notes/a.md",
        description: "My hand-edited description.",
        base_revision: "rev-1",
      }),
    );
    // The approved (edited) text is handed back to PageEditor's reconcile.
    expect(onApplied).toHaveBeenCalledWith("My hand-edited description.");
  });

  it("Regenerate re-fetches generateDescription", async () => {
    vi.mocked(client.generateDescription)
      .mockResolvedValueOnce(DRAFT)
      .mockResolvedValueOnce({
        ...DRAFT,
        description: "A freshly regenerated description.",
        base_revision: "rev-2",
      });
    const user = userEvent.setup();
    renderGenerate();
    await openSurface(user);

    // Regenerate (the in-surface button) re-calls generate and refreshes the draft.
    await user.click(screen.getByRole("button", { name: /^regenerate$/i }));
    await waitFor(() =>
      expect(client.generateDescription).toHaveBeenCalledTimes(2),
    );
    const textarea = screen.getByLabelText("Description draft") as HTMLTextAreaElement;
    await waitFor(() =>
      expect(textarea.value).toBe("A freshly regenerated description."),
    );
  });

  it("Cancel is the initially-focused control; Esc cancels and calls applyDescription ZERO times", async () => {
    vi.mocked(client.generateDescription).mockResolvedValue(DRAFT);
    vi.mocked(client.applyDescription).mockResolvedValue(undefined);
    const user = userEvent.setup();
    renderGenerate();
    await openSurface(user);

    // Trust contract: Cancel (not Apply, not the textarea) holds initial focus.
    expect(screen.getByRole("button", { name: /^cancel$/i })).toHaveFocus();

    await user.keyboard("{Escape}");
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: /generated description/i }),
      ).toBeNull(),
    );
    expect(client.applyDescription).not.toHaveBeenCalled();
  });

  it("a 409 on apply shows the stale/regenerate state and does NOT call onApplied", async () => {
    vi.mocked(client.generateDescription).mockResolvedValue(DRAFT);
    const conflict = Object.assign(new Error("conflict"), { status: 409 });
    vi.mocked(client.applyDescription).mockRejectedValue(conflict);
    const onApplied = vi.fn();
    const user = userEvent.setup();
    renderGenerate("editor", { onApplied });
    await openSurface(user);

    await user.click(screen.getByRole("button", { name: /^apply$/i }));

    expect(
      await screen.findByText(/this page changed since the description was generated/i),
    ).toBeInTheDocument();
    // The Apply path is removed; Regenerate is offered; onApplied never fired.
    expect(screen.queryByRole("button", { name: /^apply$/i })).toBeNull();
    expect(screen.getByRole("button", { name: /^regenerate$/i })).toBeInTheDocument();
    expect(onApplied).not.toHaveBeenCalled();
  });

  it("a non-409 apply error shows the recoverable error line and keeps the surface open", async () => {
    vi.mocked(client.generateDescription).mockResolvedValue(DRAFT);
    const boom = Object.assign(new Error("boom"), { status: 500 });
    vi.mocked(client.applyDescription).mockRejectedValue(boom);
    const onApplied = vi.fn();
    const user = userEvent.setup();
    renderGenerate("editor", { onApplied });
    await openSurface(user);

    await user.click(screen.getByRole("button", { name: /^apply$/i }));

    expect(
      await screen.findByText(/couldn’t apply the description/i),
    ).toBeInTheDocument();
    // The surface stays open (Apply still present) and onApplied never fired.
    expect(screen.getByRole("button", { name: /^apply$/i })).toBeInTheDocument();
    expect(onApplied).not.toHaveBeenCalled();
  });
});
