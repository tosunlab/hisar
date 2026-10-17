import type { ChildProcess } from "node:child_process";
import { expect, test, type Page } from "@playwright/test";
import { fillSignIn, readStoredSession, saveScreenshot, startAdminPageFixture, type AdminPageFixture } from "./fixture";

let fixtureProcess: ChildProcess | undefined;
let fixture: AdminPageFixture;

test.describe.configure({ mode: "serial" });

test.beforeAll(async () => {
  ({ fixture, fixtureProcess } = await startAdminPageFixture());
});

test.afterAll(() => {
  fixtureProcess?.kill("SIGTERM");
});

function collectConsoleErrorsAndDialogs(page: Page): string[] {
  const allError: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") {
      allError.push(message.text());
    }
  });
  page.on("pageerror", (error) => allError.push(error.message));
  page.on("dialog", (dialog) => {
    allError.push(`dialog: ${dialog.message()}`);
    void dialog.dismiss();
  });
  return allError;
}

async function getAdminServerStatus(page: Page, token: string): Promise<number> {
  const response = await page.request.get(`${fixture.baseURL}/api/v1/admin/server`, { headers: { Authorization: `Bearer ${token}` } });
  return response.status();
}

test("serves the dashboard with its security headers", async ({ request }) => {
  const index = await request.get(`${fixture.baseURL}/admin`);
  expect(index.status()).toBe(200);
  expect(index.headers()["content-security-policy"]).toBe(
    "default-src 'none'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self'; connect-src 'self'; img-src 'self' data:; worker-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'",
  );
  expect(index.headers()["cache-control"]).toBe("no-store");
  expect(index.headers()["referrer-policy"]).toBe("no-referrer");
  expect(index.headers()["x-robots-tag"]).toBe("noindex, nofollow");
  expect(index.headers()["cross-origin-opener-policy"]).toBe("same-origin");
  const script = (await index.text()).match(/src="(\/admin\/assets\/[^"]+\.js)"/)?.[1];
  expect(script).toBeTruthy();
  const asset = await request.get(`${fixture.baseURL}${script}`, { headers: { "Accept-Encoding": "br" } });
  expect(asset.headers()["cache-control"]).toBe("public, max-age=31536000, immutable");
  expect(asset.headers()["content-encoding"]).toBe("br");
});

test("signs an admin in while the page stays responsive, then shows Overview", async ({ page }) => {
  const allPageError = collectConsoleErrorsAndDialogs(page);
  await page.goto(`${fixture.baseURL}/admin`);
  await fillSignIn(page, fixture);
  await saveScreenshot(page, "sign-in");
  const secretKeyField = page.getByLabel("Secret Key");
  await expect(secretKeyField).toHaveAttribute("type", "password");

  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("button", { name: "Signing in…" })).toBeVisible();
  await page.getByRole("button", { name: "Show" }).click();
  await expect(page.getByRole("button", { name: "Hide" })).toBeVisible();
  await expect(secretKeyField).toHaveAttribute("type", "text");
  await expect(page.getByRole("button", { name: "Signing in…" })).toBeVisible();

  await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible({ timeout: 30_000 });
  await expect(page.getByTestId("admin-name")).toHaveText(fixture.name);
  await expect(page.getByTestId("public-url")).toHaveText(fixture.baseURL);
  await saveScreenshot(page, "overview");
  const stored = JSON.parse((await readStoredSession(page))!) as Record<string, string>;
  expect(Object.keys(stored).sort()).toEqual(["accountId", "deviceId", "name", "token"]);
  expect(JSON.stringify(stored)).not.toContain(fixture.password);
  expect(JSON.stringify(stored)).not.toContain(fixture.secretKey);

  await page.reload();
  await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
  await expect(page.getByTestId("admin-name")).toHaveText(fixture.name);
  const otherTab = await page.context().newPage();
  await otherTab.goto(`${fixture.baseURL}/admin`);
  await expect(otherTab.getByRole("heading", { name: "Sign in to the admin dashboard" })).toBeVisible();
  await otherTab.close();

  expect(await getAdminServerStatus(page, stored.token)).toBe(200);
  await page.getByRole("button", { name: "Sign out" }).click();
  await expect(page.getByRole("heading", { name: "Sign in to the admin dashboard" })).toBeVisible();
  expect(await readStoredSession(page)).toBeNull();
  expect(await getAdminServerStatus(page, stored.token)).toBe(401);
  expect(allPageError).toEqual([]);
});

test("refuses a member on the sign-in screen and keeps no token", async ({ page }) => {
  const allPageError = collectConsoleErrorsAndDialogs(page);
  await page.goto(`${fixture.baseURL}/admin`);
  await fillSignIn(page, fixture.members[0]);
  const deleteRequest = page.waitForRequest((request) => request.method() === "DELETE" && request.url().includes("/api/v1/devices/"));
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("alert")).toHaveText("This account is not an admin on the Hisar server.", { timeout: 30_000 });
  const deleteResponse = await (await deleteRequest).response();
  expect(deleteResponse?.status()).toBe(204);
  await expect(page.getByRole("heading", { name: "Sign in to the admin dashboard" })).toBeVisible();
  expect(await readStoredSession(page)).toBeNull();
  await saveScreenshot(page, "member-refused");
  expect(allPageError).toEqual([]);
});

test("shows the desktop app's message for a wrong master password", async ({ page }) => {
  await page.goto(`${fixture.baseURL}/admin`);
  await fillSignIn(page, fixture, "not the master password");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("alert")).toHaveText("incorrect email, master password or Secret Key", { timeout: 30_000 });
  expect(await readStoredSession(page)).toBeNull();
  await saveScreenshot(page, "wrong-password");
});

test("opens Overview for any path under /admin once signed in", async ({ page }) => {
  await page.goto(`${fixture.baseURL}/admin/some/deep/link`);
  await fillSignIn(page, fixture);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible({ timeout: 30_000 });
  await expect(page.getByRole("link", { name: "Overview" })).toHaveAttribute("aria-current", "page");
  await page.getByRole("button", { name: "Sign out" }).click();
  await expect(page.getByRole("heading", { name: "Sign in to the admin dashboard" })).toBeVisible();
});

test("shows the server's version, usage and checks on Overview", async ({ page }) => {
  const allPageError = collectConsoleErrorsAndDialogs(page);
  await page.goto(`${fixture.baseURL}/admin`);
  await fillSignIn(page, fixture);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible({ timeout: 30_000 });
  await expect(page.getByTestId("version")).toHaveText("dev");
  await expect(page.getByTestId("update")).toHaveCount(0);
  await expect(page.getByTestId("database")).toContainText("migrations applied");
  await expect(page.getByTestId("count-accounts")).toContainText("Accounts");
  const statusByCheck = {
    "public-url": "ok",
    https: "ok",
    proxy: "ok",
    migrations: "ok",
    "secrets-backup": "warning",
    email: "warning",
  };
  for (const [checkID, status] of Object.entries(statusByCheck)) {
    await expect(page.getByTestId(`check-${checkID}`)).toHaveAttribute("data-status", status);
  }
  await expect(page.getByTestId("check-email")).toContainText("Email is not set up. Invites from the apps will fail.");
  await saveScreenshot(page, "overview-health");
  await page.getByRole("button", { name: "Sign out" }).click();
  expect(allPageError).toEqual([]);
});
