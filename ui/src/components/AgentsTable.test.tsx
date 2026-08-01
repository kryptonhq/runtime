import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import AgentsTable, {
  imageCell,
  modeCell,
  nameCell,
  namespaceCell,
  phaseCell,
  replicasCell,
} from "./AgentsTable";
import { agent, agentList, mcpAgent } from "../test/fixtures";
import { renderWithProviders } from "../test/render";
import { server } from "../test/server";

const columns = [
  {
    field: "name" as const,
    label: "Name",
    cell: nameCell((a) => `/agents/${a.namespace}/${a.name}`),
  },
  { field: "namespace" as const, label: "Namespace", cell: namespaceCell },
  { field: null, label: "Mode", cell: modeCell },
  { field: "phase" as const, label: "Phase", cell: phaseCell },
  { field: "replicas" as const, label: "Replicas", cell: replicasCell },
  { field: "image" as const, label: "Image", cell: imageCell },
];

function renderTable(props: Partial<React.ComponentProps<typeof AgentsTable>> = {}) {
  return renderWithProviders(
    <AgentsTable
      detailHref={(a) => `/agents/${a.namespace}/${a.name}`}
      columns={columns}
      {...props}
    />,
  );
}

describe("AgentsTable", () => {
  it("renders a row per agent", async () => {
    renderTable();
    expect(await screen.findByRole("link", { name: "travel-agent" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "mcp-hello" })).toBeInTheDocument();
  });

  it("renders the column headers", async () => {
    renderTable();
    await screen.findByRole("link", { name: "travel-agent" });
    for (const label of ["Name", "Namespace", "Mode", "Phase", "Replicas", "Image"]) {
      expect(screen.getByText(label)).toBeInTheDocument();
    }
  });

  it("shows a result count", async () => {
    renderTable();
    expect(await screen.findByText(/2 results/)).toBeInTheDocument();
  });

  it("uses the singular for exactly one result", async () => {
    server.use(
      http.get("/v1/agents", () => HttpResponse.json(agentList([agent()]))),
    );
    renderTable();
    expect(await screen.findByText(/1 result$/)).toBeInTheDocument();
  });

  it("renders an empty state when there are no agents", async () => {
    server.use(http.get("/v1/agents", () => HttpResponse.json(agentList([]))));
    renderTable();
    expect(await screen.findByText("Nothing here yet.")).toBeInTheDocument();
  });

  // The Agents page shows everything except MCP servers, which get their own
  // page. The API has no negative filter, so this happens client-side.
  it("excludes a protocol client-side", async () => {
    renderTable({ excludeProtocol: "mcp" });
    expect(await screen.findByRole("link", { name: "travel-agent" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "mcp-hello" })).not.toBeInTheDocument();
  });

  it("sends the protocol filter to the server", async () => {
    let protocol: string | null = null;
    server.use(
      http.get("/v1/agents", ({ request }) => {
        protocol = new URL(request.url).searchParams.get("protocol");
        return HttpResponse.json(agentList([mcpAgent()]));
      }),
    );
    renderTable({ protocol: "mcp" });
    await screen.findByRole("link", { name: "mcp-hello" });
    expect(protocol).toBe("mcp");
  });

  // Search is debounced by 250ms. Typing must not fire a request per key.
  it("debounces the search box before querying", async () => {
    const user = userEvent.setup();
    let requests = 0;
    server.use(
      http.get("/v1/agents", ({ request }) => {
        requests++;
        const q = new URL(request.url).searchParams.get("q")?.toLowerCase();
        let items = [agent(), mcpAgent()];
        if (q) items = items.filter((a) => a.name.toLowerCase().includes(q));
        return HttpResponse.json(agentList(items));
      }),
    );

    renderTable();
    await screen.findByRole("link", { name: "travel-agent" });
    const before = requests;

    await user.type(screen.getByRole("searchbox"), "travel");

    await waitFor(
      () => {
        expect(screen.queryByRole("link", { name: "mcp-hello" })).not.toBeInTheDocument();
      },
      { timeout: 2000 },
    );
    // Six keystrokes must not produce six extra fetches.
    expect(requests - before).toBeLessThan(6);
  });

  it("shows a no-matches message naming the search term", async () => {
    const user = userEvent.setup();
    server.use(
      http.get("/v1/agents", ({ request }) => {
        const q = new URL(request.url).searchParams.get("q");
        return HttpResponse.json(agentList(q ? [] : [agent()]));
      }),
    );

    renderTable();
    await screen.findByRole("link", { name: "travel-agent" });
    await user.type(screen.getByRole("searchbox"), "zzzz");

    expect(
      await screen.findByText(/No matches for/, {}, { timeout: 2000 }),
    ).toBeInTheDocument();
  });

  // Clicking a sortable header sorts ascending; clicking the same header
  // again flips direction rather than re-sorting ascending.
  it("toggles sort order on repeated header clicks", async () => {
    const user = userEvent.setup();
    const seen: Array<{ sort: string | null; order: string | null }> = [];
    server.use(
      http.get("/v1/agents", ({ request }) => {
        const p = new URL(request.url).searchParams;
        seen.push({ sort: p.get("sort"), order: p.get("order") });
        return HttpResponse.json(agentList([agent()]));
      }),
    );

    renderTable();
    await screen.findByRole("link", { name: "travel-agent" });

    await user.click(screen.getByRole("button", { name: /Phase/ }));
    await waitFor(() =>
      expect(seen.at(-1)).toEqual({ sort: "phase", order: "asc" }),
    );

    await user.click(screen.getByRole("button", { name: /Phase/ }));
    await waitFor(() =>
      expect(seen.at(-1)).toEqual({ sort: "phase", order: "desc" }),
    );
  });

  it("resets to ascending when switching sort field", async () => {
    const user = userEvent.setup();
    const seen: Array<{ sort: string | null; order: string | null }> = [];
    server.use(
      http.get("/v1/agents", ({ request }) => {
        const p = new URL(request.url).searchParams;
        seen.push({ sort: p.get("sort"), order: p.get("order") });
        return HttpResponse.json(agentList([agent()]));
      }),
    );

    renderTable();
    await screen.findByRole("link", { name: "travel-agent" });

    await user.click(screen.getByRole("button", { name: /Phase/ }));
    await user.click(screen.getByRole("button", { name: /Phase/ })); // desc
    await user.click(screen.getByRole("button", { name: /Namespace/ }));

    await waitFor(() =>
      expect(seen.at(-1)).toEqual({ sort: "namespace", order: "asc" }),
    );
  });

  it("does not render a sort control for non-sortable columns", async () => {
    renderTable();
    await screen.findByRole("link", { name: "travel-agent" });
    // "Mode" has field: null, so it's plain header text, not a button.
    expect(screen.queryByRole("button", { name: /Mode/ })).not.toBeInTheDocument();
  });

  it("hides pagination when everything fits on one page", async () => {
    renderTable();
    await screen.findByRole("link", { name: "travel-agent" });
    expect(screen.queryByRole("button", { name: /Next/ })).not.toBeInTheDocument();
  });

  it("paginates when the server reports more than one page", async () => {
    const user = userEvent.setup();
    const pages: string[] = [];
    server.use(
      http.get("/v1/agents", ({ request }) => {
        const p = new URL(request.url).searchParams;
        pages.push(p.get("page") ?? "1");
        // 45 total across 20-per-page => 3 pages.
        return HttpResponse.json({
          items: Array.from({ length: 20 }, (_, i) =>
            agent({ name: `agent-${p.get("page")}-${i}`, uid: `u${i}` }),
          ),
          page: Number(p.get("page") ?? 1),
          pageSize: 20,
          total: 45,
        });
      }),
    );

    renderTable();
    await screen.findByText("1 / 3");

    await user.click(screen.getByRole("button", { name: /Next/ }));
    await waitFor(() => expect(pages.at(-1)).toBe("2"));
    expect(await screen.findByText("2 / 3")).toBeInTheDocument();
  });

  it("disables Prev on the first page", async () => {
    server.use(
      http.get("/v1/agents", () =>
        HttpResponse.json({
          items: Array.from({ length: 20 }, (_, i) => agent({ name: `a${i}`, uid: `u${i}` })),
          page: 1,
          pageSize: 20,
          total: 45,
        }),
      ),
    );
    renderTable();
    await screen.findByText("1 / 3");
    expect(screen.getByRole("button", { name: /Prev/ })).toBeDisabled();
  });

  it("shows the visible range", async () => {
    server.use(
      http.get("/v1/agents", () =>
        HttpResponse.json({
          items: Array.from({ length: 20 }, (_, i) => agent({ name: `a${i}`, uid: `u${i}` })),
          page: 1,
          pageSize: 20,
          total: 45,
        }),
      ),
    );
    renderTable();
    expect(await screen.findByText("Showing 1–20 of 45")).toBeInTheDocument();
  });

  it("accepts a custom search placeholder", async () => {
    renderTable({ searchPlaceholder: "Find an MCP server…" });
    expect(await screen.findByPlaceholderText("Find an MCP server…")).toBeInTheDocument();
  });
});

