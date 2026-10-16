import { expect, test, type Frame, type Page } from "@playwright/test";
import { extensionID } from "../tools/register-fake-host.mjs";
import { launchHarness, type Harness } from "./harness";

let harness: Harness;

async function waitForMenu(page: Page): Promise<Frame> {
  await expect.poll(() => harness.findInlineMenuFrame(page) !== undefined).toBe(true);
  return harness.findInlineMenuFrame(page)!;
}

// The menu ignores clicks until it has been visible and unobscured for 300 ms.
async function waitForClickGuard(page: Page): Promise<void> {
  await page.waitForTimeout(500);
}

// openPopup opens the popup for the checkout tab; its hello also tells the extension which features the app has.
async function openPopup(page: Page): Promise<Page> {
  const popup = await harness.context.newPage();
  await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${await harness.getTabId(page.url())}`);
  await popup.getByTestId("unlocked-view").waitFor();
  return popup;
}

async function openCheckout(): Promise<Page> {
  await harness.pairWithoutPopup();
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/address`);
  return page;
}

test.beforeEach(async () => {
  harness = await launchHarness();
});

test.afterEach(async () => {
  await harness?.close();
});

test("lists addresses under an address field and fills the form", async () => {
  const page = await openCheckout();
  await (await openPopup(page)).close();
  await page.locator("#city").focus();
  const menu = await waitForMenu(page);
  await expect(menu.getByTestId("inline-address")).toHaveCount(1);
  await expect(menu.getByTestId("inline-address")).toContainText("Springfield, US");
  expect(harness.readState().allAddressRequest ?? []).toEqual([]);
  await waitForClickGuard(page);
  await menu.getByTestId("inline-address").click();
  await expect(page.locator("#line1")).toHaveValue("1 Main St");
  await expect(page.locator("#line2")).toHaveValue("Apt 4");
  await expect(page.locator("#city")).toHaveValue("Springfield");
  await expect(page.locator("#region")).toHaveValue("IL");
  await expect(page.locator("#postal")).toHaveValue("62704");
  await expect(page.locator("#country")).toHaveValue("United States");
  expect(await page.frameLocator("#same-origin").locator("#frame-city").inputValue()).toBe("");
  await page.close();
});

test("fills an address from the popup into a same-origin frame, never a cross-origin one", async () => {
  const page = await openCheckout();
  await page.frameLocator("#same-origin").locator("#frame-postal").focus();
  const popup = await openPopup(page);
  await popup.getByTestId("section-addresses").click();
  await popup.getByTestId("fill-address").click();
  const frame = page.frameLocator("#same-origin");
  await expect(frame.locator("#frame-postal")).toHaveValue("62704");
  await expect(frame.locator("#frame-line1")).toHaveValue("1 Main St");
  await expect(page.locator("#postal")).toHaveValue("");
  await expect(page.frameLocator("#other-origin").locator("#frame-postal")).toHaveValue("");
  await page.close();
});

test("offers no address filling with an app that does not do it", async () => {
  harness.updateState((state) => {
    state.features = [];
  });
  const page = await openCheckout();
  const popup = await openPopup(page);
  await expect(popup.getByTestId("section-logins")).toBeVisible();
  await expect(popup.getByTestId("section-addresses")).toHaveCount(0);
  await popup.close();
  await page.locator("#city").focus();
  await page.waitForTimeout(500);
  expect(await page.evaluate(() => document.querySelector("hisar-inline-menu") !== null)).toBe(false);
  await page.close();
});
