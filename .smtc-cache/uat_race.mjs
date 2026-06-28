// COLL-01/02/04 race harness — drives two authenticated sessions against the
// live server (localhost:8098) via the real HTTP/SSE API. No external deps.
const BASE = "http://localhost:8098";
const PAGE = "deploy.md"; // existing page; COLL-04 saves new versions (git-versioned, reversible)

function parseSetCookie(res, jar) {
  // Node fetch exposes getSetCookie() (undici). Fall back to raw header.
  const all = res.headers.getSetCookie ? res.headers.getSetCookie() : [];
  for (const sc of all) {
    const [pair] = sc.split(";");
    const idx = pair.indexOf("=");
    if (idx > 0) jar[pair.slice(0, idx).trim()] = pair.slice(idx + 1).trim();
  }
}
function cookieHeader(jar) {
  return Object.entries(jar).map(([k, v]) => `${k}=${v}`).join("; ");
}

class Session {
  constructor(name) { this.name = name; this.jar = {}; this.csrf = null; }
  async req(method, path, body, extraHeaders = {}) {
    // nosurf v1.2.0 enforces same-origin via Sec-Fetch-Site / Origin (a browser
    // sends these; Node fetch does not), so set them explicitly.
    const headers = { "Origin": BASE, "Sec-Fetch-Site": "same-origin", ...extraHeaders };
    if (Object.keys(this.jar).length) headers["Cookie"] = cookieHeader(this.jar);
    if (body !== undefined) headers["Content-Type"] = "application/json";
    if (method !== "GET" && this.csrf) headers["X-CSRF-Token"] = this.csrf;
    const res = await fetch(BASE + path, {
      method, headers, body: body !== undefined ? JSON.stringify(body) : undefined, redirect: "manual",
    });
    parseSetCookie(res, this.jar);
    return res;
  }
  async getCSRF() {
    const res = await this.req("GET", "/api/v1/csrf");
    const j = await res.json();
    this.csrf = j.csrf_token;
    return this.csrf;
  }
  async login(username, password) {
    await this.getCSRF();
    const res = await this.req("POST", "/api/v1/auth/login", { username, password });
    if (res.status !== 200) throw new Error(`${this.name} login ${res.status}: ${await res.text()}`);
    await this.getCSRF(); // refresh token bound to the now-authenticated csrf cookie
    const me = await (await this.req("GET", "/api/v1/auth/me")).json();
    return me;
  }
  // Read exactly one SSE presence frame, then abort.
  async presenceOnce(conn) {
    const ac = new AbortController();
    const headers = { Cookie: cookieHeader(this.jar) };
    const res = await fetch(`${BASE}/api/v1/pages/${PAGE}/presence?conn=${encodeURIComponent(conn)}`,
      { headers, signal: ac.signal });
    const reader = res.body.getReader();
    const dec = new TextDecoder();
    let buf = "";
    try {
      while (true) {
        const { value, done } = await reader.read();
        if (done) break;
        buf += dec.decode(value, { stream: true });
        const m = buf.match(/data: (.*)\n\n/);
        if (m) { ac.abort(); return JSON.parse(m[1]); }
      }
    } catch (e) { /* aborted */ }
    return null;
  }
}

const results = [];
function check(label, cond, detail) {
  results.push({ label, ok: !!cond, detail });
  console.log(`${cond ? "  ✓" : "  ✗"} ${label}${detail ? ` — ${detail}` : ""}`);
}

const A = new Session("admin");
const B = new Session("ed");

const meA = await A.login("admin", "testpass1234");
const meB = await B.login("ed", "testpass1234");
console.log(`Logged in: A=${meA.username}(${meA.role})  B=${meB.username}(${meB.role})\n`);

const connA = "conn-admin-1";
const connB = "conn-ed-1";

// Clean slate: a leftover lock from a prior real session won't match our conn on
// release (Release only deletes a SessionID-matching lock), so force it to connA
// first (force always wins), then release that — leaving the page lock-free.
await A.req("POST", `/api/v1/pages/${PAGE}/lock/force`, { conn: connA });
await A.req("POST", `/api/v1/pages/${PAGE}/lock/release`, { conn: connA });

// ─────────────────────────────────────────────────────────────────────────
console.log("TEST 1 — COLL-01 cross-session presence");
// admin enters edit → acquires the lock.
const acqA = await (await A.req("POST", `/api/v1/pages/${PAGE}/lock`, { conn: connA })).json();
check("admin acquires lock", acqA.result === "acquired", `result=${acqA.result}`);

const presA = await A.presenceOnce(connA);
check("holder (admin) sees only itself, you=true (UI shows no name)",
  presA && presA.editors.length === 1 && presA.editors[0].username === "admin" && presA.editors[0].you === true && presA.you_hold_lock === true,
  JSON.stringify(presA));

const presB = await B.presenceOnce(connB);
check("other session (ed) sees 'admin is editing', you=false",
  presB && presB.editors.length === 1 && presB.editors[0].username === "admin" && presB.editors[0].you === false && presB.you_hold_lock === false,
  JSON.stringify(presB));

