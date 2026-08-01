// Fixtures mirroring the control plane's actual JSON, so a backend shape
// change surfaces here rather than in production.
//
// Provenance — keep these in sync with the Go types:
//   AgentView / ModelView   internal/controlplane/handlers.go
//   list envelope           {items, page, pageSize, total}, same file
//   MCP tools envelope      {tools: [...]}, internal/controlplane/mcp.go
//   Tool                    internal/mcp/types.go
//
// Note the list envelope wraps items; a common mistake is assuming the API
// returns a bare array.

import type {
  AgentView,
  ListResponse,
  ListModelsResponse,
  MCPTool,
  MCPToolResult,
  ModelView,
} from "../api";

export function agent(overrides: Partial<AgentView> = {}): AgentView {
  return {
    name: "travel-agent",
    namespace: "agents",
    uid: "uid-travel-agent",
    spec: {
      image: "ghcr.io/org/travel-agent:v1",
      protocol: "a2a",
      mode: "always-on",
      minReplicas: 1,
      maxReplicas: 10,
      concurrency: 8,
      port: 8080,
      invocationPath: "/a2a",
      runtime: "python",
      framework: "langgraph",
      ...overrides.spec,
    },
    status: {
      phase: "Ready",
      replicas: 2,
      readyReplicas: 2,
      desiredReplicas: 2,
      url: "http://travel-agent.agents.svc:8080/a2a",
      observedGeneration: 1,
      ...overrides.status,
    },
    ...overrides,
  };
}

export function mcpAgent(overrides: Partial<AgentView> = {}): AgentView {
  return agent({
    name: "mcp-hello",
    uid: "uid-mcp-hello",
    ...overrides,
    spec: {
      image: "ghcr.io/kryptonhq/mcp-hello:v1",
      protocol: "mcp",
      mode: "serverless",
      minReplicas: 0,
      port: 8080,
      invocationPath: "/",
      ...overrides.spec,
    },
  });
}

export function model(overrides: Partial<ModelView> = {}): ModelView {
  return {
    name: "qwen2-0-5b",
    namespace: "models",
    uid: "uid-qwen",
    spec: {
      source: {
        huggingface: "Qwen/Qwen2.5-0.5B-Instruct-GGUF",
        file: "qwen2.5-0.5b-instruct-q4_k_m.gguf",
      },
      runtime: "llama.cpp",
      port: 8080,
      minReplicas: 1,
      maxReplicas: 1,
      ...overrides.spec,
    },
    status: {
      phase: "Ready",
      replicas: 1,
      readyReplicas: 1,
      url: "http://qwen2-0-5b.models.svc:8080",
      observedGeneration: 1,
      ...overrides.status,
    },
    ...overrides,
  };
}

export function agentList(items: AgentView[]): ListResponse {
  return { items, page: 1, pageSize: 20, total: items.length };
}

export function modelList(items: ModelView[]): ListModelsResponse {
  return { items, page: 1, pageSize: 20, total: items.length };
}

export const echoTool: MCPTool = {
  name: "echo",
  description: "Echo the message back",
  inputSchema: {
    type: "object",
    properties: {
      message: { type: "string" },
      times: { type: "integer" },
      loud: { type: "boolean" },
    },
    required: ["message"],
  },
};

export const nowTool: MCPTool = {
  name: "now",
  description: "Return the current time",
  inputSchema: { type: "object", properties: {} },
};

export function toolResult(text: string, isError = false): MCPToolResult {
  return { content: [{ type: "text", text }], isError };
}
