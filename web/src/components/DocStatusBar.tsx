import { useMemo } from "react";

import { useBacklinks } from "../hooks/useBacklinks";
import "./DocStatusBar.css";

// DocStatusBar is the Obsidian-style footer strip under a page: backlink count,
// word count, and character count. Counts are derived from the current body (live
// in the editor, committed in the read view); backlinks reuse the same useBacklinks
// query the BacklinksPanel already loads, so no extra request is made.
//
// Word counting mirrors Obsidian's simple whitespace tokenizer (runs of non-space
// separated by whitespace) — good enough for a status readout; it is NOT a
// linguistic count and intentionally counts markdown punctuation-words as words.
function countWords(body: string): number {
  const trimmed = body.trim();
  if (trimmed === "") return 0;
  return trimmed.split(/\s+/).length;
}

export default function DocStatusBar({
  path,
  body,
}: {
  path: string;
  body: string;
}) {
  const { data: backlinks } = useBacklinks(path);
  const words = useMemo(() => countWords(body), [body]);
  const chars = body.length;
  const backlinkCount = backlinks?.length ?? 0;

  const plural = (n: number, one: string) => `${n} ${one}${n === 1 ? "" : "s"}`;

  return (
    <div className="docstatusbar" aria-label="Page statistics">
      <span className="docstatusbar-item">{plural(backlinkCount, "backlink")}</span>
      <span className="docstatusbar-sep" aria-hidden="true">
        ·
      </span>
      <span className="docstatusbar-item">{plural(words, "word")}</span>
      <span className="docstatusbar-sep" aria-hidden="true">
        ·
      </span>
      <span className="docstatusbar-item">
        {chars.toLocaleString()} character{chars === 1 ? "" : "s"}
      </span>
    </div>
  );
}
