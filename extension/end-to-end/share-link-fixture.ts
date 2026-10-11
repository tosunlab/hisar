import { spawn, type ChildProcess } from "node:child_process";
import { mkdirSync } from "node:fs";
import { join, resolve } from "node:path";
import { createInterface } from "node:readline";
import type { Page } from "@playwright/test";

export interface ShareLinkFixture {
  baseURL: string;
  password: string;
  passphrase: string;
  links: Record<"item" | "passphrase" | "oneView" | "text" | "screenshot" | "screenshotPassphrase", string>;
}

const fixtureBinary = process.env.HISAR_SHARE_LINK_PAGE_FIXTURE ?? resolve(import.meta.dirname, "../../bin/hisar-share-link-page-fixture");
export const screenshotDirectory = process.env.SHARE_LINK_SCREENSHOT_DIRECTORY ?? "";

// The fixture runs the real API server and creates the links with the Go client, in the database HISAR_DATABASE_URL names.
export async function startShareLinkFixture(): Promise<{ fixture: ShareLinkFixture; fixtureProcess: ChildProcess }> {
  const fixtureProcess = spawn(fixtureBinary, ["-addr", process.env.SHARE_LINK_PAGE_ADDRESS ?? "127.0.0.1:0"], { stdio: ["ignore", "pipe", "inherit"] });
  const fixture = await new Promise<ShareLinkFixture>((resolveFixture, reject) => {
    createInterface({ input: fixtureProcess.stdout! }).once("line", (line) => resolveFixture(JSON.parse(line) as ShareLinkFixture));
    fixtureProcess.once("exit", (code) => reject(new Error(`the fixture server exited with ${code}`)));
    fixtureProcess.once("error", reject);
  });
  return { fixture, fixtureProcess };
}

export async function saveScreenshot(page: Page, name: string): Promise<void> {
  if (!screenshotDirectory) {
    return;
  }
  mkdirSync(screenshotDirectory, { recursive: true });
  await page.screenshot({ path: join(screenshotDirectory, `share-link-${name}.png`), fullPage: true });
}
