import type { ChildProcess } from "node:child_process";
import { writeFileSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";
import { fillSignIn, startAdminPageFixture, type AdminPageFixture } from "./fixture";

const maximumColdLoadBytes = 3.5 * 1024 * 1024;
const maximumMedianSignInMilliseconds = 3500;
const runCount = 5;

let fixtureProcess: ChildProcess | undefined;
let fixture: AdminPageFixture;

test.beforeAll(async () => {
  ({ fixture, fixtureProcess } = await startAdminPageFixture());
});

test.afterAll(() => {
  fixtureProcess?.kill("SIGTERM");
});

// countTransferredBytes adds the page's and the crypto worker's resource timings; the worker fetches the WebAssembly.
async function countTransferredBytes(page: Page): Promise<number> {
  const worker = page.workers()[0];
  await expect.poll(() => worker.evaluate(() => performance.getEntriesByType("resource").some((entry) => entry.name.endsWith(".wasm")))).toBe(true);
  const sumTransferSize = () =>
    [...performance.getEntriesByType("navigation"), ...performance.getEntriesByType("resource")].reduce(
      (sum, entry) => sum + (entry as PerformanceResourceTiming).transferSize,
      0,
    );
  return (await page.evaluate(sumTransferSize)) + (await worker.evaluate(sumTransferSize));
}

function findMedian(allValue: number[]): number {
  const sorted = [...allValue].sort((first, second) => first - second);
  return sorted[Math.floor(sorted.length / 2)];
}

test("a cold load and a sign-in stay within the budget", async ({ browser }) => {
  const allColdLoadBytes: number[] = [];
  const allSignInMilliseconds: number[] = [];
  for (let run = 0; run < runCount; run++) {
    const context = await browser.newContext();
    const page = await context.newPage();
    await page.goto(`${fixture.baseURL}/admin`);
    await expect.poll(() => page.workers().length).toBe(1);
    allColdLoadBytes.push(await countTransferredBytes(page));

    await fillSignIn(page, fixture);
    const startedAt = Date.now();
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible({ timeout: 30_000 });
    allSignInMilliseconds.push(Date.now() - startedAt);
    await page.getByRole("button", { name: "Sign out" }).click();
    await expect(page.getByRole("heading", { name: "Sign in to the admin dashboard" })).toBeVisible();
    await context.close();
  }
  const result = {
    allColdLoadBytes,
    maximumColdLoadMiB: Math.max(...allColdLoadBytes) / 1024 / 1024,
    allSignInMilliseconds,
    medianSignInMilliseconds: findMedian(allSignInMilliseconds),
  };
  console.log(JSON.stringify(result));
  if (process.env.ADMIN_PERF_OUTPUT) {
    writeFileSync(process.env.ADMIN_PERF_OUTPUT, JSON.stringify(result, null, 2));
  }
  expect(Math.max(...allColdLoadBytes)).toBeLessThanOrEqual(maximumColdLoadBytes);
  expect(result.medianSignInMilliseconds).toBeLessThanOrEqual(maximumMedianSignInMilliseconds);
});
