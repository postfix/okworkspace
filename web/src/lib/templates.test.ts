import { describe, expect, it } from "vitest";

import { substitutePlaceholders } from "./templates";

describe("substitutePlaceholders (ED-06a / D-03)", () => {
  // A fixed instant; assertions derive the EXPECTED local date/time from the
  // same Intl APIs so the test is timezone-independent (it asserts the function
  // matches Intl's user-local formatting, not a hard-coded zone).
  const now = new Date("2026-06-30T13:05:00Z");
  const expectedDate = (() => {
    const p = new Intl.DateTimeFormat("en-CA", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
    }).formatToParts(now);
    const g = (t: string) => p.find((x) => x.type === t)?.value ?? "";
    return `${g("year")}-${g("month")}-${g("day")}`;
  })();
  const expectedTime = (() => {
    const p = new Intl.DateTimeFormat("en-GB", {
      hour: "2-digit",
      minute: "2-digit",
      hourCycle: "h23",
    }).formatToParts(now);
    const g = (t: string) => p.find((x) => x.type === t)?.value ?? "";
    return `${g("hour")}:${g("minute")}`;
  })();

  it("replaces the three placeholders", () => {
    const out = substitutePlaceholders(
      "# {{title}}\n\nCreated {{date}} at {{time}}.",
      { title: "My Page", now },
    );
    expect(out).toBe(`# My Page\n\nCreated ${expectedDate} at ${expectedTime}.`);
  });

  it("date is YYYY-MM-DD and time is HH:mm (24h)", () => {
    expect(expectedDate).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    expect(expectedTime).toMatch(/^\d{2}:\d{2}$/);
    const out = substitutePlaceholders("{{date}} {{time}}", {
      title: "x",
      now,
    });
    expect(out).toBe(`${expectedDate} ${expectedTime}`);
  });

  it("replaces every occurrence (replaceAll)", () => {
    const out = substitutePlaceholders("{{title}} / {{title}} / {{title}}", {
      title: "T",
      now,
    });
    expect(out).toBe("T / T / T");
  });

  it("leaves any other {{…}} placeholder LITERAL (no engine)", () => {
    const out = substitutePlaceholders(
      "{{title}} {{author}} {{custom_field}} {{ title }}",
      { title: "Real", now },
    );
    // Only the exact {{title}} token is replaced; spaced/unknown tokens are kept.
    expect(out).toBe("Real {{author}} {{custom_field}} {{ title }}");
  });

  it("passes through text with no placeholders", () => {
    const text = "Plain body with no placeholders.";
    expect(substitutePlaceholders(text, { title: "x", now })).toBe(text);
  });

  it("does not execute or interpret the title value (pure replace)", () => {
    const out = substitutePlaceholders("{{title}}", {
      title: "${alert(1)} <script>",
      now,
    });
    expect(out).toBe("${alert(1)} <script>");
  });

  // Frontmatter call (title omitted): {{title}} is LEFT LITERAL so a raw title
  // is never injected into a YAML value position (the server owns the frontmatter
  // title). {{date}}/{{time}} are still substituted.
  it("omitting title leaves {{title}} literal but still fills date/time", () => {
    const out = substitutePlaceholders("title: {{title}}\ncreated: {{date}}", {
      now,
    });
    expect(out).toBe(`title: {{title}}\ncreated: ${expectedDate}`);
  });

  // A colon-bearing title in the BODY is safe (opaque Markdown), proving the fix:
  // the title goes in the body, not the frontmatter.
  it("substitutes a colon-bearing title into the body verbatim", () => {
    const out = substitutePlaceholders("# {{title}}", {
      title: "Meeting: Kickoff",
      now,
    });
    expect(out).toBe("# Meeting: Kickoff");
  });
});
