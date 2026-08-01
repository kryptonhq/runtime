import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import {
  callMCPTool,
  config,
  getAgent,
  getModel,
  invokeAgent,
  invokeModelChat,
  listAgents,
  listMCPTools,
  listModels,
} from "./api";
import { server } from "./test/server";

describe("config", () => {
  it("defaults both bases to same-origin", () => {
    expect(config.getApiBase()).toBe("");
    expect(config.getGatewayBase()).toBe("");
  });

  it("persists and clears the api base", () => {
    config.setApiBase("http://localhost:8090");
    expect(config.getApiBase()).toBe("http://localhost:8090");
    expect(localStorage.getItem("krypton.apiBase")).toBe("http://localhost:8090");

    // Setting an empty value removes the key rather than storing "", so the
    // ?? fallback to same-origin kicks in on the next read.
    config.setApiBase("");
    expect(localStorage.getItem("krypton.apiBase")).toBeNull();
    expect(config.getApiBase()).toBe("");
  });

  it("persists and clears the gateway base independently", () => {
    config.setGatewayBase("http://localhost:8080");
    expect(config.getGatewayBase()).toBe("http://localhost:8080");
    expect(config.getApiBase()).toBe("");

    config.setGatewayBase("");
    expect(localStorage.getItem("krypton.gatewayBase")).toBeNull();
  });
});

describe("listAgents", () => {
  it("requests a bare path when no params are given", async () => {
    let seen: string | undefined;
    server.use(
      http.get("/v1/agents", ({ request }) => {
        seen = new URL(request.url).search;
        return HttpResponse.json({ items: [], page: 1, pageSize: 20, total: 0 });
      }),
    );
    await listAgents();
    expect(seen).toBe("");
  });

  it("serializes every supported query param", async () => {
    let params: URLSearchParams | undefined;
    server.use(
      http.get("/v1/agents", ({ request }) => {
        params = new URL(request.url).searchParams;
        return HttpResponse.json({ items: [], page: 2, pageSize: 50, total: 0 });
      }),
    );

    await listAgents({
      namespace: "agents",
      protocol: "mcp",
      q: "travel",
      sort: "phase",
      order: "desc",
      page: 2,
      pageSize: 50,
    });

    expect(params?.get("namespace")).toBe("agents");
    expect(params?.get("protocol")).toBe("mcp");
    expect(params?.get("q")).toBe("travel");
    expect(params?.get("sort")).toBe("phase");
    expect(params?.get("order")).toBe("desc");
    expect(params?.get("page")).toBe("2");
    expect(params?.get("pageSize")).toBe("50");
  });

  // page=0 and pageSize=0 are falsy, so they're dropped rather than sent.
  // That's intentional — the server would clamp them anyway — but it means
  // callers can't request page 0, which doesn't exist (pages are 1-based).
  it("omits falsy pagination values", async () => {
    let params: URLSearchParams | undefined;
    server.use(
      http.get("/v1/agents", ({ request }) => {
        params = new URL(request.url).searchParams;
        return HttpResponse.json({ items: [], page: 1, pageSize: 20, total: 0 });
      }),
    );
    await listAgents({ page: 0, pageSize: 0, q: "" });
    expect(params?.has("page")).toBe(false);
    expect(params?.has("pageSize")).toBe(false);
    expect(params?.has("q")).toBe(false);
  });

  it("returns the list envelope, not a bare array", async () => {
    const res = await listAgents();
    expect(Array.isArray(res.items)).toBe(true);
    expect(res).toHaveProperty("total");
    expect(res).toHaveProperty("page");
    expect(res).toHaveProperty("pageSize");
  });

  it("prefixes requests with a configured api base", async () => {
    config.setApiBase("http://cp.example.com");
    let url: string | undefined;
    server.use(
      http.get("http://cp.example.com/v1/agents", ({ request }) => {
        url = request.url;
        return HttpResponse.json({ items: [], page: 1, pageSize: 20, total: 0 });
      }),
    );
    await listAgents();
    expect(url).toContain("http://cp.example.com/v1/agents");
  });
});

describe("error handling", () => {
  // The control plane returns {"error": "..."} bodies (writeErr in
  // handlers.go). Surfacing that beats showing a bare status code.
  it("prefers the server's error field over statusText", async () => {
    server.use(
      http.get("/v1/agents/:ns/:name", () =>
        HttpResponse.json({ error: "agent protocol is \"a2a\", not \"mcp\"" }, { status: 400 }),
      ),
    );
    await expect(getAgent("agents", "travel")).rejects.toThrow(
      /400 agent protocol is/,
    );
  });

  it("falls back to statusText when the body has no error field", async () => {
    server.use(
      http.get("/v1/agents/:ns/:name", () =>
        HttpResponse.json({ nope: true }, { status: 503 }),
      ),
    );
    await expect(getAgent("agents", "travel")).rejects.toThrow(/^503/);
  });

  // A gateway or proxy can return HTML for a 502. Parsing must not throw a
  // confusing JSON syntax error on top of the real failure.
  it("tolerates a non-JSON error body", async () => {
    server.use(
      http.get("/v1/agents/:ns/:name", () =>
        HttpResponse.text("<html>502 Bad Gateway</html>", { status: 502 }),
      ),
    );
    await expect(getAgent("agents", "travel")).rejects.toThrow(/^502/);
  });

  it("propagates 404 for a missing model", async () => {
    server.use(
      http.get("/v1/models/:ns/:name", () =>
        HttpResponse.json({ error: "models.krypton.ai \"ghost\" not found" }, { status: 404 }),
      ),
    );
    await expect(getModel("models", "ghost")).rejects.toThrow(/404.*not found/);
  });
});

