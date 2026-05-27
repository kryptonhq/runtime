import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { getModel } from "../api";
import {
  Badge,
  Card,
  ErrorMessage,
  phaseTone,
} from "../components/ui";
import { ChatBox } from "./LLMList";

export default function LLMDetail() {
  const { namespace, name } = useParams<{ namespace: string; name: string }>();
  const { data, isLoading, error } = useQuery({
    queryKey: ["model", namespace, name],
    queryFn: () => getModel(namespace!, name!),
    refetchInterval: 2000,
    enabled: Boolean(namespace && name),
  });

  if (isLoading) return <div className="text-sm text-slate-500">Loading...</div>;
  if (error) return <ErrorMessage error={error} />;
  if (!data) return null;

  return (
    <div className="space-y-6">
      <div>
        <Link
          to="/llms"
          className="text-sm text-slate-500 hover:text-slate-900 dark:hover:text-slate-200"
        >
          ← LLMs
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
          <Badge tone="indigo">{data.spec.runtime ?? "llama.cpp"}</Badge>
          <span className="text-slate-500">
            {data.status.readyReplicas ?? 0}/{data.status.replicas ?? 0}{" "}
            replicas
          </span>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <Card>
          <h2 className="text-sm font-semibold mb-3">Spec</h2>
          <KVList
            rows={[
              ["Runtime", data.spec.runtime ?? "llama.cpp"],
              ["Hugging Face repo", data.spec.source.huggingface],
              ["File", data.spec.source.file],
              ["Image", data.spec.image || "default llama.cpp server"],
              ["Port", String(data.spec.port ?? "-")],
              ["Replicas", `${data.spec.minReplicas ?? 1} - ${data.spec.maxReplicas ?? 1}`],
              ["Args", data.spec.args?.join(" ") || "-"],
            ]}
          />
        </Card>
        <Card>
          <h2 className="text-sm font-semibold mb-3">Status</h2>
          <KVList
            rows={[
              ["Phase", data.status.phase ?? "-"],
              ["Replicas", String(data.status.replicas ?? 0)],
              ["Ready", String(data.status.readyReplicas ?? 0)],
              ["URL", data.status.url ?? "-"],
            ]}
          />
          {data.status.conditions?.length ? (
            <details className="mt-4">
              <summary className="text-xs text-slate-500 cursor-pointer">
                Conditions ({data.status.conditions.length})
              </summary>
              <ul className="mt-2 space-y-1 text-xs font-mono">
                {data.status.conditions.map((condition) => (
                  <li key={condition.type}>
                    <span className="text-slate-500">{condition.type}=</span>
                    <span
                      className={
                        condition.status === "True"
                          ? "text-emerald-600 dark:text-emerald-300"
                          : "text-rose-600 dark:text-rose-300"
                      }
                    >
                      {condition.status}
                    </span>
                    {condition.reason && (
                      <span className="text-slate-500">
                        {" "}
                        ({condition.reason})
                      </span>
                    )}
                  </li>
                ))}
              </ul>
            </details>
          ) : null}
        </Card>
      </div>

      <ChatBox model={data} />
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
