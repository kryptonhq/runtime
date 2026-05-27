import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// All assets ship under /ui/ because the control plane serves them at that
// path (kr-runtime's HTTP root is reserved for the REST API).
export default defineConfig({
  plugins: [react()],
  base: "/ui/",
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      // OpenAI-compatible model invocation paths are served by the gateway,
      // while registry/status APIs below are served by the control plane.
      "/v1/chat/completions": {
        target: process.env.VITE_GATEWAY_URL || "http://localhost:8080",
        changeOrigin: true,
      },
      "/v1/completions": {
        target: process.env.VITE_GATEWAY_URL || "http://localhost:8080",
        changeOrigin: true,
      },
      "/v1/embeddings": {
        target: process.env.VITE_GATEWAY_URL || "http://localhost:8080",
        changeOrigin: true,
      },
      // Dev mode: the API runs separately. Override KRYPTON_API in
      // VITE_API_URL or override directly in production by configuring
      // the control plane to serve UI + API on the same origin.
      "/v1": {
        target: process.env.VITE_API_URL || "http://localhost:8090",
        changeOrigin: true,
      },
    },
  },
});
