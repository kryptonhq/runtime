import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { clsx } from "clsx";
import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  ChatMessage,
  InvokeResult,
  ListModelsParams,
  ModelView,
  invokeModelChat,
  listModels,
} from "../api";
import {
  Badge,
  Button,
  Card,
  ErrorMessage,
  Input,
  Muted,
  phaseTone,
} from "../components/ui";

type SortField = NonNullable<ListModelsParams["sort"]>;
type SortOrder = NonNullable<ListModelsParams["order"]>;

const PAGE_SIZE = 20;

export default function LLMList() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">LLMs</h1>
        <p className="text-sm text-slate-500 mt-1">
          Self-hosted llama.cpp models registered across the cluster.
        </p>
      </div>

      <ModelsTable />
    </div>
  );
}

function ModelsTable() {
  const navigate = useNavigate();
  const [search, setSearch] = useState("");
  const [debounced, setDebounced] = useState("");
  const [sort, setSort] = useState<SortField>("name");
  const [order, setOrder] = useState<SortOrder>("asc");
  const [page, setPage] = useState(1);

  useEffect(() => {
    const t = setTimeout(() => setDebounced(search), 250);
    return () => clearTimeout(t);
  }, [search]);

  useEffect(() => {
    setPage(1);
  }, [debounced, sort, order]);

  const params: ListModelsParams = {
    q: debounced || undefined,
    sort,
    order,
    page,
    pageSize: PAGE_SIZE,
  };
  const { data, isLoading, isFetching, error } = useQuery({
    queryKey: ["models", params],
    queryFn: () => listModels(params),
    refetchInterval: 3000,
    placeholderData: keepPreviousData,
  });

  const rows = useMemo(() => data?.items ?? [], [data]);

  const total = data?.total ?? rows.length;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  function onSort(field: SortField) {
    if (sort === field) setOrder(order === "asc" ? "desc" : "asc");
    else {
      setSort(field);
      setOrder("asc");
    }
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-col sm:flex-row sm:items-center gap-3">
        <div className="flex-1 min-w-0">
          <Input
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, namespace, runtime, or Hugging Face source..."
          />
        </div>
        <Muted className="text-xs whitespace-nowrap">
          {isLoading
            ? "Loading..."
            : `${total} ${total === 1 ? "model" : "models"}${isFetching && !isLoading ? " · syncing" : ""}`}
        </Muted>
      </div>

      {error && <ErrorMessage error={error} />}

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900/50">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-slate-50/70 dark:bg-slate-900/60 text-slate-500 text-left text-[11px] uppercase tracking-wider border-b border-slate-200 dark:border-slate-800">
              <tr>
                <Header
                  label="Name"
                  field="name"
                  sort={sort}
                  order={order}
                  onSort={onSort}
                />
                <Header
                  label="Namespace"
                  field="namespace"
                  sort={sort}
                  order={order}
                  onSort={onSort}
                />
                <Header
                  label="Phase"
                  field="phase"
                  sort={sort}
                  order={order}
                  onSort={onSort}
                />
                <Header
                  label="Replicas"
                  field="replicas"
                  sort={sort}
                  order={order}
                  onSort={onSort}
                />
                <Header
                  label="Runtime"
                  field="runtime"
                  sort={sort}
                  order={order}
                  onSort={onSort}
                />
                <Header
                  label="Source"
                  field="source"
                  sort={sort}
                  order={order}
                  onSort={onSort}
                />
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-200 dark:divide-slate-800">
              {rows.length === 0 && !isLoading ? (
                <tr>
                  <td
                    colSpan={6}
                    className="px-5 py-12 text-center text-sm text-slate-500"
                  >
                    {debounced
                      ? `No matches for "${debounced}".`
                      : "No LLMs deployed yet."}
                  </td>
                </tr>
              ) : (
                rows.map((model) => (
                  <tr
                    key={`${model.namespace}/${model.name}`}
                    className="transition cursor-pointer hover:bg-slate-50 dark:hover:bg-slate-900/40"
                    onClick={() =>
                      navigate(`/llms/${model.namespace}/${model.name}`)
                    }
                  >
                    <td className="px-5 py-3.5 font-medium text-accent">
                      {model.name}
                    </td>
                    <td className="px-5 py-3.5 text-slate-500">
                      {model.namespace}
                    </td>
                    <td className="px-5 py-3.5">
                      <Badge tone={phaseTone(model.status.phase)}>
                        {model.status.phase ?? "-"}
                      </Badge>
                    </td>
                    <td className="px-5 py-3.5 font-mono text-slate-700 dark:text-slate-300">
                      {model.status.readyReplicas ?? 0}/
                      {model.status.replicas ?? 0}
                    </td>
                    <td className="px-5 py-3.5 font-mono text-xs text-slate-500">
                      {model.spec.runtime ?? "-"}
                    </td>
                    <td className="px-5 py-3.5">
                      <span className="block max-w-sm truncate font-mono text-xs text-slate-500">
                        {model.spec.source.huggingface}/
                        {model.spec.source.file}
                      </span>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>

      {totalPages > 1 && (
        <div className="flex items-center justify-between text-xs text-slate-500">
          <span>
            Page {page} of {totalPages}
          </span>
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              className="text-xs px-2 py-1"
              disabled={page === 1}
              onClick={() => setPage(page - 1)}
            >
              Prev
            </Button>
            <Button
              variant="ghost"
              className="text-xs px-2 py-1"
              disabled={page >= totalPages}
              onClick={() => setPage(page + 1)}
            >
              Next
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

function Header({
  label,
  field,
  sort,
  order,
  onSort,
}: {
  label: string;
  field: SortField;
  sort: SortField;
  order: SortOrder;
  onSort: (field: SortField) => void;
}) {
  const active = sort === field;
  return (
    <th className="px-5 py-3 font-medium">
      <button
        type="button"
        onClick={() => onSort(field)}
        className="inline-flex items-center gap-1 hover:text-slate-900 dark:hover:text-slate-200"
      >
        {label}
        <span
          aria-hidden
          className={
            active
              ? "text-accent text-[10px]"
              : "text-slate-300 dark:text-slate-700 text-[10px]"
          }
        >
          {active ? (order === "asc" ? "▲" : "▼") : "↕"}
        </span>
      </button>
    </th>
  );
}

export function ChatBox({ model }: { model: ModelView | null }) {
  const [prompt, setPrompt] = useState("Say hi in one word.");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [result, setResult] = useState<InvokeResult | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  async function send() {
    if (!model || prompt.trim() === "") return;
    const nextMessages: ChatMessage[] = [
      ...messages,
      { role: "user", content: prompt.trim() },
    ];
    setMessages(nextMessages);
    setPrompt("");
    setPending(true);
    setErr(null);
    setResult(null);
    try {
      const response = await invokeModelChat(model.name, nextMessages);
      setResult(response);
      const assistantText = extractAssistantText(response.body);
      if (response.status < 400 && assistantText) {
        setMessages([
          ...nextMessages,
          { role: "assistant", content: assistantText },
        ]);
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setPending(false);
    }
  }

  return (
    <Card>
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 mb-4">
        <div>
          <h2 className="text-sm font-semibold">Chat test</h2>
          <p className="text-xs text-slate-500 mt-1">
            {model
              ? `Invokes ${model.name} through /v1/chat/completions. Set the gateway URL in Settings if it is on a different origin.`
              : "Select a model from the table to invoke it."}
          </p>
        </div>
        {model && (
          <Badge tone={phaseTone(model.status.phase)}>
            {model.namespace}/{model.name}
          </Badge>
        )}
      </div>

      <div className="space-y-3">
        <div className="min-h-32 rounded-md border border-slate-200 bg-slate-50 p-3 dark:border-slate-800 dark:bg-slate-950/50">
          {messages.length === 0 ? (
            <div className="text-sm text-slate-500">
              Messages will appear here after the first test call.
            </div>
          ) : (
            <div className="space-y-2">
              {messages.map((message, index) => (
                <div
                  key={index}
                  className={clsx(
                    "rounded-md px-3 py-2 text-sm",
                    message.role === "user"
                      ? "bg-white text-slate-900 dark:bg-slate-900 dark:text-slate-100"
                      : "bg-accent/10 text-slate-900 dark:text-slate-100",
                  )}
                >
                  <div className="mb-1 text-[11px] uppercase tracking-wider text-slate-500">
                    {message.role}
                  </div>
                  <div className="whitespace-pre-wrap">{message.content}</div>
                </div>
              ))}
            </div>
          )}
        </div>

        <div>
          <label className="text-xs text-slate-500">Message</label>
          <textarea
            className="w-full rounded-md border px-3 py-2 text-sm h-24 bg-white text-slate-900 border-slate-300 placeholder:text-slate-400 dark:bg-slate-900 dark:text-slate-100 dark:border-slate-800 focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            disabled={!model || pending}
          />
        </div>

        <div className="flex items-center gap-2">
          <Button
            onClick={send}
            disabled={!model || pending || prompt.trim() === ""}
          >
            {pending ? "Sending..." : "Send"}
          </Button>
          <Button
            variant="ghost"
            onClick={() => {
              setMessages([]);
              setResult(null);
              setErr(null);
            }}
            disabled={pending || messages.length === 0}
          >
            Clear
          </Button>
        </div>
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
          {result.status >= 400 && (
            <pre className="text-xs font-mono rounded-md p-3 overflow-auto max-h-72 border bg-slate-50 border-slate-200 dark:bg-slate-950 dark:border-slate-800">
              {result.body || "(empty)"}
            </pre>
          )}
        </div>
      )}
    </Card>
  );
}

function extractAssistantText(body: string): string {
  try {
    const parsed = JSON.parse(body);
    return parsed?.choices?.[0]?.message?.content ?? "";
  } catch {
    return body;
  }
}
