import { expect, test, type BrowserContext } from "@playwright/test";
import { readState } from "../tools/fake-native-host.mjs";
import { extensionID } from "../tools/register-fake-host.mjs";
import { launchHarness, type Harness } from "./harness";
import type { TestSite } from "./test-site";

let harness: Harness;
let stateFile: string;
let site: TestSite;
let context: BrowserContext;


function getTabId(url: string): Promise<number> {
  return harness.getTabId(url);
}

// Every test gets its own browser profile and fake host state, so a failure cannot leak into the next test.
test.beforeEach(async () => {
  harness = await launchHarness();
  ({ stateFile, site, context } = harness);
});

test.afterEach(async () => {
  await harness?.close();
});

test("loads with the extension ID from the manifest key", async () => {
  expect(await harness.evaluateInExtension(() => chrome.runtime.id, undefined)).toBe(extensionID);
});

test("pairs, fills only same-origin frames, and offers to update a changed password", async () => {
  const loginPage = await context.newPage();
  await loginPage.goto(`${site.origin}/login`);
  const tabId = await getTabId(`${site.origin}/login`);
  const openSavePopup = async () => {
    const confirmation = await context.newPage();
    await confirmation.goto(`chrome-extension://${extensionID}/popup.html?tabId=${tabId}`);
    return confirmation;
  };

  const popup = await context.newPage();
  await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${tabId}`);
  await popup.getByTestId("start-pairing").click();
  await expect(popup.getByTestId("pairing-code")).toBeVisible();
  expect(readState(stateFile).pendingPairing?.browser).toBeTruthy();
  await popup.getByTestId("pairing-code").fill("246810");
  await popup.getByTestId("finish-pairing").click();
  await expect(popup.getByTestId("login-row")).toHaveCount(1);
  await expect(popup.getByTestId("login-row")).toContainText("Test site");
  expect(readState(stateFile).allPairing).toHaveLength(1);

  await popup.getByTestId("fill").click();
  await expect(loginPage.locator("#username")).toHaveValue("alex@example.com");
  await expect(loginPage.locator("#password")).toHaveValue("old password");
  const crossOriginFrame = loginPage.frameLocator("#other-origin");
  await expect(crossOriginFrame.locator("#frame-username")).toHaveValue("");
  await expect(crossOriginFrame.locator("#frame-password")).toHaveValue("");

  await popup.getByTestId("copy-password").click();
  await expect(popup.getByTestId("notice")).toContainText("Password copied");

  await popup.getByTestId("section-generator").click();
  await expect(popup.getByTestId("generated-password")).not.toBeEmpty();
  await popup.close();

  await loginPage.bringToFront();
  await loginPage.locator("#password").fill("new password");
  await loginPage.locator("#sign-in").click();
  await expect(loginPage.locator("h1")).toHaveText("Welcome back");
  expect(await harness.readShadowHost(loginPage, "hisar-save-bar")).toBeUndefined();
  expect(readState(stateFile).allLogin[0].password).toBe("old password");
  const confirmation = await openSavePopup();
  await expect(confirmation.getByTestId("save-confirmation")).toContainText("Test site");
  await expect(confirmation.getByTestId("save-confirmation")).toContainText("Personal");
  await confirmation.getByTestId("save-confirm").click();
  await expect.poll(() => readState(stateFile).allLogin[0].password).toBe("new password");
  expect(readState(stateFile).allLogin).toHaveLength(1);
});

test("offers to save a new login and stores it in the desktop", async () => {
  await harness.pairWithoutPopup();
  const loginPage = await context.newPage();
  await loginPage.goto(`${site.origin}/login`);
  const tabId = await getTabId(loginPage.url());
  await loginPage.locator("#username").fill("someone-new@example.com");
  await loginPage.locator("#password").fill("brand new");
  await loginPage.locator("#password").press("Enter");
  await expect(loginPage.locator("h1")).toHaveText("Welcome back");
  const popup = await context.newPage();
  await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${tabId}`);
  await popup.getByTestId("save-confirm").click();
  await expect
    .poll(() => readState(stateFile).allLogin.find((login) => login.username === "someone-new@example.com")?.password)
    .toBe("brand new");
});

test("copies a password from the popup, and it stays on the clipboard", async () => {
  await harness.pairWithoutPopup();
  await context.grantPermissions(["clipboard-read"], { origin: site.origin });
  const loginPage = await context.newPage();
  await loginPage.goto(`${site.origin}/login`);
  const popup = await context.newPage();
  await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${await getTabId(`${site.origin}/login`)}`);
  await popup.getByTestId("copy-password").click();
  await expect(popup.getByTestId("notice")).toHaveText("Password copied");
  await loginPage.bringToFront();
  expect(await loginPage.evaluate(() => navigator.clipboard.readText())).toBe("old password");
});

test("Never for this site stops offers for the origin", async () => {
  await harness.pairWithoutPopup();
  const loginPage = await context.newPage();
  await loginPage.goto(`${site.origin}/login`);
  const tabId = await getTabId(loginPage.url());
  await loginPage.locator("#username").fill("third@example.com");
  await loginPage.locator("#password").fill("pw");
  await loginPage.locator("#sign-in").click();
  const popup = await context.newPage();
  await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${tabId}`);
  await popup.getByTestId("save-never").click();
  await expect(popup.getByTestId("save-confirmation")).toHaveCount(0);
  const allNeverOrigin = await harness.evaluateInExtension(async () => (await chrome.storage.local.get(["allNeverOrigin"])).allNeverOrigin, undefined);
  expect(allNeverOrigin).toEqual([site.origin]);

  await loginPage.goto(`${site.origin}/login`);
  await loginPage.locator("#username").fill("fourth@example.com");
  await loginPage.locator("#password").fill("pw");
  await loginPage.locator("#sign-in").click();
  await expect(loginPage.locator("h1")).toHaveText("Welcome back");
  await loginPage.waitForTimeout(1500);
  await popup.reload();
  await expect(popup.getByTestId("unlocked-view")).toBeVisible();
  await expect(popup.getByTestId("save-confirmation")).toHaveCount(0);
  expect(readState(stateFile).allLogin.some((login) => login.username === "fourth@example.com")).toBe(false);
});
