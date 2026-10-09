import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "end-to-end",
  timeout: 60_000,
  workers: 1,
  reporter: [["list"]],
  use: { trace: "retain-on-failure" },
});
