/**
 * Regression tests for the API client's mutate() body handling (UAT blocker).
 *
 * mutate() used to call res.json() unconditionally on any non-204 success,
 * which threw "Unexpected end of JSON input" when a handler returned a 2xx with
 * an EMPTY body (e.g. createFolder → 201, deletePage → 204). It must now read
 * the body as text and only JSON.parse when non-empty: empty 2xx → undefined,
 * JSON 2xx → parsed object, 204 → undefined.
 *
 * Every mutating call first does a GET /api/v1/csrf for the CSRF token, so the
 * fetch mock returns that token on the GET and the scenario response on the
 * mutating request.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// A minimal Response-like stub: only the fields mutate()/ensureCSRF() read.
function makeRes(opts: {
  ok?: boolean;
  status: number;
  body?: string;
}): Response {
  const ok = opts.ok ?? (opts.status >= 200 && opts.status < 300);
  return {
    ok,
    status: opts.status,
    text: async () => opts.body ?? "",
    json: async () => JSON.parse(opts.body ?? ""),
  } as unknown as Response;
}

// Install a fetch mock that answers the CSRF GET with a token, then routes
// every subsequent call to `responder` (the mutating request under test).
function installFetch(responder: (url: string) => Response) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === "string" ? input : input.toString();
    if (url === "/api/v1/csrf") {
      return makeRes({ status: 200, body: JSON.stringify({ csrf_token: "t0ken" }) });
    }
    return responder(url);
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

// Each test imports a FRESH client module so the module-level CSRF cache does
// not leak between cases.
async function freshClient() {
  vi.resetModules();
  return import("./client");
}

describe("mutate() empty-body tolerance (UAT blocker)", () => {
  beforeEach(() => {
    vi.resetModules();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("resolves (does not throw) on a 201 with an empty body — createFolder", async () => {
    installFetch(() => makeRes({ status: 201, body: "" }));
    const client = await freshClient();
    await expect(client.createFolder("", "Runbooks")).resolves.toBeUndefined();
  });

  it("resolves on a 200 with an empty body", async () => {
    installFetch(() => makeRes({ status: 200, body: "" }));
    const client = await freshClient();
    // updateProfile goes through mutate() and returns void on an empty body.
    await expect(client.updateProfile("New Name")).resolves.toBeUndefined();
  });

  it("returns undefined on a 204 No Content — logout", async () => {
    installFetch(() => makeRes({ status: 204, body: "" }));
    const client = await freshClient();
    await expect(client.logout()).resolves.toBeUndefined();
  });

  it("still parses a non-empty JSON body on success — createPage", async () => {
    installFetch(() =>
      makeRes({ status: 201, body: JSON.stringify({ path: "notes/hello.md" }) }),
    );
    const client = await freshClient();
    await expect(client.createPage("notes", "Hello")).resolves.toEqual({
      path: "notes/hello.md",
    });
  });

  it("surfaces the server error message on a non-2xx with a JSON error body", async () => {
    installFetch(() =>
      makeRes({ status: 400, body: JSON.stringify({ error: "Give your folder a name to create it." }) }),
    );
    const client = await freshClient();
    await expect(client.createFolder("", "")).rejects.toThrow(
      "Give your folder a name to create it.",
    );
  });
});

describe("admin rebuild api fns POST the right route", () => {
  beforeEach(() => {
    vi.resetModules();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("reindexSearch POSTs /api/v1/admin/search/reindex and resolves on 202", async () => {
    let hit: string | null = null;
    const fetchMock = installFetch((url) => {
      hit = url;
      return makeRes({ status: 202, body: "" });
    });
    const client = await freshClient();
    await expect(client.reindexSearch()).resolves.toBeUndefined();
    expect(hit).toBe("/api/v1/admin/search/reindex");
    expect(fetchMock).toHaveBeenCalled();
  });

  it("reindexGraph POSTs /api/v1/admin/graph/reindex and resolves on 202", async () => {
    let hit: string | null = null;
    const fetchMock = installFetch((url) => {
      hit = url;
      return makeRes({ status: 202, body: "" });
    });
    const client = await freshClient();
    await expect(client.reindexGraph()).resolves.toBeUndefined();
    expect(hit).toBe("/api/v1/admin/graph/reindex");
    expect(fetchMock).toHaveBeenCalled();
  });
});
