// Collaboration race-condition stress harness.
// Launches 6 real Playwright browser sessions (admin, ed, alice, bob, carol, dave),
// each authenticated as a distinct account, then fires CONFLICTING operations
// (edit / delete / move / rename / copy / create) concurrently to hunt for race bugs.
//
// Invariant under test: NO operation may ever produce a 5xx, the server must stay
// healthy, and the git repo + page tree must stay internally consistent no matter
// how operations interleave. Expected, well-defined losers (409 conflict, 404 moved,
// 409 collision) are fine — a 500 or a corrupted/contradictory final state is a bug.
import { chromium } from "playwright";

const BASE = "http://localhost:8098";
const PW = "testpass1234";
const USERS = ["admin", "ed", "alice", "bob", "carol", "dave"];

const fatal = [];   // 5xx / unexpected hard failures (bugs)
const log = [];     // every op, for the trace
function rec(scenario, user, label, res) {
  const row = { scenario, user, label, status: res.status, body: (res.text || "").slice(0, 120) };
  log.push(row);
  if (res.status >= 500) fatal.push(row);
  return res;
}

// In-page authenticated request — runs in the browser context so cookies + same-origin
// (Sec-Fetch-Site) are handled natively; we only fetch the CSRF token per mutation.
async function op(page, method, path, body) {
  return await page.evaluate(async ({ method, path, body }) => {
    let token = null;
    if (method !== "GET") {
      const c = await fetch("/api/v1/csrf", { credentials: "same-origin" });
      token = (await c.json()).csrf_token;
    }
    const headers = {};
    if (body !== null && body !== undefined) headers["Content-Type"] = "application/json";
    if (token) headers["X-CSRF-Token"] = token;
    let res;
    try {
      res = await fetch(path, {
        method, headers, credentials: "same-origin",
        body: body !== null && body !== undefined ? JSON.stringify(body) : undefined,
      });
    } catch (e) {
      return { status: -1, text: "fetch-threw: " + e.message, json: null };
    }
    const text = await res.text();
    let json = null; try { json = JSON.parse(text); } catch {}
    return { status: res.status, text, json };
  }, { method, path, body: body ?? null });
}

async function login(browser, user) {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(BASE + "/login", { waitUntil: "domcontentloaded" });
  await page.fill("#username", user);
  await page.fill("#password", PW);
  await Promise.all([
    page.waitForResponse(r => r.url().includes("/api/v1/auth/login"), { timeout: 15000 }).catch(() => {}),
    page.click("button[type=submit]"),
  ]);
  // Confirm the session via the real /me endpoint, with a short retry.
  for (let i = 0; i < 20; i++) {
    const me = await op(page, "GET", "/api/v1/auth/me", null);
    if (me.json && me.json.username === user) return { context, page, user };
    await page.waitForTimeout(150);
  }
  throw new Error(`login failed for ${user}`);
}

// Seed helpers (run as admin).
let admin;
async function seedPage(folder, title) {
  const r = await op(admin.page, "POST", "/api/v1/pages", { folder, title });
  if (!r.json || !r.json.path) throw new Error(`seedPage(${folder}/${title}) -> ${r.status} ${r.text}`);
  return r.json.path;
}
async function seedFolder(parent, name) {
  const r = await op(admin.page, "POST", "/api/v1/folders", { parent, name });
  if (r.status !== 201) throw new Error(`seedFolder(${name}) -> ${r.status} ${r.text}`);
  return r.json.path;
}
async function getRev(page, path) {
  const r = await op(page, "GET", `/api/v1/pages/${path}`, null);
  return { rev: r.json?.revision, body: r.json?.body ?? "", fm: r.json?.frontmatter ?? "", status: r.status };
}
const savePayload = (body, fm, rev) => ({ body, frontmatter: fm, base_revision: rev });

const findings = [];
function assert(scenario, cond, detail) {
  findings.push({ scenario, ok: !!cond, detail });
  console.log(`  ${cond ? "✓" : "✗ BUG"} [${scenario}] ${detail}`);
}

// ─────────────────────────────────────────────────────────────────────────────
const browser = await chromium.launch({ headless: true });
console.log("Logging in 6 sessions…");
const sessions = {};
for (const u of USERS) sessions[u] = await login(browser, u);
admin = sessions.admin;
const { ed, alice, bob, carol, dave } = sessions;
console.log("All 6 logged in.\n");

const ROOT = "stress"; // a folder we own for the run
await seedFolder("", ROOT).catch(() => {}); // ignore if exists

