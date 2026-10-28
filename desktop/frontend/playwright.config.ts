import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./end-to-end",
  workers: 1,
  use: { baseURL: "http://127.0.0.1:19244", browserName: "chromium" },
  webServer: {
    command: "VITE_PREVIEW=true npm run dev -- --port 19244 --strictPort",
    url: "http://127.0.0.1:19244",
    reuseExistingServer: !process.env.CI,
  },
});
