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
  token: string;
}


export interface AdminPageFixture extends FixtureAccount {
  baseURL: string;
  smtpSink: string;
  members: FixtureAccount[];
}

// ReceivedEmail is a later line: one email the fixture's SMTP sink received.
export interface ReceivedEmail {
  recipients: string[];
  data: string;
}

export interface RunningFixture {
  fixture: AdminPageFixture;
  fixtureProcess: ChildProcess;
  receivedEmails: ReceivedEmail[];
}

const fixtureBinary = process.env.HISAR_ADMIN_PAGE_FIXTURE ?? resolve(import.meta.dirname, "../../bin/hisar-admin-page-fixture");
const screenshotDirectory = process.env.ADMIN_SCREENSHOT_DIRECTORY ?? "";

export async function startAdminPageFixture(environment: Record<string, string> = {}): Promise<RunningFixture> {
  const fixtureProcess = spawn(fixtureBinary, ["-addr", process.env.ADMIN_PAGE_ADDRESS ?? "127.0.0.1:0"], {
    stdio: ["ignore", "pipe", "inherit"],
    env: { ...process.env, ...environment },
  });
  const { promise, resolve: resolveFixture, reject } = Promise.withResolvers<AdminPageFixture>();
  const receivedEmails: ReceivedEmail[] = [];
  let isFirstLine = true;
  createInterface({ input: fixtureProcess.stdout! }).on("line", (line) => {
    if (isFirstLine) {
      isFirstLine = false;
      resolveFixture(JSON.parse(line) as AdminPageFixture);
      return;
    }
    receivedEmails.push((JSON.parse(line) as { receivedEmail: ReceivedEmail }).receivedEmail);
  });
  fixtureProcess.once("exit", (code) => reject(new Error(`the fixture server exited with ${code}`)));
  fixtureProcess.once("error", reject);
  return { fixture: await promise, fixtureProcess, receivedEmails };
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
