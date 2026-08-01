import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { ThemeToggle } from "./ThemeToggle";

describe("ThemeToggle", () => {
  it("marks the active theme with aria-pressed", async () => {
    render(<ThemeToggle />);
    // aria-pressed is what a screen reader announces as the current choice,
    // so it has to track state — not just the highlight class.
    expect(screen.getByRole("button", { name: "Light" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("button", { name: "Dark" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
  });

  it("switches to dark and applies it to the document", async () => {
    const user = userEvent.setup();
    render(<ThemeToggle />);

    await user.click(screen.getByRole("button", { name: "Dark" }));

    expect(screen.getByRole("button", { name: "Dark" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(localStorage.getItem("krypton.theme")).toBe("dark");
  });

  it("switches back to light", async () => {
    const user = userEvent.setup();
    render(<ThemeToggle />);

    await user.click(screen.getByRole("button", { name: "Dark" }));
    await user.click(screen.getByRole("button", { name: "Light" }));

    expect(document.documentElement.classList.contains("dark")).toBe(false);
    expect(localStorage.getItem("krypton.theme")).toBe("light");
  });

  // The component seeds its state from getStoredTheme(), so a reload with a
  // stored preference must not flash the wrong theme.
  it("initializes from the stored preference", () => {
    localStorage.setItem("krypton.theme", "dark");
    render(<ThemeToggle />);

    expect(screen.getByRole("button", { name: "Dark" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });

  it("re-clicking the active theme is a no-op", async () => {
    const user = userEvent.setup();
    render(<ThemeToggle />);

    await user.click(screen.getByRole("button", { name: "Light" }));
    expect(document.documentElement.classList.contains("dark")).toBe(false);
    expect(screen.getByRole("button", { name: "Light" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });
});
