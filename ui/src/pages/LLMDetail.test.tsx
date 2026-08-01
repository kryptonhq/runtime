import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import LLMDetail from "./LLMDetail";
import { model } from "../test/fixtures";
import { renderWithProviders } from "../test/render";
import { server } from "../test/server";

function renderDetail(route = "/llms/models/qwen2-0-5b") {
  return renderWithProviders(<LLMDetail />, {
    route,
    path: "/llms/:namespace/:name",
  });
}

describe("LLMDetail", () => {
  it("renders the model's identity, phase and runtime", async () => {
    renderDetail();

    const heading = await screen.findByRole("heading", { level: 1 });
    expect(heading).toHaveTextContent("models/qwen2-0-5b");

    // Phase and runtime also appear in the Spec/Status panels below.
    const header = heading.parentElement!;
    expect(within(header).getByText("Ready")).toBeInTheDocument();
    expect(within(header).getByText("llama.cpp")).toBeInTheDocument();
    expect(within(header).getByText("1/1 replicas")).toBeInTheDocument();
  });

  it("links back to the LLMs list", async () => {
    renderDetail();
    await screen.findByRole("heading", { level: 1 });
    expect(screen.getByRole("link", { name: /LLMs/ })).toHaveAttribute("href", "/llms");
  });

  it("renders the spec panel with the weights source", async () => {
    renderDetail();
    await screen.findByRole("heading", { level: 1 });

    const spec = screen.getByText("Spec").closest("div")!;
    expect(
      within(spec).getByText("Qwen/Qwen2.5-0.5B-Instruct-GGUF"),
    ).toBeInTheDocument();
    expect(
      within(spec).getByText("qwen2.5-0.5b-instruct-q4_k_m.gguf"),
    ).toBeInTheDocument();
  });

  // An empty spec.image means the controller picks its built-in llama.cpp
  // image. Showing a blank cell would look like a broken field.
  it("labels an unset image as the default", async () => {
    renderDetail();
    await screen.findByRole("heading", { level: 1 });

    const spec = screen.getByText("Spec").closest("div")!;
    expect(within(spec).getByText("default llama.cpp server")).toBeInTheDocument();
  });

  it("shows an overridden image", async () => {
    server.use(
      http.get("/v1/models/:ns/:name", () =>
        HttpResponse.json(
          model({
            spec: {
              source: {
                huggingface: "Qwen/Qwen2.5-0.5B-Instruct-GGUF",
                file: "qwen2.5-0.5b-instruct-q4_k_m.gguf",
              },
              image: "registry.internal/llama.cpp:pinned",
            },
          }),
        ),
      ),
    );

    renderDetail();
    await screen.findByRole("heading", { level: 1 });
    expect(
      screen.getByText("registry.internal/llama.cpp:pinned"),
    ).toBeInTheDocument();
  });

  it("joins runtime args for display", async () => {
    server.use(
      http.get("/v1/models/:ns/:name", () =>
        HttpResponse.json(
          model({
            spec: {
              source: {
                huggingface: "Qwen/Qwen2.5-0.5B-Instruct-GGUF",
                file: "qwen2.5-0.5b-instruct-q4_k_m.gguf",
              },
              args: ["--ctx-size", "4096", "--threads", "4"],
            },
          }),
        ),
      ),
    );

    renderDetail();
    await screen.findByRole("heading", { level: 1 });
    expect(screen.getByText("--ctx-size 4096 --threads 4")).toBeInTheDocument();
  });

  it("renders the status panel", async () => {
    renderDetail();
    await screen.findByRole("heading", { level: 1 });

    const status = screen.getByText("Status").closest("div")!;
    expect(
      within(status).getByText("http://qwen2-0-5b.models.svc:8080"),
    ).toBeInTheDocument();
  });

  it("shows a loading state first", async () => {
    renderDetail();
    expect(screen.getByText("Loading...")).toBeInTheDocument();
    await screen.findByRole("heading", { level: 1 });
  });

  it("renders an error when the model cannot be fetched", async () => {
    server.use(
      http.get("/v1/models/:ns/:name", () =>
        HttpResponse.json(
          { error: 'models.krypton.ai "ghost" not found' },
          { status: 404 },
        ),
      ),
    );

    renderDetail("/llms/models/ghost");
    expect(
      await screen.findByText(/404 models.krypton.ai "ghost" not found/),
    ).toBeInTheDocument();
  });

  // The detail page embeds the chat tester, wired to this model.
  it("embeds the chat box bound to this model", async () => {
    renderDetail();
    await screen.findByRole("heading", { level: 1 });

    expect(screen.getByText("Chat test")).toBeInTheDocument();
    expect(screen.getByText(/Invokes qwen2-0-5b through/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Send" })).toBeEnabled();
  });
});
