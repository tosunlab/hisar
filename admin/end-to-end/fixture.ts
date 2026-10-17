import { spawn, type ChildProcess } from "node:child_process";
import { mkdirSync } from "node:fs";
import { join, resolve } from "node:path";
import { createInterface } from "node:readline";
import { expect, type Page } from "@playwright/test";

export interface FixtureAccount {
  name: string;
  email: string;
  password: string;
  secretKey: string;
}

export interface AdminPageFixture extends FixtureAccount {
  baseURL: string;
  members: FixtureAccount[];
}

const fixtureBinary = process.env.HISAR_ADMIN_PAGE_FIXTURE ?? resolve(import.meta.dirname, "../../bin/hisar-admin-page-fixture");
const screenshotDirectory = process.env.ADMIN_SCREENSHOT_DIRECTORY ?? "";

export async function startAdminPageFixture(): Promise<{ fixture: AdminPageFixture; fixtureProcess: ChildProcess }> {
  const fixtureProcess = spawn(fixtureBinary, ["-addr", process.env.ADMIN_PAGE_ADDRESS ?? "127.0.0.1:0"], { stdio: ["ignore", "pipe", "inherit"] });
  const { promise, resolve: resolveFixture, reject } = Promise.withResolvers<AdminPageFixture>();
  createInterface({ input: fixtureProcess.stdout! }).once("line", (line) => resolveFixture(JSON.parse(line) as AdminPageFixture));
  fixtureProcess.once("exit", (code) => reject(new Error(`the fixture server exited with ${code}`)));
  fixtureProcess.once("error", reject);
  return { fixture: await promise, fixtureProcess };
}

export async function fillSignIn(page: Page, account: FixtureAccount, password = account.password): Promise<void> {
  await expect(page.getByRole("heading", { name: "Sign in to the admin dashboard" })).toBeVisible();
  await page.getByLabel("Email").fill(account.email);
  await page.getByLabel("Secret Key").fill(account.secretKey);
  await page.getByLabel("Master password").fill(password);
}

export function readStoredSession(page: Page): Promise<string | null> {
  return page.evaluate(() => sessionStorage.getItem("hisar.admin.session"));
}

export async function saveScreenshot(page: Page, name: string): Promise<void> {
  if (!screenshotDirectory) {
    return;
  }
  mkdirSync(screenshotDirectory, { recursive: true });
  await page.screenshot({ path: join(screenshotDirectory, `${name}.png`), fullPage: true });
}
