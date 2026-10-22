import { spawn } from "node:child_process";
import { existsSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { expect, test, type Page, type Route } from "@playwright/test";
import type { AdminAccountDetail, AdminAccountPage } from "../src/sections/accounts/accounts";
import { fillSignIn, saveScreenshot, startAdminPageFixture, type AdminPageFixture, type FixtureAccount, type RunningFixture } from "./fixture";

const cliBinary = process.env.HISAR_CLI ?? resolve(import.meta.dirname, "../../bin/hisar");
const suspendedMessage = "This account is suspended. Contact the admin of the Hisar server.";
const signedOutMessage = "this device was signed out";

let running: RunningFixture;
let fixture: AdminPageFixture;
let cliDirectory: string;

test.describe.configure({ mode: "serial" });

test.beforeAll(async () => {
  running = await startAdminPageFixture();
  fixture = running.fixture;
  cliDirectory = mkdtempSync(join(tmpdir(), "hisar-admin-accounts-"));
});

test.afterAll(() => {
  running?.fixtureProcess.kill("SIGTERM");
  rmSync(cliDirectory, { recursive: true, force: true });
});

interface CLIResult {
  code: number | null;
  stdout: string;
  stderr: string;
}

// runCLI runs the standalone hisar for one device of the account, whose state lives in its own directory, and answers
// its prompts with input.
function runCLI(device: string, account: FixtureAccount, args: string[], input: string): Promise<CLIResult> {
  const child = spawn(cliBinary, args, {
    env: {
      ...process.env,
      HISAR_STANDALONE: "1",
      HISAR_NO_LAUNCH: "1",
      HISAR_STATE: join(cliDirectory, device, "state.json"),
      HISAR_SECRET_KEY: account.secretKey,
    },
  });
  let stdout = "";
  let stderr = "";
  child.stdout.on("data", (chunk: Buffer) => (stdout += chunk));
  child.stderr.on("data", (chunk: Buffer) => (stderr += chunk));
  child.stdin.end(input);
  const { promise, resolve: resolveResult, reject } = Promise.withResolvers<CLIResult>();
  child.once("error", reject);
  child.once("close", (code) => resolveResult({ code, stdout, stderr }));
  return promise;
}

// logIn answers hisar login: the server and email prompts keep their defaults once the device has signed in before, and
// the Secret Key comes from HISAR_SECRET_KEY then.
function logIn(device: string, account: FixtureAccount, password = account.password): Promise<CLIResult> {
  const hasSignedIn = existsSync(join(cliDirectory, device, "state.json"));
  const input = hasSignedIn ? `\n\n${password}\n` : `${fixture.baseURL}\n${account.email}\n${account.secretKey}\n${password}\n`;
  return runCLI(device, account, ["login"], input);
}

function listVaults(device: string, account: FixtureAccount): Promise<CLIResult> {
  return runCLI(device, account, ["vaults"], `${account.password}\n`);
}

function fetchAsAdmin(path: string, token = fixture.token, method = "GET"): Promise<Response> {
  return fetch(`${fixture.baseURL}/api/v1${path}`, { method, headers: { Authorization: `Bearer ${token}` } });
}

async function findAccountId(email: string, token = fixture.token): Promise<string> {
  const page: AdminAccountPage = await (await fetchAsAdmin(`/admin/accounts?query=${encodeURIComponent(email)}`, token)).json();
  return page.accounts.find((account) => account.email === email)!.id;
}

async function readRole(email: string, token: string): Promise<string> {
  const detail: AdminAccountDetail = await (await fetchAsAdmin(`/admin/accounts/${await findAccountId(email, token)}`, token)).json();
  return detail.account.role;
}

async function signIn(page: Page, account: FixtureAccount, path: string, heading: string): Promise<void> {
  await page.goto(`${fixture.baseURL}${path}`);
  await fillSignIn(page, account);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: heading, level: 1 })).toBeVisible({ timeout: 30_000 });
}

async function openAccount(page: Page, account: FixtureAccount, signedInAs: FixtureAccount = fixture): Promise<void> {
  await signIn(page, signedInAs, `/admin/accounts/${await findAccountId(account.email)}`, account.name);
}

async function fillProof(page: Page, account: FixtureAccount, password = account.password): Promise<void> {
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Your Secret Key").fill(account.secretKey);
  await dialog.getByLabel("Your master password").fill(password);
}

