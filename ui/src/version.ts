// Build info — injected at Vite build time via VITE_KRYPTON_VERSION and
// VITE_KRYPTON_COMMIT. Both default to "dev" so the UI also works in
// development without a build step.

// Normalize to a bare semver (strip any leading "v") so consumers
// can prefix "v" themselves without risking "vv0.0.2".
const raw = (import.meta.env.VITE_KRYPTON_VERSION as string | undefined) ?? "dev";
export const version: string = raw.replace(/^v/, "");

export const commit: string =
  (import.meta.env.VITE_KRYPTON_COMMIT as string | undefined) ?? "";

export const repoURL = "https://github.com/kryptonhq/runtime";
