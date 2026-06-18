import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// Separate vitest config so the production vite.config.ts stays clean
// (avoids the vite/vitest bundled-vite version conflict at the type level).
export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    globals: true,
  },
});
