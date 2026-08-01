import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterAll, afterEach, beforeAll } from "vitest";
import { server } from "./server";

// MSW intercepts at the network layer, so components under test make real
// fetch() calls and exercise the actual api.ts code path — no module mocks.
beforeAll(() => {
  // Fail loudly on a request no handler covers. Silently returning a
  // passthrough would let a typo'd URL look like an empty result set.
  server.listen({ onUnhandledRequest: "error" });
});

afterEach(() => {
  server.resetHandlers();
  cleanup();
  // api.ts reads its base URLs from localStorage; leaking a value set by one
  // test would silently repoint another test's requests.
  localStorage.clear();
  document.documentElement.classList.remove("dark");
});

afterAll(() => server.close());
