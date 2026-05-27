import { NavLink } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { clsx } from "clsx";
import { ReactNode } from "react";

import { AgentView, listAgents } from "../api";
import { Logo } from "./Logo";
import { ThemeToggle } from "./ThemeToggle";
import { version, commit, repoURL } from "../version";

export default function Sidebar() {
  // Sidebar pulls a small page from each list (separately) so the rail
  // shows real counts without dragging hundreds of rows over the wire.
  const agentsQ = useQuery({
    queryKey: ["sidebar", "agents"],
    queryFn: () => listAgents({ pageSize: 8, sort: "name" }),
    refetchInterval: 5000,
  });
  const mcpQ = useQuery({
    queryKey: ["sidebar", "mcp"],
    queryFn: () => listAgents({ protocol: "mcp", pageSize: 8, sort: "name" }),
    refetchInterval: 5000,
  });

  const allAgents = agentsQ.data?.items ?? [];
  const agents = allAgents.filter((a) => a.spec.protocol !== "mcp");
  // Defensive: older control planes ignore ?protocol=mcp. Apply the
  // filter client-side so MCP doesn't double-count into Agents.
  const mcpServers = (mcpQ.data?.items ?? []).filter(
    (a) => a.spec.protocol === "mcp",
  );
  const agentsTotal = Math.max(
    0,
    (agentsQ.data?.total ?? 0) - mcpServers.length,
  );
  const mcpTotal =
    mcpServers.length < (mcpQ.data?.items?.length ?? 0)
      ? mcpServers.length
      : (mcpQ.data?.total ?? mcpServers.length);

  return (
    <aside
      className={clsx(
        "w-64 shrink-0 flex flex-col border-r",
        "border-slate-200 bg-slate-50",
        "dark:border-slate-800 dark:bg-slate-900",
      )}
    >
      <div className="px-5 py-4 border-b border-slate-200 dark:border-slate-800">
        <NavLink
          to="/"
          className="flex items-center gap-2 font-semibold tracking-tight"
        >
          <Logo className="h-7 w-7" />
          <span className="text-lg">Krypton</span>
        </NavLink>
      </div>

      <nav className="flex-1 overflow-y-auto px-3 py-4 space-y-6 text-sm">
        <Section title="Agents" count={agentsTotal}>
          <NavRow to="/agents" end>
            All agents
          </NavRow>
          {agents.map((a) => (
            <AgentRow key={agentKey(a)} agent={a} />
          ))}
          {agentsTotal > agents.length && (
            <div className="px-3 py-1 text-xs text-slate-500">
              +{agentsTotal - agents.length} more
            </div>
          )}
        </Section>

        <Section title="MCP servers" count={mcpTotal}>
          <NavRow to="/mcp" end>
            All MCP servers
          </NavRow>
          {mcpServers.map((a) => (
            <AgentRow key={agentKey(a)} agent={a} />
          ))}
          {mcpTotal > mcpServers.length && (
            <div className="px-3 py-1 text-xs text-slate-500">
              +{mcpTotal - mcpServers.length} more
            </div>
          )}
        </Section>

        <Section title="System">
          <NavRow to="/settings" end>
            Settings
          </NavRow>
        </Section>
      </nav>

      <div className="border-t border-slate-200 dark:border-slate-800 px-5 py-3 flex items-center justify-between text-xs text-slate-500">
        <a
          href={repoURL}
          target="_blank"
          rel="noopener noreferrer"
          className="hover:text-slate-900 dark:hover:text-slate-200 flex items-center gap-1"
          title={commit ? `commit ${commit}` : undefined}
        >
          <svg
            viewBox="0 0 16 16"
            className="h-3.5 w-3.5"
            fill="currentColor"
            aria-hidden
          >
            <path d="M8 0C3.58 0 0 3.58 0 8a8 8 0 0 0 5.47 7.59c.4.07.55-.17.55-.38v-1.34c-2.22.48-2.69-1.07-2.69-1.07-.36-.92-.89-1.16-.89-1.16-.73-.5.05-.49.05-.49.8.06 1.22.83 1.22.83.72 1.22 1.87.87 2.33.67.07-.52.28-.87.5-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.01.08-2.12 0 0 .67-.21 2.2.82A7.65 7.65 0 0 1 8 4.07c.68 0 1.36.09 2 .27 1.53-1.03 2.2-.82 2.2-.82.44 1.11.16 1.92.08 2.12.51.56.82 1.28.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48v2.2c0 .21.15.46.55.38A8 8 0 0 0 16 8c0-4.42-3.58-8-8-8Z" />
          </svg>
          kryptonhq/runtime
        </a>
        <span className="font-mono">v{version}</span>
      </div>

      <div className="border-t border-slate-200 dark:border-slate-800 px-5 py-2 flex items-center justify-between">
        <span className="text-xs text-slate-500">Theme</span>
        <ThemeToggle />
      </div>
    </aside>
  );
}

function Section({
  title,
  count,
  children,
}: {
  title: string;
  count?: number;
  children: ReactNode;
}) {
  return (
    <div>
      <div className="px-3 mb-1 flex items-center justify-between">
        <span className="text-[11px] font-semibold uppercase tracking-wider text-slate-500">
          {title}
        </span>
        {typeof count === "number" && (
          <span className="text-[11px] font-mono text-slate-400">{count}</span>
        )}
      </div>
      <div className="space-y-0.5">{children}</div>
    </div>
  );
}

function NavRow({
  to,
  end,
  children,
}: {
  to: string;
  end?: boolean;
  children: ReactNode;
}) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) =>
        clsx(
          "block rounded px-3 py-1.5 text-sm transition truncate",
          isActive
            ? "bg-accent/10 text-accent dark:bg-accent/15"
            : "text-slate-700 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800",
        )
      }
    >
      {children}
    </NavLink>
  );
}

function AgentRow({ agent }: { agent: AgentView }) {
  const isMCP = agent.spec.protocol === "mcp";
  const to = isMCP
    ? `/mcp/${agent.namespace}/${agent.name}`
    : `/agents/${agent.namespace}/${agent.name}`;
  return (
    <NavLink
      to={to}
      className={({ isActive }) =>
        clsx(
          "flex items-center justify-between rounded px-3 py-1 text-xs transition",
          isActive
            ? "bg-accent/10 text-accent dark:bg-accent/15"
            : "text-slate-600 hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-slate-800",
        )
      }
    >
      <span className="truncate font-mono">{agent.name}</span>
      <span className="ml-2 text-[10px] text-slate-400 truncate">
        {agent.namespace}
      </span>
    </NavLink>
  );
}

function agentKey(a: AgentView): string {
  return `${a.namespace}/${a.name}`;
}
