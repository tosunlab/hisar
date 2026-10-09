/// <reference types="vitest/config" />
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { defineConfig, type Plugin } from "vite";
import vue from "@vitejs/plugin-vue";

const allIconSize = [16, 32, 48, 128];

function emitIcons(): Plugin {
  return {
    name: "hisar-emit-icons",
    generateBundle() {
      for (const size of allIconSize) {
        this.emitFile({
          type: "asset",
          fileName: `icons/icon-${size}.png`,
          source: readFileSync(resolve(import.meta.dirname, `../assets/logo/png/hisar-mark-${size}.png`)),
        });
      }
    },
  };
}

export default defineConfig({
  plugins: [vue(), emitIcons()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    target: "chrome116",
    modulePreload: { polyfill: false },
    rollupOptions: {
      input: {
        popup: resolve(import.meta.dirname, "popup.html"),
        "inline-menu": resolve(import.meta.dirname, "inline-menu.html"),
        "passkey-prompt": resolve(import.meta.dirname, "passkey-prompt.html"),
        "service-worker": resolve(import.meta.dirname, "src/background/service-worker.ts"),
      },
      output: {
        entryFileNames: (chunk) => (chunk.name === "service-worker" ? "service-worker.js" : "assets/[name]-[hash].js"),
      },
    },
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.spec.ts", "tools/**/*.spec.ts"],
  },
});