// admin leaves → lock released → presence clears for ed.
await A.req("POST", `/api/v1/pages/${PAGE}/lock/release`, { conn: connA });
const presBafter = await B.presenceOnce(connB);
check("presence clears after holder leaves (ed sees empty)",
  presBafter && (presBafter.editors == null || presBafter.editors.length === 0) && presBafter.you_hold_lock === false,
  JSON.stringify(presBafter));

// ─────────────────────────────────────────────────────────────────────────
console.log("\nTEST 2 — COLL-02 soft-lock + force-edit");
const acqB = await (await B.req("POST", `/api/v1/pages/${PAGE}/lock`, { conn: connB })).json();
check("ed acquires lock", acqB.result === "acquired", `result=${acqB.result}`);

const acqAblocked = await (await A.req("POST", `/api/v1/pages/${PAGE}/lock`, { conn: connA })).json();
check("admin sees held-by-other (→ SoftLockBanner, read-only)",
  acqAblocked.result === "held-by-other" && acqAblocked.holder && acqAblocked.holder.username === "ed",
  JSON.stringify(acqAblocked));

const forceA = await A.req("POST", `/api/v1/pages/${PAGE}/lock/force`, { conn: connA });
check("admin force-edit succeeds (200)", forceA.status === 200, `status=${forceA.status}`);

const presAforced = await A.presenceOnce(connA);
check("after force, admin holds the lock",
  presAforced && presAforced.you_hold_lock === true && presAforced.editors[0].username === "admin",
  JSON.stringify(presAforced));
const presBlost = await B.presenceOnce(connB);
check("ed no longer holds the lock (you_hold_lock=false)",
  presBlost && presBlost.you_hold_lock === false,
  JSON.stringify(presBlost));
await A.req("POST", `/api/v1/pages/${PAGE}/lock/release`, { conn: connA });

// ─────────────────────────────────────────────────────────────────────────
console.log("\nTEST 3 — COLL-04 three-way save conflict");
const page0A = await (await A.req("GET", `/api/v1/pages/${PAGE}`)).json();
const page0B = await (await B.req("GET", `/api/v1/pages/${PAGE}`)).json();
const baseRev = page0A.revision;
check("both sessions load the same base_revision", baseRev && baseRev === page0B.revision, `rev=${baseRev}`);

const origBody = page0A.body;
const origFm = page0A.frontmatter ?? "";

// admin saves first → new revision.
const saveA = await A.req("PUT", `/api/v1/pages/${PAGE}`,
  { body: origBody + "\n\n<!-- edited by admin -->", frontmatter: origFm, base_revision: baseRev });
check("admin saves first (200)", [200,204].includes(saveA.status), `status=${saveA.status}`);

// ed saves with the now-stale base_revision → 409 conflict (→ DiffReviewDialog).
const saveBstale = await B.req("PUT", `/api/v1/pages/${PAGE}`,
  { body: origBody + "\n\n<!-- edited by ed -->", frontmatter: origFm, base_revision: baseRev });
check("ed's stale save is rejected with 409 (→ conflict dialog)", saveBstale.status === 409, `status=${saveBstale.status}`);
const conflictMsg = await saveBstale.text();
check("409 carries a human conflict message", /changed somewhere else|reload/i.test(conflictMsg), conflictMsg.slice(0, 80));

// Branch: "Save as copy" — create a new page, original untouched.
const copyRes = await B.req("POST", "/api/v1/pages", { folder: "", title: "deploy (ed conflict copy)" });
check("'Save as copy' branch creates a new page", copyRes.status === 200 || copyRes.status === 201, `status=${copyRes.status}`);
let copyPath = null;
if (copyRes.ok) { copyPath = (await copyRes.json()).path; }

// Branch: "Overwrite" — ed reloads current revision, then saves on top (no silent loss; explicit).
const cur = await (await B.req("GET", `/api/v1/pages/${PAGE}`)).json();
check("admin's first save is the live content (no silent loss)", /edited by admin/.test(cur.body), "admin marker present pre-overwrite");
const overwrite = await B.req("PUT", `/api/v1/pages/${PAGE}`,
  { body: cur.body + "\n<!-- ed overwrite -->", frontmatter: cur.frontmatter ?? "", base_revision: cur.revision });
check("'Overwrite' branch succeeds once revision is current (200)", [200,204].includes(overwrite.status), `status=${overwrite.status}`);

// Restore the page to its original content so the test is side-effect-light.
const live = await (await A.req("GET", `/api/v1/pages/${PAGE}`)).json();
const restore = await A.req("PUT", `/api/v1/pages/${PAGE}`,
  { body: origBody, frontmatter: origFm, base_revision: live.revision });
check("page restored to original body", [200,204].includes(restore.status), `status=${restore.status}`);

// Clean up the conflict-copy page if one was created.
if (copyPath) { await B.req("DELETE", `/api/v1/pages/${copyPath}`); console.log(`  (cleaned up copy: ${copyPath})`); }

// ─────────────────────────────────────────────────────────────────────────
const passed = results.filter(r => r.ok).length;
console.log(`\n=== ${passed}/${results.length} assertions passed ===`);
process.exit(passed === results.length ? 0 : 1);
