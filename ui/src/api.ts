// Krypton API client.
//
// The control plane serves the REST surface (read path). Invocations go
// directly to the gateway. For local dev both are usually on the same
// origin via vite's proxy + an ingress in prod.

export interface AgentSpec {
  image: string;
  protocol?: string;
  mode?: "serverless" | "always-on";
  minReplicas?: number;
  maxReplicas?: number;
  concurrency?: number;
  port?: number;
  scaleToZeroAfter?: string;
  invocationPath?: string;
  runtime?: string;
  framework?: string;
}

export interface AgentCondition {
  type: string;
  status: string;
  reason?: string;
  message?: string;
  lastTransitionTime?: string;
}

export interface AgentStatus {
  phase?: "Pending" | "Ready" | "Scaling" | "Failed";
  replicas?: number;
  readyReplicas?: number;
  desiredReplicas?: number;
  url?: string;
  lastInvocationAt?: string;
  observedGeneration?: number;
  conditions?: AgentCondition[];
}

export interface AgentView {
  name: string;
  namespace: string;
  uid?: string;
  spec: AgentSpec;
  status: AgentStatus;
}

export interface ListAgentsParams {
  namespace?: string;
  protocol?: "a2a" | "mcp" | "http";
  q?: string;
  sort?: "name" | "namespace" | "phase" | "replicas" | "image";
  order?: "asc" | "desc";
  page?: number;
  pageSize?: number;
}

export interface ListResponse {
  items: AgentView[];
  page: number;
  pageSize: number;
  total: number;
}

const DEFAULT_API_BASE = ""; // same-origin
const DEFAULT_GATEWAY_BASE = ""; // same-origin

function apiBase(): string {
  return localStorage.getItem("krypton.apiBase") ?? DEFAULT_API_BASE;
}

function gatewayBase(): string {
  return localStorage.getItem("krypton.gatewayBase") ?? DEFAULT_GATEWAY_BASE;
}

export const config = {
  getApiBase: apiBase,
  getGatewayBase: gatewayBase,
  setApiBase(v: string) {
    if (v) localStorage.setItem("krypton.apiBase", v);
    else localStorage.removeItem("krypton.apiBase");
  },
  setGatewayBase(v: string) {
    if (v) localStorage.setItem("krypton.gatewayBase", v);
    else localStorage.removeItem("krypton.gatewayBase");
  },
};

async function getJSON<T>(url: string): Promise<T> {
  const resp = await fetch(url, { headers: { Accept: "application/json" } });
  if (!resp.ok) {
    let detail = resp.statusText;
    try {
      const body = await resp.json();
      if (body?.error) detail = body.error;
    } catch {
      /* ignore */
    }
    throw new Error(`${resp.status} ${detail}`);
  }
  return resp.json() as Promise<T>;
}

export async function listAgents(
  params: ListAgentsParams = {},
): Promise<ListResponse> {
  const q = new URLSearchParams();
  if (params.namespace) q.set("namespace", params.namespace);
  if (params.protocol) q.set("protocol", params.protocol);
  if (params.q) q.set("q", params.q);
  if (params.sort) q.set("sort", params.sort);
  if (params.order) q.set("order", params.order);
  if (params.page) q.set("page", String(params.page));
  if (params.pageSize) q.set("pageSize", String(params.pageSize));
  const qs = q.toString();
  return getJSON<ListResponse>(
    `${apiBase()}/v1/agents${qs ? "?" + qs : ""}`,
  );
}

export function getAgent(namespace: string, name: string): Promise<AgentView> {
  return getJSON<AgentView>(
    `${apiBase()}/v1/agents/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`,
  );
}

export interface InvokeResult {
  status: number;
  body: string;
  durationMs: number;
}

export async function invokeAgent(
  namespace: string,
  name: string,
  body: string,
  suffix: string = "/",
): Promise<InvokeResult> {
  const start = performance.now();
  const url = `${gatewayBase()}/v1/agents/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}${suffix.startsWith("/") ? suffix : "/" + suffix}`;
  const resp = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body,
  });
  const text = await resp.text();
  return { status: resp.status, body: text, durationMs: performance.now() - start };
}

// ---- MCP ---------------------------------------------------------------

export interface MCPTool {
  name: string;
  description?: string;
  inputSchema: Record<string, unknown>;
}

interface ToolsResponse {
  tools: MCPTool[];
}

export interface MCPToolResult {
  content: Array<{ type: string; text?: string; mimeType?: string }>;
  isError?: boolean;
}

export async function listMCPTools(
  namespace: string,
  name: string,
): Promise<MCPTool[]> {
  const res = await getJSON<ToolsResponse>(
    `${apiBase()}/v1/agents/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/mcp/tools`,
  );
  return res.tools;
}

export async function callMCPTool(
  namespace: string,
  name: string,
  tool: string,
  args: unknown,
): Promise<MCPToolResult> {
  return getJSONPost<MCPToolResult>(
    `${apiBase()}/v1/agents/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/mcp/tools/${encodeURIComponent(tool)}`,
    args,
  );
}

async function getJSONPost<T>(url: string, body: unknown): Promise<T> {
  const resp = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(body ?? {}),
  });
  if (!resp.ok) {
    let detail = resp.statusText;
    try {
      const j = await resp.json();
      if (j?.error) detail = j.error;
    } catch {
      /* ignore */
    }
    throw new Error(`${resp.status} ${detail}`);
  }
  return resp.json() as Promise<T>;
}
