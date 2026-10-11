import type { ChildProcess } from "node:child_process";
import { expect, test } from "@playwright/test";
import { saveScreenshot, screenshotDirectory, startShareLinkFixture, type ShareLinkFixture } from "./share-link-fixture";

let fixtureProcess: ChildProcess | undefined;
let fixture: ShareLinkFixture;

test.beforeAll(async () => {
  if (!screenshotDirectory) {
    throw new Error("set SHARE_LINK_SCREENSHOT_DIRECTORY");
  }
  ({ fixture, fixtureProcess } = await startShareLinkFixture());
});

test.afterAll(() => {
  fixtureProcess?.kill("SIGTERM");
});

test("screenshots of each step", async ({ browser }) => {
  for (const colorScheme of ["light", "dark"] as const) {
    const context = await browser.newContext({ colorScheme, viewport: { width: 760, height: 900 }, deviceScaleFactor: 2 });
    const page = await context.newPage();
    const suffix = colorScheme === "dark" ? "-dark" : "";
    await page.goto(fixture.links.screenshot);
    await saveScreenshot(page, `page-before-reveal${suffix}`);
    await page.getByRole("button", { name: "Reveal" }).click();
    await expect(page.getByRole("heading", { name: "GitHub" })).toBeVisible();
    await page.getByRole("button", { name: "Show Password" }).click();
    await saveScreenshot(page, `page-revealed${suffix}`);
    await page.goto(fixture.links.screenshotPassphrase);
    await page.getByRole("button", { name: "Reveal" }).click();
    await page.getByLabel("Passphrase").fill("not the passphrase");
    await page.getByRole("button", { name: "Unlock" }).click();
    await expect(page.getByRole("alert")).toBeVisible();
    await saveScreenshot(page, `page-passphrase${suffix}`);
    await context.close();
  }
  const phone = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 3, isMobile: true, hasTouch: true });
  const phonePage = await phone.newPage();
  await phonePage.goto(fixture.links.screenshot);
  await phonePage.getByRole("button", { name: "Reveal" }).click();
  await expect(phonePage.getByRole("heading", { name: "GitHub" })).toBeVisible();
  await saveScreenshot(phonePage, "page-revealed-phone");
  await phone.close();
});