test("lists the accounts with their last activity and narrows them as you type", async ({ page }) => {
  const [mira, milo] = fixture.members;
  await signIn(page, fixture, "/admin/accounts", "Accounts");
  for (const account of [fixture, mira, milo]) {
    await expect(page.getByTestId(`account-${account.email}`)).toContainText(account.name);
  }
  await expect(page.getByTestId(`account-${mira.email}`)).toContainText("Just now");
  await saveScreenshot(page, "a2-list");

  await page.getByLabel("Search").fill(mira.email.split("@")[0]);
  await expect(page.locator("tbody tr")).toHaveCount(1);
  await expect(page.getByTestId(`account-${mira.email}`)).toContainText("Member");
  await saveScreenshot(page, "a2-search");
});

test("suspends a member, whose CLI keeps its session but is refused until reactivation", async ({ page }) => {
  const mira = fixture.members[0];
  expect((await logIn("mira-1", mira)).code).toBe(0);
  expect((await listVaults("mira-1", mira)).stdout).toContain("Personal");

  await openAccount(page, mira);
  await page.getByRole("button", { name: "Suspend…" }).click();
  await page.getByRole("dialog", { name: "Suspend account" }).getByRole("button", { name: "Suspend", exact: true }).click();
  await expect(page.getByTestId("state")).toHaveText("Suspended");

  await expect(page.locator('[data-testid^="device-"]').first()).toContainText("Suspended · last activity");
  const suspendedList = await listVaults("mira-1", mira);
  expect(suspendedList.stderr).toContain(`hisar: ${suspendedMessage}\n`);
  expect(suspendedList.stderr).not.toContain(signedOutMessage);
  const refused = await logIn("mira-3", mira);
  expect(refused.code).toBe(1);
  expect(refused.stderr).toContain(`hisar: ${suspendedMessage}\n`);
  expect((await logIn("mira-3", mira, "not the master password")).stderr).toContain("hisar: incorrect email, password or Secret Key\n");
  await saveScreenshot(page, "a2-suspended");

  await page.getByRole("button", { name: "Reactivate" }).click();
  await expect(page.getByTestId("state")).toHaveText("Active");
  // The device that was signed in works again with the same session, without signing in.
  expect((await listVaults("mira-1", mira)).stdout).toContain("Personal");
  await saveScreenshot(page, "a2-reactivated");
});

test("signs a member out on every device", async ({ page }) => {
  const mira = fixture.members[0];
  expect((await logIn("mira-2", mira)).code).toBe(0);

  await openAccount(page, mira);
  await page.getByRole("button", { name: "Sign out everywhere…" }).click();
  await page.getByRole("dialog", { name: "Sign out everywhere" }).getByRole("button", { name: "Sign out everywhere" }).click();
  await expect(page.getByRole("dialog")).toBeHidden();
  const devices = page.locator(".devices li");
  await expect(devices.getByRole("button", { name: "Sign out" })).toHaveCount(0);
  for (const device of await devices.all()) {
    await expect(device).toContainText("Signed out");
  }

  for (const device of ["mira-1", "mira-2"]) {
    expect((await listVaults(device, mira)).stderr).toContain(signedOutMessage);
  }
  await saveScreenshot(page, "a2-signed-out");
});

test("makes a member an admin after the admin's proof, and the new admin reaches Overview", async ({ page, browser }) => {
  const milo = fixture.members[1];
  await openAccount(page, milo);
  await page.getByRole("button", { name: "Change role…" }).click();
  const dialog = page.getByRole("dialog", { name: "Change role" });
  await expect(dialog.getByLabel(/^Admin/)).toBeChecked();
  await fillProof(page, fixture);
  await dialog.getByRole("button", { name: "Change role" }).click();
  await expect(dialog.getByRole("button", { name: "Checking master password…" })).toBeDisabled();
  await expect(page.getByTestId("role")).toHaveText("Admin", { timeout: 30_000 });
  await saveScreenshot(page, "a2-role-changed");

  const miloPage = await (await browser.newContext()).newPage();
  await signIn(miloPage, milo, "/admin", "Overview");
  await saveScreenshot(miloPage, "a2-promoted");
});

