import { useEffect, useState } from "react";
import { applyTheme, getStoredTheme, Theme } from "../theme";

export function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>(getStoredTheme());

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  return (
    <div className="inline-flex rounded-md border border-slate-200 dark:border-slate-700 overflow-hidden text-xs">
      <button
        type="button"
        onClick={() => setTheme("light")}
        className={
          theme === "light"
            ? "px-2 py-1 bg-accent text-accent-fg"
            : "px-2 py-1 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800"
        }
        aria-pressed={theme === "light"}
      >
        Light
      </button>
      <button
        type="button"
        onClick={() => setTheme("dark")}
        className={
          theme === "dark"
            ? "px-2 py-1 bg-accent text-accent-fg"
            : "px-2 py-1 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800"
        }
        aria-pressed={theme === "dark"}
      >
        Dark
      </button>
    </div>
  );
}
