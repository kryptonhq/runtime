import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import MCPTools from "./MCPTools";
import { renderWithProviders } from "../test/render";
import { server } from "../test/server";

describe("MCPTools", () => {
  it("lists the tools the server advertises", async () => {
    renderWithProviders(<MCPTools namespace="agents" name="mcp-hello" />);

    expect(await screen.findByText("echo")).toBeInTheDocument();
    expect(screen.getByText("now")).toBeInTheDocument();
    expect(screen.getByText("Echo the message back")).toBeInTheDocument();
  });

  it("shows an empty message when the server advertises no tools", async () => {
    server.use(
      http.get("/v1/agents/:ns/:name/mcp/tools", () =>
        HttpResponse.json({ tools: [] }),
      ),
    );
    renderWithProviders(<MCPTools namespace="agents" name="mcp-hello" />);

    expect(
      await screen.findByText("No tools advertised by this MCP server."),
    ).toBeInTheDocument();
  });

  // An MCP server that's scaled to zero or crashed surfaces as a 502 from
  // the control plane. The operator needs to see why, not an empty list.
  it("renders the upstream error", async () => {
    server.use(
      http.get("/v1/agents/:ns/:name/mcp/tools", () =>
        HttpResponse.json(
          { error: "mcp initialize: connection refused" },
          { status: 502 },
        ),
      ),
    );
    renderWithProviders(<MCPTools namespace="agents" name="mcp-hello" />);

    expect(
      await screen.findByText(/502 mcp initialize: connection refused/),
    ).toBeInTheDocument();
  });

  // placeholderArgs walks inputSchema.properties and seeds a typed skeleton
  // so the operator doesn't have to hand-write the JSON.
  it("prefills arguments from the tool's input schema", async () => {
    renderWithProviders(<MCPTools namespace="agents" name="mcp-hello" />);
    await screen.findByText("echo");

    const textareas = screen.getAllByRole("textbox");
    const parsed = JSON.parse((textareas[0] as HTMLTextAreaElement).value);
    expect(parsed).toEqual({ message: "", times: 0, loud: false });
  });

  it("prefills an empty object for a tool with no properties", async () => {
    renderWithProviders(<MCPTools namespace="agents" name="mcp-hello" />);
    await screen.findByText("now");

    const textareas = screen.getAllByRole("textbox");
    expect(JSON.parse((textareas[1] as HTMLTextAreaElement).value)).toEqual({});
  });

  it("calls a tool and renders the text result", async () => {
    const user = userEvent.setup();
    renderWithProviders(<MCPTools namespace="agents" name="mcp-hello" />);
    await screen.findByText("echo");

    await user.click(screen.getAllByRole("button", { name: "Call" })[0]);

    expect(await screen.findByText(/echo called with/)).toBeInTheDocument();
    expect(screen.getByText("OK")).toBeInTheDocument();
  });

  // Invalid JSON is caught client-side by JSON.parse inside run(), so the
  // request is never sent. onUnhandledRequest:"error" would fail the test if
  // it were.
  it("reports malformed argument JSON without calling the server", async () => {
    const user = userEvent.setup();
    renderWithProviders(<MCPTools namespace="agents" name="mcp-hello" />);
    await screen.findByText("echo");

    const textarea = screen.getAllByRole("textbox")[0];
    await user.clear(textarea);
    // "{{" is user-event's escape for a literal "{" — a bare "{" is parsed
    // as the start of a special key descriptor.
    await user.type(textarea, "{{not json");
    await user.click(screen.getAllByRole("button", { name: "Call" })[0]);

    // Match the parser's message, not the "Arguments (JSON)" label that
    // appears once per tool card. The exact phrasing is V8's, so accept
    // either the current or the older wording.
    await waitFor(() => {
      expect(
        screen.getByText(/in JSON at position|Unexpected token/i),
      ).toBeInTheDocument();
    });
  });

  it("treats an empty argument box as an empty object", async () => {
    const user = userEvent.setup();
    let body: unknown;
    server.use(
      http.post("/v1/agents/:ns/:name/mcp/tools/:tool", async ({ request }) => {
        body = await request.json();
        return HttpResponse.json({ content: [{ type: "text", text: "ok" }] });
      }),
    );

    renderWithProviders(<MCPTools namespace="agents" name="mcp-hello" />);
    await screen.findByText("echo");

    const textarea = screen.getAllByRole("textbox")[0];
    await user.clear(textarea);
    await user.click(screen.getAllByRole("button", { name: "Call" })[0]);

    await waitFor(() => expect(body).toEqual({}));
  });

  // isError:true is MCP's in-band failure signal — the HTTP call succeeds
  // but the tool itself failed. It must not look like a success.
  it("badges an isError result as an error", async () => {
    const user = userEvent.setup();
    server.use(
      http.post("/v1/agents/:ns/:name/mcp/tools/:tool", () =>
        HttpResponse.json({
          content: [{ type: "text", text: "upstream API key missing" }],
          isError: true,
        }),
      ),
    );

    renderWithProviders(<MCPTools namespace="agents" name="mcp-hello" />);
    await screen.findByText("echo");
    await user.click(screen.getAllByRole("button", { name: "Call" })[0]);

    expect(await screen.findByText("Error")).toBeInTheDocument();
    expect(screen.getByText("upstream API key missing")).toBeInTheDocument();
  });

  it("renders non-text content blocks as JSON", async () => {
    const user = userEvent.setup();
    server.use(
      http.post("/v1/agents/:ns/:name/mcp/tools/:tool", () =>
        HttpResponse.json({
          content: [{ type: "image", mimeType: "image/png" }],
        }),
      ),
    );

    renderWithProviders(<MCPTools namespace="agents" name="mcp-hello" />);
    await screen.findByText("echo");
    await user.click(screen.getAllByRole("button", { name: "Call" })[0]);

    expect(await screen.findByText(/"mimeType": "image\/png"/)).toBeInTheDocument();
  });

  it("surfaces a failed tool call as an error message", async () => {
    const user = userEvent.setup();
    server.use(
      http.post("/v1/agents/:ns/:name/mcp/tools/:tool", () =>
        HttpResponse.json({ error: "tool not found" }, { status: 502 }),
      ),
    );

    renderWithProviders(<MCPTools namespace="agents" name="mcp-hello" />);
    await screen.findByText("echo");
    await user.click(screen.getAllByRole("button", { name: "Call" })[0]);

    expect(await screen.findByText(/502 tool not found/)).toBeInTheDocument();
  });

  it("shows the input schema in a disclosure", async () => {
    renderWithProviders(<MCPTools namespace="agents" name="mcp-hello" />);
    await screen.findByText("echo");

    const summaries = screen.getAllByText("Input schema");
    expect(summaries.length).toBeGreaterThan(0);
    // The schema JSON is rendered inside the <details>, collapsed but present.
    const details = summaries[0].closest("details");
    expect(within(details!).getByText(/"type": "object"/)).toBeInTheDocument();
  });
});
