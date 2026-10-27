import { mkdirSync } from "node:fs";
import { join, resolve } from "node:path";
import { expect, test, type Page } from "@playwright/test";
import { extensionID } from "../tools/register-fake-host.mjs";
import { launchHarness } from "./harness";

const screenshotDirectory = process.env.HISAR_SCREENSHOT_DIRECTORY;

test.skip(!screenshotDirectory, "Set HISAR_SCREENSHOT_DIRECTORY to capture screenshots");

test("captures the inline icon and menu states", async () => {
  test.setTimeout(120_000);
  const directory = resolve(screenshotDirectory!);
  mkdirSync(directory, { recursive: true });
  const allLogin = ["Test site", "Test site (shared)", "Test site admin", "Test site billing", "Test site staging", "Test site support", "Test site old"].map(
    (title, index) => ({
      vaultID: index % 2 ? "family" : "personal",
      vaultName: index % 2 ? "Family" : "Personal",
      itemID: `item-${index}`,
      title,
      username: index === 0 ? "alex@example.com" : `user${index}@example.com`,
      password: `password ${index}`,
      url: "",
    }),
  );
  const harness = await launchHarness({ chromePath: process.env.HISAR_CHROME_PATH, deviceScaleFactor: 2, allLogin: allLogin.slice(0, 2) });
  const capture = async (page: Page, name: string): Promise<void> => {
    await page.waitForTimeout(400);
    await page.screenshot({ path: join(directory, `inline-${name}.png`), clip: { x: 0, y: 40, width: 520, height: 540 } });
  };
  const openMenu = async (path: string, selector: string) => {
    const page = await harness.context.newPage();
    await page.setViewportSize({ width: 900, height: 640 });
    await page.goto(`${harness.site.origin}${path}`);
    await page.locator(selector).focus();
    await expect.poll(() => harness.findInlineMenuFrame(page) !== undefined).toBe(true);
    return { page, menu: harness.findInlineMenuFrame(page)! };
  };

  try {
    const notPaired = await openMenu("/login", "#username");
    await expect(notPaired.menu.getByTestId("inline-action")).toBeVisible();
    await capture(notPaired.page, "not-paired");
    await harness.pair(await harness.getTabId(`${harness.site.origin}/login`));
    await notPaired.page.close();

    await harness.evaluateInExtension(() => chrome.storage.local.set({ isInlineMenuOpenedOnFocus: false }), undefined);
    const iconOnly = await harness.context.newPage();
    await iconOnly.setViewportSize({ width: 900, height: 640 });
    await iconOnly.goto(`${harness.site.origin}/login`);
    await iconOnly.locator("#username").focus();
    await expect.poll(() => iconOnly.evaluate(() => document.querySelector("hisar-inline-menu") !== null)).toBe(true);
    await capture(iconOnly, "icon");
    await iconOnly.close();
    await harness.evaluateInExtension(() => chrome.storage.local.set({ isInlineMenuOpenedOnFocus: true }), undefined);

    const logins = await openMenu("/login", "#username");
    await expect(logins.menu.getByTestId("inline-login")).toHaveCount(2);
    await capture(logins.page, "logins");
    await logins.page.keyboard.press("ArrowDown");
    await logins.page.keyboard.press("ArrowDown");
    await capture(logins.page, "keyboard-selection");
    await logins.page.close();

    harness.updateState((state) => (state.allLogin = allLogin.map((login) => ({ ...login, url: harness.site.origin }))));
    const many = await openMenu("/login", "#password");
    await expect(many.menu.getByTestId("inline-more")).toBeVisible();
    await capture(many.page, "more-logins");
    await many.page.close();

    harness.updateState((state) => (state.allLogin = []));
    const empty = await openMenu("/login", "#username");
    await expect(empty.menu.getByTestId("inline-no-logins")).toBeVisible();
    await capture(empty.page, "no-logins");
    await empty.page.close();

    const signUp = await openMenu("/sign-up", "#new-password");
    await expect(signUp.menu.getByTestId("suggested-password")).toBeVisible();
    await capture(signUp.page, "suggested-password");
    await signUp.page.close();

    harness.updateState((state) => (state.state = "locked"));
    const locked = await openMenu("/login", "#username");
    await expect(locked.menu.getByTestId("inline-unlock")).toContainText("Unlock Hisar");
    await capture(locked.page, "locked");
    await locked.page.close();

    harness.updateState((state) => {
      state.state = "unlocked";
      state.isDesktopUnavailable = true;
    });
    const unavailable = await openMenu("/login", "#username");
    await expect(unavailable.menu.getByTestId("inline-action")).toHaveText("Open Hisar");
    await capture(unavailable.page, "desktop-unavailable");
    await unavailable.page.close();
    harness.updateState((state) => (state.isDesktopUnavailable = false));

    const popup = await harness.context.newPage();
    await popup.setViewportSize({ width: 360, height: 480 });
    await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=0`);
    await popup.getByTestId("open-settings").click();
    await expect(popup.getByTestId("settings-view")).toBeVisible();
    await popup.waitForTimeout(300);
    await popup.screenshot({ path: join(directory, "inline-popup-settings.png") });
  } finally {
    await harness.close();
  }
});