try {
  // S1 — N-way concurrent save on one page (write conflict).
  console.log("S1 — 3-way concurrent save (optimistic concurrency)");
  {
    const p = await seedPage(ROOT, "s1 conflict");
    const base = await getRev(alice.page, p);
    const writers = [["alice", alice], ["bob", bob], ["carol", carol]];
    const res = await Promise.all(writers.map(([name, s]) =>
      op(s.page, "PUT", `/api/v1/pages/${p}`, savePayload(`body from ${name}`, base.fm, base.rev))
        .then(r => rec("S1", name, "concurrent-save", r))));
    const ok = res.filter(r => r.status === 204).length;
    const conflict = res.filter(r => r.status === 409).length;
    assert("S1", ok === 1 && conflict === 2, `winners=${ok} conflicts=${conflict} (expect 1 win / 2×409) statuses=${res.map(r=>r.status)}`);
    const final = await getRev(admin.page, p);
    assert("S1", /body from (alice|bob|carol)/.test(final.body), `final content is exactly one writer's: ${JSON.stringify(final.body)}`);
  }

  // S2 — edit vs delete (save racing a trash).
  console.log("S2 — save vs delete-to-trash");
  {
    const p = await seedPage(ROOT, "s2 editdelete");
    const base = await getRev(alice.page, p);
    const [save, del] = await Promise.all([
      op(alice.page, "PUT", `/api/v1/pages/${p}`, savePayload("alice edit during delete", base.fm, base.rev)).then(r => rec("S2", "alice", "save", r)),
      op(bob.page, "DELETE", `/api/v1/pages/${p}`, null).then(r => rec("S2", "bob", "delete", r)),
    ]);
    const after = await getRev(admin.page, p);
    // Acceptable outcomes: page deleted (404 now) OR saved-then-deleted OR saved (still present). Never a 5xx, never contradictory.
    assert("S2", save.status < 500 && del.status < 500, `no 5xx (save=${save.status} del=${del.status})`);
    assert("S2", after.status === 404 || after.status === 200, `final page state coherent (GET=${after.status})`);
  }

  // S3 — rename vs save (path moves out from under a writer).
  console.log("S3 — rename vs save (stale path)");
  {
    const p = await seedPage(ROOT, "s3 renamesave");
    const base = await getRev(alice.page, p);
    const [ren, save] = await Promise.all([
      op(bob.page, "POST", `/api/v1/pages/${p}/rename`, { new_title: "s3 renamed" }).then(r => rec("S3", "bob", "rename", r)),
      op(alice.page, "PUT", `/api/v1/pages/${p}`, savePayload("alice writes stale path", base.fm, base.rev)).then(r => rec("S3", "alice", "save-old-path", r)),
    ]);
    assert("S3", ren.status < 500 && save.status < 500, `no 5xx (rename=${ren.status} save=${save.status})`);
    // If rename won, the old path must not be silently writable to a ghost file.
    if (ren.status === 200) {
      assert("S3", save.status === 404 || save.status === 409 || save.status === 204,
        `save on renamed-away path resolved cleanly (got ${save.status})`);
    } else {
      assert("S3", true, `rename did not win (status ${ren.status}); no 5xx is the bar`);
    }
  }

  // S4 — concurrent rename collision (two renames of the same page to different names).
  console.log("S4 — concurrent rename of same page");
  {
    const p = await seedPage(ROOT, "s4 doublerename");
    const [a, b] = await Promise.all([
      op(alice.page, "POST", `/api/v1/pages/${p}/rename`, { new_title: "s4 alice" }).then(r => rec("S4", "alice", "rename", r)),
      op(bob.page, "POST", `/api/v1/pages/${p}/rename`, { new_title: "s4 bob" }).then(r => rec("S4", "bob", "rename", r)),
    ]);
    const wins = [a, b].filter(r => r.status === 200).length;
    assert("S4", a.status < 500 && b.status < 500, `no 5xx (${a.status}/${b.status})`);
    assert("S4", wins >= 1 && wins <= 2, `at least one rename succeeded (wins=${wins})`);
  }

  // S5 — concurrent create of the SAME name (path collision).
  console.log("S5 — concurrent create of identical title");
  {
    const title = "s5 sametitle";
    const [a, b] = await Promise.all([
      op(alice.page, "POST", "/api/v1/pages", { folder: ROOT, title }).then(r => rec("S5", "alice", "create", r)),
      op(bob.page, "POST", "/api/v1/pages", { folder: ROOT, title }).then(r => rec("S5", "bob", "create", r)),
    ]);
    assert("S5", a.status < 500 && b.status < 500, `no 5xx (${a.status}/${b.status})`);
    const paths = [a.json?.path, b.json?.path].filter(Boolean);
    const created = [a, b].filter(r => r.status === 200 || r.status === 201);
    // Either one collides (409) or both succeed with DISTINCT paths (no overwrite).
    const distinct = new Set(paths).size === paths.length;
    assert("S5", distinct, `no two creates collapsed to the same path (paths=${JSON.stringify(paths)}, created=${created.length})`);
  }

  // S6 — folder move vs editing a page inside it.
  console.log("S6 — folder move vs edit inside");
  {
    const sub = await seedFolder(ROOT, "s6sub");
    const inner = await seedPage(sub, "s6 inner");
    const base = await getRev(carol.page, inner);
    const [mv, save] = await Promise.all([
      op(dave.page, "POST", `/api/v1/pages/${sub}/move-folder`, { new_parent: "" }).then(r => rec("S6", "dave", "move-folder", r)),
      op(carol.page, "PUT", `/api/v1/pages/${inner}`, savePayload("carol edits inside moving folder", base.fm, base.rev)).then(r => rec("S6", "carol", "save-inside", r)),
    ]);
    assert("S6", mv.status < 500 && save.status < 500, `no 5xx (move=${mv.status} save=${save.status})`);
    if (mv.status === 200) {
      assert("S6", save.status === 404 || save.status === 409 || save.status === 204,
        `save inside a moved folder resolved cleanly (got ${save.status})`);
    } else {
      assert("S6", true, `move did not win (${mv.status}); no 5xx is the bar`);
    }
  }

  // S7 — copy (save-as-copy = create) racing a delete of the source.
  console.log("S7 — copy vs delete source");
  {
    const p = await seedPage(ROOT, "s7 source");
    await getRev(alice.page, p);
    const [copy, del] = await Promise.all([
      op(alice.page, "POST", "/api/v1/pages", { folder: ROOT, title: "s7 copy of source" }).then(r => rec("S7", "alice", "copy", r)),
      op(bob.page, "DELETE", `/api/v1/pages/${p}`, null).then(r => rec("S7", "bob", "delete-source", r)),
    ]);
    assert("S7", copy.status < 500 && del.status < 500, `no 5xx (copy=${copy.status} del=${del.status})`);
    assert("S7", copy.status === 200 || copy.status === 201, `copy created independently of source delete (${copy.status})`);
  }

  // S8 — the storm: every user fires a random burst of mixed ops over a shared page pool.
  console.log("S8 — mixed-operation storm (6 users × random ops, all concurrent)");
  {
    const pool = [];
    for (let i = 0; i < 6; i++) pool.push(await seedPage(ROOT, `storm ${i}`));
    const pick = () => pool[Math.floor(Math.random() * pool.length)];
    const actors = Object.entries(sessions);
    const bursts = [];
    for (const [name, s] of actors) {
      for (let k = 0; k < 5; k++) {
        const path = pick();
        const roll = Math.floor(Math.random() * 5);
        let pr;
        if (roll === 0) {
          pr = getRev(s.page, path).then(b => op(s.page, "PUT", `/api/v1/pages/${path}`, savePayload(`${name}-storm-${k}`, b.fm, b.rev || "x")));
        } else if (roll === 1) {
          pr = op(s.page, "POST", `/api/v1/pages/${path}/rename`, { new_title: `${name} r${k}` });
        } else if (roll === 2) {
          pr = op(s.page, "DELETE", `/api/v1/pages/${path}`, null);
        } else if (roll === 3) {
          pr = op(s.page, "POST", "/api/v1/pages", { folder: ROOT, title: `${name} new ${k}` });
        } else {
          pr = op(s.page, "POST", `/api/v1/pages/${path}/rename`, { new_parent: "" });
        }
        bursts.push(pr.then(r => rec("S8", name, `op${roll}`, r)).catch(e => rec("S8", name, "threw", { status: -1, text: e.message })));
      }
    }
    const all = await Promise.all(bursts);
    const codes = {};
    for (const r of all) codes[r.status] = (codes[r.status] || 0) + 1;
    assert("S8", fatal.filter(f => f.scenario === "S8").length === 0, `no 5xx in storm — status histogram ${JSON.stringify(codes)}`);
  }

  // ── Post-storm invariants ──────────────────────────────────────────────────
  console.log("\nPost-storm invariants");
  const health = await op(admin.page, "GET", "/api/v1/health", null);
  assert("INV", health.status === 200, `server healthy after storm (GET /health = ${health.status})`);
  const tree = await op(admin.page, "GET", "/api/v1/tree", null);
  assert("INV", tree.status === 200 && Array.isArray(tree.json), `page tree loads & parses (${tree.status})`);
  const search = await op(admin.page, "GET", "/api/v1/search?q=storm", null);
  assert("INV", search.status === 200, `search responds after churn (${search.status})`);

} finally {
  await browser.close();
}

// ── Report ────────────────────────────────────────────────────────────────────
const bugs = findings.filter(f => !f.ok);
console.log(`\n===== ${findings.length - bugs.length}/${findings.length} invariants held =====`);
console.log(`5xx / hard failures: ${fatal.length}`);
if (fatal.length) console.log(JSON.stringify(fatal.slice(0, 20), null, 2));
if (bugs.length) { console.log("\nFAILED INVARIANTS:"); for (const b of bugs) console.log(`  ✗ [${b.scenario}] ${b.detail}`); }
// dump op trace to a file for forensic review
import { writeFileSync } from "fs";
writeFileSync(".smtc-cache/uat_stress_trace.json", JSON.stringify(log, null, 2));
process.exit(fatal.length === 0 && bugs.length === 0 ? 0 : 1);
