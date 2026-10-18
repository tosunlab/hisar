import { expect, test, type Page } from "@playwright/test";
import { extensionID } from "../tools/register-fake-host.mjs";
import { launchHarness, type Harness } from "./harness";

let harness: Harness;

async function hasInlineHost(page: Page): Promise<boolean> {
  return page.evaluate(() => document.querySelector("hisar-inline-menu") !== null);
}

async function waitForMenu(page: Page) {
  await expect.poll(() => harness.findInlineMenuFrame(page) !== undefined).toBe(true);
  return harness.findInlineMenuFrame(page)!;
}

// The menu ignores clicks until it has been visible and unobscured for 300 ms.
async function waitForClickGuard(page: Page): Promise<void> {
  await page.waitForTimeout(500);
}

// Every test gets its own browser profile and fake host state, so a failure cannot leak into the next test.
test.beforeEach(async () => {
  harness = await launchHarness();
});

test.afterEach(async () => {
  await harness?.close();
});

test("offers pairing in the field before the browser is paired", async () => {
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/login`);
  await page.locator("#username").focus();
  await expect.poll(() => hasInlineHost(page)).toBe(true);
  const menu = await waitForMenu(page);
  await expect(menu.getByTestId("inline-status")).toHaveText("This browser is not paired with Hisar");
  await expect(menu.getByTestId("inline-action")).toHaveText("Pair with Hisar");
  await page.close();
});

test("lists matching logins under the focused field and fills the chosen one", async () => {
  await harness.pairWithoutPopup();
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/login`);
  await page.locator("#password").focus();
  const menu = await waitForMenu(page);
  await expect(menu.getByTestId("inline-login")).toHaveCount(1);
  await expect(menu.getByTestId("inline-login")).toContainText("Test site");
  await expect(menu.getByTestId("inline-login")).toContainText("alex@example.com");
  expect(await page.evaluate(() => document.body.innerHTML.includes("alex@example.com"))).toBe(false);
  await waitForClickGuard(page);
  await menu.getByTestId("inline-login").click();
  await expect(page.locator("#username")).toHaveValue("alex@example.com");
  await expect(page.locator("#password")).toHaveValue("old password");
  await expect.poll(() => harness.findInlineMenuFrame(page) === undefined).toBe(true);
  await page.close();
});

test("serves the menu from a dynamic extension URL", async () => {
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/login`);
  await page.locator("#username").focus();
  await waitForMenu(page);
  const menuUrl = new URL((await harness.readInlineMenuSource(page))!);
  expect(menuUrl.protocol).toBe("chrome-extension:");
  expect(menuUrl.pathname).toBe("/inline-menu.html");
  expect(menuUrl.host).not.toBe(extensionID);
  await page.close();
});

test("ignores clicks while the page makes the menu transparent", async () => {
  await harness.pairWithoutPopup();
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/login`);
  await page.locator("#username").focus();
  const menu = await waitForMenu(page);
  await expect(menu.getByTestId("inline-login")).toHaveCount(1);
  await page.evaluate(() => (document.documentElement.style.opacity = "0.01"));
  await waitForClickGuard(page);
  await menu.getByTestId("inline-login").click({ force: true });
  await page.waitForTimeout(500);
  await expect(page.locator("#password")).toHaveValue("");
  await page.evaluate(() => (document.documentElement.style.opacity = ""));
  await waitForClickGuard(page);
  await menu.getByTestId("inline-login").click();
  await expect(page.locator("#password")).toHaveValue("old password");
  await page.close();
});

test("ignores clicks through an overlay that lets pointer events pass", async () => {
  await harness.pairWithoutPopup();
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/login`);
  await page.locator("#username").focus();
  const menu = await waitForMenu(page);
  await expect(menu.getByTestId("inline-login")).toHaveCount(1);
  const isTracked = await menu.evaluate(() => "isVisible" in IntersectionObserverEntry.prototype);
  test.skip(!isTracked, "IntersectionObserver v2 is not available in this browser");
  await page.evaluate(() => {
    const overlay = document.createElement("div");
    overlay.style.cssText = "position:fixed;inset:0;background:#f4f4f5;pointer-events:none;z-index:2147483647";
    document.documentElement.append(overlay);
  });
  await waitForClickGuard(page);
  await menu.getByTestId("inline-login").click({ force: true });
  await page.waitForTimeout(500);
  await expect(page.locator("#password")).toHaveValue("");
  await page.close();
});

test("fills with the arrow keys and Enter from the field", async () => {
  await harness.pairWithoutPopup();
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/login`);
  await page.locator("#username").focus();
  const menu = await waitForMenu(page);
  await expect(menu.getByTestId("inline-login")).toHaveCount(1);
  await waitForClickGuard(page);
  await page.keyboard.press("ArrowDown");
  await expect(menu.getByTestId("inline-login")).toHaveClass(/selected/);
  await page.keyboard.press("Enter");
  await expect(page.locator("#password")).toHaveValue("old password");
  await page.close();
});

