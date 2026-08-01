import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import App from "./App";
import { renderWithProviders } from "./test/render";

// App owns the route table. These assertions are about which page each URL
// resolves to — the kind of thing that breaks silently when a route is
// renamed and only one of two references is updated.
describe("App routing", () => {
  it("redirects the root to /agents", async () => {
    renderWithProviders(<App />, { route: "/" });
    expect(await screen.findByRole("heading", { name: "Agents" })).toBeInTheDocument();
  });

  it("redirects an unknown path to /agents", async () => {
    renderWithProviders(<App />, { route: "/does/not/exist" });
    expect(await screen.findByRole("heading", { name: "Agents" })).toBeInTheDocument();
  });

  it("renders the agents list", async () => {
    renderWithProviders(<App />, { route: "/agents" });
    expect(await screen.findByRole("heading", { name: "Agents" })).toBeInTheDocument();
  });

  it("renders the LLMs list", async () => {
    renderWithProviders(<App />, { route: "/llms" });
    expect(await screen.findByRole("heading", { name: "LLMs" })).toBeInTheDocument();
  });

  it("renders the MCP list", async () => {
    renderWithProviders(<App />, { route: "/mcp" });
    expect(await screen.findByRole("heading", { name: "MCP servers" })).toBeInTheDocument();
  });

  it("renders settings", async () => {
    renderWithProviders(<App />, { route: "/settings" });
    expect(await screen.findByRole("heading", { name: "Settings" })).toBeInTheDocument();
  });

  it("renders the agent detail route", async () => {
    renderWithProviders(<App />, { route: "/agents/agents/travel-agent" });
    const heading = await screen.findByRole("heading", { level: 1 });
    expect(heading).toHaveTextContent("agents/travel-agent");
  });

  // /mcp/:ns/:name deliberately renders AgentDetail — MCP servers are
  // Agents with protocol=mcp, not a separate resource.
  it("renders the MCP detail route with AgentDetail", async () => {
    renderWithProviders(<App />, { route: "/mcp/agents/mcp-hello" });
    const heading = await screen.findByRole("heading", { level: 1 });
    expect(heading).toHaveTextContent("agents/mcp-hello");
    expect(await screen.findByText("MCP Tools")).toBeInTheDocument();
  });

  it("renders the LLM detail route", async () => {
    renderWithProviders(<App />, { route: "/llms/models/qwen2-0-5b" });
    const heading = await screen.findByRole("heading", { level: 1 });
    expect(heading).toHaveTextContent("models/qwen2-0-5b");
  });

  it("always renders the sidebar alongside the page", async () => {
    renderWithProviders(<App />, { route: "/settings" });
    expect(await screen.findByRole("link", { name: "All agents" })).toBeInTheDocument();
  });

  // App calls initTheme() on mount so a stored dark preference survives a
  // reload without a flash of light theme.
  it("applies the stored theme on mount", async () => {
    localStorage.setItem("krypton.theme", "dark");
    renderWithProviders(<App />, { route: "/settings" });
    await screen.findByRole("heading", { name: "Settings" });
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });
});
