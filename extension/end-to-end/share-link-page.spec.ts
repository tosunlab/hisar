import type { ChildProcess } from "node:child_process";
import { expect, test, type Page } from "@playwright/test";
import { saveScreenshot, startShareLinkFixture, type ShareLinkFixture } from "./share-link-fixture";

let fixtureProcess: ChildProcess | undefined;
let fixture: ShareLinkFixture;

test.describe.configure({ mode: "serial" });

test.beforeAll(async () => {
  ({ fixture, fixtureProcess } = await startShareLinkFixture());
});

test.afterAll(() => {
  fixtureProcess?.kill("SIGTERM");
});

function trackPublicRequest(page: Page): string[] {
  const allPublicRequest: string[] = [];
  page.on("request", (request) => {
    if (request.url().includes("/api/v1/public/share-links/")) {
      allPublicRequest.push(request.url());
    }
  });
  return allPublicRequest;
}

function trackConsoleError(page: Page): string[] {
  const allError: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") {
      allError.push(message.text());
    }
  });
  page.on("pageerror", (error) => allError.push(error.message));
  return allError;
}

test("decrypts a link created by the Go client, and counts a view only on Reveal", async ({ page, request, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"], { origin: fixture.baseURL });
  const allPublicRequest = trackPublicRequest(page);
  const allConsoleError = trackConsoleError(page);

  const preview = await request.get(fixture.links.item);
  expect(preview.status()).toBe(200);
  expect(preview.headers()["content-security-policy"]).toContain("default-src 'none'");
  expect(preview.headers()["referrer-policy"]).toBe("no-referrer");
  expect(preview.headers()["cache-control"]).toBe("no-store");
  expect(await preview.text()).not.toContain(fixture.password);

  await page.goto(fixture.links.item);
  await expect(page.getByRole("heading", { name: "Someone shared a secret with you" })).toBeVisible();
  await expect(page.locator("footer")).toHaveText(
    "Shared with Hisar, the end-to-end encrypted password, SSH key and secrets manager for developers. Nothing you see here is stored by this page.",
  );
  await expect(page.locator("footer a")).toHaveCount(0);
  await page.reload();
  await page.waitForLoadState("networkidle");
  await page.waitForTimeout(500);
  expect(allPublicRequest).toHaveLength(0);

  await page.getByRole("button", { name: "Reveal" }).click();
  await expect(page.getByRole("heading", { name: "GitHub" })).toBeVisible();
  expect(allPublicRequest).toHaveLength(1);
  await expect(page.locator("#link-status")).toContainText("2 more views allowed");

  const passwordField = page.locator(".field", { has: page.getByText("Password", { exact: true }) });
  await expect(passwordField.locator(".field-value")).not.toContainText(fixture.password);
  await passwordField.getByRole("button", { name: "Show Password" }).click();
  await expect(passwordField.locator(".field-value")).toHaveText(fixture.password);
  await passwordField.getByRole("button", { name: "Copy Password" }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(fixture.password);

  await expect(page.locator(".field", { has: page.getByText("Username", { exact: true }) }).locator(".field-value")).toHaveText("octocat");
  await expect(page.locator(".field", { has: page.getByText("Website", { exact: true }) }).locator(".field-value")).toHaveText("https://github.com/login");
  await expect(page.locator(".field", { has: page.getByText("Notes", { exact: true }) }).locator(".field-value")).toHaveText(
    "Recovery codes are in the team runbook.\nRotate after offboarding.",
  );
  const recoveryField = page.locator(".field", { has: page.getByText("Recovery code", { exact: true }) });
  await expect(recoveryField.locator(".field-value")).not.toContainText("a1b2-c3d4-e5f6");
  await expect(page.locator("body")).not.toContainText("GEZDGNBV");
  expect(allConsoleError).toEqual([]);
});

test("asks for the passphrase, and a wrong one costs no view", async ({ page }) => {
  const allPublicRequest = trackPublicRequest(page);
  await page.goto(fixture.links.passphrase);
  await page.getByRole("button", { name: "Reveal" }).click();
  await expect(page.getByRole("heading", { name: "Enter the passphrase" })).toBeVisible();

  await page.getByLabel("Passphrase").fill("wrong horse battery staple");
  await page.getByRole("button", { name: "Unlock" }).click();
  await expect(page.getByRole("alert")).toHaveText("That passphrase doesn't open this link. Check it and try again.");
  await expect(page.getByLabel("Passphrase")).toHaveAttribute("aria-invalid", "true");

  await page.getByLabel("Passphrase").fill(fixture.passphrase);
  await page.getByRole("button", { name: "Unlock" }).click();
  await expect(page.getByRole("heading", { name: "GitHub" })).toBeVisible();
  await page.getByRole("button", { name: "Show Password" }).click();
  await expect(page.locator(".field", { has: page.getByText("Password", { exact: true }) }).locator(".field-value")).toHaveText(fixture.password);
  await expect(page.locator("#link-status")).toContainText("1 more view allowed");
  expect(allPublicRequest).toHaveLength(1);
});

test("says expired or already viewed after the last view, and for links that never existed", async ({ page, browser }) => {
  await page.goto(fixture.links.oneView);
  await page.getByRole("button", { name: "Reveal" }).click();
  await expect(page.locator("#link-status")).toContainText("That was the last view");

  const secondPage = await browser.newPage();
  await secondPage.goto(fixture.links.oneView);
  await secondPage.getByRole("button", { name: "Reveal" }).click();
  await expect(secondPage.getByRole("heading", { name: "This link has expired or was already viewed" })).toBeVisible();
  await expect(secondPage.getByRole("button", { name: "Try again" })).toBeHidden();

  const neverExisted = fixture.links.oneView.replace(/\/s\/[^#]+/, "/s/AAAAAAAAAAAAAAAAAAAAAA");
  await secondPage.goto(neverExisted);
  await secondPage.getByRole("button", { name: "Reveal" }).click();
  await expect(secondPage.getByRole("heading", { name: "This link has expired or was already viewed" })).toBeVisible();
  await saveScreenshot(secondPage, "page-expired");
  await secondPage.close();
});

test("refuses a link without its key before fetching anything", async ({ page }) => {
  const allPublicRequest = trackPublicRequest(page);
  await page.goto(fixture.links.text.split("#")[0]);
  await expect(page.getByRole("heading", { name: "This link is incomplete" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Reveal" })).toBeHidden();

  await page.goto("about:blank");
  await page.goto(`${fixture.links.text.split("#")[0]}#${"A".repeat(43)}`);
  await page.getByRole("button", { name: "Reveal" }).click();
  await expect(page.getByRole("heading", { name: "This link can't be opened" })).toBeVisible();
  expect(allPublicRequest).toHaveLength(1);
});

test("shows shared text", async ({ page }) => {
  await page.goto(fixture.links.text);
  await page.getByRole("button", { name: "Reveal" }).click();
  await expect(page.getByRole("heading", { name: "Staging database URL" })).toBeVisible();
  await expect(page.locator(".field-value")).toHaveText("postgres://deploy:s3cr3t@db.internal:5432/app");
  await expect(page.locator("#link-status")).toContainText("more views allowed. The link expires on");
});
