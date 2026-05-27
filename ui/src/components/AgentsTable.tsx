import { useQuery, keepPreviousData } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { clsx } from "clsx";
import {
  AgentView,
  ListAgentsParams,
  listAgents,
} from "../api";
import { Badge, Input, Muted, phaseTone } from "./ui";

type SortField = NonNullable<ListAgentsParams["sort"]>;
type SortOrder = NonNullable<ListAgentsParams["order"]>;

interface Column {
  field: SortField | null; // null = not sortable
  label: string;
  className?: string;
  cell: (a: AgentView) => React.ReactNode;
}

interface Props {
  // What this table filters server-side. The MCP list passes "mcp"; the
  // Agents list passes nothing (and filters out MCP client-side via the
  // `excludeProtocol` prop).
  protocol?: "mcp" | "a2a" | "http";
  // Excluding a protocol can't be done server-side today (the API only
  // takes a positive match). For the "all non-MCP" agents page we drop
  // MCP rows on the client. Cheap; the page size is small.
  excludeProtocol?: "mcp";
  detailHref: (a: AgentView) => string;
  columns: Column[];
  // Optional UI tweaks
  searchPlaceholder?: string;
}

const PAGE_SIZE = 20;
const DEFAULT_SORT: SortField = "name";
const DEFAULT_ORDER: SortOrder = "asc";

