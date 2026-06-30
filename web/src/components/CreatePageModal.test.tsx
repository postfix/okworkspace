/**
 * CreatePageModal — the new-page picker under test (ED-06a). Locked contract:
 *   1. Blank (the default) → createPage(folder, title) with NO seed.
 *   2. Choosing a template → getPage(templatePath) then createPage WITH the
 *      substituted seed ({{title}} replaced by the entered title).
 *   3. No templates → only "Blank" (no select rendered).
 *   4. The existing title-required validation is intact (empty title → no call).
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";

vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return {
    ...actual,
    listTemplates: vi.fn(),
    getPage: vi.fn(),
    createPage: vi.fn(),
  };
});

import * as client from "../api/client";
import CreatePageModal from "./CreatePageModal";

function renderModal() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <CreatePageModal
          open
          folder="runbooks"
          folderName="Runbooks"
          onClose={() => {}}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(client.createPage).mockResolvedValue({ path: "runbooks/x.md" });
});

describe("CreatePageModal template picker (ED-06a)", () => {
  it("Blank default → createPage with NO seed", async () => {
    vi.mocked(client.listTemplates).mockResolvedValue([
      { name: "Meeting", path: "Templates/Meeting.md" },
    ]);
    renderModal();
    // Wait for the picker to appear (templates loaded).
    await screen.findByLabelText("Template");

    await userEvent.type(screen.getByLabelText("Page title"), "My Note");
    await userEvent.click(screen.getByRole("button", { name: "Create page" }));

    await waitFor(() =>
      expect(client.createPage).toHaveBeenCalledWith("runbooks", "My Note"),
    );
    expect(client.getPage).not.toHaveBeenCalled();
  });

  it("choosing a template → getPage then createPage WITH the substituted seed", async () => {
    vi.mocked(client.listTemplates).mockResolvedValue([
      { name: "Meeting", path: "Templates/Meeting.md" },
    ]);
    vi.mocked(client.getPage).mockResolvedValue({
      // Frontmatter carries {{date}} and a {{title}} in a value position — the
      // latter must be LEFT LITERAL (server owns the title), the former filled.
      frontmatter: "type: Meeting\ncreated: {{date}}\nalias: {{title}}",
      body: "# {{title}}\n\nNotes.",
      revision: "abc",
    });
    renderModal();

    const select = await screen.findByLabelText("Template");
    // A colon-bearing title — the canonical case that broke before the fix.
    await userEvent.type(screen.getByLabelText("Page title"), "Meeting: Kickoff");
    await userEvent.selectOptions(select, "Templates/Meeting.md");
    await userEvent.click(screen.getByRole("button", { name: "Create page" }));

    await waitFor(() =>
      expect(client.getPage).toHaveBeenCalledWith("Templates/Meeting.md"),
    );
    await waitFor(() => expect(client.createPage).toHaveBeenCalledTimes(1));
    const call = vi.mocked(client.createPage).mock.calls[0];
    expect(call[0]).toBe("runbooks");
    expect(call[1]).toBe("Meeting: Kickoff");
    const seed = call[2] as { frontmatter: string; body: string };
    // BODY: {{title}} substituted with the (colon-bearing) entered title.
    expect(seed.body).toContain("# Meeting: Kickoff");
    expect(seed.body).not.toContain("{{title}}");
    // FRONTMATTER: {{date}} filled, but {{title}} LEFT LITERAL (never a raw title
    // injected into a YAML value position — the server owns the title).
    expect(seed.frontmatter).toContain("type: Meeting");
    expect(seed.frontmatter).toContain("alias: {{title}}");
    expect(seed.frontmatter).not.toContain("Meeting: Kickoff");
    expect(seed.frontmatter).toMatch(/created: \d{4}-\d{2}-\d{2}/);
  });

  it("no templates → only Blank (no select rendered)", async () => {
    vi.mocked(client.listTemplates).mockResolvedValue([]);
    renderModal();
    await screen.findByLabelText("Page title");
    expect(screen.queryByLabelText("Template")).toBeNull();
  });

  it("empty title → validation error, no createPage call", async () => {
    vi.mocked(client.listTemplates).mockResolvedValue([]);
    renderModal();
    await screen.findByLabelText("Page title");
    await userEvent.click(screen.getByRole("button", { name: "Create page" }));
    expect(
      await screen.findByText("Give your page a title to create it."),
    ).toBeInTheDocument();
    expect(client.createPage).not.toHaveBeenCalled();
  });
});
