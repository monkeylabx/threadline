import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The Tauri host serves the built files from `web/` (see src-tauri/tauri.conf.json).
export default defineConfig({
  plugins: [react()],
  build: { outDir: "web", emptyOutDir: true },
  server: { port: 1420, strictPort: true },
  test: {
    environment: "jsdom",
    include: ["test/**/*.test.tsx", "test/**/*.test.ts"],
  },
});
