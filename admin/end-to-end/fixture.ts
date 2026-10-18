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

// FreshAdminPageFixture is the first line of a fixture started with -fresh: an empty server that waits for setup.
export interface FreshAdminPageFixture {
  baseURL: string;
  smtpSink: string;
  setupURL: string;
}

// ReceivedEmail is a later line: one email the fixture's SMTP sink received.
export interface ReceivedEmail {
  recipients: string[];
  data: string;
}

export interface RunningFixture<Fixture = AdminPageFixture> {
  fixture: Fixture;
  fixtureProcess: ChildProcess;
  receivedEmails: ReceivedEmail[];
  // logLines is the fixture's stderr so far, which it also passes through.
  logLines: string[];
}

const fixtureBinary = process.env.HISAR_ADMIN_PAGE_FIXTURE ?? resolve(import.meta.dirname, "../../bin/hisar-admin-page-fixture");
const screenshotDirectory = process.env.ADMIN_SCREENSHOT_DIRECTORY ?? "";

export async function startAdminPageFixture<Fixture = AdminPageFixture>(environment: Record<string, string> = {}, flags: string[] = []): Promise<RunningFixture<Fixture>> {
  const fixtureProcess = spawn(fixtureBinary, ["-addr", process.env.ADMIN_PAGE_ADDRESS ?? "127.0.0.1:0", ...flags], {
    stdio: ["ignore", "pipe", "pipe"],
    env: { ...process.env, ...environment },
  });
  const { promise, resolve: resolveFixture, reject } = Promise.withResolvers<Fixture>();
  const receivedEmails: ReceivedEmail[] = [];
  const logLines: string[] = [];
  let isFirstLine = true;
  createInterface({ input: fixtureProcess.stdout! }).on("line", (line) => {
    if (isFirstLine) {
      isFirstLine = false;
      resolveFixture(JSON.parse(line) as Fixture);
      return;
    }
    receivedEmails.push((JSON.parse(line) as { receivedEmail: ReceivedEmail }).receivedEmail);
  });
  createInterface({ input: fixtureProcess.stderr! }).on("line", (line) => {
    logLines.push(line);
    process.stderr.write(`${line}\n`);
  });
  fixtureProcess.once("exit", (code) => reject(new Error(`the fixture server exited with ${code}`)));
  fixtureProcess.once("error", reject);
  return { fixture: await promise, fixtureProcess, receivedEmails, logLines };
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
