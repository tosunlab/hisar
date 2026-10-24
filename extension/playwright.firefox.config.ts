import { defineConfig } from "@playwright/test";

// The Firefox end-to-end tests drive Firefox through geckodriver and WebDriver BiDi (end-to-end-firefox/harness.ts), since
// Playwright's Firefox cannot load an extension; Playwright only runs them.
export default defineConfig({
  testDir: "end-to-end-firefox",
  timeout: 90_000,
  workers: 1,
  reporter: [["list"]],
});
