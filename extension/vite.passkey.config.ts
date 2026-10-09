import { resolve } from "node:path";
import { defineConfig } from "vite";

// The passkey content scripts run at document_start, one in the page's MAIN world and one in the isolated world.
const allEntry: Record<string, { entry: string; fileName: string }> = {
  "main-world": { entry: "src/content/passkey/main-world-entry.ts", fileName: "passkey-main.js" },
  "isolated-world": { entry: "src/content/passkey/isolated-entry.ts", fileName: "passkey-content.js" },
};

export default defineConfig(({ mode }) => {
  const selected = allEntry[mode];
  if (!selected) throw new Error(`unknown passkey build mode ${mode}`);
  return {
    publicDir: false,
    build: {
      outDir: "dist",
      emptyOutDir: false,
      target: "chrome116",
      lib: {
        entry: resolve(import.meta.dirname, selected.entry),
        formats: ["iife"],
        name: "hisarPasskey",
        fileName: () => selected.fileName,
      },
    },
  };
});
