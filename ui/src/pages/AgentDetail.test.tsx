import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import AgentDetail from "./AgentDetail";
import { agent, mcpAgent } from "../test/fixtures";
import { renderWithProviders } from "../test/render";
import { server } from "../test/server";

/** Renders AgentDetail with :namespace/:name bound from the URL. */
function renderDetail(route = "/agents/agents/travel-agent") {
  const path = route.startsWith("/mcp")
    ? "/mcp/:namespace/:name"
    : "/agents/:namespace/:name";
  return renderWithProviders(<AgentDetail />, { route, path });
}

describe("AgentDetail", () => {
  it("renders the agent's identity and phase", async () => {
    renderDetail();

    const heading = await screen.findByRole("heading", { level: 1 });
    expect(heading).toHaveTextContent("agents/travel-agent");

    // "Ready" also appears in the Status panel, so scope to the header.
    const header = heading.parentElement!;
    expect(within(header).getByText("Ready")).toBeInTheDocument();
    expect(within(header).getByText("always-on")).toBeInTheDocument();
    expect(within(header).getByText("2/2 replicas")).toBeInTheDocument();
  });

  it("renders the spec panel", async () => {
    renderDetail();
    await screen.findByText("travel-agent");

    const spec = screen.getByText("Spec").closest("div")!;
    expect(within(spec).getByText("ghcr.io/org/travel-agent:v1")).toBeInTheDocument();
    expect(within(spec).getByText("python")).toBeInTheDocument();
    expect(within(spec).getByText("langgraph")).toBeInTheDocument();
    expect(within(spec).getByText("8080")).toBeInTheDocument();
    // Replica bounds render as "min – max".
    expect(within(spec).getByText("1 – 10")).toBeInTheDocument();
  });

  it("renders the status panel", async () => {
    renderDetail();
    await screen.findByText("travel-agent");

    const status = screen.getByText("Status").closest("div")!;
    expect(
      within(status).getByText("http://travel-agent.agents.svc:8080/a2a"),
    ).toBeInTheDocument();
    // An agent that has never been invoked shows "never", not an epoch date.
    expect(within(status).getByText("never")).toBeInTheDocument();
  });

  it("formats a last-invocation timestamp when present", async () => {
    server.use(
      http.get("/v1/agents/:ns/:name", () =>
        HttpResponse.json(
          agent({
            status: {
              phase: "Ready",
              replicas: 1,
              readyReplicas: 1,
              lastInvocationAt: "2026-07-30T10:15:00Z",
            },
          }),
        ),
      ),
    );

    renderDetail();
    await screen.findByText("travel-agent");
    expect(screen.queryByText("never")).not.toBeInTheDocument();
  });

  it("shows conditions in a disclosure", async () => {
    server.use(
      http.get("/v1/agents/:ns/:name", () =>
        HttpResponse.json(
          agent({
            status: {
              phase: "Pending",
              conditions: [
                { type: "Available", status: "False", reason: "MinimumReplicasUnavailable" },
                { type: "Progressing", status: "True", reason: "NewReplicaSetAvailable" },
              ],
            },
          }),
        ),
      ),
    );

    renderDetail();
    await screen.findByText("travel-agent");

    expect(screen.getByText("Conditions (2)")).toBeInTheDocument();
    expect(screen.getByText("Available=")).toBeInTheDocument();
    expect(screen.getByText("(MinimumReplicasUnavailable)")).toBeInTheDocument();
  });

  it("omits the conditions disclosure when there are none", async () => {
    renderDetail();
    await screen.findByText("travel-agent");
    expect(screen.queryByText(/^Conditions/)).not.toBeInTheDocument();
  });

  it("shows a loading state then the content", async () => {
    renderDetail();
    expect(screen.getByText("Loading…")).toBeInTheDocument();
    expect(await screen.findByText("travel-agent")).toBeInTheDocument();
  });

  it("renders an error when the agent cannot be fetched", async () => {
    server.use(
      http.get("/v1/agents/:ns/:name", () =>
        HttpResponse.json({ error: 'agents.krypton.ai "ghost" not found' }, { status: 404 }),
      ),
    );

    renderDetail("/agents/agents/ghost");
    expect(await screen.findByText(/404 agents.krypton.ai "ghost" not found/)).toBeInTheDocument();
  });

  describe("back link", () => {
    it("points at Agents for a non-MCP agent", async () => {
      renderDetail();
      await screen.findByText("travel-agent");
      expect(screen.getByRole("link", { name: /Agents/ })).toHaveAttribute("href", "/agents");
    });

    // Reached via the MCP list, the breadcrumb must return there — the same
    // component serves both routes.
    it("points at MCP servers when reached from the MCP route", async () => {
      renderDetail("/mcp/agents/mcp-hello");
      await screen.findByText("mcp-hello");
      expect(screen.getByRole("link", { name: /MCP servers/ })).toHaveAttribute("href", "/mcp");
    });

    // Even on the /agents/ route, an MCP-protocol agent belongs to the MCP
    // section.
    it("points at MCP servers for an MCP agent on the agents route", async () => {
      server.use(
        http.get("/v1/agents/:ns/:name", () => HttpResponse.json(mcpAgent())),
      );
      renderDetail("/agents/agents/mcp-hello");
      await screen.findByText("mcp-hello");
      expect(screen.getByRole("link", { name: /MCP servers/ })).toHaveAttribute("href", "/mcp");
    });
  });

  describe("MCP tools panel", () => {
    it("is shown for an MCP agent", async () => {
      renderDetail("/mcp/agents/mcp-hello");
      expect(await screen.findByText("MCP Tools")).toBeInTheDocument();
      expect(await screen.findByText("echo")).toBeInTheDocument();
    });

    it("is hidden for a non-MCP agent", async () => {
      renderDetail();
      await screen.findByText("travel-agent");
      expect(screen.queryByText("MCP Tools")).not.toBeInTheDocument();
    });
  });

  describe("Invoke panel", () => {
    it("posts the body to the gateway and shows the response", async () => {
      const user = userEvent.setup();
      let received: string | undefined;
      server.use(
        http.post("/v1/agents/:ns/:name/", async ({ request }) => {
          received = await request.text();
          return HttpResponse.text('{"reply":"pong"}');
        }),
      );

      renderDetail();
      await screen.findByText("travel-agent");
      await user.click(screen.getByRole("button", { name: "Invoke" }));

      expect(await screen.findByText("HTTP 200")).toBeInTheDocument();
      expect(screen.getByText('{"reply":"pong"}')).toBeInTheDocument();
      expect(JSON.parse(received!)).toEqual({ hello: "world" });
    });

    it("sends a custom path suffix", async () => {
      const user = userEvent.setup();
      let path: string | undefined;
      server.use(
        http.post("/v1/agents/:ns/:name/*", ({ request }) => {
          path = new URL(request.url).pathname;
          return HttpResponse.text("ok");
        }),
      );

      renderDetail();
      await screen.findByText("travel-agent");

      const suffixInput = screen.getByPlaceholderText("/");
      await user.clear(suffixInput);
      await user.type(suffixInput, "/a2a");
      await user.click(screen.getByRole("button", { name: "Invoke" }));

      await waitFor(() =>
        expect(path).toBe("/v1/agents/agents/travel-agent/a2a"),
      );
    });

    // A cold-start timeout is the most common failure here; the operator
    // needs the status and body, not a generic error.
    it("shows a failed invocation with its status and body", async () => {
      const user = userEvent.setup();
      server.use(
        http.post("/v1/agents/:ns/:name/", () =>
          HttpResponse.text("cold start timed out", { status: 504 }),
        ),
      );

      renderDetail();
      await screen.findByText("travel-agent");
      await user.click(screen.getByRole("button", { name: "Invoke" }));

      expect(await screen.findByText("HTTP 504")).toBeInTheDocument();
      expect(screen.getByText("cold start timed out")).toBeInTheDocument();
    });

    it("renders (empty) for a body-less response", async () => {
      const user = userEvent.setup();
      server.use(
        http.post("/v1/agents/:ns/:name/", () =>
          HttpResponse.text("", { status: 204 }),
        ),
      );

      renderDetail();
      await screen.findByText("travel-agent");
      await user.click(screen.getByRole("button", { name: "Invoke" }));

      expect(await screen.findByText("(empty)")).toBeInTheDocument();
    });
  });
});