describe("cell renderers", () => {
  it("nameCell links to the detail page", async () => {
    renderTable();
    const link = await screen.findByRole("link", { name: "travel-agent" });
    expect(link).toHaveAttribute("href", "/agents/agents/travel-agent");
  });

  it("replicasCell renders ready/total", async () => {
    renderTable();
    const link = await screen.findByRole("link", { name: "travel-agent" });
    // Scope to the row: both fixture agents report 2/2, so an unscoped
    // query matches twice.
    const row = link.closest("tr")!;
    expect(within(row).getByText("2/2")).toBeInTheDocument();
  });

  it("replicasCell treats missing counts as zero", async () => {
    server.use(
      http.get("/v1/agents", () =>
        HttpResponse.json(agentList([agent({ status: { phase: "Pending" } })])),
      ),
    );
    renderTable();
    expect(await screen.findByText("0/0")).toBeInTheDocument();
  });

  it("phaseCell renders an em dash when the phase is absent", async () => {
    server.use(
      http.get("/v1/agents", () =>
        HttpResponse.json(agentList([agent({ status: {} })])),
      ),
    );
    renderTable();
    await screen.findByRole("link", { name: "travel-agent" });
    const row = screen.getByRole("link", { name: "travel-agent" }).closest("tr")!;
    expect(within(row).getAllByText("—").length).toBeGreaterThan(0);
  });

  it("imageCell renders the image reference", async () => {
    renderTable();
    expect(
      await screen.findByText("ghcr.io/org/travel-agent:v1"),
    ).toBeInTheDocument();
  });

  it("modeCell renders the mode", async () => {
    renderTable();
    await screen.findByRole("link", { name: "travel-agent" });
    expect(screen.getByText("always-on")).toBeInTheDocument();
    expect(screen.getByText("serverless")).toBeInTheDocument();
  });
});
