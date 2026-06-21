// EDIT-04 — sanitizeImageSrc image-src allowlist (T-06-01). Asserts that exec/XSS
// schemes are blocked and that http(s) + app-relative paths pass through verbatim.
import { describe, it, expect } from "vitest";

import { sanitizeImageSrc } from "./sanitizeSrc";

describe("sanitizeImageSrc (EDIT-04 / T-06-01)", () => {
  it("blocks javascript: and vbscript: schemes", () => {
    expect(sanitizeImageSrc("javascript:alert(1)")).toBeNull();
    expect(sanitizeImageSrc("JavaScript:alert(1)")).toBeNull();
    expect(sanitizeImageSrc("vbscript:msgbox(1)")).toBeNull();
    // whitespace-padded scheme still classified by scheme, not bypassed
    expect(sanitizeImageSrc("  javascript:alert(1)  ")).toBeNull();
  });

  it("blocks executable/opaque data: URLs", () => {
    expect(sanitizeImageSrc("data:text/html,<script>alert(1)</script>")).toBeNull();
    expect(
      sanitizeImageSrc("data:image/svg+xml;base64,PHN2Zz48L3N2Zz4="),
    ).toBeNull();
  });

  it("blocks other non-http schemes (file:, ftp:, etc.)", () => {
    expect(sanitizeImageSrc("file:///etc/passwd")).toBeNull();
    expect(sanitizeImageSrc("ftp://host/x.png")).toBeNull();
  });

  it("blocks empty / whitespace-only src", () => {
    expect(sanitizeImageSrc("")).toBeNull();
    expect(sanitizeImageSrc("   ")).toBeNull();
    expect(sanitizeImageSrc("\t\n")).toBeNull();
  });

  it("blocks protocol-relative (//host) src", () => {
    expect(sanitizeImageSrc("//evil.example/x.png")).toBeNull();
  });

  it("passes through http and https URLs", () => {
    expect(sanitizeImageSrc("http://example.com/logo.png")).toBe(
      "http://example.com/logo.png",
    );
    expect(sanitizeImageSrc("https://example.com/logo.png")).toBe(
      "https://example.com/logo.png",
    );
    expect(sanitizeImageSrc("HTTPS://example.com/logo.png")).toBe(
      "HTTPS://example.com/logo.png",
    );
  });

  it("passes through app-relative / attachment paths (no scheme)", () => {
    expect(sanitizeImageSrc("./assets/logo.png")).toBe("./assets/logo.png");
    expect(sanitizeImageSrc("../images/x.png")).toBe("../images/x.png");
    expect(sanitizeImageSrc("/attachments/2026/diagram.svg")).toBe(
      "/attachments/2026/diagram.svg",
    );
    expect(sanitizeImageSrc("assets/logo.png")).toBe("assets/logo.png");
  });

  it("trims surrounding whitespace on allowed values", () => {
    expect(sanitizeImageSrc("  ./assets/logo.png  ")).toBe("./assets/logo.png");
    expect(sanitizeImageSrc(" https://example.com/x.png ")).toBe(
      "https://example.com/x.png",
    );
  });
});
