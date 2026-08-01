import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import AgentsList from "./AgentsList";
import LLMList from "./LLMList";
import MCPList from "./MCPList";
import Settings from "./Settings";
import { agentList, mcpAgent } from "../test/fixtures";
import { renderWithProviders } from "../test/render";
import { server } from "../test/server";

describe("AgentsList", () => {
  it("renders the heading and the non-MCP agents", async () => {
    renderWithProviders(<AgentsList />);

    expect(screen.getByRole("heading", { name: "Agents" })).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: "travel-agent" })).toBeInTheDocument();
    // MCP servers have their own page and are filtered out here.
    expect(screen.queryByRole("link", { name: "mcp-hello" })).not.toBeInTheDocument();
  });

  it("links rows to the agent detail route", async () => {
    renderWithProviders(<AgentsList />);
    const link = await screen.findByRole("link", { name: "travel-agent" });
    expect(link).toHaveAttribute("href", "/agents/agents/travel-agent");
  });

  it("shows a Mode column, which the MCP page omits", async () => {
    renderWithProviders(<AgentsList />);
    await screen.findByRole("link", { name: "travel-agent" });
    expect(screen.getByText("Mode")).toBeInTheDocument();
  });
});

describe("MCPList", () => {
  it("requests only MCP-protocol agents", async () => {
    let protocol: string | null = null;
    server.use(
      http.get("/v1/agents", ({ request }) => {
        protocol = new URL(request.url).searchParams.get("protocol");
        return HttpResponse.json(agentList([mcpAgent()]));
      }),
    );

    renderWithProviders(<MCPList />);

    expect(screen.getByRole("heading", { name: "MCP servers" })).toBeInTheDocument();
    await screen.findByRole("link", { name: "mcp-hello" });
    expect(protocol).toBe("mcp");
  });

  it("links rows to the MCP detail route, not the agent route", async () => {
    renderWithProviders(<MCPList />);
    const link = await screen.findByRole("link", { name: "mcp-hello" });
    expect(link).toHaveAttribute("href", "/mcp/agents/mcp-hello");
  });
});

describe("LLMList", () => {
  // Unlike the agents table, model rows are not links — the whole <tr> has
  // an onClick that calls navigate(). So query by text, not by role=link.
  it("renders models with their runtime and source", async () => {
    renderWithProviders(<LLMList />);

    expect(screen.getByRole("heading", { name: "LLMs" })).toBeInTheDocument();
    expect(await screen.findByText("qwen2-0-5b")).toBeInTheDocument();
    expect(screen.getByText("llama.cpp")).toBeInTheDocument();
    // Source renders as "<repo>/<file>".
    expect(
      screen.getByText(
        "Qwen/Qwen2.5-0.5B-Instruct-GGUF/qwen2.5-0.5b-instruct-q4_k_m.gguf",
      ),
    ).toBeInTheDocument();
  });

  it("counts models with the right plural", async () => {
    renderWithProviders(<LLMList />);
    expect(await screen.findByText(/^1 model/)).toBeInTheDocument();
  });

  it("renders an empty state when no models are registered", async () => {
    server.use(
      http.get("/v1/models", () =>
        HttpResponse.json({ items: [], page: 1, pageSize: 20, total: 0 }),
      ),
    );
    renderWithProviders(<LLMList />);
    expect(await screen.findByText("No LLMs deployed yet.")).toBeInTheDocument();
  });

  it("surfaces a control-plane error", async () => {
    server.use(
      http.get("/v1/models", () =>
        HttpResponse.json({ error: "list models: connection refused" }, { status: 500 }),
      ),
    );
    renderWithProviders(<LLMList />);
    expect(
      await screen.findByText(/500 list models: connection refused/),
    ).toBeInTheDocument();
  });

  it("sorts by a column and toggles direction", async () => {
    const user = userEvent.setup();
    const seen: Array<{ sort: string | null; order: string | null }> = [];
    server.use(
      http.get("/v1/models", ({ request }) => {
        const p = new URL(request.url).searchParams;
        seen.push({ sort: p.get("sort"), order: p.get("order") });
        return HttpResponse.json({ items: [], page: 1, pageSize: 20, total: 0 });
      }),
    );

    renderWithProviders(<LLMList />);
    await screen.findByText("No LLMs deployed yet.");

    await user.click(screen.getByRole("button", { name: /Source/ }));
    await waitFor(() => expect(seen.at(-1)).toEqual({ sort: "source", order: "asc" }));

    await user.click(screen.getByRole("button", { name: /Source/ }));
    await waitFor(() => expect(seen.at(-1)).toEqual({ sort: "source", order: "desc" }));
  });
});

describe("Settings", () => {
  it("starts from the persisted endpoint overrides", () => {
    localStorage.setItem("krypton.apiBase", "http://cp.local:8090");
    localStorage.setItem("krypton.gatewayBase", "http://gw.local:8080");

    renderWithProviders(<Settings />);

    const inputs = screen.getAllByPlaceholderText("(same-origin)");
    expect(inputs[0]).toHaveValue("http://cp.local:8090");
    expect(inputs[1]).toHaveValue("http://gw.local:8080");
  });

  it("persists both bases on save", async () => {
    const user = userEvent.setup();
    renderWithProviders(<Settings />);

    const inputs = screen.getAllByPlaceholderText("(same-origin)");
    await user.type(inputs[0], "http://cp.example:8090");
    await user.type(inputs[1], "http://gw.example:8080");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(localStorage.getItem("krypton.apiBase")).toBe("http://cp.example:8090");
    expect(localStorage.getItem("krypton.gatewayBase")).toBe("http://gw.example:8080");
    expect(await screen.findByText("Saved")).toBeInTheDocument();
  });

  // Whitespace pasted from a terminal would otherwise produce an unusable
  // base URL that fails every request with an opaque network error.
  it("trims whitespace before saving", async () => {
    const user = userEvent.setup();
    renderWithProviders(<Settings />);

    const inputs = screen.getAllByPlaceholderText("(same-origin)");
    await user.type(inputs[0], "  http://cp.example:8090  ");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(localStorage.getItem("krypton.apiBase")).toBe("http://cp.example:8090");
  });

  // Clearing a field must remove the key so the app falls back to
  // same-origin, rather than storing "" and short-circuiting the ?? default.
  it("clearing a field removes the override", async () => {
    const user = userEvent.setup();
    localStorage.setItem("krypton.apiBase", "http://cp.local:8090");

    renderWithProviders(<Settings />);
    const inputs = screen.getAllByPlaceholderText("(same-origin)");
    await user.clear(inputs[0]);
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(localStorage.getItem("krypton.apiBase")).toBeNull();
  });
});
