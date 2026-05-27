import { useQuery } from "@tanstack/react-query";
import { Link, useLocation, useParams } from "react-router-dom";
import { useState } from "react";
import { getAgent, invokeAgent, InvokeResult } from "../api";
import {
  Badge,
  Button,
  Card,
  ErrorMessage,
  Input,
  phaseTone,
} from "../components/ui";
import MCPTools from "../components/MCPTools";

export default function AgentDetail() {
  const { namespace, name } = useParams<{ namespace: string; name: string }>();
  const location = useLocation();
  const fromMCP = location.pathname.startsWith("/mcp/");
  const { data, isLoading, error } = useQuery({
    queryKey: ["agent", namespace, name],
    queryFn: () => getAgent(namespace!, name!),
    refetchInterval: 2000,
    enabled: Boolean(namespace && name),
  });

  if (isLoading) return <div className="text-sm text-slate-500">Loading…</div>;
  if (error) return <ErrorMessage error={error} />;
  if (!data) return null;

  const isMCP = data.spec.protocol === "mcp";

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <Link
            to={fromMCP || isMCP ? "/mcp" : "/agents"}
            className="text-sm text-slate-500 hover:text-slate-900 dark:hover:text-slate-200"
          >
            ← {fromMCP || isMCP ? "MCP servers" : "Agents"}
          </Link>
          <h1 className="text-2xl font-semibold tracking-tight mt-1">
            <span className="text-slate-500 font-normal">
              {data.namespace}/
            </span>
            {data.name}
          </h1>
          <div className="mt-2 flex items-center gap-2 text-sm">
            <Badge tone={phaseTone(data.status.phase)}>
              {data.status.phase ?? "Unknown"}
            </Badge>
            <Badge tone={data.spec.mode === "always-on" ? "indigo" : "slate"}>
              {data.spec.mode ?? "—"}
            </Badge>
            {isMCP && <Badge tone="indigo">MCP</Badge>}
            <span className="text-slate-500">
              {data.status.readyReplicas ?? 0}/{data.status.replicas ?? 0}{" "}
              replicas
            </span>
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <Card>
          <h2 className="text-sm font-semibold mb-3">Spec</h2>
          <KVList
            rows={[
              ["Image", data.spec.image],
              ["Runtime", data.spec.runtime ?? "—"],
              ["Framework", data.spec.framework ?? "—"],
              ["Protocol", data.spec.protocol ?? "—"],
              ["Mode", data.spec.mode ?? "—"],
              ["Port", String(data.spec.port ?? "—")],
              ["Concurrency", String(data.spec.concurrency ?? "—")],
              [
                "Replicas",
                `${data.spec.minReplicas ?? 0} – ${data.spec.maxReplicas ?? "∞"}`,
              ],
              ["Idle window", data.spec.scaleToZeroAfter ?? "—"],
            ]}
          />
        </Card>
        <Card>
          <h2 className="text-sm font-semibold mb-3">Status</h2>
          <KVList
            rows={[
              ["Phase", data.status.phase ?? "—"],
              ["Desired", String(data.status.desiredReplicas ?? 0)],
              ["Replicas", String(data.status.replicas ?? 0)],
              ["Ready", String(data.status.readyReplicas ?? 0)],
              ["URL", data.status.url ?? "—"],
              [
                "Last invocation",
                data.status.lastInvocationAt
                  ? new Date(data.status.lastInvocationAt).toLocaleString()
                  : "never",
              ],
            ]}
          />
          {data.status.conditions?.length ? (
            <details className="mt-4">
              <summary className="text-xs text-slate-500 cursor-pointer">
                Conditions ({data.status.conditions.length})
              </summary>
              <ul className="mt-2 space-y-1 text-xs font-mono">
                {data.status.conditions.map((c) => (
                  <li key={c.type}>
                    <span className="text-slate-500">{c.type}=</span>
                    <span
                      className={
                        c.status === "True"
                          ? "text-emerald-600 dark:text-emerald-300"
                          : "text-rose-600 dark:text-rose-300"
                      }
                    >
                      {c.status}
                    </span>
                    {c.reason && (
                      <span className="text-slate-500"> ({c.reason})</span>
                    )}
                  </li>
                ))}
              </ul>
            </details>
          ) : null}
        </Card>
      </div>

      {isMCP && <MCPTools namespace={data.namespace} name={data.name} />}

      <Invoke namespace={data.namespace} name={data.name} />
    </div>
  );
}

function KVList({ rows }: { rows: [string, string][] }) {
  return (
    <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1.5 text-sm">
      {rows.map(([k, v]) => (
        <Row key={k} k={k} v={v} />
      ))}
    </dl>
  );
}

function Row({ k, v }: { k: string; v: string }) {
  return (
    <>
      <dt className="text-slate-500">{k}</dt>
      <dd className="font-mono text-xs break-all">{v}</dd>
    </>
  );
}

function Invoke({ namespace, name }: { namespace: string; name: string }) {
  const [body, setBody] = useState<string>(
    JSON.stringify({ hello: "world" }, null, 2),
  );
  const [suffix, setSuffix] = useState<string>("/");
  const [result, setResult] = useState<InvokeResult | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  async function run() {
    setPending(true);
    setErr(null);
    setResult(null);
    try {
      const r = await invokeAgent(namespace, name, body, suffix);
      setResult(r);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setPending(false);
    }
  }

  return (
    <Card>
      <h2 className="text-sm font-semibold mb-3">Invoke</h2>
      <p className="text-xs text-slate-500 mb-3">
        Calls the gateway at{" "}
        <code className="font-mono">
          /v1/agents/{namespace}/{name}{suffix}
        </code>
        . Set the gateway URL in Settings if it's on a different origin.
      </p>
      <div className="space-y-3">
        <div>
          <label className="text-xs text-slate-500">Path suffix</label>
          <Input
            value={suffix}
            onChange={(e) => setSuffix(e.target.value)}
            placeholder="/"
          />
        </div>
        <div>
          <label className="text-xs text-slate-500">Body (JSON or any)</label>
          <textarea
            className="w-full rounded-md border px-3 py-2 text-xs font-mono h-32 bg-white text-slate-900 border-slate-300 dark:bg-slate-900 dark:text-slate-100 dark:border-slate-800 focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent"
            value={body}
            onChange={(e) => setBody(e.target.value)}
          />
        </div>
        <Button onClick={run} disabled={pending}>
          {pending ? "Invoking…" : "Invoke"}
        </Button>
      </div>
      {err && (
        <div className="mt-4">
          <ErrorMessage error={err} />
        </div>
      )}
      {result && (
        <div className="mt-4 space-y-2">
          <div className="text-xs text-slate-500">
            <Badge tone={result.status < 400 ? "green" : "red"}>
              HTTP {result.status}
            </Badge>{" "}
            in {Math.round(result.durationMs)} ms
          </div>
          <pre className="text-xs font-mono rounded-md p-3 overflow-auto max-h-72 border bg-slate-50 border-slate-200 dark:bg-slate-950 dark:border-slate-800">
            {result.body || "(empty)"}
          </pre>
        </div>
      )}
    </Card>
  );
}
