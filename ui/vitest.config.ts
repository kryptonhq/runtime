import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// Kept separate from vite.config.ts so the dev-server proxy config (which
// MSW replaces in tests) doesn't leak into the test environment.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    css: false,
    // JUnit output feeds CI test-result analytics; the default reporter
    // stays on for readable local runs.
    reporters: process.env.CI ? ["default", "junit"] : ["default"],
    outputFile: { junit: "./coverage/junit.xml" },
    coverage: {
      provider: "v8",
      reportsDirectory: "./coverage",
      reporter: ["text-summary", "lcov"],
      include: ["src/**/*.{ts,tsx}"],
      exclude: [
        "src/test/**",
        "src/**/*.test.{ts,tsx}",
        "src/vite-env.d.ts",
        // Bootstrap only: mounts <App/> into the DOM. Exercised by the e2e
        // tier loading the real page, not worth a jsdom test.
        "src/main.tsx",
      ],
    },
  },
});
