/**
 * PAGE-02 — PageEditor saves on click and surfaces the 409 conflict banner.
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
    // Lock calls: default to "you hold it" (acquired) so existing tests stay editable.
    acquireLock: vi.fn().mockResolvedValue({ result: "acquired" }),
    releaseLock: vi.fn().mockResolvedValue(undefined),
    // Phase 23: the embedded TagSuggest needs `me` (editor gate) + the suggest/apply
    // endpoints. Defaults are set per-test; `me` resolves editor so the trigger shows.
    me: vi.fn(),
    suggestTags: vi.fn(),
    applyTags: vi.fn(),
    // Phase 24: the embedded DescriptionGenerate needs the generate/apply endpoints.
    generateDescription: vi.fn(),
    applyDescription: vi.fn(),
  };
});

// Mock the CM6 LivePreviewEditor with a plain textarea so the test stays fast and
// deterministic (the editor's exact rendering is covered by LivePreviewEditor.test;
// here we only exercise PageEditor's save machinery via the value/onChange seam).
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
  // Seed the cached ["me"] session so the embedded TagSuggest's editor gate resolves
  // synchronously (mirrors App.tsx). Harmless to the pre-Phase-23 save tests — they
  // never assert on the "Suggest tags" trigger.
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

describe("PageEditor", () => {
  beforeEach(() => {
    // Reset call history between tests so per-test call-count assertions are not
    // polluted by a prior test's savePage/getPage calls.
    vi.clearAllMocks();
    vi.mocked(client.getPage).mockResolvedValue({
      frontmatter: "type: Page\ntitle: Notes\n",
      body: "original",
      revision: "rev-1",
    });
    vi.mocked(client.getTree).mockResolvedValue([]);
    // Default lock state: this session holds the lock (editable). Tests that need a
    // held-by-other state override acquireLock per-test.
    vi.mocked(client.acquireLock).mockResolvedValue({ result: "acquired" });
    vi.mocked(client.releaseLock).mockResolvedValue(undefined);
    // Phase 23 embedded TagSuggest: default `me` to an editor so the trigger renders.
    vi.mocked(client.me).mockResolvedValue({
      username: "u",
      display_name: "U",
      role: "editor",
      must_change_password: false,
    });
  });

  it("issues a save PUT on Save page click", async () => {
    vi.mocked(client.savePage).mockResolvedValue(undefined);
    // After save, doSave refetches the page for the new revision.
    renderEditor("notes.md");
    const body = await screen.findByLabelText("body");
    fireEvent.change(body, { target: { value: "edited body" } });

    fireEvent.click(screen.getByRole("button", { name: "Save page" }));

    await waitFor(() =>
      expect(client.savePage).toHaveBeenCalledWith(
        "notes.md",
        expect.objectContaining({ body: "edited body", base_revision: "rev-1" }),
      ),
    );
  });

  it("does not start an overlapping save while one is in flight (WR-03)", async () => {
    // savePage resolves on a deferred promise so we can hold the first save
    // in-flight and prove a burst of Save clicks during that window is dropped.
    let releaseSave: () => void = () => {};
    let savePromise = new Promise<undefined>((resolve) => {
      releaseSave = () => resolve(undefined);
    });
    vi.mocked(client.savePage).mockImplementation(() => savePromise);

    renderEditor("notes.md");
    await screen.findByLabelText("body");

    // Fire a burst of Save clicks synchronously. The first starts a save (the
    // returned promise is held unresolved); every subsequent click while it is
    // in flight must be dropped by the in-flight guard, so savePage is called
    // exactly once — two PUTs never race on the same base revision (WR-03). No
    // fireEvent.change is used, so no autosave draft timer can perturb the count.
    const saveBtn = screen.getByRole("button", { name: "Save page" });
    fireEvent.click(saveBtn);
    fireEvent.click(saveBtn);
    fireEvent.click(saveBtn);
    fireEvent.click(saveBtn);
    expect(client.savePage).toHaveBeenCalledTimes(1);

    // Release the in-flight save (resolve immediately); the getPage refetch
    // advances the revision and clears the guard.
    savePromise = Promise.resolve(undefined);
    releaseSave();
    await waitFor(() => expect(client.getPage).toHaveBeenCalled());

    // A fresh save after the in-flight one settled is allowed again.
    fireEvent.click(saveBtn);
    await waitFor(() => expect(client.savePage).toHaveBeenCalledTimes(2));
  });

  it("flushes a trailing edit typed during an in-flight save (no lost write)", async () => {
    vi.useFakeTimers();
    // Drain timers + microtasks repeatedly so a chain of awaited promises settles.
    const flush = async () => {
      for (let i = 0; i < 8; i++) {
        await act(async () => {
          await vi.advanceTimersByTimeAsync(0);
        });
      }
    };
    try {
      // The first autosave is held in flight; later saves resolve immediately.
      let release: () => void = () => {};
      const held = new Promise<undefined>((resolve) => {
        release = () => resolve(undefined);
      });
      vi.mocked(client.savePage)
        .mockImplementationOnce(() => held)
        .mockResolvedValue(undefined);
      // Initial load returns rev-1; every post-save refetch returns a fresh rev.
      vi.mocked(client.getPage)
        .mockResolvedValueOnce({
          frontmatter: "type: Page\ntitle: Notes\n",
          body: "original",
          revision: "rev-1",
        })
        .mockResolvedValue({
          frontmatter: "type: Page\ntitle: Notes\n",
          body: "original",
          revision: "rev-2",
        });

      renderEditor("notes.md");
      await flush();
      const body = screen.getByLabelText("body");

      // First edit "A": the 1s draft timer fires the first save (held in flight).
      fireEvent.change(body, { target: { value: "A" } });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000);
      });
      expect(client.savePage).toHaveBeenCalledTimes(1);
      expect(vi.mocked(client.savePage).mock.calls[0][1]).toEqual(
        expect.objectContaining({ body: "A" }),
      );

      // Trailing edit "AB" typed WHILE the first save is in flight. Its draft
      // timer fires but is dropped by the in-flight guard — it must not be lost.
      fireEvent.change(body, { target: { value: "AB" } });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000);
      });
      expect(client.savePage).toHaveBeenCalledTimes(1); // dropped while in flight

      // Release the in-flight save; the success path must flush the trailing edit.
      release();
      await flush();

      expect(client.savePage).toHaveBeenCalledTimes(2);
      expect(vi.mocked(client.savePage).mock.calls[1][1]).toEqual(
        expect.objectContaining({ body: "AB" }),
      );
    } finally {
      vi.useRealTimers();
    }
  });

  it("opens the conflict dialog on a 409 (supersedes the Phase 1 banner)", async () => {
    const err = new Error("conflict") as Error & { status?: number };
    err.status = 409;
    vi.mocked(client.savePage).mockRejectedValue(err);
    // First getPage = initial load; the 409 path then fetches the (different)
    // server version to diff my edit against.
    vi.mocked(client.getPage)
      .mockResolvedValueOnce({
        frontmatter: "type: Page\ntitle: Notes\n",
        body: "original",
        revision: "rev-1",
      })
      .mockResolvedValue({
        frontmatter: "type: Page\ntitle: Notes\n",
        body: "their server version",
        revision: "rev-2",
      });

    renderEditor("notes.md");
    const body = await screen.findByLabelText("body");
    fireEvent.change(body, { target: { value: "my edited body" } });
    fireEvent.click(screen.getByRole("button", { name: "Save page" }));

    // The conflict dialog (a real diff + three risk-ranked choices) replaces the
    // old "Reload page" banner.
    expect(
      await screen.findByText(/this page was changed somewhere else/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: /overwrite with my version/i,
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /save my version as a new page/i }),
    ).toBeInTheDocument();
    // The old banner is gone.
    expect(
      screen.queryByRole("button", { name: "Reload page" }),
    ).toBeNull();
  });

  // COLL-02 regression: a session that does NOT hold the lock (held-by-other) must
  // never autosave. The save path is deliberately lock-independent server-side, so
  // this client gate is what enforces the SoftLockBanner's "won't be saved until you
  // take over." The original bug let a locked-out, still-editable "View only" surface
  // autosave + commit its edits.
  it("does NOT autosave while another session holds the lock (COLL-02)", async () => {
    vi.useFakeTimers();
    const flush = async () => {
      for (let i = 0; i < 8; i++) {
        await act(async () => {
          await vi.advanceTimersByTimeAsync(0);
        });
      }
    };
    try {
      vi.mocked(client.acquireLock).mockResolvedValue({
        result: "held-by-other",
        holder: { username: "alice" },
      });
      vi.mocked(client.savePage).mockResolvedValue(undefined);

      renderEditor("notes.md");
      await flush();

      // Precondition: the page is held by alice → "View only" soft lock is active.
      expect(
        screen.getByText(/alice is editing this page/i),
      ).toBeInTheDocument();

      // A change slips through the stubbed editor (the real CM6 surface is genuinely
      // non-editable when locked — see LivePreviewEditor.test; this isolates the
      // autosave gate). Advance well past the debounce.
      const body = screen.getByLabelText("body");
      fireEvent.change(body, { target: { value: "edit while locked out" } });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(5000);
      });
      await flush();

      expect(client.savePage).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });

  // ED-02b — body #tags are materialized into the canonical frontmatter `tags:` on
  // save, as a DIFF (add AND remove), reusing the shared Lezer parse. The save path
  // is the only sink; search/graph keep reading frontmatter unchanged.
  const fmOfLastSave = (): string =>
    vi.mocked(client.savePage).mock.calls.at(-1)![1].frontmatter;

  it("materializes a body #deploy into frontmatter tags on save (ED-02b add)", async () => {
    vi.mocked(client.getPage).mockResolvedValue({
      frontmatter: "title: Notes\n",
      body: "start",
      revision: "rev-1",
    });
    vi.mocked(client.savePage).mockResolvedValue(undefined);

    renderEditor("notes.md");
    const body = await screen.findByLabelText("body");
    fireEvent.change(body, { target: { value: "start #deploy" } });
    fireEvent.click(screen.getByRole("button", { name: "Save page" }));

    await waitFor(() => expect(client.savePage).toHaveBeenCalled());
    expect(readTags(fmOfLastSave())).toContain("deploy");
  });

  it("removes a deleted #deploy AND keeps an LLM-only tag (ED-02b remove + persist)", async () => {
    // Frontmatter carries an `llm` tag that was never written inline — it must survive
    // both the add and the remove (it is in readTags, never in removedInline).
    vi.mocked(client.getPage).mockResolvedValue({
      frontmatter: "title: Notes\ntags:\n  - llm\n",
      body: "start",
      revision: "rev-1",
    });
    vi.mocked(client.savePage).mockResolvedValue(undefined);

    renderEditor("notes.md");
    const body = await screen.findByLabelText("body");

    // Add #deploy → saved tags contain BOTH llm and deploy.
    fireEvent.change(body, { target: { value: "start #deploy" } });
    fireEvent.click(screen.getByRole("button", { name: "Save page" }));
    await waitFor(() =>
      expect(readTags(fmOfLastSave())).toEqual(
        expect.arrayContaining(["llm", "deploy"]),
      ),
    );

    // Delete #deploy → saved tags lose deploy but keep llm.
    fireEvent.change(body, { target: { value: "start" } });
    fireEvent.click(screen.getByRole("button", { name: "Save page" }));
    await waitFor(() => expect(readTags(fmOfLastSave())).not.toContain("deploy"));
    expect(readTags(fmOfLastSave())).toContain("llm");
  });

  it("a save with NO tag change sends byte-identical frontmatter (no 409 thrash)", async () => {
    // Body #deploy already matches frontmatter tags: [deploy] → reconcile is a no-op,
    // so setTags is never called and the exact frontmatter bytes are sent (D-03).
    const seededFm = "title: Notes\ntags:\n  - deploy\n";
    vi.mocked(client.getPage).mockResolvedValue({
      frontmatter: seededFm,
      body: "has #deploy",
      revision: "rev-1",
    });
    vi.mocked(client.savePage).mockResolvedValue(undefined);

    renderEditor("notes.md");
    await screen.findByLabelText("body");
    fireEvent.click(screen.getByRole("button", { name: "Save page" }));

    await waitFor(() => expect(client.savePage).toHaveBeenCalled());
    expect(fmOfLastSave()).toBe(seededFm); // byte-identical — no churn
  });

  // ADVERSARIAL (invariant 5) — a reconciled save must NOT re-fire. After a
  // materializing save the loop sets frontmatterRef.current AND
  // lastSavedFrontmatter.current to the reconciled value, so the single-flight
  // while-loop's condition is false and exactly ONE PUT is issued. A debounced
  // autosave (not a click) is used so the whole keystroke→reconcile→save path runs.
  it("an autosave that materializes a tag fires EXACTLY ONE save (no re-fire loop)", async () => {
    vi.useFakeTimers();
    const flush = async () => {
      for (let i = 0; i < 8; i++) {
        await act(async () => {
          await vi.advanceTimersByTimeAsync(0);
        });
      }
    };
    try {
      vi.mocked(client.getPage).mockResolvedValue({
        frontmatter: "title: Notes\n",
        body: "start",
        revision: "rev-1",
      });
      vi.mocked(client.savePage).mockResolvedValue(undefined);

      renderEditor("notes.md");
      await flush();
      const body = screen.getByLabelText("body");

      // Type a tag, then let the 1s debounce fire the autosave.
      fireEvent.change(body, { target: { value: "start #deploy" } });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000); // DRAFT_DEBOUNCE_MS
      });
      await flush();

      expect(client.savePage).toHaveBeenCalledTimes(1);
      expect(readTags(fmOfLastSave())).toContain("deploy");

      // Let any further timers/microtasks settle — the reconcile already advanced
      // lastSavedFrontmatter, so no spurious second save (no 409-thrash loop).
      await act(async () => {
        await vi.advanceTimersByTimeAsync(5000);
      });
      await flush();
      expect(client.savePage).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });

  // ADVERSARIAL (invariant 2, integration) — a page authored on disk with a QUOTED
  // block tag (yaml.v3 quotes reserved/numeric/special tags; data-open files may quote
  // freely) whose tag also appears inline must save BYTE-IDENTICAL frontmatter. Before
  // the readTags block-form quote-strip fix, `"deploy"` never matched the inline
  // `#deploy`, so every save appended a `- deploy` duplicate and 409-thrashed.
  it("a QUOTED block tag matching an inline tag saves byte-identical frontmatter (no 409 thrash)", async () => {
    const seededFm = 'title: Notes\ntags:\n  - "deploy"\n';
    vi.mocked(client.getPage).mockResolvedValue({
      frontmatter: seededFm,
      body: "has #deploy",
      revision: "rev-1",
    });
    vi.mocked(client.savePage).mockResolvedValue(undefined);

    renderEditor("notes.md");
    await screen.findByLabelText("body");
    fireEvent.click(screen.getByRole("button", { name: "Save page" }));

    await waitFor(() => expect(client.savePage).toHaveBeenCalled());
    expect(fmOfLastSave()).toBe(seededFm); // quotes preserved, no duplicate appended
  });

  // ED-02c — the badge ✕ on a non-inline (LLM/imported) tag must PERSIST the removal.
  // onRemoveTag schedules an autosave, so the removal is durable (not lost on navigate).
  it("clicking the badge ✕ removes a non-inline tag and persists it via autosave", async () => {
    vi.useFakeTimers();
    const flush = async () => {
      for (let i = 0; i < 8; i++) {
        await act(async () => {
          await vi.advanceTimersByTimeAsync(0);
        });
      }
    };
    try {
      vi.mocked(client.getPage).mockResolvedValue({
        frontmatter: "title: Notes\ntags:\n  - llm\n",
        body: "prose with no inline tags",
        revision: "rev-1",
      });
      vi.mocked(client.savePage).mockResolvedValue(undefined);

      renderEditor("notes.md");
      await flush();

      // `llm` is not inline in the body ⇒ the badge carries a Remove ✕.
      fireEvent.click(screen.getByRole("button", { name: "Remove tag llm" }));
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000); // DRAFT_DEBOUNCE_MS
      });
      await flush();

      expect(client.savePage).toHaveBeenCalled();
      expect(readTags(fmOfLastSave())).not.toContain("llm");
    } finally {
      vi.useRealTimers();
    }
  });

  // BLOCKER regression — a server-repaired page has 4-space frontmatter (yaml.v3
  // re-marshal). With a matching inline tag the save must be BYTE-IDENTICAL: the
  // reconcile reads the 4-space block correctly (no phantom-empty → no corruption).
  it("a server-repaired 4-space frontmatter with a matching inline tag saves byte-identical", async () => {
    const seededFm = "title: Notes\ntags:\n    - deploy\n"; // 4-space, as yaml.v3 emits
    vi.mocked(client.getPage).mockResolvedValue({
      frontmatter: seededFm,
      body: "has #deploy",
      revision: "rev-1",
    });
    vi.mocked(client.savePage).mockResolvedValue(undefined);

    renderEditor("notes.md");
    await screen.findByLabelText("body");
    fireEvent.click(screen.getByRole("button", { name: "Save page" }));

    await waitFor(() => expect(client.savePage).toHaveBeenCalled());
    expect(fmOfLastSave()).toBe(seededFm); // 4-space preserved, NOT corrupted
  });

  // ED-03a — the embedded TagSuggest applies via the server apply-tags path, then
  // PageEditor.reconcileAppliedTags makes the apply autosave-safe. These three tests
  // are the criterion-5 hard bar: apply → tag in frontmatter; apply → keep typing →
  // no 409 / no revert; an unsaved title is NOT lost by the reconcile.
  it("approving suggested tags lands the tag in frontmatter and renders it in PropertiesPanel (non-inline → removable)", async () => {
    vi.mocked(client.getPage)
      .mockResolvedValueOnce({
        frontmatter: "type: Page\ntitle: Notes\n",
        body: "original",
        revision: "rev-1",
      })
      .mockResolvedValue({
        // The server's post-apply view (reconcile fetches this for the fresh revision).
        frontmatter: "type: Page\ntitle: Notes\ntags:\n  - release\n",
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
    await screen.findByLabelText("body");

    fireEvent.click(screen.getByRole("button", { name: /suggest tags/i }));
    // `release` is existing → checked by default. Approve.
    fireEvent.click(await screen.findByRole("button", { name: /apply tags/i }));

    // After apply→reconcile the tag is in local frontmatter and rendered by
    // PropertiesPanel as a NON-INLINE (LLM) tag → it carries the Remove ✕ (it never
    // entered the body).
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Remove tag release" }),
      ).toBeInTheDocument(),
    );
  });

  it("autosave after applying tags uses the post-apply revision and does NOT 409 (no conflict dialog, no revert)", async () => {
    vi.useFakeTimers();
    const flush = async () => {
      for (let i = 0; i < 10; i++) {
        await act(async () => {
          await vi.advanceTimersByTimeAsync(0);
        });
      }
    };
    try {
      vi.mocked(client.getPage)
        .mockResolvedValueOnce({
          frontmatter: "type: Page\ntitle: Notes\n",
          body: "original",
          revision: "rev-1",
        })
        .mockResolvedValue({
          frontmatter: "type: Page\ntitle: Notes\ntags:\n  - release\n",
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

      // reconcile advanced baseRevision to the post-apply rev-2. Keep typing the body.
      const body = screen.getByLabelText("body");
      fireEvent.change(body, { target: { value: "original and more typing" } });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000); // DRAFT_DEBOUNCE_MS
      });
      await flush();

      // The autosave saved against the POST-APPLY revision (rev-2) — never a self-409.
      const calls = vi.mocked(client.savePage).mock.calls;
      expect(calls.length).toBeGreaterThan(0);
      const last = calls.at(-1)!;
      expect(last[1].base_revision).toBe("rev-2");
      // The applied tag rode along in the saved frontmatter (NOT reverted).
      expect(readTags(last[1].frontmatter)).toContain("release");
      // No conflict dialog opened (criterion 5: no 409 thrash).
      expect(
        screen.queryByText(/this page was changed somewhere else/i),
      ).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("an unsaved title typed BEFORE applying tags is NOT lost by the reconcile (no data loss)", async () => {
    vi.useFakeTimers();
    const flush = async () => {
      for (let i = 0; i < 10; i++) {
        await act(async () => {
          await vi.advanceTimersByTimeAsync(0);
        });
      }
    };
    try {
      vi.mocked(client.getPage)
        .mockResolvedValueOnce({
          frontmatter: "type: Page\ntitle: Notes\n",
          body: "original",
          revision: "rev-1",
        })
        .mockResolvedValue({
          // The server never saw the unsaved title — its post-apply view keeps "Notes".
          frontmatter: "type: Page\ntitle: Notes\ntags:\n  - release\n",
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

      // Type a NEW title but do NOT let its 1s autosave fire — apply happens inside
      // the debounce window, so the title is purely LOCAL (unsaved) at apply time.
      const title = screen.getByLabelText("Title");
      fireEvent.change(title, { target: { value: "New Title" } });

      fireEvent.click(screen.getByRole("button", { name: /suggest tags/i }));
      await flush(); // 0ms elapsed — the title autosave (1s) has NOT fired
      expect(client.savePage).not.toHaveBeenCalled(); // title still unsaved
      fireEvent.click(screen.getByRole("button", { name: /apply tags/i }));
      await flush();

      // reconcile merged the tag into the LOCAL frontmatter (which still holds the
      // unsaved title) and scheduled the convergence autosave. Let it fire.
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000);
      });
      await flush();

      // The save preserved BOTH the unsaved title AND the applied tag, against the
      // post-apply revision — the title was never overwritten by the server bytes.
      const last = vi.mocked(client.savePage).mock.calls.at(-1)!;
      expect(last[1].frontmatter).toContain("title: New Title");
      expect(readTags(last[1].frontmatter)).toContain("release");
      expect(last[1].base_revision).toBe("rev-2");
    } finally {
      vi.useRealTimers();
    }
  });

  // ─── Phase 24 (ED-01a/ED-01b) — LLM-generated description ──────────────────

  // ED-01a: the manual Description <input> is gone, and an existing description in
  // frontmatter is PRESERVED (okf.Repair never blanks it; nothing writes to it on a
  // plain body autosave) — not silently dropped by the editor.
  it("removes the Description input and preserves an existing description across a body autosave (ED-01a)", async () => {
    vi.useFakeTimers();
    const flush = async () => {
      for (let i = 0; i < 10; i++) {
        await act(async () => {
          await vi.advanceTimersByTimeAsync(0);
        });
      }
    };
    try {
      vi.mocked(client.getPage).mockResolvedValue({
        frontmatter: "type: Page\ntitle: Notes\ndescription: Existing blurb\n",
        body: "original",
        revision: "rev-1",
      });
      vi.mocked(client.savePage).mockResolvedValue(undefined);

      renderEditor("notes.md");
      await flush();

      // The manual Description input is gone; the generate affordance replaces it.
      expect(screen.queryByLabelText("Description")).toBeNull();
      expect(
        screen.getByRole("button", { name: /generate description/i }),
      ).toBeInTheDocument();

      // Type the body and let the autosave fire — the existing description rides along
      // unchanged (it is part of frontmatterRef, never blanked).
      const body = screen.getByLabelText("body");
      fireEvent.change(body, { target: { value: "original and more" } });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000);
      });
      await flush();

      const last = vi.mocked(client.savePage).mock.calls.at(-1)!;
      expect(last[1].frontmatter).toContain("description: Existing blurb");
    } finally {
      vi.useRealTimers();
    }
  });

  // ED-01b D-07: mounting the editor issues NO generate or apply call — generate fires
  // only on the explicit click, apply only on the explicit approve (never auto-runs).
  it("mounting issues no generate/apply call (never-auto, ED-01b)", async () => {
    vi.mocked(client.getPage).mockResolvedValue({
      frontmatter: "type: Page\ntitle: Notes\n",
      body: "original",
      revision: "rev-1",
    });
    renderEditor("notes.md");
    await screen.findByLabelText("body");
    // The trigger is present (editor) but nothing has been called.
    expect(
      screen.getByRole("button", { name: /generate description/i }),
    ).toBeInTheDocument();
    expect(client.generateDescription).not.toHaveBeenCalled();
    expect(client.applyDescription).not.toHaveBeenCalled();
  });

  // ED-01b criterion 4 (HARD BAR): generate → approve → keep typing the body → the
  // next autosave uses the POST-APPLY revision and does NOT 409 / does NOT revert the
  // applied description (merge-first reconcile + advanced baseRevision).
  it("autosave after applying a description uses the post-apply revision and does NOT 409 / does NOT revert it", async () => {
    vi.useFakeTimers();
    const flush = async () => {
      for (let i = 0; i < 10; i++) {
        await act(async () => {
          await vi.advanceTimersByTimeAsync(0);
        });
      }
    };
    try {
      vi.mocked(client.getPage)
        .mockResolvedValueOnce({
          frontmatter: "type: Page\ntitle: Notes\n",
          body: "original",
          revision: "rev-1",
        })
        .mockResolvedValue({
          // The server view after the apply (description present), post-apply rev-2.
          frontmatter: "type: Page\ntitle: Notes\ndescription: Draft desc\n",
          body: "original",
          revision: "rev-2",
        });
      vi.mocked(client.generateDescription).mockResolvedValue({
        page_path: "notes.md",
        description: "Draft desc",
        base_revision: "rev-1",
      });
      vi.mocked(client.applyDescription).mockResolvedValue(undefined);
      vi.mocked(client.savePage).mockResolvedValue(undefined);

      renderEditor("notes.md");
      await flush();

      fireEvent.click(
        screen.getByRole("button", { name: /generate description/i }),
      );
      await flush();
      fireEvent.click(screen.getByRole("button", { name: /^apply$/i }));
      await flush();

      // reconcile advanced baseRevision to the post-apply rev-2. Keep typing the body.
      const body = screen.getByLabelText("body");
      fireEvent.change(body, { target: { value: "original and more typing" } });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000);
      });
      await flush();

      const calls = vi.mocked(client.savePage).mock.calls;
      expect(calls.length).toBeGreaterThan(0);
      const last = calls.at(-1)!;
      // Saved against the POST-APPLY revision — never a self-409.
      expect(last[1].base_revision).toBe("rev-2");
      // The applied description rode along in the saved frontmatter (NOT reverted).
      expect(last[1].frontmatter).toContain("description: Draft desc");
      // No conflict dialog opened (no 409 thrash).
      expect(
        screen.queryByText(/changed somewhere else/i),
      ).toBeNull();
      expect(
        screen.queryByText(/overlapped with a save in progress/i),
      ).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  // D-08b — the targeted apply-vs-in-flight-autosave interleave Phase 23 lacked: a
  // body autosave is IN FLIGHT (held) when the description apply lands and advances
  // the server revision; releasing the held save 409s ON THE USER'S OWN APPLY. The
  // data must be safe (the approved description is in frontmatterRef, so Overwrite
  // preserves it) AND the conflict copy must be attributed accurately (D-08a) — never
  // the misleading "changed somewhere else".
  it("an apply landing while a body save is in flight 409s as OWN-apply, stays data-safe, and is attributed (D-08a/b)", async () => {
    vi.useFakeTimers();
    const flush = async () => {
      for (let i = 0; i < 12; i++) {
        await act(async () => {
          await vi.advanceTimersByTimeAsync(0);
        });
      }
    };
    try {
      vi.mocked(client.getPage)
        .mockResolvedValueOnce({
          frontmatter: "type: Page\ntitle: Notes\n",
          body: "original",
          revision: "rev-1",
        })
        // reconcile's getPage (apply committed) — EQUAL to the local merge so no
        // convergence autosave fires; advances baseRevision to rev-2.
        .mockResolvedValueOnce({
          frontmatter: "type: Page\ntitle: Notes\ndescription: Draft desc\n",
          body: "original",
          revision: "rev-2",
        })
        // the conflict-fetch + overwrite read-backs.
        .mockResolvedValue({
          frontmatter: "type: Page\ntitle: Notes\ndescription: Draft desc\n",
          body: "original",
          revision: "rev-3",
        });

      // The FIRST savePage (the body autosave) is held in flight, then REJECTED with a
      // 409 (its base_revision rev-1 is stale once the apply advanced to rev-2). The
      // subsequent Overwrite save succeeds.
      const conflict = Object.assign(new Error("conflict"), { status: 409 });
      let reject409: () => void = () => {};
      const held = new Promise<undefined>((_, rej) => {
        reject409 = () => rej(conflict);
      });
      vi.mocked(client.savePage)
        .mockImplementationOnce(() => held)
        .mockResolvedValue(undefined);

      vi.mocked(client.generateDescription).mockResolvedValue({
        page_path: "notes.md",
        description: "Draft desc",
        base_revision: "rev-1",
      });
      vi.mocked(client.applyDescription).mockResolvedValue(undefined);

      renderEditor("notes.md");
      await flush();

      // Type the body and let the 1s debounce start the (now-held) body save.
      const body = screen.getByLabelText("body");
      fireEvent.change(body, { target: { value: "original X" } });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000);
      });
      expect(client.savePage).toHaveBeenCalledTimes(1); // body save in flight

      // While that save is held, generate + apply a description → reconcile advances
      // baseRevision to rev-2 and merges the description into frontmatterRef.
      fireEvent.click(
        screen.getByRole("button", { name: /generate description/i }),
      );
      await flush();
      fireEvent.click(screen.getByRole("button", { name: /^apply$/i }));
      await flush();

      // Release the held body save → it 409s ON THE OWN APPLY.
      reject409();
      await flush();

      // D-08a: attributed as the user's own apply, NOT the misleading external copy.
      // (getByText, not findByText — under fake timers findByText's polling never
      // advances; the conflict opens synchronously once flush drains the catch.)
      expect(
        screen.getByText(/your change overlapped with a save in progress/i),
      ).toBeInTheDocument();
      expect(screen.queryByText(/changed somewhere else/i)).toBeNull();

      // Resolve via Overwrite — the approved description is PRESERVED (it was merged
      // into frontmatterRef before the conflict could arise).
      fireEvent.click(
        screen.getByRole("button", { name: /overwrite with my version/i }),
      );
      await flush();

      const last = vi.mocked(client.savePage).mock.calls.at(-1)!;
      expect(last[1].frontmatter).toContain("description: Draft desc");
    } finally {
      vi.useRealTimers();
    }
  });
});
