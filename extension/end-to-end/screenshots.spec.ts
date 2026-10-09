import { mkdirSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { chromium, expect, test, type Page } from "@playwright/test";
import { createInitialState, readState, writeState, type FakeHostState } from "../tools/fake-native-host.mjs";
import { extensionID, registerFakeHost } from "../tools/register-fake-host.mjs";
import { startTestSite } from "./test-site";

const screenshotDirectory = process.env.HISAR_SCREENSHOT_DIRECTORY;
const chromePath = process.env.HISAR_CHROME_PATH;
const distDirectory = resolve(import.meta.dirname, "../dist");

test.skip(!screenshotDirectory, "Set HISAR_SCREENSHOT_DIRECTORY to capture screenshots");

test("captures the popup states and the save bar", async () => {
  const directory = resolve(screenshotDirectory!);
  mkdirSync(directory, { recursive: true });
  const temporaryDirectory = mkdtempSync(join(tmpdir(), "hisar-screenshots-"));
  const site = await startTestSite();
  const { stateFile } = registerFakeHost({
    userDataDirectory: join(temporaryDirectory, "profile"),
    stateFile: join(temporaryDirectory, "state.json"),
    hostDirectory: join(temporaryDirectory, "host"),
  });
  const updateState = (change: (state: FakeHostState) => void): void => {
    const state = readState(stateFile);
    change(state);
    writeState(stateFile, state);
  };
  updateState((state) => {
    Object.assign(state, createInitialState(), { pairingCode: "246810", unlockDelayMilliseconds: 600000 });
    state.allLogin = [
      { vaultID: "p", vaultName: "Personal", itemID: "1", title: "Test site", username: "alex@example.com", password: "a", url: site.origin },
      { vaultID: "f", vaultName: "Family", itemID: "2", title: "Test site (shared)", username: "family@example.com", password: "b", url: site.origin },
    ];
  });
  const context = await chromium.launchPersistentContext(join(temporaryDirectory, "profile"), {
    ...(chromePath ? { executablePath: chromePath } : { channel: "chromium" }),
    headless: true,
    ignoreDefaultArgs: ["--disable-extensions"],
    viewport: { width: 900, height: 640 },
    deviceScaleFactor: 2,
    // Branded Chrome ignores --load-extension, so it loads the build over the DevTools protocol instead.
    args: chromePath
      ? ["--enable-unsafe-extension-debugging"]
      : [`--disable-extensions-except=${distDirectory}`, `--load-extension=${distDirectory}`],
  });
  try {
    if (chromePath) {
      const session = await context.browser()!.newBrowserCDPSession();
      const { id } = (await session.send("Extensions.loadUnpacked" as never, { path: distDirectory } as never)) as { id: string };
      expect(id).toBe(extensionID);
    }
    const loginPage = await context.newPage();
    await loginPage.goto(`${site.origin}/login`);
    const popup = await context.newPage();
    await popup.setViewportSize({ width: 360, height: 480 });
    await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=0`);
    const tabId = await popup.evaluate(async (url) => (await chrome.tabs.query({ url }))[0].id as number, `${site.origin}/login`);
    const popupUrl = `chrome-extension://${extensionID}/popup.html?tabId=${tabId}`;
    const capture = async (page: Page, name: string): Promise<void> => {
      await page.waitForTimeout(300);
      await page.screenshot({ path: join(directory, `extension-${name}.png`) });
    };

    await popup.goto(popupUrl);
    await expect(popup.getByTestId("start-pairing")).toBeVisible();
    await capture(popup, "popup-not-paired");
    await popup.getByTestId("start-pairing").click();
    await popup.getByTestId("pairing-code").fill("246810");
    await capture(popup, "popup-pairing-code");
    await popup.getByTestId("finish-pairing").click();
    await expect(popup.getByTestId("login-row")).toHaveCount(2);
    await capture(popup, "popup-unlocked");
    await popup.getByTestId("section-generator").click();
    await expect(popup.getByTestId("generated-password")).not.toBeEmpty();
    await capture(popup, "popup-generator");

    updateState((state) => (state.state = "locked"));
    await popup.goto(popupUrl);
    await expect(popup.getByTestId("unlock")).toBeVisible();
    await capture(popup, "popup-locked");

    updateState((state) => (state.state = "signed-out"));
    await popup.goto(popupUrl);
    await expect(popup.getByTestId("status-signed-out")).toBeVisible();
    await capture(popup, "popup-signed-out");

    updateState((state) => (state.isDesktopUnavailable = true));
    await popup.goto(popupUrl);
    await expect(popup.getByTestId("open-hisar")).toBeVisible();
    await capture(popup, "popup-desktop-unavailable");

    updateState((state) => {
      state.isDesktopUnavailable = false;
      state.state = "unlocked";
    });
    await popup.close();
    await loginPage.bringToFront();
    await loginPage.goto(`${site.origin}/login`);
    await loginPage.locator("#username").fill("alex@example.com");
    await loginPage.locator("#password").fill("changed");
    await loginPage.locator("#sign-in").click();
    await expect(loginPage.locator("h1")).toHaveText("Welcome back");
    await loginPage.waitForTimeout(1500);
    await capture(loginPage, "save-bar-update");

    await loginPage.goto(`${site.origin}/login`);
    await loginPage.locator("#username").fill("new-person@example.com");
    await loginPage.locator("#password").fill("pw");
    await loginPage.locator("#sign-in").click();
    await expect(loginPage.locator("h1")).toHaveText("Welcome back");
    await loginPage.waitForTimeout(1500);
    await capture(loginPage, "save-bar-new");
  } finally {
    await context.close();
    await site.close();
    rmSync(temporaryDirectory, { recursive: true, force: true });
  }
});
