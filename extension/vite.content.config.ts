import { resolve } from "node:path";
import { defineConfig } from "vite";

// Content scripts cannot be ES modules, so they get their own single-file IIFE build.
export default defineConfig({
  publicDir: false,
  build: {
    outDir: "dist",
    emptyOutDir: false,
    target: "chrome116",
    lib: {
      entry: resolve(import.meta.dirname, "src/content/main.ts"),
      formats: ["iife"],
      name: "hisarContent",
      fileName: () => "content.js",
    },
  },
});
