import { expect, test } from "@playwright/test";
import { until } from "selenium-webdriver";
import { By, extensionBaseUrl, launchFirefoxHarness, type FirefoxHarness } from "./harness";

let harness: FirefoxHarness;

const byTestId = (testId: string) => By.css(`[data-testid="${testId}"]`);

async function readValue(selector: string): Promise<string> {
  return (await harness.driver.findElement(By.css(selector)).getAttribute("value")) ?? "";
}

async function readFrameValue(frameSelector: string, fieldSelector: string): Promise<string> {
  await harness.driver.switchTo().frame(await harness.driver.findElement(By.css(frameSelector)));
  try {
    return await readValue(fieldSelector);
  } finally {
    await harness.driver.switchTo().defaultContent();
  }
}

// openPopup opens the popup for the page's tab in a tab of its own, as the extension does when it cannot open the popup.
async function openPopup(pageUrl: string): Promise<string> {
  return harness.openTab(`${extensionBaseUrl}popup.html?tabId=${await harness.getTabId(pageUrl)}`);
}

async function click(testId: string): Promise<void> {
  const element = await harness.driver.wait(until.elementLocated(byTestId(testId)), 10_000);
  await harness.driver.wait(until.elementIsVisible(element), 10_000);
  await element.click();
}

// openMenuAt focuses the field and returns the inline menu's frame once it lists testId; the menu ignores clicks until it
// has been visible and uncovered for 300 ms, so it waits a little longer.
async function openMenuAt(fieldSelector: string, testId: string): Promise<string> {
  await harness.waitForContentScripts(await harness.driver.getCurrentUrl());
  await harness.driver.executeScript("document.querySelector(arguments[0]).focus()", fieldSelector);
  const menu = await harness.waitForFrame("/inline-menu.html");
  await expect.poll(() => harness.readFrame(menu, `document.querySelectorAll('[data-testid="${testId}"]').length`)).toBeGreaterThan(0);
  await harness.driver.sleep(500);
  return menu;
}

test.beforeEach(async ({}, testInfo) => {
  harness = await launchFirefoxHarness(
    testInfo.title.includes("one-time code")
      ? { allLogin: [{ vaultID: "vault-personal", vaultName: "Personal", itemID: "item-test-site", title: "Test site", username: "alex@example.com", password: "old password", url: "", oneTimePassword: "JBSWY3DPEHPK3PXP" }] }
      : {},
  );
});

test.afterEach(async () => {
  await harness?.close();
});

test("pairs as Firefox, fills only same-origin frames from the popup, and saves a changed password", async () => {
  const loginUrl = `${harness.site.origin}/login`;
  const page = await harness.openTab(loginUrl);
  await openPopup(loginUrl);
  await click("start-pairing");
  await expect.poll(() => harness.readState().pendingPairing?.browser).toBe("Firefox");
  await harness.driver.findElement(byTestId("pairing-code")).sendKeys("246810");
  await click("finish-pairing");
  await harness.driver.wait(until.elementLocated(byTestId("login-row")), 10_000);
  expect(await harness.driver.findElement(byTestId("login-row")).getText()).toContain("Test site");

  await click("fill");
  await harness.driver.switchTo().window(page);
  await expect.poll(() => readValue("#username")).toBe("alex@example.com");
  expect(await readValue("#password")).toBe("old password");
  expect(await readFrameValue("#other-origin", "#frame-username")).toBe("");
  expect(await readFrameValue("#other-origin", "#frame-password")).toBe("");

  await harness.driver.findElement(By.css("#password")).clear();
  await harness.driver.findElement(By.css("#password")).sendKeys("new password");
  await harness.driver.findElement(By.css("#sign-in")).click();
  await expect.poll(() => harness.driver.findElement(By.css("h1")).getText()).toBe("Welcome back");
  await openPopup(`${harness.site.origin}/welcome`);
  await click("save-confirm");
  await expect.poll(() => harness.readState().allLogin[0].password).toBe("new password");
});

test("fills a login from the inline menu inside its closed shadow root, and copies the one-time code", async () => {
  await harness.pairWithoutPopup();
  await harness.openTab(`${harness.site.origin}/login`);
  await harness.writeClipboard("before the fill");
  const menu = await openMenuAt("#username", "inline-login");
  expect(String(await harness.readFrame(menu, "document.body.innerText"))).toContain("Test site");
  await harness.clickInFrame(menu, '[data-testid="inline-login"]');
  await expect.poll(() => readValue("#username")).toBe("alex@example.com");
  expect(await readValue("#password")).toBe("old password");
  await expect.poll(() => harness.findFrame("/inline-menu.html")).toBeUndefined();
  // Without an offscreen document the event page copies the code itself.
  await expect.poll(() => harness.readClipboard()).toMatch(/^\d{6}$/);
});

test("fills a card from the inline menu and leaves the payment form in another site's frame empty", async () => {
  await harness.pairWithoutPopup();
  const cardUrl = `${harness.site.origin}/card`;
  const page = await harness.openTab(cardUrl);
  // The popup learns from Hisar that it fills cards, which the content script then offers.
  await openPopup(cardUrl);
  await harness.driver.wait(until.elementLocated(byTestId("unlocked-view")), 10_000);
  await harness.driver.close();
  await harness.driver.switchTo().window(page);
  const menu = await openMenuAt("#card-number", "inline-card");
  await harness.clickInFrame(menu, '[data-testid="inline-card"]');
  await expect.poll(() => readValue("#card-number")).toBe("4111111111111111");
  expect(await readValue("#card-name")).toBe("Alex Example");
  expect(await readValue("#card-code")).toBe("123");
  expect(await readFrameValue("#payment-frame", "#frame-card-number")).toBe("");
});

test("asks for website access once Firefox has taken it away", async () => {
  await harness.pairWithoutPopup();
  const loginUrl = `${harness.site.origin}/login`;
  await harness.openTab(loginUrl);
  expect(await harness.evaluateInExtension("return chrome.permissions.remove({ origins: ['<all_urls>'] });")).toBe(true);
  await openPopup(loginUrl);
  const banner = await harness.driver.wait(until.elementLocated(byTestId("site-access")), 10_000);
  expect(await banner.getText()).toContain("Hisar can't fill or save logins on websites until you allow it.");
});

// Firefox 140 has no location.ancestorOrigins (153 has). Another site may frame a login page of the test site, whose login
// is saved; the frame rule, deciding from the origins of the frame's parents, shows no Hisar icon or menu there.
test("shows no icon or menu in a login frame that another site frames", async () => {
  await harness.pairWithoutPopup();
  const outerUrl = `${harness.site.crossOrigin}/outer-frame`;
  await harness.openTab(outerUrl);
  await harness.waitForContentScripts(outerUrl);
  await harness.driver.switchTo().frame(await harness.driver.findElement(By.css("#inner")));
  // A real click, which also focuses the frame, as a person would.
  await harness.driver.findElement(By.css("#frame-username")).click();
  await harness.driver.sleep(1500);
  expect(await harness.driver.executeScript("return document.querySelector('hisar-inline-menu') !== null")).toBe(false);
  await harness.driver.switchTo().defaultContent();
  expect(await harness.findFrame("/inline-menu.html")).toBeUndefined();

  // The same frame on its own site does get the menu.
  await harness.openTab(`${harness.site.origin}/frame`);
  await openMenuAt("#frame-username", "inline-login");
});
