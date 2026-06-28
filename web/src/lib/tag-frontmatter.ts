import { readField } from "../lib/frontmatter";

/**
 * Read tags from a raw YAML frontmatter string.
 * Handles both block-sequence (`- tag` per line) and flow-sequence (`[a, b]`).
 */
export function readTags(frontmatter: string): string[] {
  const tagsSection = frontmatter.match(/^tags:\s*\n((?:  - .*\n?)*)/m);
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
    .map((line) => line.replace(/^\s*-\s*/, "").trim())
    .filter(Boolean);
}

/**
 * Replace the tags section in a raw YAML frontmatter string.
 * Emits block-sequence format: `tags:\n  - tag1\n  - tag2\n`
 */
export function setTags(frontmatter: string, tags: string[]): string {
  const block = tags.map((t) => `  - ${t}`).join("\n");
  const replacement = `tags:\n${block}`;

  // Replace existing tags block (block sequence)
  if (/^tags:\s*\n(?:  - .*\n?)*/m.test(frontmatter)) {
    return frontmatter.replace(
      /^tags:\s*\n(?:  - .*\n?)*/m,
      replacement + "\n",
    );
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
