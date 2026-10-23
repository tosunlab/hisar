import { readFile } from "node:fs/promises";
import { expect, test, type Page } from "@playwright/test";
import { readStoredSession, saveScreenshot, startAdminPageFixture, type FreshAdminPageFixture, type RunningFixture } from "./fixture";

const invalidSetupLinkMessage = "This setup link is no longer valid. Use the link in the latest server log.";
const password = "first admin master password";
let running: RunningFixture<FreshAdminPageFixture> | undefined;

test.describe.configure({ mode: "serial" });
test.afterEach(() => { running?.fixtureProcess.kill("SIGTERM"); running = undefined; });

async function startFreshFixture(): Promise<RunningFixture<FreshAdminPageFixture>> {
  running = await startAdminPageFixture<FreshAdminPageFixture>({}, ["-fresh"]);
  return running;
}

async function fillAccountStep(page: Page, email: string): Promise<void> {
  await expect(page.getByRole("heading", { name: "Create the first admin" })).toBeVisible();
  await page.getByLabel("Name").fill("Ada Admin");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Master password", { exact: true }).fill(password);
  await page.getByLabel("Repeat master password").fill(password);
}

async function savePrecommitKit(page: Page): Promise<string> {
  await expect(page.getByTestId("setup-recovery")).toContainText("The account has not been created", { timeout: 30_000 });
  await expect(page.getByRole("button", { name: "Verify and resume setup" })).toBeDisabled();
  const secretKey = (await page.getByTestId("recovery-secret-key").textContent())!.trim();
  await page.getByLabel("Master password used for this account").fill(password);
  const [download] = await Promise.all([page.waitForEvent("download"), page.getByRole("button", { name: "Download recovery Emergency Kit" }).click()]);
  expect(download.suggestedFilename()).toBe("Hisar Emergency Kit.pdf");
  expect((await readFile((await download.path())!)).subarray(0, 5).toString("latin1")).toBe("%PDF-");
  const confirm = page.getByRole("button", { name: "Confirm kit saved before account creation" });
  await expect(confirm).toBeDisabled();
  await page.getByLabel("I saved this Emergency Kit somewhere safe").check();
  await page.getByLabel("Master password used for this account").fill(password);
  await confirm.click();
  await expect(page.getByTestId("setup-recovery")).toContainText("Your kit is confirmed saved");
  return secretKey;
}

async function resume(page: Page, secretKey = ""): Promise<void> {
  if (secretKey) await page.getByLabel("Secret Key from the saved Emergency Kit, if this page reloaded").fill(secretKey);
  await page.getByLabel("Master password used for this account").fill(password);
  await page.getByRole("button", { name: "Verify and resume setup" }).click();
}

async function confirmDelivery(page: Page): Promise<void> {
  await expect(page.getByTestId("setup-recovery")).toContainText("Account setup is complete", { timeout: 30_000 });
  await page.getByLabel("I saved this Emergency Kit somewhere safe").check();
  await page.getByLabel("Master password used for this account").fill(password);
  await page.getByRole("button", { name: "Confirm saved and continue" }).click();
  await expect(page.getByRole("heading", { name: "Check the server" })).toBeVisible();
}

test("sets up a fresh server with a saved kit before account creation and resumes after restart", async ({ page, browser }) => {
  const { fixture, logLines } = await startFreshFixture();
  expect(logLines.some((line) => line.endsWith(`Open ${fixture.setupURL} to create the first admin.`))).toBe(true);
  await page.goto(fixture.setupURL);
  await fillAccountStep(page, "ada@example.com");
  await page.getByRole("button", { name: "Create account" }).click();
  await expect(page.getByTestId("setup-recovery")).toBeVisible({ timeout: 30_000 });
  expect(await (await page.request.get(`${fixture.baseURL}/api/v1/setup`)).json()).toEqual({ state: "needsAdmin" });
  const secretKey = await savePrecommitKit(page);
  const records = await page.evaluate(async () => {
    const { promise, resolve, reject } = Promise.withResolvers<unknown[]>();
    const open = indexedDB.open("hisar.admin.recovery", 1);
    open.onsuccess = () => {
      const database = open.result;
      const read = database.transaction("pending", "readonly").objectStore("pending").getAll();
      read.onsuccess = () => { database.close(); resolve(read.result); };
      read.onerror = () => reject(new Error("read recovery journal"));
    };
    open.onerror = () => reject(new Error("open recovery journal"));
    return promise;
  });
  expect(records).toHaveLength(1);
  expect(records[0]).toMatchObject({ metadata: { phase: "prepared", email: "ada@example.com" }, envelope: expect.any(String) });
  expect(JSON.stringify(records)).not.toContain(password);
  expect(JSON.stringify(records)).not.toContain("H1-");
  await page.goto(`${fixture.baseURL}/admin`);
  await expect(page.getByTestId("setup-recovery")).toContainText("Your kit is confirmed saved");
  await expect(page.getByTestId("recovery-secret-key")).toHaveCount(0);
  await saveScreenshot(page, "a05-prepared-restart");
  await resume(page, secretKey);
  await confirmDelivery(page);
  await expect(page.getByTestId("check-migrations")).toHaveAttribute("data-status", "ok");
  await page.getByRole("button", { name: "I have a backup" }).click();
  await page.getByRole("button", { name: "Continue" }).click();
  await page.getByLabel("Admins only").check();
  await page.getByRole("button", { name: "Finish setup" }).click();
  await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
  await expect(page.getByTestId("admin-name")).toHaveText("Ada Admin");
  await expect(page.getByTestId("count-accounts").locator(".count-value")).toHaveText("1");
  await saveScreenshot(page, "a05-setup-complete");
  await page.goto(fixture.setupURL);
  await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
  const signedOut = await browser.newPage();
  await signedOut.goto(fixture.setupURL);
  await expect(signedOut.getByRole("heading", { name: "Sign in to the admin dashboard" })).toBeVisible();
  await signedOut.close();
});

