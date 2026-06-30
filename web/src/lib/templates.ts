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

// substitutePlaceholders replaces {{title}}/{{date}}/{{time}} in text with the
// given title and the user-local date/time derived from now. Every occurrence is
// replaced (replaceAll); any other {{…}} is left untouched. Pure, no eval.
export function substitutePlaceholders(
  text: string,
  vars: { title: string; now: Date },
): string {
  return text
    .replaceAll("{{title}}", vars.title)
    .replaceAll("{{date}}", formatDate(vars.now))
    .replaceAll("{{time}}", formatTime(vars.now));
}
