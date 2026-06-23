import { describe, it, expect, beforeEach } from "vitest";

import { useRecent } from "./recent";

describe("recent store (NAV-05)", () => {
  beforeEach(() => {
    localStorage.clear();
    useRecent.getState().clear();
  });

  it("pushes an opened page to the front, most-recent-first", () => {
    const { visit } = useRecent.getState();
    visit({ path: "a.md", title: "A" });
    visit({ path: "b.md", title: "B" });
    const recents = useRecent.getState().recents;
    expect(recents.map((r) => r.path)).toEqual(["b.md", "a.md"]);
  });

  it("de-dupes by path (re-visiting moves it to the front)", () => {
    const { visit } = useRecent.getState();
    visit({ path: "a.md", title: "A" });
    visit({ path: "b.md", title: "B" });
    visit({ path: "a.md", title: "A" });
    const recents = useRecent.getState().recents;
    expect(recents.map((r) => r.path)).toEqual(["a.md", "b.md"]);
  });

  it("caps the list length", () => {
    const { visit } = useRecent.getState();
    for (let i = 0; i < 20; i++) {
      visit({ path: `p${i}.md`, title: `P${i}` });
    }
    expect(useRecent.getState().recents.length).toBeLessThanOrEqual(8);
    // The most recent visit is at the front.
    expect(useRecent.getState().recents[0].path).toBe("p19.md");
  });

  it("persists to localStorage", () => {
    useRecent.getState().visit({ path: "x.md", title: "X" });
    const raw = localStorage.getItem("okf-recent-pages");
    expect(raw).toBeTruthy();
    expect(raw).toContain("x.md");
  });
});
