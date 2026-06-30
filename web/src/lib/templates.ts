// Template placeholder substitution (ED-06a, D-03). Obsidian-parity: a template
// is plain text in which exactly THREE placeholders are filled — {{title}},
// {{date}}, {{time}} — via String.replaceAll + Intl.DateTimeFormat (user-local
// date/time). There is NO templating engine: any other {{…}} text is left
// LITERAL, and substitution is a pure string replace that can never execute code.
//
// The authoritative frontmatter merge happens SERVER-side via okf (D-04); this
// only fills placeholders before the (already-substituted) content is POSTed.

// formatDate renders the user-local calendar date as YYYY-MM-DD. It builds the
// string from Intl.DateTimeFormat parts so it is locale-proof (the locale only
// coerces numbering; the timezone defaults to the user's local zone).
function formatDate(now: Date): string {
  const parts = new Intl.DateTimeFormat("en-CA", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(now);
  const get = (type: string) => parts.find((p) => p.type === type)?.value ?? "";
  return `${get("year")}-${get("month")}-${get("day")}`;
}

// formatTime renders the user-local time as HH:mm (24h). hourCycle "h23" pins
// the hour to 00–23 so midnight is "00:00", never the "24:00" some engines emit.
function formatTime(now: Date): string {
  const parts = new Intl.DateTimeFormat("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).formatToParts(now);
  const get = (type: string) => parts.find((p) => p.type === type)?.value ?? "";
  return `${get("hour")}:${get("minute")}`;
}

// substitutePlaceholders replaces {{date}}/{{time}} (user-local) and — only when
// a title is supplied — {{title}} in text. Every occurrence is replaced
// (replaceAll); any other {{…}} is left untouched. Pure, no eval.
//
// title is OPTIONAL on purpose: it is substituted into the page BODY (opaque
// Markdown, where a raw title is always safe) but NOT into the frontmatter. The
// canonical frontmatter `title:` is owned by the SERVER's okf merge (SetField,
// which yaml-quotes it correctly), so injecting a raw title into a YAML value
// position would corrupt valid templates — a common title like "Meeting: Kickoff"
// would turn `title: {{title}}` into invalid YAML. Callers substitute title in
// the body and omit it for the frontmatter; {{date}}/{{time}} are YAML-safe
// scalars and substituted in both.
export function substitutePlaceholders(
  text: string,
  vars: { title?: string; now: Date },
): string {
  let out = text;
  if (vars.title !== undefined) {
    out = out.replaceAll("{{title}}", vars.title);
  }
  return out
    .replaceAll("{{date}}", formatDate(vars.now))
    .replaceAll("{{time}}", formatTime(vars.now));
}
