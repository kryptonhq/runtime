import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { ChatBox } from "./LLMList";
import { model } from "../test/fixtures";
import { renderWithProviders } from "../test/render";
import { server } from "../test/server";

function chatResponse(content: string) {
  return { choices: [{ message: { role: "assistant", content } }] };
}

describe("ChatBox", () => {
  it("is inert until a model is selected", () => {
    renderWithProviders(<ChatBox model={null} />);

    expect(
      screen.getByText("Select a model from the table to invoke it."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
    expect(screen.getByRole("textbox")).toBeDisabled();
  });

  it("names the selected model", () => {
    renderWithProviders(<ChatBox model={model()} />);
    expect(screen.getByText(/Invokes qwen2-0-5b through/)).toBeInTheDocument();
    expect(screen.getByText("models/qwen2-0-5b")).toBeInTheDocument();
  });

  it("shows a placeholder before the first call", () => {
    renderWithProviders(<ChatBox model={model()} />);
    expect(
      screen.getByText("Messages will appear here after the first test call."),
    ).toBeInTheDocument();
  });

  // The happy path: the user message is echoed into the transcript, then the
  // assistant's reply is extracted from the OpenAI response envelope.
  it("sends a prompt and appends the assistant reply", async () => {
    const user = userEvent.setup();
    let body: any;
    server.use(
      http.post("/v1/chat/completions", async ({ request }) => {
        body = await request.json();
        return HttpResponse.json(chatResponse("Hello!"));
      }),
    );

    renderWithProviders(<ChatBox model={model()} />);
    await user.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText("Hello!")).toBeInTheDocument();
    expect(screen.getByText("Say hi in one word.")).toBeInTheDocument();
    expect(body.model).toBe("qwen2-0-5b");
    expect(body.messages).toEqual([
      { role: "user", content: "Say hi in one word." },
    ]);
  });

  it("clears the prompt box after sending", async () => {
    const user = userEvent.setup();
    server.use(
      http.post("/v1/chat/completions", () => HttpResponse.json(chatResponse("hi"))),
    );

    renderWithProviders(<ChatBox model={model()} />);
    await user.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() => expect(screen.getByRole("textbox")).toHaveValue(""));
  });

  // Multi-turn: the second request must carry the whole transcript, or the
  // model loses context.
  it("accumulates conversation history across turns", async () => {
    const user = userEvent.setup();
    const bodies: any[] = [];
    server.use(
      http.post("/v1/chat/completions", async ({ request }) => {
        bodies.push(await request.json());
        return HttpResponse.json(chatResponse(`reply ${bodies.length}`));
      }),
    );

    renderWithProviders(<ChatBox model={model()} />);
    await user.click(screen.getByRole("button", { name: "Send" }));
    await screen.findByText("reply 1");

    await user.type(screen.getByRole("textbox"), "and again");
    await user.click(screen.getByRole("button", { name: "Send" }));
    await screen.findByText("reply 2");

    expect(bodies[1].messages).toEqual([
      { role: "user", content: "Say hi in one word." },
      { role: "assistant", content: "reply 1" },
      { role: "user", content: "and again" },
    ]);
  });

  it("renders the HTTP status badge", async () => {
    const user = userEvent.setup();
    server.use(
      http.post("/v1/chat/completions", () => HttpResponse.json(chatResponse("hi"))),
    );

    renderWithProviders(<ChatBox model={model()} />);
    await user.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText("HTTP 200")).toBeInTheDocument();
  });

  // A model that's still pulling weights returns 503 from the gateway. The
  // raw body is shown so the operator can see why.
  it("shows the raw body for an error response", async () => {
    const user = userEvent.setup();
    server.use(
      http.post("/v1/chat/completions", () =>
        HttpResponse.text("model qwen2-0-5b has no ready endpoints", { status: 503 }),
      ),
    );

    renderWithProviders(<ChatBox model={model()} />);
    await user.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText("HTTP 503")).toBeInTheDocument();
    expect(
      screen.getByText("model qwen2-0-5b has no ready endpoints"),
    ).toBeInTheDocument();
  });

  // A failed call must not append a bogus assistant turn, or the next
  // request would send a fabricated reply back to the model.
  it("does not append an assistant turn on failure", async () => {
    const user = userEvent.setup();
    server.use(
      http.post("/v1/chat/completions", () =>
        HttpResponse.text("boom", { status: 500 }),
      ),
    );

    renderWithProviders(<ChatBox model={model()} />);
    await user.click(screen.getByRole("button", { name: "Send" }));
    await screen.findByText("HTTP 500");

    expect(screen.queryByText("assistant")).not.toBeInTheDocument();
  });

  // extractAssistantText falls back to the raw body when the response isn't
  // the expected JSON envelope — better than rendering nothing.
  it("falls back to the raw body when the response is not OpenAI-shaped", async () => {
    const user = userEvent.setup();
    server.use(
      http.post("/v1/chat/completions", () =>
        HttpResponse.text("plain text reply"),
      ),
    );

    renderWithProviders(<ChatBox model={model()} />);
    await user.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText("plain text reply")).toBeInTheDocument();
  });

  it("clears the transcript", async () => {
    const user = userEvent.setup();
    server.use(
      http.post("/v1/chat/completions", () => HttpResponse.json(chatResponse("hi"))),
    );

    renderWithProviders(<ChatBox model={model()} />);
    await user.click(screen.getByRole("button", { name: "Send" }));
    await screen.findByText("hi");

    await user.click(screen.getByRole("button", { name: "Clear" }));

    expect(
      screen.getByText("Messages will appear here after the first test call."),
    ).toBeInTheDocument();
    expect(screen.queryByText("HTTP 200")).not.toBeInTheDocument();
  });

  it("disables Clear until there is something to clear", () => {
    renderWithProviders(<ChatBox model={model()} />);
    expect(screen.getByRole("button", { name: "Clear" })).toBeDisabled();
  });

  it("refuses to send a whitespace-only prompt", async () => {
    const user = userEvent.setup();
    renderWithProviders(<ChatBox model={model()} />);

    const textarea = screen.getByRole("textbox");
    await user.clear(textarea);
    await user.type(textarea, "   ");

    // Disabled, so no request is made — onUnhandledRequest:"error" would
    // fail the test if one slipped through.
    expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
  });
});
