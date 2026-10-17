/// <reference types="vitest/config" />
import { readdir, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { promisify } from "node:util";
import { brotliCompress, constants, gzip } from "node:zlib";
import vue from "@vitejs/plugin-vue";
import { defineConfig, type Plugin } from "vite";

// hisar-server embeds this directory; its .gitkeep stays tracked so the server builds without the dashboard.
const serverDirectory = "../server/internal/api/adminpage/dist";

function writeCompressedAssets(): Plugin {
  const compressWithBrotli = promisify(brotliCompress);
  const compressWithGzip = promisify(gzip);
  return {
    name: "hisar-compressed-assets",
    apply: "build",
    async closeBundle() {
      const assetDirectory = join(serverDirectory, "assets");
      const allAsset = await readdir(assetDirectory, { recursive: true, withFileTypes: true });
      await Promise.all(
        allAsset
          .filter((entry) => entry.isFile())
          .map(async (entry) => {
            const path = join(entry.parentPath, entry.name);
            const content = await readFile(path);
            const brotliOptions = { params: { [constants.BROTLI_PARAM_QUALITY]: 11, [constants.BROTLI_PARAM_SIZE_HINT]: content.length } };
            await writeFile(`${path}.br`, await compressWithBrotli(content, brotliOptions));
            await writeFile(`${path}.gz`, await compressWithGzip(content, { level: 9 }));
          }),
      );
    },
  };
}

function restoreGitkeep(): Plugin {
  return { name: "hisar-restore-gitkeep", apply: "build", closeBundle: () => writeFile(join(serverDirectory, ".gitkeep"), "") };
}

export default defineConfig({
  base: "/admin/",
  plugins: [vue(), writeCompressedAssets(), restoreGitkeep()],
  build: { outDir: serverDirectory, emptyOutDir: true },
  worker: { format: "es" },
  server: { proxy: { "/api": process.env.HISAR_SERVER_URL ?? "http://127.0.0.1:8080" } },
  test: { environment: "jsdom", include: ["src/**/*.spec.ts"] },
});
