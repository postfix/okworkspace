// Expanded structural race suite — rename / move / delete / restore collisions.
// 6 real Playwright sessions (distinct accounts). For each scenario the final
// TREE is flattened and checked for the invariants that actually matter:
//   • no two pages at the same path (no clobber / no duplicate)
//   • no ghost (a page that exists at neither old nor new path = data loss)
//   • no 5xx
import { chromium } from "playwright";

const BASE = "http://localhost:8098";
const PW = "testpass1234";
const USERS = ["admin", "ed", "alice", "bob", "carol", "dave"];
const fatal = [];
function note5xx(tag, s) { if (s >= 500) fatal.push(`${tag}:${s}`); return s; }

async function op(page, method, path, body) {
  return await page.evaluate(async ({ method, path, body }) => {
    let token = null;
    if (method !== "GET") token = (await (await fetch("/api/v1/csrf", { credentials: "same-origin" })).json()).csrf_token;
    const headers = {};
    if (body != null) headers["Content-Type"] = "application/json";
    if (token) headers["X-CSRF-Token"] = token;
    let res;
    try { res = await fetch(path, { method, headers, credentials: "same-origin", body: body != null ? JSON.stringify(body) : undefined }); }
    catch (e) { return { status: -1, text: "threw:" + e.message, json: null }; }
    const text = await res.text(); let json = null; try { json = JSON.parse(text); } catch {}
    return { status: res.status, text, json };
  }, { method, path, body: body ?? null });
}
async function login(browser, user) {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(BASE + "/login", { waitUntil: "domcontentloaded" });
  await page.fill("#username", user); await page.fill("#password", PW);
  await Promise.all([page.waitForResponse(r => r.url().includes("/auth/login"), { timeout: 15000 }).catch(() => {}), page.click("button[type=submit]")]);
  for (let i = 0; i < 20; i++) { const me = await op(page, "GET", "/api/v1/auth/me", null); if (me.json?.username === user) return { page, user }; await page.waitForTimeout(150); }
  throw new Error("login failed " + user);
}

let admin;
const seedPage = async (folder, title) => (await op(admin.page, "POST", "/api/v1/pages", { folder, title })).json.path;
const seedFolder = async (parent, name) => { const r = await op(admin.page, "POST", "/api/v1/folders", { parent, name }); return r.json?.path; };
function flatten(tree, acc = []) { for (const n of tree || []) { acc.push({ type: n.type, path: n.path }); if (n.children) flatten(n.children, acc); } return acc; }
async function pagePaths() { const t = await op(admin.page, "GET", "/api/v1/tree", null); return flatten(t.json).filter(n => n.type === "page").map(n => n.path); }
const dup = arr => arr.filter((p, i) => arr.indexOf(p) !== i);
async function exists(path) { return (await op(admin.page, "GET", `/api/v1/pages/${path}`, null)).status === 200; }

const findings = [];
function assert(sc, cond, detail) { findings.push({ sc, ok: !!cond }); console.log(`  ${cond ? "✓" : "✗ BUG"} [${sc}] ${detail}`); }

const browser = await chromium.launch({ headless: true });
console.log("Logging in 6 sessions…");
const S = {}; for (const u of USERS) S[u] = await login(browser, u);
admin = S.admin;
console.log("ready.\n");
const ROOT = "rstress";
await seedFolder("", ROOT);

