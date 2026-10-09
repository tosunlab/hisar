import { mkdirSync } from "node:fs";
import { join, resolve } from "node:path";
import { expect, test, type Frame, type Page } from "@playwright/test";
import { launchHarness } from "./harness";
import { startPasskeySite } from "./passkey-site";

const screenshotDirectory = process.env.HISAR_SCREENSHOT_DIRECTORY;

test.skip(!screenshotDirectory, "Set HISAR_SCREENSHOT_DIRECTORY to capture screenshots");

test("captures the passkey prompts", async () => {
  test.setTimeout(120_000);
  const directory = resolve(screenshotDirectory!);
  mkdirSync(directory, { recursive: true });
  const harness = await launchHarness({ chromePath: process.env.HISAR_CHROME_PATH, deviceScaleFactor: 2 });
  const site = await startPasskeySite();
  harness.updateState((state) => {
    state.allLogin.push({ vaultID: "vault-personal", vaultName: "Personal", itemID: "item-site", title: "Passkey test site", username: "alex@example.com", password: "pw", url: site.origin });
  });
  await harness.pairWithoutPopup();
  const page = await harness.context.newPage();
  await page.setViewportSize({ width: 960, height: 560 });
  await page.goto(site.origin);
  const findPrompt = async (): Promise<Frame> => {
    await expect.poll(() => page.frames().some((frame) => frame.url().includes("/passkey-prompt.html"))).toBe(true);
    return page.frames().find((frame) => frame.url().includes("/passkey-prompt.html"))!;
  };
  const capture = async (name: string): Promise<void> => {
    await page.waitForTimeout(500);
    await page.screenshot({ path: join(directory, `passkey-${name}.png`) });
  };
  const click = async (prompt: Frame, testID: string): Promise<void> => {
    const box = (await prompt.getByTestId(testID).first().boundingBox())!;
    void page.mouse.click(box.x + box.width / 2, box.y + box.height / 2).catch(() => undefined);
  };
  const waitForStatus = (page: Page) => expect(page.locator("#status")).not.toHaveText("ready");
  const resetStatus = () => page.locator("#status").evaluate((element) => (element.textContent = "ready"));

  try {
    await page.locator("#register").click();
    const savePrompt = await findPrompt();
    await expect(savePrompt.getByTestId("save-passkey")).toBeVisible();
    await capture("save");
    await click(savePrompt, "save-passkey");
    await waitForStatus(page);

    await resetStatus();
    await page.locator("#sign-in").click();
    const choosePrompt = await findPrompt();
    await expect(choosePrompt.getByTestId("passkey-choice")).toBeVisible();
    await capture("sign-in");
    await click(choosePrompt, "passkey-choice");
    await waitForStatus(page);

    await resetStatus();
    await page.locator("#register-excluded").click();
    const excludedPrompt = await findPrompt();
    await expect(excludedPrompt.getByTestId("passkey-excluded")).toBeVisible();
    await capture("already-saved");
    await click(excludedPrompt, "confirm-excluded");
    await waitForStatus(page);

    harness.updateState((state) => (state.state = "locked"));
    await resetStatus();
    await page.locator("#register").click();
    const lockedPrompt = await findPrompt();
    await expect(lockedPrompt.getByTestId("unlock-hisar")).toBeVisible();
    await capture("locked");
  } finally {
    await site.close();
    await harness.close();
  }
});
