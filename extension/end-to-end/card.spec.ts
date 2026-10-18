import { expect, test, type Frame, type Page } from "@playwright/test";
import { extensionID } from "../tools/register-fake-host.mjs";
import { launchHarness, type Harness } from "./harness";

let harness: Harness;

async function waitForMenu(page: Page): Promise<Frame> {
  await expect.poll(() => harness.findInlineMenuFrame(page) !== undefined).toBe(true);
  return harness.findInlineMenuFrame(page)!;
}

// openCheckout pairs, opens the payment page, and opens the popup once so the extension knows the app fills cards.
async function openCheckout(): Promise<Page> {
  await harness.pairWithoutPopup();
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/card`);
  const popup = await harness.context.newPage();
  await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${await harness.getTabId(page.url())}`);
  await popup.getByTestId("unlocked-view").waitFor();
  await popup.close();
  return page;
}

test.beforeEach(async () => {
  harness = await launchHarness();
});

test.afterEach(async () => {
  await harness?.close();
});

test("lists cards under a card field and fills the payment form, lists included, without offering to save a login", async () => {
  const page = await openCheckout();
  await page.locator("#card-number").focus();
  const menu = await waitForMenu(page);
  await expect(menu.getByTestId("inline-card")).toHaveText(/Visa.*1111.*12\/30/);
  expect(harness.readState().allCardRequest ?? []).toEqual([]);
  // The menu ignores clicks until it has been visible and unobscured for 300 ms.
  await page.waitForTimeout(500);
  await menu.getByTestId("inline-card").click();
  await expect(page.locator("#card-number")).toHaveValue("4111111111111111");
  await expect(page.locator("#card-name")).toHaveValue("Alex Example");
  await expect(page.locator("#card-month")).toHaveValue("12");
  await expect(page.locator("#card-year")).toHaveValue("2030");
  await expect(page.locator("#card-code")).toHaveValue("123");
  // The payment form in another site's frame stays empty.
  await expect(page.frameLocator("#payment-frame").locator("#frame-card-number")).toHaveValue("");

  await page.locator("#card-code").press("Enter");
  await expect(page.locator("h1")).toHaveText("Welcome back");
  await page.waitForTimeout(1000);
  expect(await harness.readShadowHost(page, "hisar-save-bar")).toBeUndefined();
});

test("fills a card from the popup's Cards tab", async () => {
  const page = await openCheckout();
  const popup = await harness.context.newPage();
  await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${await harness.getTabId(page.url())}`);
  await popup.getByTestId("section-cards").click();
  await expect(popup.getByTestId("card-row")).toContainText("Visa");
  await popup.getByTestId("fill-card").click();
  await page.bringToFront();
  await expect(page.locator("#card-number")).toHaveValue("4111111111111111");
});
