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

test("unlocks Hisar with the master password typed in the popup", async () => {
  const { context, site } = harness;
  await harness.pairWithoutPopup();
  harness.updateState((state) => {
    state.state = "locked";
  });
  const loginPage = await context.newPage();
  await loginPage.goto(`${site.origin}/login`);
  const popup = await context.newPage();
  await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${await harness.getTabId(`${site.origin}/login`)}`);

  await expect(popup.getByTestId("status-locked")).toBeVisible();
  await expect(popup.getByTestId("master-password")).toBeFocused();
  await popup.getByTestId("master-password").fill("not it");
  await popup.getByTestId("master-password").press("Enter");
  await expect(popup.getByTestId("unlock-error")).toHaveText("That master password is not right");

  await popup.getByTestId("master-password").fill("master password");
  await popup.getByTestId("unlock-with-password").click();
  await expect(popup.getByTestId("unlocked-view")).toBeVisible();
  await expect(popup.getByTestId("login-row")).toHaveCount(1);
});
