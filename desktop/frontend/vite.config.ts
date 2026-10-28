/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import wails from "@wailsio/runtime/plugins/vite";

export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
    // The theme test reads assets/themes and the generated Swift file from outside the frontend.
    fs: process.env.VITEST ? { allow: ["../.."] } : undefined,
  },
  plugins: [vue(), wails("./bindings")],
  test: {
    environment: "jsdom",
    include: ["src/**/*.spec.ts"],
    setupFiles: ["src/testsetup.ts"],
    css: { include: [/(themes|style)\.css/] },
  },
});
