// Theme management — class strategy via Tailwind's `dark:` variants.
// Default is light; user choice persists in localStorage and survives
// reloads.

export type Theme = "light" | "dark";

const STORAGE_KEY = "krypton.theme";

export function getStoredTheme(): Theme {
  const stored = localStorage.getItem(STORAGE_KEY);
  return stored === "dark" ? "dark" : "light";
}

export function applyTheme(theme: Theme) {
  const root = document.documentElement;
  if (theme === "dark") {
    root.classList.add("dark");
  } else {
    root.classList.remove("dark");
  }
  localStorage.setItem(STORAGE_KEY, theme);
}

export function initTheme() {
  applyTheme(getStoredTheme());
}
