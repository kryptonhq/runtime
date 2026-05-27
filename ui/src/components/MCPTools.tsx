import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import {
  callMCPTool,
  listMCPTools,
  MCPTool,
  MCPToolResult,
} from "../api";
import { Badge, Button, Card, ErrorMessage } from "./ui";

export default function MCPTools({
  namespace,
  name,
}: {
  namespace: string;
  name: string;
}) {
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ["mcp", "tools", namespace, name],
    queryFn: () => listMCPTools(namespace, name),
    staleTime: 30_000,
  });

  return (
    <Card>
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-semibold">MCP Tools</h2>
        <Button variant="ghost" onClick={() => refetch()}>
          Refresh
        </Button>
      </div>
      <p className="text-xs text-slate-500 mb-4">
        Discovered via <code className="font-mono">tools/list</code> on the
        agent. Call a tool to send <code className="font-mono">tools/call</code>{" "}
        through the control plane.
      </p>

      {isLoading && (
        <div className="text-sm text-slate-500">Loading tools…</div>
      )}
      {error && <ErrorMessage error={error} />}
      {data && data.length === 0 && (
        <div className="text-sm text-slate-500">
          No tools advertised by this MCP server.
        </div>
      )}
      {data && data.length > 0 && (
        <div className="space-y-3">
          {data.map((t) => (
            <ToolCard key={t.name} tool={t} namespace={namespace} name={name} />
          ))}
        </div>
      )}
    </Card>
  );
}

function ToolCard({
  tool,
  namespace,
  name,
}: {
  tool: MCPTool;
  namespace: string;
  name: string;
}) {
  const [args, setArgs] = useState(() => placeholderArgs(tool.inputSchema));
  const [pending, setPending] = useState(false);
  const [result, setResult] = useState<MCPToolResult | null>(null);
  const [err, setErr] = useState<string | null>(null);

  async function run() {
    setPending(true);
    setErr(null);
    setResult(null);
    try {
      const parsed = args.trim() === "" ? {} : JSON.parse(args);
      const r = await callMCPTool(namespace, name, tool.name, parsed);
      setResult(r);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="rounded-md border p-3 border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-950/50">
      <div className="flex items-center justify-between">
        <div>
          <div className="font-mono text-sm text-accent">{tool.name}</div>
          {tool.description && (
            <div className="text-xs text-slate-500 mt-0.5">
              {tool.description}
            </div>
          )}
        </div>
        {result?.isError ? (
          <Badge tone="red">Error</Badge>
        ) : result ? (
          <Badge tone="green">OK</Badge>
        ) : null}
      </div>

      <details className="mt-2">
        <summary className="text-xs text-slate-500 cursor-pointer">
          Input schema
        </summary>
        <pre className="text-xs font-mono rounded p-2 mt-1 overflow-auto max-h-32 border bg-slate-50 border-slate-200 dark:bg-slate-900 dark:border-slate-800">
          {JSON.stringify(tool.inputSchema, null, 2)}
        </pre>
      </details>

      <div className="mt-3">
        <label className="text-xs text-slate-500">Arguments (JSON)</label>
        <textarea
          className="w-full rounded-md border px-3 py-2 text-xs font-mono h-24 bg-white text-slate-900 border-slate-300 dark:bg-slate-900 dark:text-slate-100 dark:border-slate-800 focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent"
          value={args}
          onChange={(e) => setArgs(e.target.value)}
        />
        <div className="mt-2 flex items-center gap-2">
          <Button onClick={run} disabled={pending}>
            {pending ? "Calling…" : "Call"}
          </Button>
        </div>
      </div>

      {err && (
        <div className="mt-3">
          <ErrorMessage error={err} />
        </div>
      )}
      {result && (
        <div className="mt-3 space-y-2">
          {result.content.map((c, i) =>
            c.type === "text" ? (
              <pre
                key={i}
                className="text-xs font-mono rounded p-3 overflow-auto max-h-48 whitespace-pre-wrap border bg-slate-50 border-slate-200 dark:bg-slate-950 dark:border-slate-800"
              >
                {c.text ?? ""}
              </pre>
            ) : (
              <pre
                key={i}
                className="text-xs font-mono rounded p-3 overflow-auto max-h-48 border bg-slate-50 border-slate-200 dark:bg-slate-950 dark:border-slate-800"
              >
                {JSON.stringify(c, null, 2)}
              </pre>
            ),
          )}
        </div>
      )}
    </div>
  );
}

function placeholderArgs(schema: Record<string, unknown>): string {
  if (!schema || schema.type !== "object") return "{}";
  const props = (schema.properties ?? {}) as Record<string, { type?: string }>;
  const example: Record<string, unknown> = {};
  for (const [k, p] of Object.entries(props)) {
    switch (p.type) {
      case "string":
        example[k] = "";
        break;
      case "number":
      case "integer":
        example[k] = 0;
        break;
      case "boolean":
        example[k] = false;
        break;
      case "array":
        example[k] = [];
        break;
      case "object":
        example[k] = {};
        break;
      default:
        example[k] = null;
    }
  }
  return JSON.stringify(example, null, 2);
}
