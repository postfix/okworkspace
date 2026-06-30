import { readField } from "../lib/frontmatter";

/**
 * Read tags from a raw YAML frontmatter string.
 * Handles both block-sequence (`- tag` per line) and flow-sequence (`[a, b]`).
 */
export function readTags(frontmatter: string): string[] {
  // Block sequence — `tags:` then one or more indented `- item` lines at ANY indent.
  // The app emits 2 spaces, but the SERVER re-marshals frontmatter via yaml.v3 at
  // 4-space indent whenever Repair adds a missing required field (internal/okf/emit.go
  // → yaml.Marshal, no SetIndent), and a hand-authored / imported file (data-openness)
  // may use any valid indent. A 2-space-only capture read a 4-space block as EMPTY, so
  // the reconcile thought the page had no tags and corrupted it on the first autosave.
  const tagsSection = frontmatter.match(/^tags:[ \t]*\n((?:[ \t]*-[ \t].*\n?)+)/m);
  if (!tagsSection) {
    // Try flow sequence: tags: [a, b]
    const flowMatch = frontmatter.match(/^tags:\s*\[(.*)\]/m);
    if (flowMatch) {
      return flowMatch[1]
        .split(",")
        .map((t) => t.trim().replace(/^['"]|['"]$/g, ""))
        .filter(Boolean);
    }
    // Try scalar: tags: single-tag
    const scalar = readField(frontmatter, "tags");
    return scalar ? [scalar] : [];
  }
  const lines = tagsSection[1].split("\n");
  return lines
    .map((line) =>
      // Strip the `- ` sequence marker, then the SAME surrounding-quote strip the
      // flow branch applies. A block item is often quoted (`- "deploy"`, `- 'true'`)
      // — yaml.v3 quotes any reserved-word/numeric/special tag, and a hand-authored
      // file (data-openness) may quote freely. Without this, a quoted block tag read
      // as `"deploy"` would never equal an inline `#deploy`, so the reconcile would
      // spuriously rewrite + duplicate it on every save (byte-stability violation).
      line
        .replace(/^\s*-\s*/, "")
        .trim()
        .replace(/^['"]|['"]$/g, ""),
    )
    .filter(Boolean);
}

/**
 * Case-insensitive union of two tag lists. Normalizes every tag to trimmed +
 * lower-cased, de-dupes, and preserves `existing` order first then appends the new
 * tags from `added` in their given order. Mirrors the `norm` (trim+lowercase) the
 * body-save reconcile (`reconcileFrontmatterTags`) applies, so an apply-tags merge and
 * a subsequent autosave agree on the tag SET byte-for-byte and never churn each other.
 */
export function unionTags(existing: string[], added: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const raw of [...existing, ...added]) {
    const t = raw.trim().toLowerCase();
    if (!t || seen.has(t)) continue;
    seen.add(t);
    out.push(t);
  }
  return out;
}

/**
 * Replace the tags section in a raw YAML frontmatter string.
 * Emits block-sequence format: `tags:\n  - tag1\n  - tag2\n`
 */
export function setTags(frontmatter: string, tags: string[]): string {
  const block = tags.map((t) => `  - ${t}`).join("\n");
  const replacement = `tags:\n${block}`;

  // Replace an existing block sequence at ANY indent (a server-repaired page is
  // 4-space; see readTags). Matching the indent-agnostic item lines is what makes the
  // whole old block get CONSUMED — a 2-space-only replace left the 4-space item lines
  // orphaned after the new header, which yaml.v3 then folded into a corrupt scalar.
  const blockRe = /^tags:[ \t]*\n(?:[ \t]*-[ \t].*\n?)+/m;
  if (blockRe.test(frontmatter)) {
    return frontmatter.replace(blockRe, replacement + "\n");
  }
  // Replace flow sequence: tags: [a, b]
  if (/^tags:\s*\[.*\]/m.test(frontmatter)) {
    return frontmatter.replace(/^tags:\s*\[.*\]/m, replacement);
  }
  // Replace scalar: tags: value
  if (/^tags:.*$/m.test(frontmatter)) {
    return frontmatter.replace(/^tags:.*$/m, replacement);
  }
  // Append
  const sep = frontmatter.endsWith("\n") || frontmatter === "" ? "" : "\n";
  return `${frontmatter}${sep}${replacement}\n`;
}