test("refuses an earlier setup invite before staging and accepts the latest link", async ({ page }) => {
  const { fixture: earlier } = await startFreshFixture();
  const oldToken = new URL(earlier.setupURL).hash;
  running?.fixtureProcess.kill("SIGTERM");
  const { fixture } = await startFreshFixture();
  await page.goto(`${fixture.baseURL}/admin/setup${oldToken}`);
  await fillAccountStep(page, "ada@example.com");
  await page.getByRole("button", { name: "Create account" }).click();
  await expect(page.getByRole("alert")).toContainText(invalidSetupLinkMessage, { timeout: 30_000 });
  expect(await (await page.request.get(`${fixture.baseURL}/api/v1/setup`)).json()).toEqual({ state: "needsAdmin" });
  await page.goto(fixture.setupURL);
  await fillAccountStep(page, "ada@example.com");
  await page.getByRole("button", { name: "Create account" }).click();
  await expect(page.getByTestId("setup-recovery")).toContainText("The account has not been created", { timeout: 30_000 });
});

test("recovers interrupted Personal vault setup after restart using the saved kit", async ({ page }) => {
  const { fixture } = await startFreshFixture();
  await page.context().route("**/api/v1/vaults/personal", (route) => route.request().method() === "POST"
    ? route.fulfill({ status: 500, contentType: "application/json", body: '{"error":"internal error"}' }) : route.fallback());
  await page.goto(fixture.setupURL);
  await fillAccountStep(page, "ada@example.com");
  await page.getByRole("button", { name: "Create account" }).click();
  const secretKey = await savePrecommitKit(page);
  await resume(page);
  await expect(page.getByTestId("setup-recovery")).toContainText("setup still needs to finish", { timeout: 30_000 });
  await page.reload();
  await expect(page.getByTestId("setup-recovery")).toContainText("setup still needs to finish");
  await page.context().unroute("**/api/v1/vaults/personal");
  await resume(page, secretKey);
  await confirmDelivery(page);
  await saveScreenshot(page, "a05-partial-setup-recovered");
});

test("discards setup safely when restart loses an unsaved precommit kit", async ({ page }) => {
  const { fixture } = await startFreshFixture();
  await page.goto(fixture.setupURL);
  await fillAccountStep(page, "ada@example.com");
  await page.getByRole("button", { name: "Create account" }).click();
  await expect(page.getByTestId("setup-recovery")).toBeVisible({ timeout: 30_000 });
  await page.reload();
  await page.getByLabel("Master password used for this account").fill(password);
  await page.getByRole("button", { name: "Discard uncommitted setup" }).click();
  await expect(page.getByTestId("setup-recovery")).toHaveCount(0);
  expect(await (await page.request.get(`${fixture.baseURL}/api/v1/setup`)).json()).toEqual({ state: "needsAdmin" });
  await page.goto(fixture.setupURL);
  await expect(page.getByRole("heading", { name: "Create the first admin" })).toBeVisible();
});

test("makes exactly one admin when prepared browser operations commit at once", async ({ browser }) => {
  const { fixture } = await startFreshFixture();
  const pages = await Promise.all([browser.newPage(), browser.newPage()]);
  await Promise.all(pages.map((page) => page.goto(fixture.setupURL)));
  await Promise.all(pages.map((page, index) => fillAccountStep(page, `admin${index}@example.com`)));
  await Promise.all(pages.map((page) => page.getByRole("button", { name: "Create account" }).click()));
  await Promise.all(pages.map(savePrecommitKit));
  await Promise.all(pages.map((page) => resume(page)));
  const outcomes = await Promise.all(pages.map((page) => Promise.race([
    page.getByRole("status").filter({ hasText: "Account setup is complete" }).waitFor({ timeout: 30_000 }).then(() => "admin"),
    page.getByRole("alert").filter({ hasText: invalidSetupLinkMessage }).waitFor({ timeout: 30_000 }).then(() => "refused"),
  ])));
  expect([...outcomes].sort()).toEqual(["admin", "refused"]);
  const winner = pages[outcomes.indexOf("admin")]!;
  const storedSession: { token: string } = JSON.parse((await readStoredSession(winner))!);
  const health = await winner.request.get(`${fixture.baseURL}/api/v1/admin/health`, { headers: { Authorization: `Bearer ${storedSession.token}` } });
  expect(await health.json()).toMatchObject({ counts: { accounts: 1 } });
  await Promise.all(pages.map((page) => page.close()));
});
