import { randomBytes } from "node:crypto";
import { expect, test } from "@playwright/test";
import { fillSignIn, startAdminPageFixture, type RunningFixture } from "./fixture";

let running: RunningFixture;
const prefix = `paged-${randomBytes(5).toString("hex")}-`;

test.beforeAll(async () => {
  running = await startAdminPageFixture();
  for (let index = 0; index < 120; index++) {
    const response = await fetch(`${running.fixture.baseURL}/api/v1/admin/invites`, {
      method: "POST",
      headers: { Authorization: `Bearer ${running.fixture.token}`, "Content-Type": "application/json" },
      body: JSON.stringify({ email: `${prefix}${index}@example.com`, role: "member" }),
    });
    expect(response.status).toBe(201);
  }
});

test.afterAll(() => running?.fixtureProcess.kill("SIGTERM"));

test("publishes a complete invite list only after the last real API page", async ({ page }) => {
  let releaseSecond!: () => void;
  let reachedSecond!: () => void;
  const secondBlocked = new Promise<void>((resolve) => { reachedSecond = resolve; });
  const proceed = new Promise<void>((resolve) => { releaseSecond = resolve; });
  const pages: number[] = [];
  // The page streams every API page, and Chromium reports a streamed no-store fetch that arrived in several chunks as
  // aborted and keeps no body for response.json() (crbug.com/568852319), so each real page is read here instead.
  await page.route((url) => url.pathname === "/api/v1/admin/invites", async (route) => {
    if (new URL(route.request().url()).searchParams.has("cursor")) {
      reachedSecond();
      await proceed;
    }
    const response = await route.fetch();
    if (response.ok()) {
      expect(response.headers()["hisar-collection-pages"]).toBe("1");
      const body = await response.json();
      expect(body.records.length).toBeLessThanOrEqual(100);
      pages.push(body.records.length);
    }
    await route.fulfill({ response });
  });
  try {
    await page.goto(`${running.fixture.baseURL}/admin/invites`);
    await fillSignIn(page, running.fixture);
    await page.getByRole("button", { name: "Sign in" }).click();
    await secondBlocked;
    await expect(page.locator(`tr[data-invite^="${prefix}"]`)).toHaveCount(0);
    releaseSecond();
    await expect(page.locator(`tr[data-invite^="${prefix}"]`)).toHaveCount(120);
    expect(pages.length).toBeGreaterThanOrEqual(2);
  } finally {
    releaseSecond();
  }
});
