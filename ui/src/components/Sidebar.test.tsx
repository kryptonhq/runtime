import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import Sidebar from "./Sidebar";
import { agent, agentList, mcpAgent, model, modelList } from "../test/fixtures";
import { renderWithProviders } from "../test/render";
import { server } from "../test/server";

/** The <div> holding a section's rows, found via its heading. */
function section(title: string) {
  return screen.getByText(title).closest("div")!.parentElement!;
}

describe("Sidebar", () => {
  it("renders the top-level nav rows", async () => {
    renderWithProviders(<Sidebar />);

    expect(await screen.findByRole("link", { name: "All agents" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "All LLMs" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "All MCP servers" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Settings" })).toBeInTheDocument();
  });

  it("links the brand back to the root", () => {
    renderWithProviders(<Sidebar />);
    expect(screen.getByRole("link", { name: /Krypton/ })).toHaveAttribute("href", "/");
  });

  it("shows the build version and repo link", () => {
    renderWithProviders(<Sidebar />);
    expect(screen.getByRole("link", { name: /kryptonhq\/runtime/ })).toHaveAttribute(
      "href",
      "https://github.com/kryptonhq/runtime",
    );
    // Rendered as "v{version}"; version.ts strips any leading v so this
    // can't come out as "vv0.0.4".
    expect(screen.getByText(/^v/)).toBeInTheDocument();
  });

  it("embeds the theme toggle", () => {
    renderWithProviders(<Sidebar />);
    expect(screen.getByRole("button", { name: "Light" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Dark" })).toBeInTheDocument();
  });

  // MCP servers get their own section; they must not also appear under
  // Agents, and must not inflate the Agents count.
  it("splits agents from MCP servers", async () => {
    renderWithProviders(<Sidebar />);

    // Await the dynamic row, not the static "All agents" link — the rows
    // only appear once the list query resolves.
    const agentLink = await screen.findByRole("link", { name: /travel-agent/ });
    expect(agentLink).toHaveAttribute("href", "/agents/agents/travel-agent");

    // MCP entries route to /mcp/..., not /agents/...
    const mcpLink = screen.getByRole("link", { name: /mcp-hello/ });
    expect(mcpLink).toHaveAttribute("href", "/mcp/agents/mcp-hello");
  });

  it("subtracts MCP servers from the agents count", async () => {
    server.use(
      // The list endpoint returns everything; the sidebar's mcp query
      // filters to MCP only.
      http.get("/v1/agents", ({ request }) => {
        const protocol = new URL(request.url).searchParams.get("protocol");
        if (protocol === "mcp") {
          return HttpResponse.json({
            items: [mcpAgent()],
            page: 1,
            pageSize: 8,
            total: 1,
          });
        }
        return HttpResponse.json({
          items: [agent(), mcpAgent()],
          page: 1,
          pageSize: 8,
          total: 5, // 5 total objects, 1 of which is MCP
        });
      }),
    );

    renderWithProviders(<Sidebar />);
    await screen.findByRole("link", { name: /travel-agent/ });

    // 5 total - 1 MCP = 4 agents.
    expect(within(section("Agents")).getByText("4")).toBeInTheDocument();
    expect(within(section("MCP servers")).getByText("1")).toBeInTheDocument();
  });

  it("shows a '+N more' hint when the rail is truncated", async () => {
    server.use(
      http.get("/v1/agents", ({ request }) => {
        const protocol = new URL(request.url).searchParams.get("protocol");
        if (protocol === "mcp") {
          return HttpResponse.json({ items: [], page: 1, pageSize: 8, total: 0 });
        }
        // The sidebar requests pageSize 8 but there are 30 agents.
        return HttpResponse.json({
          items: Array.from({ length: 8 }, (_, i) =>
            agent({ name: `agent-${i}`, uid: `uid-${i}` }),
          ),
          page: 1,
          pageSize: 8,
          total: 30,
        });
      }),
    );

    renderWithProviders(<Sidebar />);
    expect(await screen.findByText("+22 more")).toBeInTheDocument();
  });

  it("lists models and links to their detail pages", async () => {
    renderWithProviders(<Sidebar />);
    const link = await screen.findByRole("link", { name: /qwen2-0-5b/ });
    expect(link).toHaveAttribute("href", "/llms/models/qwen2-0-5b");
  });

  it("renders section counts of zero on an empty cluster", async () => {
    server.use(
      http.get("/v1/agents", () => HttpResponse.json(agentList([]))),
      http.get("/v1/models", () => HttpResponse.json(modelList([]))),
    );

    renderWithProviders(<Sidebar />);
    await screen.findByRole("link", { name: "All agents" });

    expect(within(section("Agents")).getByText("0")).toBeInTheDocument();
    expect(within(section("LLMs")).getByText("0")).toBeInTheDocument();
    expect(within(section("MCP servers")).getByText("0")).toBeInTheDocument();
  });

  // A failing control plane must not blank the whole rail — the nav links
  // are the only way to get to Settings and fix the endpoint.
  it("still renders navigation when the API is down", async () => {
    server.use(
      http.get("/v1/agents", () => HttpResponse.json({ error: "down" }, { status: 500 })),
      http.get("/v1/models", () => HttpResponse.json({ error: "down" }, { status: 500 })),
    );

    renderWithProviders(<Sidebar />);

    expect(await screen.findByRole("link", { name: "All agents" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Settings" })).toBeInTheDocument();
  });

  // NavLink's isActive drives the highlight. `end` on the top-level rows
  // stops /agents staying highlighted while on /agents/ns/name.
  it("marks the active section", async () => {
    renderWithProviders(<Sidebar />, { route: "/settings" });

    const settings = await screen.findByRole("link", { name: "Settings" });
    expect(settings.className).toMatch(/accent/);

    const agents = screen.getByRole("link", { name: "All agents" });
    expect(agents.className).not.toMatch(/bg-accent/);
  });

  it("does not keep 'All agents' active on a detail route", async () => {
    renderWithProviders(<Sidebar />, { route: "/agents/agents/travel-agent" });

    // The specific agent row is the active one instead.
    const row = await screen.findByRole("link", { name: /travel-agent/ });
    expect(row.className).toMatch(/accent/);

    const allAgents = screen.getByRole("link", { name: "All agents" });
    expect(allAgents.className).not.toMatch(/bg-accent/);
  });

  // The sidebar asks for a small page so the rail doesn't drag hundreds of
  // rows over the wire on every 5s refetch.
  it("requests only a small page from each list", async () => {
    const pageSizes: Array<string | null> = [];
    server.use(
      http.get("/v1/agents", ({ request }) => {
        pageSizes.push(new URL(request.url).searchParams.get("pageSize"));
        return HttpResponse.json(agentList([agent()]));
      }),
      http.get("/v1/models", ({ request }) => {
        pageSizes.push(new URL(request.url).searchParams.get("pageSize"));
        return HttpResponse.json(modelList([model()]));
      }),
    );

    renderWithProviders(<Sidebar />);
    await screen.findByRole("link", { name: /travel-agent/ });

    expect(pageSizes.length).toBeGreaterThan(0);
    for (const size of pageSizes) {
      expect(Number(size)).toBeLessThanOrEqual(8);
    }
  });
});
