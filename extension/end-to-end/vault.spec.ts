import { expect, test } from "@playwright/test";
import { readState } from "../tools/fake-native-host.mjs";
import { extensionID } from "../tools/register-fake-host.mjs";
import { launchHarness, type Harness } from "./harness";

let harness: Harness;

test.beforeEach(async () => {
  harness = await launchHarness();
});

test.afterEach(async () => {
  await harness?.close();
});

test("browses every vault from the popup: searches, opens a card, reveals and copies its number", async () => {
  const { context, site, stateFile } = harness;
  await harness.pairWithoutPopup();
  await context.grantPermissions(["clipboard-read"], { origin: site.origin });
  const loginPage = await context.newPage();
  await loginPage.goto(`${site.origin}/login`);
  const popup = await context.newPage();
  await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${await harness.getTabId(`${site.origin}/login`)}`);

  // The page's login is suggested first, and the rest of the vault is listed below it.
  await expect(popup.getByTestId("login-row")).toHaveCount(1);
  await expect(popup.getByTestId("item-row")).toHaveText([/Visa/, /Wi-Fi/]);
  await popup.getByTestId("vault-search").fill("wi-fi");
  await expect(popup.getByTestId("item-row")).toHaveText([/Wi-Fi/]);
  await expect(popup.getByTestId("login-row")).toHaveCount(0);

  await popup.getByTestId("category-credit-card").click();
  await popup.getByTestId("vault-search").fill("");
  await expect(popup.getByTestId("item-row")).toHaveText([/Visa/]);
  await popup.getByTestId("open-item").click();
  await expect(popup.getByTestId("item-title")).toHaveText("Visa");
  const number = popup.getByTestId("field-field:1");
  await expect(number).not.toContainText("4111");
  expect(readState(stateFile).allRevealRequest ?? []).toEqual([]);

  await number.getByTestId("reveal-field").click();
  await expect(number).toContainText("4111 1111 1111 1111");
  await number.getByTestId("copy-field").click();
  await expect(popup.getByTestId("notice")).toHaveText("Card number copied");
  await loginPage.bringToFront();
  expect(await loginPage.evaluate(() => navigator.clipboard.readText())).toBe("4111 1111 1111 1111");

  await popup.bringToFront();
  await popup.getByTestId("item-favorite").click();
  await expect.poll(() => readState(stateFile).allFavoriteItemID).toEqual(["item-visa"]);
});
