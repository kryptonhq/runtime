import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  ErrorMessage,
  Input,
  Muted,
  phaseTone,
} from "./ui";

describe("phaseTone", () => {
  // The tone drives the badge colour operators scan for at a glance, so the
  // mapping is behaviour, not styling trivia.
  it("maps each Agent/Model phase to its tone", () => {
    expect(phaseTone("Ready")).toBe("green");
    expect(phaseTone("Pending")).toBe("yellow");
    expect(phaseTone("Scaling")).toBe("yellow");
    expect(phaseTone("Failed")).toBe("red");
  });

  it("falls back to slate for unknown or missing phases", () => {
    expect(phaseTone(undefined)).toBe("slate");
    expect(phaseTone("")).toBe("slate");
    // A phase added server-side that the UI doesn't know yet must render
    // neutrally rather than crash or show a misleading colour.
    expect(phaseTone("Terminating")).toBe("slate");
  });
});

describe("Badge", () => {
  it("renders its children", () => {
    render(<Badge>Ready</Badge>);
    expect(screen.getByText("Ready")).toBeInTheDocument();
  });

  it("applies the tone's classes", () => {
    render(<Badge tone="green">Ready</Badge>);
    expect(screen.getByText("Ready").className).toMatch(/emerald/);
  });

  it("defaults to the slate tone", () => {
    render(<Badge>Unknown</Badge>);
    expect(screen.getByText("Unknown").className).toMatch(/slate/);
  });
});

describe("Button", () => {
  it("renders as an accessible button", () => {
    render(<Button>Call</Button>);
    expect(screen.getByRole("button", { name: "Call" })).toBeInTheDocument();
  });

  it("supports the ghost variant", () => {
    render(<Button variant="ghost">Refresh</Button>);
    expect(screen.getByRole("button", { name: "Refresh" }).className).toMatch(/border/);
  });

  it("forwards disabled through to the DOM", () => {
    render(<Button disabled>Call</Button>);
    expect(screen.getByRole("button", { name: "Call" })).toBeDisabled();
  });
});

describe("Input", () => {
  it("forwards arbitrary input props", () => {
    render(<Input type="search" placeholder="Search agents…" defaultValue="x" />);
    const input = screen.getByPlaceholderText("Search agents…");
    expect(input).toHaveAttribute("type", "search");
    expect(input).toHaveValue("x");
  });
});

describe("Card", () => {
  it("renders children", () => {
    render(<Card>contents</Card>);
    expect(screen.getByText("contents")).toBeInTheDocument();
  });
});

describe("EmptyState", () => {
  it("renders the title alone", () => {
    render(<EmptyState title="Nothing here yet" />);
    expect(screen.getByText("Nothing here yet")).toBeInTheDocument();
  });

  it("renders an optional hint", () => {
    render(<EmptyState title="No agents" hint="Apply an Agent CR to get started" />);
    expect(screen.getByText("Apply an Agent CR to get started")).toBeInTheDocument();
  });
});

describe("ErrorMessage", () => {
  // react-query hands back an Error; api.ts throws `${status} ${detail}`.
  it("renders an Error's message", () => {
    render(<ErrorMessage error={new Error("502 mcp initialize: refused")} />);
    expect(screen.getByText("502 mcp initialize: refused")).toBeInTheDocument();
  });

  it("stringifies a non-Error value", () => {
    render(<ErrorMessage error="plain string failure" />);
    expect(screen.getByText("plain string failure")).toBeInTheDocument();
  });

  // A thrown null/undefined must still render something actionable rather
  // than an empty red box.
  it("falls back to a generic message for nullish errors", () => {
    render(<ErrorMessage error={null} />);
    expect(screen.getByText("unknown error")).toBeInTheDocument();
  });
});

describe("Muted", () => {
  it("renders children and merges a custom class", () => {
    render(<Muted className="text-xs">12 results</Muted>);
    const el = screen.getByText("12 results");
    expect(el.className).toMatch(/text-xs/);
    expect(el.className).toMatch(/slate/);
  });
});
