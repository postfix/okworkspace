// EDIT-01 — live-preview ViewPlugin decoration coverage. The ViewPlugin that walks
// the Lezer markdown tree and emits decorations (Decoration.mark / .replace /
// widget) for each construct ships in 06-02; this file is the RUNNABLE Wave-0 stub
// so the EDIT-01 verify command (`vitest run src/lib/cm/livePreview.test.ts`)
// exists now. 06-02 fills these `it.todo` bodies with real assertions that the
// plugin emits the expected decoration kinds for each input.
//
// Each placeholder names the construct + the decoration kind 06-02 must assert, so
// the coverage contract is explicit before the feature lands. These are todos (not
// skips) so they surface as pending in the report rather than silently passing.
import { describe, it } from "vitest";

describe("livePreview decorations (EDIT-01) — 06-02 fills these", () => {
  it.todo("heading line: ATXHeading* → hide HeaderMark, style the line");
  it.todo("bold: StrongEmphasis → cm-strong mark, hide EmphasisMark run");
  it.todo("italic: Emphasis → cm-em mark, hide EmphasisMark run");
  it.todo("inline code: InlineCode → cm-code mark, hide CodeMark backticks");
  it.todo("code block: FencedCode → styled block, fences kept");
  it.todo("link: Link → cm-md-link mark, hide LinkMark/URL on inactive line");
  it.todo("image: Image → ImageWidget replace decoration (sanitized src)");
  it.todo("GFM table: Table → block widget grid (reveal-to-source on edit)");
  it.todo("list: BulletList/OrderedList → ListMark styling");
  it.todo("active-line reveal: decorations under the selection are dropped");
});
