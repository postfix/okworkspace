/**
 * navSearch store (RS-02) — the navrail search field's query text, lifted into a
 * store so the fixed header (input) and scrolling body (results) share it.
 * Contract: the query starts empty, setQuery sets it, and clearing it back to ""
 * is what makes AppShell render the LeftTree instead of NavSearchResults
 * (tree-returns-when-empty, AppShell:716-720). Pure store — no React.
 */
import { describe, it, expect, beforeEach } from "vitest";

import { useNavSearch } from "./navSearch";

describe("navSearch store (RS-02)", () => {
  beforeEach(() => {
    useNavSearch.getState().setQuery("");
  });

  it("starts with an empty query", () => {
    expect(useNavSearch.getState().query).toBe("");
  });

  it("setQuery stores the text", () => {
    useNavSearch.getState().setQuery("hello");
    expect(useNavSearch.getState().query).toBe("hello");
  });

  it("setQuery('') clears the query back to empty (tree-returns-when-empty)", () => {
    useNavSearch.getState().setQuery("hello");
    expect(useNavSearch.getState().query).toBe("hello");
    useNavSearch.getState().setQuery("");
    expect(useNavSearch.getState().query).toBe("");
  });
});
