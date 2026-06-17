import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Build output goes to internal/web/dist, which the Go binary embeds via
// embed.FS. Go's //go:embed cannot traverse ".." and resolves relative to the
// embedding source file (internal/web/embed.go), so the SPA must build there.
// During dev, /api requests are proxied to the Go backend on :8080.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../internal/web/dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
