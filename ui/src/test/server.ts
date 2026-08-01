import { setupServer } from "msw/node";
import { http, HttpResponse } from "msw";
import { agent, agentList, echoTool, mcpAgent, model, modelList, nowTool, toolResult } from "./fixtures";

// Default handlers describe a small, healthy cluster. Individual tests
// override with server.use(...) for error and edge cases.
export const handlers = [
  http.get("/v1/agents", ({ request }) => {
    const url = new URL(request.url);
    const protocol = url.searchParams.get("protocol");
    const q = url.searchParams.get("q")?.toLowerCase();

    let items = [agent(), mcpAgent()];
    // Mirror the server-side filtering in handlers.go so tests that assert
    // on query params get realistic responses.
    if (protocol) items = items.filter((a) => a.spec.protocol === protocol);
    if (q) {
      items = items.filter(
        (a) =>
          a.name.toLowerCase().includes(q) ||
          a.namespace.toLowerCase().includes(q) ||
          (a.spec.image ?? "").toLowerCase().includes(q),
      );
    }
    return HttpResponse.json(agentList(items));
  }),

  http.get("/v1/agents/:namespace/:name", ({ params }) => {
    if (params.name === "mcp-hello") return HttpResponse.json(mcpAgent());
    return HttpResponse.json(agent({ name: String(params.name) }));
  }),

  http.get("/v1/agents/:namespace/:name/mcp/tools", () =>
    HttpResponse.json({ tools: [echoTool, nowTool] }),
  ),

  http.post("/v1/agents/:namespace/:name/mcp/tools/:tool", async ({ params, request }) => {
    const args = (await request.json()) as Record<string, unknown>;
    return HttpResponse.json(
      toolResult(`${params.tool} called with ${JSON.stringify(args)}`),
    );
  }),

  http.get("/v1/models", () => HttpResponse.json(modelList([model()]))),

  http.get("/v1/models/:namespace/:name", ({ params }) =>
    HttpResponse.json(model({ name: String(params.name) })),
  ),
];

export const server = setupServer(...handlers);

/** Shorthand for tests asserting error rendering. */
export function errorOnce(path: string, status: number, body?: unknown) {
  return http.get(
    path,
    () => HttpResponse.json(body ?? { error: "boom" }, { status }),
    { once: true },
  );
}
