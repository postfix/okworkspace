// Lightweight frontmatter helpers for the editor/read surfaces. The canonical
// frontmatter is the raw YAML text the backend returns (byte-stable, edited as
// text); these helpers read/patch only the simple top-level scalar fields the
// frontmatter form exposes (title/tags/description). Anything more structural is
// left to the raw-text round-trip on the server (okf), never re-serialized here.

// okfTitle returns the `title:` value from a raw frontmatter block, falling back
// to a human-readable name derived from the page path (filename without .md).
export function okfTitle(frontmatter: string, path: string): string {
  const m = frontmatter.match(/^title:\s*(.*)$/m);
  if (m) {
    const v = stripQuotes(m[1].trim());
    if (v !== "") return v;
  }
  const base = path.split("/").pop() ?? path;
  return base.replace(/\.md$/, "");
}

// readField returns a top-level scalar field's value, or "" when absent.
export function readField(frontmatter: string, field: string): string {
  const re = new RegExp(`^${field}:\\s*(.*)$`, "m");
  const m = frontmatter.match(re);
  return m ? stripQuotes(m[1].trim()) : "";
}

// setField sets (or appends) a top-level scalar field in a raw frontmatter
// block, preserving the rest of the YAML text. Used by the frontmatter form so a
// title/description edit does not disturb other keys.
export function setField(frontmatter: string, field: string, value: string): string {
  const re = new RegExp(`^(${field}:)\\s*.*$`, "m");
  const line = `${field}: ${quoteIfNeeded(value)}`;
  if (re.test(frontmatter)) {
    return frontmatter.replace(re, line);
  }
  const sep = frontmatter.endsWith("\n") || frontmatter === "" ? "" : "\n";
  return `${frontmatter}${sep}${line}\n`;
}

function stripQuotes(s: string): string {
  if (
    (s.startsWith('"') && s.endsWith('"')) ||
    (s.startsWith("'") && s.endsWith("'"))
  ) {
    return s.slice(1, -1);
  }
  return s;
}

// YAML 1.1 reserved scalars that, left unquoted, parse back as a bool or null
// instead of the literal string (so a title of "null" or "yes" must be quoted).
const YAML_RESERVED_WORDS =
  /^(?:true|false|null|yes|no|on|off|~)$/i;

// Numeric-looking scalars (ints, floats, scientific notation, sign-prefixed)
// that would round-trip as a number rather than a string.
const YAML_NUMERIC = /^[+-]?(?:\d[\d_]*(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/;

// quoteIfNeeded emits a frontmatter scalar UNQUOTED only when it is a safe plain
// YAML scalar; otherwise it double-quotes (escaping `\` then `"`). Over-quoting
// is always valid YAML — under-quoting is the bug we are avoiding (CR-02). The
// value is wrapped in double quotes when it:
//   - is empty;
//   - has leading or trailing whitespace (plain scalars get trimmed);
//   - begins with a YAML indicator char (! & * [ ] { } # | > @ ` " ' % , ? : - ~ =)
//     or with "- " / ": " style sequences;
//   - contains ": " (colon+space) or " #" (space+hash) anywhere (map / comment
//     ambiguity);
//   - contains a quote char (" or ') anywhere, which makes a plain scalar
//     ambiguous and (for ") is the char our reader strips;
//   - matches a reserved word (true/false/null/yes/no/on/off/~) or looks numeric,
//     either of which would be read back as a non-string.
function quoteIfNeeded(value: string): string {
  const needsQuote =
    value === "" ||
    /^\s|\s$/.test(value) ||
    /^[!&*[\]{}#|>@`"'%,?:\-~=]/.test(value) ||
    value.includes(": ") ||
    value.includes(" #") ||
    /["']/.test(value) ||
    YAML_RESERVED_WORDS.test(value) ||
    YAML_NUMERIC.test(value);

  if (needsQuote) {
    return `"${value.replace(/\\/g, "\\\\").replace(/"/g, '\\"')}"`;
  }
  return value;
}
