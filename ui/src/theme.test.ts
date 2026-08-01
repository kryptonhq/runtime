import { describe, expect, it } from "vitest";
import { applyTheme, getStoredTheme, initTheme } from "./theme";

describe("theme", () => {
  it("defaults to light when nothing is stored", () => {
    expect(getStoredTheme()).toBe("light");
  });

  // Anything other than the exact string "dark" means light. This guards
  // against a corrupted or hand-edited localStorage value rendering an
  // undefined theme.
  it("treats any non-'dark' stored value as light", () => {
    for (const junk of ["", "DARK", "Dark", "true", "null", "system"]) {
      localStorage.setItem("krypton.theme", junk);
      expect(getStoredTheme()).toBe("light");
    }
  });

  it("reads back a stored dark preference", () => {
    localStorage.setItem("krypton.theme", "dark");
    expect(getStoredTheme()).toBe("dark");
  });

  // Tailwind is configured with the class strategy, so the `dark` class on
  // <html> is what actually switches every dark: variant in the app.
  it("adds the dark class to the document root and persists", () => {
    applyTheme("dark");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(localStorage.getItem("krypton.theme")).toBe("dark");
  });

  it("removes the dark class when switching back to light", () => {
    applyTheme("dark");
    applyTheme("light");
    expect(document.documentElement.classList.contains("dark")).toBe(false);
    expect(localStorage.getItem("krypton.theme")).toBe("light");
  });

  it("is idempotent", () => {
    applyTheme("dark");
    applyTheme("dark");
    // classList is a set; applying twice must not leave a duplicate that a
    // single remove would fail to clear.
    expect(
      document.documentElement.className.split(/\s+/).filter((c) => c === "dark"),
    ).toHaveLength(1);

    applyTheme("light");
    expect(document.documentElement.classList.contains("dark")).toBe(false);
  });

  it("initTheme applies the stored preference on boot", () => {
    localStorage.setItem("krypton.theme", "dark");
    initTheme();
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });

  it("initTheme applies light when nothing is stored", () => {
    document.documentElement.classList.add("dark");
    initTheme();
    expect(document.documentElement.classList.contains("dark")).toBe(false);
  });
});