try {
  // R1 — two concurrent renames of the SAME page to different titles.
  console.log("R1 — double rename of same page");
  {
    const p = await seedPage(ROOT, "r1 orig");
    const before = (await pagePaths()).length;
    const [a, b] = await Promise.all([
      op(S.alice.page, "POST", `/api/v1/pages/${p}/rename`, { new_title: "r1 alpha" }).then(r => (note5xx("R1a", r.status), r)),
      op(S.bob.page, "POST", `/api/v1/pages/${p}/rename`, { new_title: "r1 beta" }).then(r => (note5xx("R1b", r.status), r)),
    ]);
    const paths = await pagePaths();
    assert("R1", a.status < 500 && b.status < 500, `no 5xx (${a.status}/${b.status})`);
    assert("R1", paths.length === before, `page count unchanged (${before}→${paths.length}) — no ghost/dup`);
    assert("R1", dup(paths).length === 0, `no duplicate paths in tree (dups=${JSON.stringify(dup(paths))})`);
    const alpha = paths.includes(`${ROOT}/r1-alpha.md`), beta = paths.includes(`${ROOT}/r1-beta.md`), orig = await exists(p);
    assert("R1", (alpha || beta) && !orig, `exactly one target exists, source gone (alpha=${alpha} beta=${beta} origGone=${!orig})`);
  }

  // R2 — two DIFFERENT pages renamed to the SAME target title (namespace collision).
  console.log("R2 — two pages renamed into one title");
  {
    const p1 = await seedPage(ROOT, "r2 one");
    const p2 = await seedPage(ROOT, "r2 two");
    const before = (await pagePaths()).length;
    const [a, b] = await Promise.all([
      op(S.carol.page, "POST", `/api/v1/pages/${p1}/rename`, { new_title: "r2 merged" }).then(r => (note5xx("R2a", r.status), r)),
      op(S.dave.page, "POST", `/api/v1/pages/${p2}/rename`, { new_title: "r2 merged" }).then(r => (note5xx("R2b", r.status), r)),
    ]);
    const paths = await pagePaths();
    assert("R2", a.status < 500 && b.status < 500, `no 5xx (${a.status}/${b.status})`);
    assert("R2", dup(paths).length === 0, `no clobber — two pages never collapse to one path (dups=${JSON.stringify(dup(paths))})`);
    assert("R2", paths.length === before, `both pages survive (count ${before}→${paths.length})`);
  }

  // R3 — double delete of the same page.
  console.log("R3 — concurrent double delete");
  {
    const p = await seedPage(ROOT, "r3 victim");
    const trashCount = async () => ((await op(admin.page, "GET", "/api/v1/trash", null)).json || []).filter(t => t.original_path === p).length;
    const before = await trashCount(); // prior runs reuse this slug, so measure the DELTA
    const [a, b] = await Promise.all([
      op(S.alice.page, "DELETE", `/api/v1/pages/${p}`, null).then(r => (note5xx("R3a", r.status), r)),
      op(S.bob.page, "DELETE", `/api/v1/pages/${p}`, null).then(r => (note5xx("R3b", r.status), r)),
    ]);
    assert("R3", a.status < 500 && b.status < 500, `no 5xx (${a.status}/${b.status})`);
    assert("R3", !(await exists(p)), `page is gone after double delete`);
    const delta = (await trashCount()) - before;
    assert("R3", delta === 1, `double delete created exactly ONE new trash row, not two (delta=${delta})`);
  }

  // R4 — rename racing a delete on the same page.
  console.log("R4 — rename vs delete same page");
  {
    const p = await seedPage(ROOT, "r4 contested");
    const before = (await pagePaths()).length;
    const [ren, del] = await Promise.all([
      op(S.carol.page, "POST", `/api/v1/pages/${p}/rename`, { new_title: "r4 renamed" }).then(r => (note5xx("R4r", r.status), r)),
      op(S.dave.page, "DELETE", `/api/v1/pages/${p}`, null).then(r => (note5xx("R4d", r.status), r)),
    ]);
    const paths = await pagePaths();
    assert("R4", ren.status < 500 && del.status < 500, `no 5xx (rename=${ren.status} del=${del.status})`);
    assert("R4", dup(paths).length === 0, `no duplicate paths`);
    const renamed = paths.includes(`${ROOT}/r4-renamed.md`), orig = await exists(p);
    // Coherent: either renamed (and not deleted) OR deleted (neither path present). Never both, never a ghost at p.
    assert("R4", (renamed && !orig) || (!renamed && !orig), `coherent end state (renamed=${renamed} origPresent=${orig})`);
    assert("R4", paths.length <= before, `no page invented by the race (${before}→${paths.length})`);
  }

  // R5 — move a page into a folder that is being deleted at the same time.
  console.log("R5 — move-into-folder vs delete-that-folder");
  {
    const destSub = await seedFolder(ROOT, "r5dest");
    const p = await seedPage(ROOT, "r5 mover");
    const [mv, del] = await Promise.all([
      op(S.alice.page, "POST", `/api/v1/pages/${p}/rename`, { new_parent: destSub }).then(r => (note5xx("R5m", r.status), r)),
      op(S.bob.page, "POST", `/api/v1/pages/${destSub}/delete-folder`, null).then(r => (note5xx("R5d", r.status), r)),
    ]);
    const paths = await pagePaths();
    assert("R5", mv.status < 500 && del.status < 500, `no 5xx (move=${mv.status} delFolder=${del.status})`);
    assert("R5", dup(paths).length === 0, `tree has no duplicate paths`);
    // The mover page must not vanish entirely: it is either live somewhere or recoverable in trash.
    const live = paths.some(pp => pp.endsWith("/r5-mover.md") || pp === `${ROOT}/r5-mover.md`);
    const trash = (await op(admin.page, "GET", "/api/v1/trash", null)).json || [];
    const trashed = trash.some(t => t.original_path.endsWith("r5-mover.md"));
    assert("R5", live || trashed, `mover page not lost (live=${live} trashed=${trashed})`);
  }

  // R6 — concurrent folder rename + folder move on the same folder.
  console.log("R6 — folder rename vs folder move (same folder)");
  {
    const f = await seedFolder(ROOT, "r6src");
    await seedPage(f, "r6 inner");
    const before = (await pagePaths()).length;
    const [ren, mv] = await Promise.all([
      op(S.carol.page, "POST", `/api/v1/pages/${f}/rename-folder`, { new_name: "r6renamed" }).then(r => (note5xx("R6r", r.status), r)),
      op(S.dave.page, "POST", `/api/v1/pages/${f}/move-folder`, { new_parent: "" }).then(r => (note5xx("R6m", r.status), r)),
    ]);
    const paths = await pagePaths();
    assert("R6", ren.status < 500 && mv.status < 500, `no 5xx (rename=${ren.status} move=${mv.status})`);
    assert("R6", dup(paths).length === 0, `no duplicate paths after folder op race`);
    assert("R6", paths.length === before, `inner page survives exactly once (${before}→${paths.length})`);
  }

  // R7 — double restore of the SAME trash entry.
  console.log("R7 — concurrent double restore from trash");
  {
    const p = await seedPage(ROOT, "r7 restoreme");
    await op(admin.page, "DELETE", `/api/v1/pages/${p}`, null);
    const trash = (await op(admin.page, "GET", "/api/v1/trash", null)).json || [];
    const entry = trash.find(t => t.original_path === p);
    const before = (await pagePaths()).length;
    let r1 = { status: 0 }, r2 = { status: 0 };
    if (entry) {
      [r1, r2] = await Promise.all([
        op(S.alice.page, "POST", `/api/v1/trash/${entry.id}/restore`, null).then(r => (note5xx("R7a", r.status), r)),
        op(S.bob.page, "POST", `/api/v1/trash/${entry.id}/restore`, null).then(r => (note5xx("R7b", r.status), r)),
      ]);
    }
    const paths = await pagePaths();
    assert("R7", r1.status < 500 && r2.status < 500, `no 5xx (${r1.status}/${r2.status})`);
    assert("R7", dup(paths).length === 0, `restore did not duplicate the page (dups=${JSON.stringify(dup(paths))})`);
    assert("R7", paths.length === before + 1, `exactly one page restored, not two (${before}→${paths.length})`);
  }

} finally {
  await browser.close();
}

const bugs = findings.filter(f => !f.ok);
console.log(`\n===== ${findings.length - bugs.length}/${findings.length} structural-race invariants held =====`);
console.log(`5xx: ${fatal.length}${fatal.length ? " " + JSON.stringify(fatal) : ""}`);
if (bugs.length) { console.log("FAILED:"); for (const b of bugs) console.log(`  ✗ ${b.sc}`); }
process.exit(fatal.length === 0 && bugs.length === 0 ? 0 : 1);
