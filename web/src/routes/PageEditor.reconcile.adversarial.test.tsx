/**
 * ED-03a — ADVERSARIAL suite for the autosave-safe apply→reconcile
 * (`reconcileAppliedTags`). These are the tests the implementer's own tests
 * structurally miss: the failure branch of the post-apply refresh, format
 * divergence termination, the in-flight-save interleave, and the union de-dup
 * case/inline collisions. They were written independently of PageEditor.tsx.
 *
 * The criterion-5 HARD BAR under attack: apply tags → keep typing → the next
 * autosave does NOT 409 into the conflict dialog and does NOT revert the applied
 * tags; an unsaved title/description is NOT lost.
 *
 * Each test states its ORACLE explicitly (a property, a metamorphic relation, or
 * the minimum "must not lose data / must not 409" bar). DEFECT-prefixed tests
 * REPRODUCE a real defect (they fail against the current code path's contract by
 * asserting the wrong-but-actual behaviour, and are labelled so the engineer can
 * see the repro); the rest are durable coverage proving the design holds.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return {
    ...actual,
    getPage: vi.fn(),
    savePage: vi.fn(),
    getTree: vi.fn().mockResolvedValue([]),
    acquireLock: vi.fn().mockResolvedValue({ result: "acquired" }),
    releaseLock: vi.fn().mockResolvedValue(undefined),
    me: vi.fn(),
    suggestTags: vi.fn(),
    applyTags: vi.fn(),
  };
});

// Stub the CM6 editor with a plain textarea (identical seam to PageEditor.test).
vi.mock("../components/LivePreviewEditor", () => ({
  default: ({
    value,
    onChange,
  }: {
    value: string;
    onChange: (v: string) => void;
    currentPath: string;
    mode: "live" | "source";
  }) => (
    <textarea
      aria-label="body"
      value={value}
      onChange={(e) => onChange(e.target.value)}
    />
  ),
}));

import * as client from "../api/client";
import { readTags } from "../lib/tag-frontmatter";
import PageEditor from "./PageEditor";

function renderEditor(path: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(["me"], {
    username: "u",
    display_name: "U",
    role: "editor",
    must_change_password: false,
  });
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[`/app/edit/${path}`]}>
        <Routes>
          <Route path="/app/edit/*" element={<PageEditor />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

// Drain timers + microtasks repeatedly so a chain of awaited promises settles.
async function flush() {
  for (let i = 0; i < 12; i++) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
  }
}

const fmOfLastSave = (): string =>
  vi.mocked(client.savePage).mock.calls.at(-1)![1].frontmatter;

describe("ED-03a reconcileAppliedTags — adversarial", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(client.getTree).mockResolvedValue([]);
    vi.mocked(client.acquireLock).mockResolvedValue({ result: "acquired" });
    vi.mocked(client.releaseLock).mockResolvedValue(undefined);
    vi.mocked(client.me).mockResolvedValue({
      username: "u",
      display_name: "U",
      role: "editor",
      must_change_password: false,
    });
  });

  // ───────────────────────────────────────────────────────────────────────────
  // HOLDS (post-fix regression — was a REPRODUCED defect) — the post-apply refresh
  // failure branch (attack vector 4, 2nd half).
  //
  // applyTags has ALREADY committed server-side (applyMutation.onSuccess fires only
  // on a 2xx), so the approved tag is AUTHORITATIVE. The fix merges it into local
  // frontmatter BEFORE/independent of reconcileAppliedTags' getPage, so a transient
  // GET failure surfaces a recoverable banner but NEVER drops the tag from the
  // editor. (The residual stale-revision 409 on a LATER save is the same recoverable
  // behaviour runSaver has on its own read-back failure — and is now data-safe; see
  // the next test for the Overwrite-preserves-it proof.)
  // ORACLE: no-data-loss of an explicitly-approved tag on a refresh failure.
  // ───────────────────────────────────────────────────────────────────────────
  it("a failed post-apply refresh keeps the approved tag merged locally (no revert)", async () => {
    vi.mocked(client.getPage)
      .mockResolvedValueOnce({
        frontmatter: "type: Page\ntitle: Notes\n",
        body: "original",
        revision: "rev-1",
      })
      // reconcileAppliedTags' getPage fails (the apply already committed rev-2).
      .mockRejectedValueOnce(new Error("network"))
      .mockResolvedValue({
        frontmatter: "type: Page\ntitle: Notes\ntags:\n  - release\n",
        body: "original",
        revision: "rev-2",
      });
    vi.mocked(client.suggestTags).mockResolvedValue({
      suggestions: [{ tag: "release", existing: true }],
      base_revision: "rev-1",
    });
    vi.mocked(client.applyTags).mockResolvedValue(undefined); // apply SUCCEEDS

    renderEditor("notes.md");
    await screen.findByLabelText("body");

    fireEvent.click(screen.getByRole("button", { name: /suggest tags/i }));
    fireEvent.click(await screen.findByRole("button", { name: /apply tags/i }));

    // The recoverable refresh-error banner shows (reconcile's getPage failed).
    expect(
      await screen.findByText(/couldn't refresh tags just now/i),
    ).toBeInTheDocument();

    // The tag the user explicitly approved STAYS on the page — merged locally
    // before the refresh, so the transient failure never reverts it from the view.
    expect(
      screen.getByRole("button", { name: "Remove tag release" }),
    ).toBeInTheDocument();
  });

  // HOLDS (post-fix regression — was the CHARACTERIZATION of the worst consequence)
  // — after a failed post-apply refresh strands the revision and the next save 409s,
  // resolving with "Overwrite with my version" PRESERVES the applied tag: the
  // merge-first fix put it in frontmatterRef, which Overwrite saves. Before the fix
  // the local frontmatter never received the tag, so Overwrite permanently reverted
  // the server's just-applied tag — that data loss is now closed.
  // ORACLE: no server-side revert of an explicitly-approved tag.
  it("after a stale 409, Overwrite PRESERVES the applied tag (no server-side revert)", async () => {
    vi.mocked(client.getPage)
      .mockResolvedValueOnce({
        frontmatter: "type: Page\ntitle: Notes\n",
        body: "original",
        revision: "rev-1",
      })
      .mockRejectedValueOnce(new Error("network")) // reconcile's getPage fails
      .mockResolvedValue({
        // The server view AFTER the successful apply (release is present there).
        frontmatter: "type: Page\ntitle: Notes\ntags:\n  - release\n",
        body: "original",
        revision: "rev-2",
      });
    vi.mocked(client.suggestTags).mockResolvedValue({
      suggestions: [{ tag: "release", existing: true }],
      base_revision: "rev-1",
    });
    vi.mocked(client.applyTags).mockResolvedValue(undefined);
    const conflict = Object.assign(new Error("conflict"), { status: 409 });
    // The autosave 409s; the subsequent Overwrite save succeeds.
    vi.mocked(client.savePage)
      .mockRejectedValueOnce(conflict)
      .mockResolvedValue(undefined);

    renderEditor("notes.md");
    await screen.findByLabelText("body");

    fireEvent.click(screen.getByRole("button", { name: /suggest tags/i }));
    fireEvent.click(await screen.findByRole("button", { name: /apply tags/i }));
    await screen.findByText(/couldn't refresh tags just now/i);

    // The stale revision was never advanced past the apply → the next save 409s.
    const body = screen.getByLabelText("body");
    fireEvent.change(body, { target: { value: "original and more" } });
    fireEvent.click(screen.getByRole("button", { name: "Save page" }));
    // D-08a: this 409 is caused by the user's OWN apply (it advanced the server
    // revision while baseRevision stayed stale after the failed refresh), so the
    // dialog attributes it accurately rather than the misleading "changed somewhere
    // else". The data-safety (Overwrite preserves the applied tag) is unchanged.
    await screen.findByText(/your change overlapped with a save in progress/i);
    expect(
      vi.mocked(client.savePage).mock.calls.at(-1)![1].base_revision,
    ).toBe("rev-1");

    // Resolve by overwriting with "my version" — the applied tag is PRESERVED
    // (it was merged into frontmatterRef before the refresh failed).
    fireEvent.click(
      screen.getByRole("button", { name: /overwrite with my version/i }),
    );

    await waitFor(() => {
      const last = vi.mocked(client.savePage).mock.calls.at(-1)!;
      expect(readTags(last[1].frontmatter)).toContain("release");
    });
  });

  // ───────────────────────────────────────────────────────────────────────────
  // HOLDS — an existing frontmatter DESCRIPTION survives the tag reconcile. ED-01a
  // removed the manual Description input, so a description is now a frontmatter value
  // (seeded/applied), never typed inline. The tag apply's setTags must touch ONLY the
  // tags region and leave the description scalar intact. A server-side tag-indent
  // divergence (4-space) forces the convergence autosave so we can inspect the bytes.
  // ORACLE: no-data-loss — the description scalar rides the convergence save unchanged.
  // ───────────────────────────────────────────────────────────────────────────
  it("an existing frontmatter description is preserved through the tag reconcile (ED-01a)", async () => {
    vi.useFakeTimers();
    try {
      vi.mocked(client.getPage)
        .mockResolvedValueOnce({
          frontmatter: "type: Page\ntitle: Notes\ndescription: Saved blurb\n",
          body: "original",
          revision: "rev-1",
        })
        .mockResolvedValue({
          // The server keeps the description + adds the tag at a 4-space indent (as
          // yaml.v3 re-marshals), diverging from the client's 2-space merge → exactly
          // one convergence autosave.
          frontmatter:
            "type: Page\ntitle: Notes\ndescription: Saved blurb\ntags:\n    - release\n",
          body: "original",
          revision: "rev-2",
        });
      vi.mocked(client.suggestTags).mockResolvedValue({
        suggestions: [{ tag: "release", existing: true }],
        base_revision: "rev-1",
      });
      vi.mocked(client.applyTags).mockResolvedValue(undefined);
      vi.mocked(client.savePage).mockResolvedValue(undefined);

      renderEditor("notes.md");
      await flush();

      fireEvent.click(screen.getByRole("button", { name: /suggest tags/i }));
      await flush();
      fireEvent.click(screen.getByRole("button", { name: /apply tags/i }));
      await flush();

      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000);
      });
      await flush();

      const last = vi.mocked(client.savePage).mock.calls.at(-1)!;
      expect(last[1].frontmatter).toContain("description: Saved blurb");
      expect(readTags(last[1].frontmatter)).toContain("release");
      expect(last[1].base_revision).toBe("rev-2");
    } finally {
      vi.useRealTimers();
    }
  });

  // HOLDS — an UNSAVED title (still a manual input) AND an EXISTING description
  // together survive the tag reconcile. Post-ED-01a the description is a frontmatter
  // value, not a manual input, but both regions are patched into the same raw
  // frontmatter the reconcile's setTags edits, so neither is lost.
  it("an unsaved title and an existing description together survive the reconcile", async () => {
    vi.useFakeTimers();
    try {
      vi.mocked(client.getPage)
        .mockResolvedValueOnce({
          frontmatter: "type: Page\ntitle: Notes\ndescription: Saved blurb\n",
          body: "original",
          revision: "rev-1",
        })
        .mockResolvedValue({
          // The server never saw the unsaved title; it keeps "Notes" + the description.
          frontmatter:
            "type: Page\ntitle: Notes\ndescription: Saved blurb\ntags:\n  - release\n",
          body: "original",
          revision: "rev-2",
        });
      vi.mocked(client.suggestTags).mockResolvedValue({
        suggestions: [{ tag: "release", existing: true }],
        base_revision: "rev-1",
      });
      vi.mocked(client.applyTags).mockResolvedValue(undefined);
      vi.mocked(client.savePage).mockResolvedValue(undefined);

      renderEditor("notes.md");
      await flush();

      fireEvent.change(screen.getByLabelText("Title"), {
        target: { value: "Brand New Title" },
      });

      fireEvent.click(screen.getByRole("button", { name: /suggest tags/i }));
      await flush();
      fireEvent.click(screen.getByRole("button", { name: /apply tags/i }));
      await flush();

      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000);
      });
      await flush();

      const last = vi.mocked(client.savePage).mock.calls.at(-1)!;
      expect(last[1].frontmatter).toContain("title: Brand New Title");
      expect(last[1].frontmatter).toContain("description: Saved blurb");
      expect(readTags(last[1].frontmatter)).toContain("release");
      expect(last[1].base_revision).toBe("rev-2");
    } finally {
      vi.useRealTimers();
    }
  });

  // ───────────────────────────────────────────────────────────────────────────
  // HOLDS (termination property) — client/server frontmatter FORMAT divergence
  // (attack vector 5). The server re-marshals tags at 4-space indent (yaml.v3);
  // the client setTags emits 2-space. The merged (2-space) != fresh (4-space) so
  // a convergence autosave fires — it must fire EXACTLY ONCE and terminate, never
  // a 409-loop or infinite churn. ORACLE: termination — savePage call count is
  // bounded at 1 and stays bounded as time advances.
  // ───────────────────────────────────────────────────────────────────────────
  it("format divergence (server 4-space tags) converges in EXACTLY ONE save and terminates", async () => {
    vi.useFakeTimers();
    try {
      vi.mocked(client.getPage)
        .mockResolvedValueOnce({
          frontmatter: "title: Notes\n",
          body: "original",
          revision: "rev-1",
        })
        .mockResolvedValue({
          // 4-space, exactly as yaml.v3 emits after a Repair re-marshal.
          frontmatter: "title: Notes\ntags:\n    - release\n",
          body: "original",
          revision: "rev-2",
        });
      vi.mocked(client.suggestTags).mockResolvedValue({
        suggestions: [{ tag: "release", existing: true }],
        base_revision: "rev-1",
      });
      vi.mocked(client.applyTags).mockResolvedValue(undefined);
      vi.mocked(client.savePage).mockResolvedValue(undefined);

      renderEditor("notes.md");
      await flush();

      fireEvent.click(screen.getByRole("button", { name: /suggest tags/i }));
      await flush();
      fireEvent.click(screen.getByRole("button", { name: /apply tags/i }));
      await flush();

      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000);
      });
      await flush();

      // Exactly one convergence save, against the post-apply revision.
      expect(client.savePage).toHaveBeenCalledTimes(1);
      const c = vi.mocked(client.savePage).mock.calls[0]!;
      expect(c[1].base_revision).toBe("rev-2");
      expect(readTags(c[1].frontmatter)).toContain("release");
      // It sent the client (2-space) form, not the server 4-space bytes.
      expect(c[1].frontmatter).toContain("  - release");

      // TERMINATION: advancing well past several debounce windows yields no
      // further save (no 409-thrash / churn loop).
      await act(async () => {
        await vi.advanceTimersByTimeAsync(10_000);
      });
      await flush();
      expect(client.savePage).toHaveBeenCalledTimes(1);
      expect(
        screen.queryByText(/this page was changed somewhere else/i),
      ).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  // ───────────────────────────────────────────────────────────────────────────
  // HOLDS — applyTags 409 (the page MOVED between suggest and apply; attack
  // vector 4, 1st half). reconcileAppliedTags must NOT run (it is only wired to
  // applyMutation.onSuccess), so no ref is mutated and the stale surface shows.
  // ORACLE: no-corruption — a rejected apply leaves the editor's save state pristine.
  // ───────────────────────────────────────────────────────────────────────────
  it("applyTags 409 does NOT run reconcile — no ref mutation, stale state shown, no save", async () => {
    vi.mocked(client.getPage).mockResolvedValue({
      frontmatter: "type: Page\ntitle: Notes\n",
      body: "original",
      revision: "rev-1",
    });
    vi.mocked(client.suggestTags).mockResolvedValue({
      suggestions: [{ tag: "release", existing: true }],
      base_revision: "rev-1",
    });
    const conflict = Object.assign(new Error("conflict"), { status: 409 });
    vi.mocked(client.applyTags).mockRejectedValue(conflict);
    vi.mocked(client.savePage).mockResolvedValue(undefined);

    renderEditor("notes.md");
    await screen.findByLabelText("body");

    fireEvent.click(screen.getByRole("button", { name: /suggest tags/i }));
    fireEvent.click(await screen.findByRole("button", { name: /apply tags/i }));

    // The approval surface flips to the stale state (no clobbering retry path).
    expect(
      await screen.findByText(/this page changed since the tags were suggested/i),
    ).toBeInTheDocument();
    // reconcile never ran: no tag merged locally, and no save was triggered.
    expect(
      screen.queryByRole("button", { name: "Remove tag release" }),
    ).toBeNull();
    expect(client.savePage).not.toHaveBeenCalled();
  });

  // ───────────────────────────────────────────────────────────────────────────
  // HOLDS — IN-FLIGHT body autosave interleaved with apply's reconcile (attack
  // vector 7). A body save is held mid-flight (awaiting savePage) when apply's
  // reconcile mutates baseRevision / frontmatterRef / lastSavedFrontmatter. The
  // single-flight runSaver loop's ref re-read must pick up the merged tag so it
  // is NOT a lost write, and the editor must not get stuck on "Saving…".
  // ORACLE: no-lost-write + liveness (terminal "Saved" state, tag + body persisted).
  // ───────────────────────────────────────────────────────────────────────────
  it("an in-flight body save interleaved with apply's reconcile loses neither the tag nor the body", async () => {
    vi.useFakeTimers();
    try {
      vi.mocked(client.getPage)
        .mockResolvedValueOnce({
          frontmatter: "type: Page\ntitle: Notes\n",
          body: "orig",
          revision: "rev-1",
        })
        // reconcile's getPage (apply committed) — 4-space to force a convergence.
        .mockResolvedValueOnce({
          frontmatter: "type: Page\ntitle: Notes\ntags:\n    - release\n",
          body: "orig",
          revision: "rev-2",
        })
        // every later read-back
        .mockResolvedValue({
          frontmatter: "type: Page\ntitle: Notes\ntags:\n    - release\n",
          body: "orig",
          revision: "rev-3",
        });

      // The FIRST savePage (the body autosave) is held in flight; later saves resolve.
      let release: () => void = () => {};
      const held = new Promise<undefined>((resolve) => {
        release = () => resolve(undefined);
      });
      vi.mocked(client.savePage)
        .mockImplementationOnce(() => held)
        .mockResolvedValue(undefined);

      vi.mocked(client.suggestTags).mockResolvedValue({
        suggestions: [{ tag: "release", existing: true }],
        base_revision: "rev-1",
      });
      vi.mocked(client.applyTags).mockResolvedValue(undefined);

      renderEditor("notes.md");
      await flush();

      // Type the body and let the 1s debounce start the (now-held) body save.
      const body = screen.getByLabelText("body");
      fireEvent.change(body, { target: { value: "orig X" } });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000);
      });
      expect(client.savePage).toHaveBeenCalledTimes(1); // body save in flight

      // While that save is held, apply tags → reconcile mutates the shared refs.
      fireEvent.click(screen.getByRole("button", { name: /suggest tags/i }));
      await flush();
      fireEvent.click(screen.getByRole("button", { name: /apply tags/i }));
      await flush();

      // Release the held body save; the loop must converge the merged tag.
      release();
      await flush();
      await act(async () => {
        await vi.advanceTimersByTimeAsync(2000);
      });
      await flush();

      // The tag survived (last save carries it) and the body was never lost.
      expect(readTags(fmOfLastSave())).toContain("release");
      const sawBody = vi
        .mocked(client.savePage)
        .mock.calls.some((c) => c[1].body === "orig X");
      expect(sawBody).toBe(true);
      // Liveness: not stuck on "Saving…" — the indicator settled to "Saved".
      expect(screen.getByText("Saved")).toBeInTheDocument();
      // No spurious conflict dialog.
      expect(
        screen.queryByText(/this page was changed somewhere else/i),
      ).toBeNull();
      // The title was never clobbered by the server bytes.
      expect(fmOfLastSave()).toContain("title: Notes");
    } finally {
      vi.useRealTimers();
    }
  });

  // ───────────────────────────────────────────────────────────────────────────
  // HOLDS — union de-dup at suggest time (attack vectors 3 + 6). A suggestion
  // already present as an UNSAVED inline body `#tag` OR as a frontmatter tag is
  // filtered out before the review surface opens — case-insensitively. ORACLE:
  // the property "no suggestion is offered for a tag already on the live page".
  // ───────────────────────────────────────────────────────────────────────────
  it("union de-dup filters an unsaved inline #tag and a frontmatter tag (case-insensitive)", async () => {
    vi.mocked(client.getPage).mockResolvedValue({
      // frontmatter already has `deploy`; body has an unsaved inline `#draft`.
      frontmatter: "type: Page\ntitle: Notes\ntags:\n  - deploy\n",
      body: "text #draft",
      revision: "rev-1",
    });
    vi.mocked(client.suggestTags).mockResolvedValue({
      suggestions: [
        { tag: "Draft", existing: true }, // collides with inline #draft (ci)
        { tag: "Deploy", existing: true }, // collides with frontmatter deploy (ci)
        { tag: "release", existing: false }, // genuinely new → survives
      ],
      base_revision: "rev-1",
    });

    renderEditor("notes.md");
    // The union de-dup reads the LIVE body/frontmatter at suggest-click time, so
    // wait until PageEditor has seeded them before triggering (in real use the
    // body is always on screen before the user can click).
    await waitFor(() =>
      expect(screen.getByLabelText("body")).toHaveValue("text #draft"),
    );

    fireEvent.click(screen.getByRole("button", { name: /suggest tags/i }));

    // Only the genuinely-new suggestion survives the union de-dup.
    expect(await screen.findByText("release")).toBeInTheDocument();
    expect(screen.queryByText("Draft")).toBeNull();
    expect(screen.queryByText("Deploy")).toBeNull();
  });

  // HOLDS — a nested tag (#a/b) applied via the apply path round-trips through
  // setTags/readTags without leak or duplication. A force Save after apply gives a
  // deterministic frontmatter to inspect. ORACLE: round-trip property —
  // readTags(saved) carries the applied nested tag EXACTLY once (one `- a/b` line).
  it("a nested applied tag (a/b) round-trips through reconcile without duplication", async () => {
    vi.mocked(client.getPage)
      .mockResolvedValueOnce({
        frontmatter: "type: Page\ntitle: Notes\n",
        body: "original",
        revision: "rev-1",
      })
      .mockResolvedValue({
        frontmatter: "type: Page\ntitle: Notes\ntags:\n  - a/b\n",
        body: "original",
        revision: "rev-2",
      });
    vi.mocked(client.suggestTags).mockResolvedValue({
      suggestions: [{ tag: "a/b", existing: false }],
      base_revision: "rev-1",
    });
    vi.mocked(client.applyTags).mockResolvedValue(undefined);
    vi.mocked(client.savePage).mockResolvedValue(undefined);

    renderEditor("notes.md");
    await screen.findByLabelText("body");

    fireEvent.click(screen.getByRole("button", { name: /suggest tags/i }));
    // "a/b" is new → unchecked by default; check it before applying.
    fireEvent.click(await screen.findByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: /apply tags/i }));

    // The reconcile merged the nested tag locally → it shows as ONE removable badge.
    expect(
      await screen.findByRole("button", { name: "Remove tag a/b" }),
    ).toBeInTheDocument();

    // A force Save flushes the merged frontmatter so we can inspect the bytes.
    fireEvent.click(screen.getByRole("button", { name: "Save page" }));
    await waitFor(() => expect(client.savePage).toHaveBeenCalled());

    const fm = fmOfLastSave();
    expect(readTags(fm).filter((t) => t === "a/b")).toHaveLength(1);
    // Exactly one block-sequence line for the nested tag (no duplicate appended).
    expect((fm.match(/^\s*-\s*a\/b\s*$/gm) ?? []).length).toBe(1);
  });
});
