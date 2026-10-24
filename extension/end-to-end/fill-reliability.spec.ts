import { expect, test, type Page } from "@playwright/test";
import { extensionID } from "../tools/register-fake-host.mjs";
import { launchHarness, type Harness } from "./harness";

let harness: Harness;

async function waitForMenu(page: Page) {
  await expect.poll(() => harness.findInlineMenuFrame(page) !== undefined).toBe(true);
  return harness.findInlineMenuFrame(page)!;
}

// The menu ignores clicks until it has been visible and unobscured for 300 ms.
async function waitForClickGuard(page: Page): Promise<void> {
  await page.waitForTimeout(500);
}

async function fillFromMenu(page: Page, fieldSelector: string): Promise<void> {
  await page.locator(fieldSelector).click();
  const menu = await waitForMenu(page);
  await expect(menu.getByTestId("inline-login")).toHaveCount(1);
  await waitForClickGuard(page);
  await menu.getByTestId("inline-login").click();
}

async function expectFilled(page: Page): Promise<void> {
  await expect(page.locator("#username")).toHaveValue("alex@example.com");
  await expect(page.locator("#password")).toHaveValue("old password");
}

test.beforeEach(async () => {
  harness = await launchHarness();
  await harness.pairWithoutPopup();
});

test.afterEach(async () => {
  await harness?.close();
});

// A click before the menu has been visible for 300 ms is refused, but never silently: the menu asks to choose again.
test("fills, or asks to choose again, when the login is clicked as soon as the menu shows it", async () => {
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/login`);
  await page.locator("#username").click();
  const menu = await waitForMenu(page);
  await expect(menu.getByTestId("inline-login")).toHaveCount(1);
  await menu.getByTestId("inline-login").click();
  const readError = () => menu.getByTestId("inline-error").count().catch(() => 0);
  await expect.poll(async () => (await page.locator("#username").inputValue()) !== "" || (await readError()) > 0).toBe(true);
  if ((await page.locator("#username").inputValue()) === "") {
    await waitForClickGuard(page);
    await menu.getByTestId("inline-login").click();
  }
  await expectFilled(page);
});

test("fills a login form in a modal dialog", async () => {
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/dialog-login`);
  await fillFromMenu(page, "#username");
  await expectFilled(page);
});

test("fills fields that the page renders again while the username is filled", async () => {
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/rerender-login`);
  await fillFromMenu(page, "#username");
  await expectFilled(page);
});

test("fills fields whose page keeps the value from key events", async () => {
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/masked-login`);
  await fillFromMenu(page, "#username");
  await expectFilled(page);
  await page.locator("h1").click();
  await expectFilled(page);
});

test("fills a username that is read-only until focused", async () => {
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/readonly-login`);
  await fillFromMenu(page, "#password");
  await expectFilled(page);
});

test("offers the menu in a login form added after the page loaded", async () => {
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/late-login`);
  await page.locator("#username").waitFor();
  await fillFromMenu(page, "#username");
  await expectFilled(page);
});

test("fills on a page inverted by a filter on the root element", async () => {
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/filtered-login`);
  await fillFromMenu(page, "#username");
  await expectFilled(page);
});

test("fills the first step of a login without a form, not the newsletter form", async () => {
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/formless-with-newsletter`);
  const tabId = await harness.getTabId(`${harness.site.origin}/formless-with-newsletter`);
  const popup = await harness.context.newPage();
  await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${tabId}`);
  await popup.getByTestId("fill").click();
  await expect(page.locator("#username")).toHaveValue("alex@example.com");
  await expect(page.locator("#newsletter-email")).toHaveValue("");
});
