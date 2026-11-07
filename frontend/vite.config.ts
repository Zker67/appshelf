import { resolve } from "node:path";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite-plus";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": resolve(import.meta.dirname, "./src"),
    },
  },
  server: {
    // The app loads this address during `mygo dev` (devUrl in mygo.json).
    port: 5173,
    strictPort: true,
    watch: { ignored: ["**/dist/**"] },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    // The page loads from the app itself, not over a network.
    chunkSizeWarningLimit: 1024,
  },
  lint: {
    ignorePatterns: ["dist/**", "src/mygo.ts", "src/components/ui/**"],
    options: {
      typeAware: true,
      typeCheck: true,
    },
  },
  fmt: {
    ignorePatterns: ["dist/**", "src/mygo.ts", "src/components/ui/**"],
  },
  test: {
    include: ["src/**/*.test.ts"],
  },
});
