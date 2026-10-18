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

test("saves a new login under the title and in the vault chosen in the save bar, after the page moves on", async () => {
  await harness.pairWithoutPopup();
  const loginPage = await harness.context.newPage();
  await loginPage.goto(`${harness.site.origin}/login`);
  await loginPage.locator("#username").fill("someone-new@example.com");
  await loginPage.locator("#password").fill("brand new");
  await loginPage.locator("#password").press("Enter");
  await expect(loginPage.locator("h1")).toHaveText("Welcome back");
  await expect.poll(async () => (await harness.readShadowHost(loginPage, "hisar-save-bar"))?.text ?? "", { timeout: 10_000 }).toContain("Save this login");
  const bar = (await harness.readShadowHost(loginPage, "hisar-save-bar"))!;
  await bar.typeInto("title", "Test shop");
  await bar.typeInto("vault", "Family");
  await bar.clickButton("Save");
  await expect
    .poll(() => harness.readState().allLogin.find((login) => login.username === "someone-new@example.com"))
    .toMatchObject({ title: "Test shop", vaultID: "vault-family", password: "brand new" });
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