test("closes the menu on Escape and opens it again with the icon", async () => {
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/login`);
  await page.locator("#username").focus();
  await waitForMenu(page);
  await page.keyboard.press("Escape");
  await expect.poll(() => harness.findInlineMenuFrame(page) === undefined).toBe(true);
  expect(await hasInlineHost(page)).toBe(true);
  await (await harness.readShadowHost(page, "hisar-inline-menu"))!.clickButton("*");
  await waitForMenu(page);
  await expect(page.locator("#username")).toBeFocused();
  await page.close();
});

test("shows no icon in a cross-origin frame", async () => {
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/login`);
  const frame = page.frames().find((candidate) => candidate.url().startsWith(harness.site.crossOrigin))!;
  await frame.locator("#frame-username").focus();
  await page.waitForTimeout(800);
  expect(await frame.evaluate(() => document.querySelector("hisar-inline-menu") !== null)).toBe(false);
  expect(await hasInlineHost(page)).toBe(false);
  await page.close();
});

test("suggests a password on a sign-up form and fills both fields", async () => {
  await harness.pairWithoutPopup();
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/sign-up`);
  await page.locator("#new-password").focus();
  const menu = await waitForMenu(page);
  const suggestion = menu.getByTestId("suggested-password");
  await expect(suggestion).toContainText("Use suggested password");
  const suggested = (await suggestion.locator(".subtitle").textContent())!.trim();
  expect(suggested.length).toBeGreaterThanOrEqual(20);
  await waitForClickGuard(page);
  await suggestion.click();
  await expect(page.locator("#new-password")).toHaveValue(suggested);
  await expect(page.locator("#confirm-password")).toHaveValue(suggested);
  // The sign-up's email offers identities, once the extension knows the app fills them.
  await (await harness.context.newPage()).goto(`chrome-extension://${extensionID}/popup.html?tabId=${await harness.getTabId(page.url())}`);
  await page.bringToFront();
  await page.locator("#email").focus();
  await expect((await waitForMenu(page)).getByTestId("inline-identity")).toContainText("Alex Example");
  await page.close();
});

test("asks to unlock when Hisar is locked and lists logins once unlocked", async () => {
  await harness.pairWithoutPopup();
  harness.updateState((state) => {
    state.state = "locked";
    state.unlockDelayMilliseconds = 500;
  });
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/login`);
  await page.locator("#username").focus();
  const menu = await waitForMenu(page);
  await expect(menu.getByTestId("inline-status")).toHaveText("Hisar is locked");
  await waitForClickGuard(page);
  await menu.getByTestId("inline-action").click();
  await expect(menu.getByTestId("inline-login")).toHaveCount(1, { timeout: 10_000 });
  await page.close();
});

test("offers Open Hisar when the app is not running", async () => {
  await harness.pairWithoutPopup();
  harness.updateState((state) => (state.isDesktopUnavailable = true));
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/login`);
  await page.locator("#username").focus();
  const menu = await waitForMenu(page);
  await expect(menu.getByTestId("inline-status")).toHaveText("Hisar is not running");
  await expect(menu.getByTestId("inline-action")).toHaveText("Open Hisar");
  harness.updateState((state) => (state.isDesktopUnavailable = false));
  await waitForClickGuard(page);
  await menu.getByTestId("inline-action").click();
  await expect(menu.getByTestId("inline-login")).toHaveCount(1);
  await page.close();
});

test("follows the settings: no icon when turned off, no menu on focus when that is off", async () => {
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/login`);
  await harness.evaluateInExtension(() => chrome.storage.local.set({ isInlineMenuEnabled: false }), undefined);
  await page.locator("#username").focus();
  await page.waitForTimeout(500);
  expect(await hasInlineHost(page)).toBe(false);
  await page.locator("#password").focus();
  await harness.evaluateInExtension(() => chrome.storage.local.set({ isInlineMenuEnabled: true, isInlineMenuOpenedOnFocus: false }), undefined);
  await page.locator("#username").focus();
  await expect.poll(() => hasInlineHost(page)).toBe(true);
  await page.waitForTimeout(500);
  expect(harness.findInlineMenuFrame(page)).toBeUndefined();
  await page.close();
});
