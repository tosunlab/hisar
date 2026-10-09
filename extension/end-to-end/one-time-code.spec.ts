import { createHmac } from "node:crypto";
import { expect, test, type Page } from "@playwright/test";
import { extensionID } from "../tools/register-fake-host.mjs";
import { launchHarness, type Harness } from "./harness";

// RFC 6238 test secret; the expected codes are computed here, independently of the fake host.
const setupKey = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ";
const secret = Buffer.from("12345678901234567890", "ascii");

let harness: Harness;

function computeCode(milliseconds: number): string {
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(milliseconds / 30_000)));
  const digest = createHmac("sha1", secret).update(counter).digest();
  const offset = digest[19] & 0x0f;
  const binary = ((digest[offset] & 0x7f) << 24) | (digest[offset + 1] << 16) | (digest[offset + 2] << 8) | digest[offset + 3];
  return String(binary % 1_000_000).padStart(6, "0");
}

// The code is read after the fill; a fill that crossed a 30 s boundary may hold the earlier code.
async function expectCurrentCode(readCode: () => Promise<string>, startedAt: number): Promise<void> {
  const filledCode = await readCode();
  expect([computeCode(startedAt), computeCode(Date.now())]).toContain(filledCode);
  expect(filledCode).toMatch(/^\d{6}$/);
}

async function waitForMenu(page: Page) {
  await expect.poll(() => harness.findInlineMenuFrame(page) !== undefined).toBe(true);
  return harness.findInlineMenuFrame(page)!;
}

// The menu ignores clicks until it has been visible and unobscured for 300 ms.
async function waitForClickGuard(page: Page): Promise<void> {
  await page.waitForTimeout(500);
}

test.beforeEach(async () => {
  harness = await launchHarness({
    allLogin: [
      {
        vaultID: "vault-personal",
        vaultName: "Personal",
        itemID: "item-test-site",
        title: "Test site",
        username: "alex@example.com",
        password: "old password",
        url: "",
        oneTimePassword: setupKey,
      },
    ],
  });
  await harness.pairWithoutPopup();
  await harness.context.grantPermissions(["clipboard-read", "clipboard-write"], { origin: harness.site.origin });
});

test.afterEach(async () => {
  await harness?.close();
});

test("fills the one-time code from the inline menu into a two-factor field", async () => {
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/two-factor`);
  await page.locator("#app_totp").focus();
  const menu = await waitForMenu(page);
  await expect(menu.getByTestId("inline-code")).toHaveCount(1);
  await expect(menu.getByTestId("inline-code")).toContainText("One-time code · alex@example.com");
  await waitForClickGuard(page);
  const startedAt = Date.now();
  await menu.getByTestId("inline-code").click();
  await expect(page.locator("#app_totp")).toHaveValue(/^\d{6}$/);
  await expectCurrentCode(() => page.locator("#app_totp").inputValue(), startedAt);
  expect(harness.readState().allCodeRequest).toEqual([{ itemID: "item-test-site", url: `${harness.site.origin}/two-factor` }]);
});

test("types the code into six separate boxes", async () => {
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/two-factor-split`);
  await page.locator("#digit-0").focus();
  const menu = await waitForMenu(page);
  await expect(menu.getByTestId("inline-code")).toHaveCount(1);
  await waitForClickGuard(page);
  const startedAt = Date.now();
  await menu.getByTestId("inline-code").click();
  const readBoxes = () => page.evaluate(() => Array.from(document.querySelectorAll<HTMLInputElement>(".boxes input"), (box) => box.value).join(""));
  await expect.poll(readBoxes).toMatch(/^\d{6}$/);
  await expectCurrentCode(readBoxes, startedAt);
});

test("copies the code to the clipboard after filling the login", async () => {
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/login`);
  await page.locator("#password").focus();
  const menu = await waitForMenu(page);
  await expect(menu.getByTestId("inline-login")).toHaveCount(1);
  await waitForClickGuard(page);
  const startedAt = Date.now();
  await menu.getByTestId("inline-login").click();
  await expect(page.locator("#password")).toHaveValue("old password");
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toMatch(/^\d{6}$/);
  await expectCurrentCode(() => page.evaluate(() => navigator.clipboard.readText()), startedAt);
});

test("shows the code with its countdown in the popup, copies it, and fills it on a two-factor page", async () => {
  const page = await harness.context.newPage();
  await page.goto(`${harness.site.origin}/two-factor`);
  const tabId = await harness.getTabId(`${harness.site.origin}/two-factor`);
  const popup = await harness.context.newPage();
  const startedAt = Date.now();
  await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${tabId}`);
  await expect(popup.getByTestId("one-time-code-value")).toHaveText(/^\d{3} \d{3}$/);
  await expectCurrentCode(async () => (await popup.getByTestId("one-time-code-value").textContent())!.replace(" ", ""), startedAt);
  const secondsLeft = Number((await popup.getByTestId("one-time-code-seconds").textContent())!.replace("s", ""));
  expect(secondsLeft).toBeGreaterThan(0);
  expect(secondsLeft).toBeLessThanOrEqual(30);

  await popup.getByTestId("copy-one-time-code").click();
  await expect(popup.getByTestId("notice")).toHaveText("One-time code copied");
  await page.bringToFront();
  await expectCurrentCode(() => page.evaluate(() => navigator.clipboard.readText()), startedAt);

  await popup.bringToFront();
  const fillStartedAt = Date.now();
  await popup.getByTestId("fill").click();
  await expect(page.locator("#app_totp")).toHaveValue(/^\d{6}$/);
  await expectCurrentCode(() => page.locator("#app_totp").inputValue(), fillStartedAt);
});
