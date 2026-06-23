/**
 * AppShell tests.
 *  - AUTH-06: the authenticated user's display_name renders in the top bar.
 *  - AGNT-02/03/06/07 dispatch (04-07 gap closure): the PromptBar submit routes
 *    rewrite → DiffReviewDialog (never auto-applies), selection-scope Ask,
 *    attachment-scope Ask, and summarize-attachment to the right client calls.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// Mock api/client so no real fetch calls happen. AppShell mounts LeftTree
// (getTree) + create modals, and the agent session (subscribeAgentChat, the
// single-shot modes, propose/apply, and the 04-07 rewrite/summarizeAttachment).
vi.mock("../api/client", () => ({
  me: vi.fn(),
  health: vi.fn(),
  logout: vi.fn(),
  getTree: vi.fn().mockResolvedValue([]),
  createPage: vi.fn(),
  createFolder: vi.fn(),
  subscribeAgentChat: vi.fn(() => () => {}),
  proposePatch: vi.fn(),
  applyPatch: vi.fn(),
  summarizePage: vi.fn(),
  summarizeAttachment: vi.fn(),
  rewrite: vi.fn(),
  draft: vi.fn(),
}));

import * as client from "../api/client";
import AppShell from "./AppShell";
import { useAgentContext } from "../stores/agentContext";
import { useAgentPanel } from "../stores/agentPanel";

function seedMe(role: "admin" | "editor" | "reader" = "editor") {
  vi.mocked(client.me).mockResolvedValue({
    username: "alice",
    display_name: "Alice Wonderland",
    role,
    must_change_password: false,
  });
  vi.mocked(client.health).mockResolvedValue({
    ok: true,
    diverged: false,
    self_healed: false,
    detail: "",
  });
}

function renderAppShell(initialPath = "/app") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[initialPath]}>
        <AppShell />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return qc;
}

// submitPrompt types into the PromptBar and presses Enter.
async function submitPrompt(user: ReturnType<typeof userEvent.setup>, text: string) {
  const input = await screen.findByLabelText("Prompt");
  await user.click(input);
  await user.keyboard(text);
  await user.keyboard("{Enter}");
}

beforeEach(() => {
  vi.clearAllMocks();
  // Reset the ephemeral agent context + panel between tests.
  useAgentContext.getState().clearSelection();
  useAgentContext.getState().clearAttachment();
  // The prompt is docked in the Assistant panel now, so it must be open to type.
  useAgentPanel.getState().setOpen(true);
});

describe("AppShell — AUTH-06", () => {
  it("renders the authenticated user's display_name in the top bar", async () => {
    seedMe("editor");
    renderAppShell();
    expect(
      await screen.findByRole("button", { name: /Alice Wonderland/i }),
    ).toBeInTheDocument();
  });

});

describe("AppShell — agent dispatch (04-07)", () => {
  it("AGNT-07: rewrite mode with a selection calls rewrite() and opens DiffReviewDialog (never auto-applies)", async () => {
    const user = userEvent.setup();
    seedMe("editor");
    vi.mocked(client.rewrite).mockResolvedValue("the rewritten span");
    // A page must be open for rewrite (page scope).
    renderAppShell("/app/page/notes");

    // Seed a live selection (what the CM6 editor would publish).
    useAgentContext.getState().setSelection("the original span");

    // Prime Rewrite from the in-window suggestion, then submit an instruction.
    await user.click(
      await screen.findByRole("button", { name: "Rewrite selection" }),
    );
    await submitPrompt(user, "make it concise");

    expect(client.rewrite).toHaveBeenCalledWith("the original span", "make it concise");
    // The diff dialog opens with the rewrite content — apply NEVER ran.
    expect(await screen.findByRole("dialog", { name: /Review the rewrite/i })).toBeInTheDocument();
    expect(client.applyPatch).not.toHaveBeenCalled();
  });

  it("AGNT-02: Ask with a selection calls subscribeAgentChat with scope=selection", async () => {
    const user = userEvent.setup();
    seedMe("editor");
    renderAppShell("/app/page/notes");

    useAgentContext.getState().setSelection("a selected sentence");
    // Mode defaults to ask.
    await submitPrompt(user, "what does this mean?");

    expect(client.subscribeAgentChat).toHaveBeenCalledTimes(1);
    const req = vi.mocked(client.subscribeAgentChat).mock.calls[0][0];
    expect(req.scope).toBe("selection");
    expect(req.selection).toBe("a selected sentence");
  });

  it("AGNT-03: Ask with an attachment (no selection) calls subscribeAgentChat with scope=attachment + attachment_id", async () => {
    const user = userEvent.setup();
    seedMe("editor");
    renderAppShell("/app/page/notes");

    useAgentContext.getState().setAttachment({ id: "att-9", name: "spec.pdf" });
    await submitPrompt(user, "summarize the intro");

    expect(client.subscribeAgentChat).toHaveBeenCalledTimes(1);
    const req = vi.mocked(client.subscribeAgentChat).mock.calls[0][0];
    expect(req.scope).toBe("attachment");
    expect(req.attachment_id).toBe("att-9");
  });

  it("AGNT-06: Summarize with an attachment calls summarizeAttachment(id) (not summarizePage)", async () => {
    const user = userEvent.setup();
    seedMe("editor");
    vi.mocked(client.summarizeAttachment).mockResolvedValue("a short summary");
    renderAppShell("/app/page/notes");

    useAgentContext.getState().setAttachment({ id: "att-42", name: "report.docx" });

    // Summarize runs immediately from the in-window suggestion (no prompt needed);
    // with an attachment in context the chip names the file and routes to it.
    await user.click(
      await screen.findByRole("button", { name: /Summarize report\.docx/i }),
    );

    await waitFor(() =>
      expect(client.summarizeAttachment).toHaveBeenCalledWith("att-42"),
    );
    expect(client.summarizePage).not.toHaveBeenCalled();
  });

  it("AGNT-05 unchanged: Summarize with only a page open still calls summarizePage", async () => {
    const user = userEvent.setup();
    seedMe("editor");
    vi.mocked(client.summarizePage).mockResolvedValue("page summary");
    renderAppShell("/app/page/notes");

    // No attachment context — Summarize runs immediately on the page from the
    // in-window suggestion (no prompt needed).
    await user.click(
      await screen.findByRole("button", { name: "Summarize this page" }),
    );

    await waitFor(() =>
      expect(client.summarizePage).toHaveBeenCalledWith("notes"),
    );
    expect(client.summarizeAttachment).not.toHaveBeenCalled();
  });
});
