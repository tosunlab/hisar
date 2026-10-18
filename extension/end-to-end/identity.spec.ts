import { expect, test, type Frame, type Page } from "@playwright/test";
import { extensionID } from "../tools/register-fake-host.mjs";
import { launchHarness, type Harness } from "./harness";

let harness: Harness;

async function waitForMenu(page: Page): Promise<Frame> {
  await expect.poll(() => harness.findInlineMenuFrame(page) !== undefined).toBe(true);
  return harness.findInlineMenuFrame(page)!;
}

// openDetails pairs, opens the form, and opens the popup once so the extension knows the app fills identities.
async function openDetails(): Promise<Page> {
  await harness.pairWithoutPopup();
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/identity`);
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

test("lists identities under a name field and fills the name, email, phone and the address beside them", async () => {
  const page = await openDetails();
  await page.locator("#full-name").focus();
  const menu = await waitForMenu(page);
  await expect(menu.getByTestId("inline-identity")).toContainText("Alex Example · alex@example.com");
  expect(harness.readState().allIdentityRequest ?? []).toEqual([]);
  // The menu ignores clicks until it has been visible and unobscured for 300 ms.
  await page.waitForTimeout(500);
  await menu.getByTestId("inline-identity").click();
  await expect(page.locator("#full-name")).toHaveValue("Alex Example");
  await expect(page.locator("#email")).toHaveValue("alex@example.com");
  await expect(page.locator("#phone")).toHaveValue("+1 555 0100");
  await expect(page.locator("#street")).toHaveValue("1 Main St, Apt 4");
  await expect(page.locator("#city")).toHaveValue("Springfield");
  await expect(page.locator("#postal")).toHaveValue("62704");
});

test("fills an identity from the popup's Identities tab", async () => {
  const page = await openDetails();
  const popup = await harness.context.newPage();
  await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${await harness.getTabId(page.url())}`);
  await popup.getByTestId("section-addresses").click();
  await expect(popup.getByTestId("section-addresses")).toHaveText("Identities");
  await popup.getByTestId("fill-identity").click();
  await page.bringToFront();
  await expect(page.locator("#email")).toHaveValue("alex@example.com");
});
