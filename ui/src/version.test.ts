import { describe, expect, it } from "vitest";
import { commit, repoURL, version } from "./version";

describe("version", () => {
  // The sidebar footer renders "v{version}", so a raw tag like "v0.0.4"
  // reaching this module unstripped would render "vv0.0.4".
  it("exposes a version with no leading v", () => {
    expect(version).not.toMatch(/^v/);
  });

  it("falls back to a non-empty value when unset at build time", () => {
    // vitest doesn't define VITE_KRYPTON_VERSION, so this exercises the
    // "dev" default path.
    expect(version).toBeTruthy();
    expect(typeof version).toBe("string");
  });

  it("exposes commit as a string, empty when unset", () => {
    expect(typeof commit).toBe("string");
  });

  it("points at the canonical repo", () => {
    expect(repoURL).toBe("https://github.com/kryptonhq/runtime");
  });
});