export default function AgentsTable({
  protocol,
  excludeProtocol,
  detailHref,
  columns,
  searchPlaceholder = "Search by name, namespace, or image…",
}: Props) {
  const [search, setSearch] = useState("");
  const [debounced, setDebounced] = useState("");
  const [sort, setSort] = useState<SortField>(DEFAULT_SORT);
  const [order, setOrder] = useState<SortOrder>(DEFAULT_ORDER);
  const [page, setPage] = useState(1);

  useEffect(() => {
    const t = setTimeout(() => setDebounced(search), 250);
    return () => clearTimeout(t);
  }, [search]);

  // Reset to page 1 whenever filters change.
  useEffect(() => {
    setPage(1);
  }, [debounced, sort, order, protocol]);

  const params: ListAgentsParams = {
    protocol,
    q: debounced || undefined,
    sort,
    order,
    page,
    pageSize: PAGE_SIZE,
  };

  const { data, isLoading, isFetching } = useQuery({
    queryKey: ["agents", params],
    queryFn: () => listAgents(params),
    refetchInterval: 3000,
    placeholderData: keepPreviousData,
  });

  // Defensive client-side filtering. We still send the params to the
  // server so a modern control plane offloads the work; if the running
  // backend ignores them we re-apply here so the UI stays correct.
  const rows = useMemo(() => {
    let items = data?.items ?? [];
    if (protocol) items = items.filter((a) => a.spec.protocol === protocol);
    if (excludeProtocol)
      items = items.filter((a) => a.spec.protocol !== excludeProtocol);
    if (debounced) {
      const needle = debounced.toLowerCase();
      items = items.filter(
        (a) =>
          a.name.toLowerCase().includes(needle) ||
          a.namespace.toLowerCase().includes(needle) ||
          (a.spec.image ?? "").toLowerCase().includes(needle),
      );
    }
    return items;
  }, [data, protocol, excludeProtocol, debounced]);

  // If client-side filtering shrank the result set, trust the local
  // count; otherwise use whatever the server reported.
  const serverItems = data?.items?.length ?? 0;
  const serverTotal = data?.total ?? 0;
  const total = rows.length < serverItems ? rows.length : serverTotal;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  function onSort(field: SortField) {
    if (sort === field) {
      setOrder(order === "asc" ? "desc" : "asc");
    } else {
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
            placeholder={searchPlaceholder}
          />
        </div>
        <Muted className="text-xs whitespace-nowrap">
          {isLoading
            ? "Loading…"
            : `${total} ${total === 1 ? "result" : "results"}${isFetching && !isLoading ? " · syncing" : ""}`}
        </Muted>
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900/50">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-slate-50/70 dark:bg-slate-900/60 text-slate-500 text-left text-[11px] uppercase tracking-wider border-b border-slate-200 dark:border-slate-800">
              <tr>
                {columns.map((c) => (
                  <th
                    key={c.label}
                    className={clsx("px-5 py-3 font-medium", c.className)}
                  >
                    {c.field ? (
                      <button
                        type="button"
                        onClick={() => onSort(c.field!)}
                        className="inline-flex items-center gap-1 hover:text-slate-900 dark:hover:text-slate-200"
                      >
                        {c.label}
                        <SortIndicator
                          active={sort === c.field}
                          order={order}
                        />
                      </button>
                    ) : (
                      c.label
                    )}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-200 dark:divide-slate-800">
              {rows.length === 0 && !isLoading ? (
                <tr>
                  <td
                    colSpan={columns.length}
                    className="px-5 py-12 text-center text-sm text-slate-500"
                  >
                    {debounced
                      ? `No matches for “${debounced}”.`
                      : "Nothing here yet."}
                  </td>
                </tr>
              ) : (
                rows.map((a) => (
                  <tr
                    key={`${a.namespace}/${a.name}`}
                    className="hover:bg-slate-50 dark:hover:bg-slate-900/40 transition cursor-pointer"
                    onClick={(e) => {
                      // Don't hijack clicks that landed on an actual link/button.
                      if (
                        (e.target as HTMLElement).closest("a,button") === null
                      ) {
                        window.location.href = detailHref(a);
                      }
                    }}
                  >
                    {columns.map((c) => (
                      <td
                        key={c.label}
                        className={clsx("px-5 py-3.5", c.className)}
                      >
                        {c.cell(a)}
                      </td>
                    ))}
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>

      {totalPages > 1 && (
        <Pagination
          page={page}
          totalPages={totalPages}
          total={total}
          pageSize={PAGE_SIZE}
          onPage={setPage}
        />
      )}
    </div>
  );
}

function SortIndicator({
  active,
  order,
}: {
  active: boolean;
  order: SortOrder;
}) {
  return (
    <span
      aria-hidden
      className={clsx(
        "text-[10px] leading-none",
        active ? "text-accent" : "text-slate-300 dark:text-slate-700",
      )}
    >
      {active ? (order === "asc" ? "▲" : "▼") : "↕"}
    </span>
  );
}

function Pagination({
  page,
  totalPages,
  total,
  pageSize,
  onPage,
}: {
  page: number;
  totalPages: number;
  total: number;
  pageSize: number;
  onPage: (p: number) => void;
}) {
  const from = Math.min(total, (page - 1) * pageSize + 1);
  const to = Math.min(total, page * pageSize);
  return (
    <div className="flex items-center justify-between text-xs text-slate-500">
      <span>
        Showing {from}–{to} of {total}
      </span>
      <div className="flex items-center gap-1">
        <PageBtn disabled={page === 1} onClick={() => onPage(page - 1)}>
          ‹ Prev
        </PageBtn>
        <span className="px-2 font-mono">
          {page} / {totalPages}
        </span>
        <PageBtn
          disabled={page >= totalPages}
          onClick={() => onPage(page + 1)}
        >
          Next ›
        </PageBtn>
      </div>
    </div>
  );
}

function PageBtn({
  children,
  disabled,
  onClick,
}: {
  children: React.ReactNode;
  disabled?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={clsx(
        "rounded-md border px-2 py-1 text-xs transition",
        "border-slate-200 dark:border-slate-800",
        "hover:bg-slate-100 dark:hover:bg-slate-800",
        "disabled:opacity-40 disabled:cursor-not-allowed disabled:hover:bg-transparent",
      )}
    >
      {children}
    </button>
  );
}

// Reusable cell renderers for the common columns shared by the Agents
// and MCP pages.
export function nameCell(detailHref: (a: AgentView) => string) {
  return (a: AgentView) => (
    <Link
      to={detailHref(a)}
      className="font-medium text-accent hover:underline"
      onClick={(e) => e.stopPropagation()}
    >
      {a.name}
    </Link>
  );
}

export function namespaceCell(a: AgentView) {
  return <span className="text-slate-500">{a.namespace}</span>;
}

export function modeCell(a: AgentView) {
  return (
    <Badge tone={a.spec.mode === "always-on" ? "indigo" : "slate"}>
      {a.spec.mode ?? "—"}
    </Badge>
  );
}

export function phaseCell(a: AgentView) {
  return (
    <Badge tone={phaseTone(a.status.phase)}>{a.status.phase ?? "—"}</Badge>
  );
}

export function replicasCell(a: AgentView) {
  return (
    <span className="font-mono text-slate-700 dark:text-slate-300">
      {a.status.readyReplicas ?? 0}/{a.status.replicas ?? 0}
    </span>
  );
}

export function imageCell(a: AgentView) {
  return (
    <span className="truncate max-w-xs font-mono text-xs text-slate-500 block">
      {a.spec.image}
    </span>
  );
}
