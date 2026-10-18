import { readFile } from "node:fs/promises";
import { expect, test, type Page } from "@playwright/test";
import { readStoredSession, saveScreenshot, startAdminPageFixture, type FreshAdminPageFixture, type RunningFixture } from "./fixture";

const invalidSetupLinkMessage = "This setup link is no longer valid. Use the link in the latest server log.";

let running: RunningFixture<FreshAdminPageFixture> | undefined;

test.describe.configure({ mode: "serial" });

test.afterEach(() => {
  running?.fixtureProcess.kill("SIGTERM");
  running = undefined;
});

async function startFreshFixture(): Promise<RunningFixture<FreshAdminPageFixture>> {
  running = await startAdminPageFixture<FreshAdminPageFixture>({}, ["-fresh"]);
  return running;
}

async function fillAccountStep(page: Page, email: string): Promise<void> {
  await expect(page.getByRole("heading", { name: "Create the first admin" })).toBeVisible();
  await page.getByLabel("Name").fill("Ada Admin");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Master password", { exact: true }).fill("first admin master password");
  await page.getByLabel("Repeat master password").fill("first admin master password");
}

test("sets up a fresh server from the log line to Overview", async ({ page, browser }) => {
  const { fixture, logLines } = await startFreshFixture();
  expect(logLines.some((line) => line.endsWith(`Open ${fixture.setupURL} to create the first admin.`))).toBe(true);
  expect(fixture.setupURL).toMatch(/\/admin\/setup#[A-Za-z0-9_-]+$/);

  await page.goto(fixture.setupURL);
  await fillAccountStep(page, "ada@example.com");
  await saveScreenshot(page, "a7-account-step");
  await page.getByRole("button", { name: "Create account" }).click();

  await expect(page.getByRole("heading", { name: "Save your Secret Key" })).toBeVisible({ timeout: 30_000 });
  expect(new URL(page.url()).hash).toBe("");
  await expect(page.getByTestId("secret-key")).toHaveText(/^H1-[A-Z0-9-]+$/);
  const continueButton = page.getByRole("button", { name: "Continue" });
  await expect(continueButton).toBeDisabled();
  await page.getByLabel("I saved my Emergency Kit").check();
  await expect(continueButton).toBeDisabled();
  await saveScreenshot(page, "a7-kit-gate");
  const [download] = await Promise.all([page.waitForEvent("download"), page.getByRole("button", { name: "Download Emergency Kit" }).click()]);
  expect(download.suggestedFilename()).toBe("Hisar Emergency Kit.pdf");
  const emergencyKit = await readFile((await download.path())!);
  expect(emergencyKit.subarray(0, 5).toString("latin1")).toBe("%PDF-");
  await expect(continueButton).toBeEnabled();
  await continueButton.click();

  await expect(page.getByRole("heading", { name: "Check the server" })).toBeVisible();
  await expect(page.getByTestId("check-migrations")).toHaveAttribute("data-status", "ok");
  await expect(page.getByTestId("check-email")).toHaveAttribute("data-status", "warning");
  await expect(page.getByTestId("check-secrets-backup")).toHaveAttribute("data-status", "warning");
  await saveScreenshot(page, "a7-checks");
  await page.getByRole("button", { name: "I have a backup" }).click();
  await expect(page.getByTestId("check-secrets-backup")).toHaveAttribute("data-status", "ok");
  await page.getByRole("button", { name: "Continue" }).click();

  await expect(page.getByRole("heading", { name: "Choose who can invite people" })).toBeVisible();
  await page.getByLabel("Admins only").check();
  await saveScreenshot(page, "a7-invite-policy");
  await page.getByRole("button", { name: "Finish setup" }).click();

  await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
  await expect(page.getByTestId("admin-name")).toHaveText("Ada Admin");
  await expect(page.getByTestId("count-accounts").locator(".count-value")).toHaveText("1");
  expect(new URL(page.url()).pathname).toBe("/admin");
  await saveScreenshot(page, "a7-overview");

  const setup = await page.request.get(`${fixture.baseURL}/api/v1/setup`);
  expect(await setup.json()).toEqual({ state: "done" });

  await page.goto(fixture.setupURL);
  await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
  const signedOutPage = await browser.newPage();
  await signedOutPage.goto(fixture.setupURL);
  await expect(signedOutPage.getByRole("heading", { name: "Sign in to the admin dashboard" })).toBeVisible();
  await expect(signedOutPage.getByRole("heading", { name: "Create the first admin" })).toHaveCount(0);
  await saveScreenshot(signedOutPage, "a7-after-done");
  await signedOutPage.close();
});

test("refuses the setup link of an earlier start and takes the latest one pasted into the same tab", async ({ page }) => {
  const { fixture: firstFixture } = await startFreshFixture();
  const oldToken = new URL(firstFixture.setupURL).hash;
  running?.fixtureProcess.kill("SIGTERM");
  const { fixture } = await startFreshFixture();

  await page.goto(`${fixture.baseURL}/admin/setup${oldToken}`);
  await fillAccountStep(page, "ada@example.com");
  await page.getByRole("button", { name: "Create account" }).click();
  await expect(page.getByRole("alert")).toHaveText(invalidSetupLinkMessage, { timeout: 30_000 });
  await saveScreenshot(page, "a7-old-token");

  await page.goto(fixture.setupURL);
  await expect(page.getByRole("alert")).toHaveCount(0);
  await page.getByRole("button", { name: "Create account" }).click();
  await expect(page.getByRole("heading", { name: "Save your Secret Key" })).toBeVisible({ timeout: 30_000 });
});

test("shows the Secret Key when the Personal vault cannot be created, and asks before the page unloads", async ({ page }) => {
  const { fixture } = await startFreshFixture();
  await page.context().route("**/api/v1/vaults", (route) =>
    route.request().method() === "POST" ? route.fulfill({ status: 500, contentType: "application/json", body: '{"error":"internal error"}' }) : route.fallback(),
  );
  await page.goto(fixture.setupURL);
  await fillAccountStep(page, "ada@example.com");
  await page.getByRole("button", { name: "Create account" }).click();

  await expect(page.getByRole("heading", { name: "Save your Secret Key" })).toBeVisible({ timeout: 30_000 });
  await expect(page.getByTestId("personal-vault-error")).toContainText("the Personal vault could not be created");
  await expect(page.getByTestId("secret-key")).toHaveText(/^H1-[A-Z0-9-]+$/);
  await saveScreenshot(page, "a7-personal-vault-failed");
  const setup = await page.request.get(`${fixture.baseURL}/api/v1/setup`);
  expect(await setup.json()).toEqual({ state: "needsChecks" });

  const { promise: unloadDialog, resolve } = Promise.withResolvers<string>();
  page.once("dialog", async (dialog) => {
    resolve(dialog.type());
    await dialog.dismiss();
  });
  await page.reload({ timeout: 5_000 }).catch(() => undefined);
  expect(await unloadDialog).toBe("beforeunload");
  await expect(page.getByTestId("secret-key")).toBeVisible();

  await page.getByLabel("I saved my Emergency Kit").check();
  await page.getByRole("button", { name: "Download Emergency Kit" }).click();
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByRole("heading", { name: "Check the server" })).toBeVisible();
});

