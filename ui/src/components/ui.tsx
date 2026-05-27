import { clsx } from "clsx";
import { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode } from "react";

export function Card({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={clsx(
        "rounded-xl border p-5",
        "border-slate-200 bg-white",
        "dark:border-slate-800 dark:bg-slate-900/50",
        className,
      )}
    >
      {children}
    </div>
  );
}

export function Button({
  className,
  variant = "primary",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "ghost";
}) {
  return (
    <button
      className={clsx(
        "inline-flex items-center justify-center rounded-md text-sm font-medium px-3 py-1.5 transition",
        "disabled:opacity-50 disabled:cursor-not-allowed",
        variant === "primary"
          ? "bg-accent text-accent-fg hover:bg-indigo-500"
          : "border text-slate-700 border-slate-300 hover:bg-slate-100 dark:text-slate-300 dark:border-slate-700 dark:hover:bg-slate-800",
        className,
      )}
      {...props}
    />
  );
}

export function Input({
  className,
  ...props
}: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={clsx(
        "w-full rounded-md border px-3 py-1.5 text-sm",
        "bg-white text-slate-900 border-slate-300 placeholder:text-slate-400",
        "dark:bg-slate-900 dark:text-slate-100 dark:border-slate-800",
        "focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent",
        className,
      )}
      {...props}
    />
  );
}

export function Badge({
  children,
  tone = "slate",
}: {
  children: ReactNode;
  tone?: "slate" | "green" | "yellow" | "red" | "indigo";
}) {
  const tones: Record<string, string> = {
    slate:
      "bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200",
    green:
      "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/60 dark:text-emerald-300",
    yellow:
      "bg-amber-100 text-amber-700 dark:bg-amber-900/60 dark:text-amber-300",
    red: "bg-rose-100 text-rose-700 dark:bg-rose-900/60 dark:text-rose-300",
    indigo:
      "bg-indigo-100 text-indigo-700 dark:bg-indigo-900/60 dark:text-indigo-300",
  };
  return (
    <span
      className={clsx(
        "inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium",
        tones[tone],
      )}
    >
      {children}
    </span>
  );
}

export function phaseTone(phase?: string): "slate" | "green" | "yellow" | "red" {
  switch (phase) {
    case "Ready":
      return "green";
    case "Pending":
    case "Scaling":
      return "yellow";
    case "Failed":
      return "red";
    default:
      return "slate";
  }
}

export function EmptyState({
  title,
  hint,
}: {
  title: string;
  hint?: string;
}) {
  return (
    <Card className="text-center py-12">
      <div className="text-lg font-medium text-slate-900 dark:text-slate-200">
        {title}
      </div>
      {hint && (
        <div className="mt-1 text-sm text-slate-500">{hint}</div>
      )}
    </Card>
  );
}

export function ErrorMessage({ error }: { error: unknown }) {
  const msg =
    error instanceof Error ? error.message : String(error ?? "unknown error");
  return (
    <div className="rounded-md border p-3 text-sm border-rose-200 bg-rose-50 text-rose-700 dark:border-rose-900 dark:bg-rose-950/40 dark:text-rose-200">
      {msg}
    </div>
  );
}

export function Muted({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <span
      className={clsx(
        "text-slate-500 dark:text-slate-500",
        className,
      )}
    >
      {children}
    </span>
  );
}
