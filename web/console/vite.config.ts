import { fileURLToPath, URL } from "node:url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const apiTarget = process.env.DS_API_PROXY || "http://127.0.0.1:9000";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  server: {
    port: 5173,
    strictPort: true,
    host: true,
    proxy: {
      "/api": {
        target: apiTarget,
        xfwd: true,
      },
      "/healthz": apiTarget,
      "/metrics": apiTarget,
    },
  },
});
