import { expect, test } from "@playwright/test";
import { extensionID } from "../tools/register-fake-host.mjs";
import { launchHarness, type Harness } from "./harness";

let harness: Harness;

test.beforeEach(async () => {
  harness = await launchHarness();
});

test.afterEach(async () => {
  await harness?.close();
});

test("saves a new login under the title and vault chosen in the popup after the submit navigation", async ({}, testInfo) => {
  await harness.pairWithoutPopup();
  const loginPage = await harness.context.newPage();
  await loginPage.goto(`${harness.site.origin}/login`);
  const tabId = await harness.getTabId(loginPage.url());
  await loginPage.locator("#username").fill("someone-new@example.com");
  await loginPage.locator("#password").fill("brand new");
  await loginPage.locator("#password").press("Enter");
  await expect(loginPage.locator("h1")).toHaveText("Welcome back");
  const popup = await harness.context.newPage();
  await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${tabId}`);
  await expect(popup.getByTestId("save-confirmation")).toBeVisible();
  await popup.getByTestId("save-title").fill("Test shop");
  await popup.getByTestId("save-vault").selectOption("vault-family");
  await expect(popup.getByTestId("save-confirmation")).toContainText("Everyone with access");
  await popup.screenshot({ path: testInfo.outputPath("save-popup-new.png"), fullPage: true });
  await popup.getByTestId("save-confirm").click();
  await expect
    .poll(() => harness.readState().allLogin.find((login) => login.username === "someone-new@example.com"))
    .toMatchObject({ title: "Test shop", vaultID: "vault-family", password: "brand new" });
});

test("keeps the offer and its badge through a same-document submit navigation and saves it", async () => {
  await harness.pairWithoutPopup();
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/login`);
  const tabId = await harness.getTabId(page.url());
  await page.evaluate(() => document.querySelector("form")!.addEventListener("submit", event => event.preventDefault()));
  await page.locator("#username").fill("single-page@example.com");
  await page.locator("#password").fill("single page password");
  await page.locator("h1").click();
  await page.evaluate(() => document.querySelector("form")!.requestSubmit());
  const badge = () => harness.evaluateInExtension(id => chrome.action.getBadgeText({ tabId: id }), tabId);
  await expect.poll(badge).toBe("+");
  await page.evaluate(() => history.pushState({}, "", "/welcome"));
  await expect.poll(() => harness.evaluateInExtension(async id => (await chrome.tabs.get(id)).url, tabId)).toBe(`${harness.site.origin}/welcome`);
  await expect.poll(badge).toBe("+");
  const popup = await harness.context.newPage();
  await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${tabId}`);
  await expect(popup.getByTestId("save-confirmation")).toBeVisible();
  await popup.getByTestId("save-confirm").click();
  await expect.poll(() => harness.readState().allLogin.find((login) => login.username === "single-page@example.com")).toMatchObject({ password: "single page password" });
});

test("creates a login from the popup for the page", async () => {
  await harness.pairWithoutPopup();
  const loginPage = await harness.context.newPage();
  await loginPage.goto(`${harness.site.origin}/login`);
  const popup = await harness.context.newPage();
  await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${await harness.getTabId(`${harness.site.origin}/login`)}`);
  await popup.getByTestId("new-item-open").click();
  await expect(popup.getByTestId("new-title")).toHaveValue("127.0.0.1");
  await expect(popup.getByTestId("new-password")).not.toHaveValue("");
  await popup.getByTestId("new-title").fill("Test site 2");
  await popup.getByTestId("new-username").fill("second@example.com");
  await popup.getByTestId("new-item-save").click();
  await expect(popup.getByTestId("item-title")).toHaveText("Test site 2");
  expect(harness.readState().allLogin.find((login) => login.title === "Test site 2")).toMatchObject({ username: "second@example.com", url: harness.site.origin });
});

test("moves a new login's site to the page the tab navigates to", async () => {
  await harness.pairWithoutPopup();
  const loginPage = await harness.context.newPage();
  await loginPage.goto(`${harness.site.origin}/login`);
  const popup = await harness.context.newPage();
  await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${await harness.getTabId(`${harness.site.origin}/login`)}`);
  await popup.getByTestId("new-item-open").click();
  await expect(popup.getByTestId("new-url")).toHaveValue(harness.site.origin);
  await loginPage.goto(`${harness.site.crossOrigin}/login`);
  await expect(popup.getByTestId("new-title")).toHaveValue("localhost");
  await expect(popup.getByTestId("new-url")).toHaveValue(harness.site.crossOrigin);
});

test("equal and different guesses produce identical page-visible behavior and page clicks never update", async () => {
  await harness.pairWithoutPopup();
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/login`);
  const tabId = await harness.getTabId(page.url());
  await page.evaluate(() => {
    document.querySelector("form")!.addEventListener("submit", event => event.preventDefault(), true);
    const style = document.createElement("style");
    style.textContent = "hisar-save-bar { opacity:0!important;position:fixed!important;inset:0!important;z-index:2147483647!important }";
    document.head.append(style);
    const decoy = document.createElement("button");
    decoy.id = "page-decoy";
    decoy.textContent = "Continue";
    document.body.append(decoy);
  });
  await page.locator("#username").fill("alex@example.com");
  const observations: unknown[] = [];
  for (const password of ["old password", "attacker-chosen"]) {
    await page.locator("#password").fill(password);
    await page.evaluate(() => {
      const world = window as unknown as { observedHosts: string[]; observer?: MutationObserver };
      world.observer?.disconnect();
      world.observedHosts = [];
      world.observer = new MutationObserver(records => {
        for (const record of records) for (const node of record.addedNodes) {
          if (node instanceof Element && node.tagName.startsWith("HISAR-")) world.observedHosts.push(node.tagName);
        }
      });
      world.observer.observe(document.documentElement, { childList: true, subtree: true });
      document.querySelector("form")!.requestSubmit();
    });
    await page.waitForTimeout(1200);
    observations.push(await page.evaluate(() => ({
      hosts: "observedHosts" in window ? window.observedHosts : [],
      saveHosts: document.querySelectorAll("hisar-save-bar").length,
      frames: [...document.querySelectorAll("iframe")].map(frame => frame.src).filter(url => url.includes("save")),
    })));
    await page.locator("#page-decoy").click();
    expect(harness.readState().allLogin[0].password).toBe("old password");
  }
  expect(observations).toEqual([{ hosts: [], saveHosts: 0, frames: [] }, { hosts: [], saveHosts: 0, frames: [] }]);
  const popup = await harness.context.newPage();
  await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${tabId}`);
  await expect(popup.getByTestId("save-confirmation")).toContainText("Update this login?");
  await popup.getByTestId("save-decline").click();
  await expect(popup.getByTestId("save-confirmation")).toHaveCount(0);
  expect(harness.readState().allLogin[0].password).toBe("old password");
});

test("navigation after review invalidates the popup offer instead of saving to a stale destination", async () => {
  await harness.pairWithoutPopup();
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/login`);
  const tabId = await harness.getTabId(page.url());
  await page.locator("#username").fill("alex@example.com");
  await page.locator("#password").fill("new password");
  await page.locator("#sign-in").click();
  await expect(page.locator("h1")).toHaveText("Welcome back");
  const popup = await harness.context.newPage();
  await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${tabId}`);
  await expect(popup.getByTestId("save-confirmation")).toBeVisible();
  await page.goto(`${harness.site.origin}/login`);
  await popup.getByTestId("save-confirm").click();
  expect(harness.readState().allLogin[0].password).toBe("old password");
});