test("makes exactly one admin when two browsers submit the first step at once", async ({ browser }) => {
  const { fixture } = await startFreshFixture();
  const pages = await Promise.all([browser.newPage(), browser.newPage()]);
  await Promise.all(pages.map((page) => page.goto(fixture.setupURL)));
  await Promise.all(pages.map((page, index) => fillAccountStep(page, `admin${index}@example.com`)));
  await Promise.all(pages.map((page) => page.getByRole("button", { name: "Create account" }).click()));

  const outcomes = await Promise.all(
    pages.map((page) =>
      Promise.race([
        page.getByRole("heading", { name: "Save your Secret Key" }).waitFor({ timeout: 30_000 }).then(() => "admin"),
        page.getByRole("alert").filter({ hasText: invalidSetupLinkMessage }).waitFor({ timeout: 30_000 }).then(() => "refused"),
      ]),
    ),
  );
  expect([...outcomes].sort()).toEqual(["admin", "refused"]);
  const winner = pages[outcomes.indexOf("admin")]!;
  await saveScreenshot(pages[outcomes.indexOf("refused")]!, "a7-race-refused");
  const storedSession: { token: string } = JSON.parse((await readStoredSession(winner))!);
  const health = await winner.request.get(`${fixture.baseURL}/api/v1/admin/health`, { headers: { Authorization: `Bearer ${storedSession.token}` } });
  expect(await health.json()).toMatchObject({ counts: { accounts: 1 } });
  await Promise.all(pages.map((page) => page.close()));
});
