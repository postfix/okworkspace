// Focused, deterministic repro of the two TOCTOU lost-update races (no browser).
// Runs each scenario 3× to show it reproduces consistently.
const BASE = "http://localhost:8098";
function setCookies(res, jar) {
  const all = res.headers.getSetCookie ? res.headers.getSetCookie() : [];
  for (const sc of all) { const [p] = sc.split(";"); const i = p.indexOf("="); if (i > 0) jar[p.slice(0, i).trim()] = p.slice(i + 1).trim(); }
}
const cookieHdr = j => Object.entries(j).map(([k, v]) => `${k}=${v}`).join("; ");
class S {
  constructor(n) { this.n = n; this.jar = {}; this.csrf = null; }
  async req(m, path, body) {
    const h = { Origin: BASE, "Sec-Fetch-Site": "same-origin" };
    if (Object.keys(this.jar).length) h.Cookie = cookieHdr(this.jar);
    if (body !== undefined) h["Content-Type"] = "application/json";
    if (m !== "GET" && this.csrf) h["X-CSRF-Token"] = this.csrf;
    const res = await fetch(BASE + path, { method: m, headers: h, body: body !== undefined ? JSON.stringify(body) : undefined });
    setCookies(res, this.jar); return res;
  }
  async csrfGet() { this.csrf = (await (await this.req("GET", "/api/v1/csrf")).json()).csrf_token; }
  async login(u, p) { await this.csrfGet(); const r = await this.req("POST", "/api/v1/auth/login", { username: u, password: p }); if (r.status !== 200) throw new Error(`${u} login ${r.status}`); await this.csrfGet(); }
}
async function mk(u) { const s = new S(u); await s.login(u, "testpass1234"); return s; }

const admin = await mk("admin"), alice = await mk("alice"), bob = await mk("bob"), carol = await mk("carol");

console.log("=== RACE A: concurrent save, same base_revision (expect 1×204 + 2×409) ===");
for (let run = 1; run <= 3; run++) {
  const p = (await (await admin.req("POST", "/api/v1/pages", { folder: "stress", title: `repro save ${run}` })).json()).path;
  const g = await (await alice.req("GET", `/api/v1/pages/${p}`)).json();
  const base = g.revision, fm = g.frontmatter ?? "";
  const writers = [["alice", alice], ["bob", bob], ["carol", carol]];
  const res = await Promise.all(writers.map(([nm, s]) =>
    s.req("PUT", `/api/v1/pages/${p}`, { body: `body-${nm}`, frontmatter: fm, base_revision: base }).then(r => r.status)));
  const win = res.filter(s => s === 204).length, conf = res.filter(s => s === 409).length;
  console.log(`  run ${run}: statuses=${res}  winners=${win} conflicts=${conf}  ${win === 1 ? "OK" : "LOST-UPDATE BUG (multiple winners)"}`);
}

console.log("\n=== RACE B: concurrent create, same title (expect distinct paths or 1 collision) ===");
for (let run = 1; run <= 3; run++) {
  const title = `repro dup ${run}`;
  const [a, b] = await Promise.all([
    alice.req("POST", "/api/v1/pages", { folder: "stress", title }).then(async r => ({ s: r.status, path: (await r.json()).path })),
    bob.req("POST", "/api/v1/pages", { folder: "stress", title }).then(async r => ({ s: r.status, path: (await r.json()).path })),
  ]);
  const same = a.path && a.path === b.path;
  console.log(`  run ${run}: A=${a.s}:${a.path}  B=${b.s}:${b.path}  ${same ? "CLOBBER BUG (identical path created twice)" : "OK (distinct/collision)"}`);
}