test("keeps the account when the delete dialog gets a wrong master password", async ({ page }) => {
  const mira = fixture.members[0];
  await openAccount(page, mira);
  await page.getByRole("button", { name: "Delete account…" }).click();
  const dialog = page.getByRole("dialog", { name: "Delete account" });
  await dialog.getByLabel(`Type ${mira.email} to confirm`).fill(mira.email);
  await fillProof(page, fixture, "not the master password");
  await dialog.getByRole("button", { name: "Delete account" }).click();
  await expect(dialog.getByRole("alert")).toHaveText("incorrect email, master password or Secret Key", { timeout: 30_000 });
  await saveScreenshot(page, "a2-bad-proof");
  expect(await findAccountId(mira.email)).toBeTruthy();
});

test("deletes a member after the admin's proof", async ({ page }) => {
  const mira = fixture.members[0];
  const accountId = await findAccountId(mira.email);
  await openAccount(page, mira);
  await page.getByRole("button", { name: "Delete account…" }).click();
  const dialog = page.getByRole("dialog", { name: "Delete account" });
  await dialog.getByLabel(`Type ${mira.email} to confirm`).fill(mira.email);
  await fillProof(page, fixture);
  await dialog.getByRole("button", { name: "Delete account" }).click();
  await expect(page.getByRole("heading", { name: "Accounts", level: 1 })).toBeVisible({ timeout: 30_000 });
  await expect(page.getByTestId(`account-${fixture.email}`)).toBeVisible();
  await expect(page.getByTestId(`account-${mira.email}`)).toHaveCount(0);
  await saveScreenshot(page, "a2-deleted");

  expect((await fetchAsAdmin(`/admin/accounts/${accountId}`)).status).toBe(404);
});

// The database may hold admins of earlier runs, so the test suspends them first. The two admins then demote each other,
// each request held until both proofs are done and both let through at once. One change is saved and the other refused:
// by the last-admin check when its request passed the admin check before the first change was saved, and as "admin only"
// when it reached the server after. Releasing both at once makes the first case likely, but the browser cannot force it.
test("refuses the change that would leave the server without an admin", async ({ page, browser }) => {
  const milo = fixture.members[1];
  let cursor = "";
  do {
    const accountPage: AdminAccountPage = await (await fetchAsAdmin(`/admin/accounts?cursor=${encodeURIComponent(cursor)}`)).json();
    for (const account of accountPage.accounts) {
      if (account.role === "admin" && account.state === "active" && account.email !== fixture.email && account.email !== milo.email) {
        expect((await fetchAsAdmin(`/admin/accounts/${account.id}/suspend`, fixture.token, "POST")).status).toBe(204);
      }
    }
    cursor = accountPage.nextCursor ?? "";
  } while (cursor);

  const miloPage = await (await browser.newContext()).newPage();
  await openAccount(page, milo);
  await openAccount(miloPage, fixture, milo);

  const allHeldRoute: Route[] = [];
  const { promise: bothHeld, resolve: resolveBothHeld } = Promise.withResolvers<void>();
  for (const adminPage of [page, miloPage]) {
    await adminPage.route("**/api/v1/admin/accounts/*/role", (route) => {
      allHeldRoute.push(route);
      if (allHeldRoute.length === 2) {
        resolveBothHeld();
      }
    });
  }
  for (const [adminPage, admin] of [
    [page, fixture],
    [miloPage, milo],
  ] as const) {
    await adminPage.getByRole("button", { name: "Change role…" }).click();
    await expect(adminPage.getByRole("dialog").getByLabel(/^Member/)).toBeChecked();
    await fillProof(adminPage, admin);
  }
  await Promise.all([page, miloPage].map((adminPage) => adminPage.getByRole("dialog").getByRole("button", { name: "Change role" }).click()));
  await bothHeld;
  await Promise.all(allHeldRoute.map((route) => route.continue()));

  const allAlert = [page, miloPage].map((adminPage) => adminPage.getByRole("dialog").getByRole("alert"));
  const readAllRefusal = async () => (await Promise.all(allAlert.map((alert) => alert.allTextContents()))).flat();
  await expect.poll(readAllRefusal, { timeout: 30_000 }).toHaveLength(1);
  expect(["The Hisar server needs at least one admin.", "admin only"]).toContain((await readAllRefusal())[0]);
  const isAdaRefused = (await allAlert[0].count()) > 0;
  const remainingAdmin = isAdaRefused ? milo : fixture;
  const roles = await Promise.all([fixture, milo].map((admin) => readRole(admin.email, remainingAdmin.token)));
  expect(roles).toEqual(isAdaRefused ? ["member", "admin"] : ["admin", "member"]);
  await saveScreenshot(isAdaRefused ? page : miloPage, "a2-last-admin");
});