describe("path encoding", () => {
  // Namespaces and names are user-controlled. Without encoding, a name
  // containing a slash would silently hit a different route.
  it("encodes namespace and name segments", async () => {
    let path: string | undefined;
    server.use(
      http.get("/v1/agents/:ns/:name", ({ request }) => {
        path = new URL(request.url).pathname;
        return HttpResponse.json({
          name: "x",
          namespace: "y",
          spec: { image: "i" },
          status: {},
        });
      }),
    );
    await getAgent("ns with space", "name/with/slash");
    expect(path).toBe("/v1/agents/ns%20with%20space/name%2Fwith%2Fslash");
  });
});

describe("listModels", () => {
  it("serializes model-specific params", async () => {
    let params: URLSearchParams | undefined;
    server.use(
      http.get("/v1/models", ({ request }) => {
        params = new URL(request.url).searchParams;
        return HttpResponse.json({ items: [], page: 1, pageSize: 20, total: 0 });
      }),
    );
    await listModels({ namespace: "models", q: "qwen", sort: "source", order: "desc" });
    expect(params?.get("namespace")).toBe("models");
    expect(params?.get("q")).toBe("qwen");
    expect(params?.get("sort")).toBe("source");
    expect(params?.get("order")).toBe("desc");
    // listModels has no protocol filter — models don't have one.
    expect(params?.has("protocol")).toBe(false);
  });

  it("returns models with their source", async () => {
    const res = await listModels();
    expect(res.items[0].spec.source.huggingface).toContain("Qwen");
  });
});

describe("invokeAgent", () => {
  it("posts to the gateway base, not the api base", async () => {
    config.setApiBase("http://cp.example.com");
    config.setGatewayBase("http://gw.example.com");

    let hit: string | undefined;
    server.use(
      http.post("http://gw.example.com/v1/agents/:ns/:name/", ({ request }) => {
        hit = request.url;
        return HttpResponse.text("pong");
      }),
    );

    const res = await invokeAgent("agents", "travel", '{"ping":true}');
    expect(hit).toContain("gw.example.com");
    expect(res.status).toBe(200);
    expect(res.body).toBe("pong");
    expect(res.durationMs).toBeGreaterThanOrEqual(0);
  });

  it("normalizes a suffix that is missing its leading slash", async () => {
    let path: string | undefined;
    server.use(
      http.post("/v1/agents/:ns/:name/*", ({ request }) => {
        path = new URL(request.url).pathname;
        return HttpResponse.text("ok");
      }),
    );
    await invokeAgent("agents", "travel", "{}", "mcp");
    expect(path).toBe("/v1/agents/agents/travel/mcp");
  });

  it("returns the raw body and status for a failed invocation", async () => {
    server.use(
      http.post("/v1/agents/:ns/:name/", () =>
        HttpResponse.text("cold start timed out", { status: 504 }),
      ),
    );
    const res = await invokeAgent("agents", "travel", "{}");
    // invokeAgent surfaces failures as data, not exceptions, so the UI can
    // show the status and body together.
    expect(res.status).toBe(504);
    expect(res.body).toBe("cold start timed out");
  });
});

describe("invokeModelChat", () => {
  it("posts an OpenAI-shaped body to the gateway", async () => {
    let body: any;
    server.use(
      http.post("/v1/chat/completions", async ({ request }) => {
        body = await request.json();
        return HttpResponse.json({ choices: [{ message: { content: "hi" } }] });
      }),
    );

    const res = await invokeModelChat("qwen2-0-5b", [
      { role: "user", content: "hello" },
    ]);

    expect(body.model).toBe("qwen2-0-5b");
    expect(body.messages).toEqual([{ role: "user", content: "hello" }]);
    expect(res.status).toBe(200);
  });
});

describe("MCP", () => {
  it("unwraps the tools envelope", async () => {
    const tools = await listMCPTools("agents", "mcp-hello");
    expect(Array.isArray(tools)).toBe(true);
    expect(tools.map((t) => t.name)).toEqual(["echo", "now"]);
  });

  it("posts tool arguments and returns the result content", async () => {
    const res = await callMCPTool("agents", "mcp-hello", "echo", { message: "ping" });
    expect(res.content[0].text).toContain("ping");
    expect(res.isError).toBe(false);
  });

  it("encodes the tool name in the path", async () => {
    let path: string | undefined;
    server.use(
      http.post("/v1/agents/:ns/:name/mcp/tools/:tool", ({ request }) => {
        path = new URL(request.url).pathname;
        return HttpResponse.json({ content: [] });
      }),
    );
    await callMCPTool("agents", "mcp-hello", "weird/tool name", {});
    expect(path).toBe("/v1/agents/agents/mcp-hello/mcp/tools/weird%2Ftool%20name");
  });

  // callMCPTool sends `body ?? {}`, so a null/undefined argument object is
  // still valid JSON rather than the literal "null" the server would reject.
  it("sends an empty object when arguments are nullish", async () => {
    let body: unknown;
    server.use(
      http.post("/v1/agents/:ns/:name/mcp/tools/:tool", async ({ request }) => {
        body = await request.json();
        return HttpResponse.json({ content: [] });
      }),
    );
    await callMCPTool("agents", "mcp-hello", "now", null);
    expect(body).toEqual({});
  });

  it("surfaces a 502 from an unreachable MCP server", async () => {
    server.use(
      http.get("/v1/agents/:ns/:name/mcp/tools", () =>
        HttpResponse.json({ error: "mcp initialize: connection refused" }, { status: 502 }),
      ),
    );
    await expect(listMCPTools("agents", "mcp-hello")).rejects.toThrow(
      /502 mcp initialize/,
    );
  });
});
