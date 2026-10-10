import { statSync } from "node:fs";
import { expect, test, type Frame, type Page } from "@playwright/test";
import { hisarAAGUID } from "../tools/fake-passkey-authenticator.mjs";
import { launchHarness, type Harness } from "./harness";
import { startPasskeySite, type PasskeySite } from "./passkey-site";

let harness: Harness;
let site: PasskeySite;

function findPromptFrame(page: Page): Frame | undefined {
  return page.frames().find((frame) => frame.url().includes("/passkey-prompt.html"));
}

async function waitForPrompt(page: Page): Promise<Frame> {
  await expect.poll(() => findPromptFrame(page) !== undefined).toBe(true);
  return findPromptFrame(page)!;
}

async function resetStatus(page: Page): Promise<void> {
  await page.locator("#status").evaluate((element) => (element.textContent = "ready"));
}

async function readStatus(page: Page): Promise<Record<string, unknown>> {
  await expect(page.locator("#status")).not.toHaveText("ready");
  return JSON.parse((await page.locator("#status").textContent()) ?? "{}") as Record<string, unknown>;
}

// The prompt frame goes away on these clicks, and Playwright's click never settles when its target frame is removed.
async function clickInPrompt(page: Page, prompt: Frame, testID: string): Promise<void> {
  await page.waitForTimeout(500);
  const box = await prompt.getByTestId(testID).first().boundingBox();
  if (!box) throw new Error(`${testID} is not visible`);
  void page.mouse.click(box.x + box.width / 2, box.y + box.height / 2).catch(() => undefined);
}

async function openSite(): Promise<Page> {
  const page = await harness.context.newPage();
  await page.goto(site.origin);
  await expect(page.locator("#status")).toHaveText("ready");
  return page;
}

async function registerWithPRF(page: Page): Promise<Record<string, unknown>> {
  await resetStatus(page);
  await page.locator("#register-prf").click();
  const prompt = await waitForPrompt(page);
  await clickInPrompt(page, prompt, "save-passkey");
  const registration = await readStatus(page);
  expect(registration).toMatchObject({ verified: true, aaguid: hisarAAGUID });
  await expect.poll(() => findPromptFrame(page)).toBeUndefined();
  return registration;
}

async function signIn(page: Page, buttonID: string): Promise<Record<string, unknown>> {
  await resetStatus(page);
  await page.locator(buttonID).click();
  const prompt = await waitForPrompt(page);
  await clickInPrompt(page, prompt, "passkey-choice");
  const result = await readStatus(page);
  await expect.poll(() => findPromptFrame(page)).toBeUndefined();
  return result;
}

test.beforeEach(async () => {
  harness = await launchHarness();
  site = await startPasskeySite();
});

test.afterEach(async () => {
  await site?.close();
  await harness?.close();
});

test("evaluates the PRF extension when creating and signing in", async () => {
  await harness.pairWithoutPopup();
  const page = await openSite();
  const registration = await registerWithPRF(page);
  const createdPRF = registration.prf as { enabled: boolean; first: string; isArrayBuffer: boolean };
  expect(createdPRF).toMatchObject({ enabled: true, isArrayBuffer: true });
  expect(createdPRF.first).toMatch(/^[A-Za-z0-9_-]{43}$/);

  const first = await signIn(page, "#sign-in-prf-a");
  expect(first).toMatchObject({ verified: true, credentialID: registration.credentialID, prf: { first: createdPRF.first, isArrayBuffer: true } });
  const second = await signIn(page, "#sign-in-prf-a");
  expect(second.prf).toEqual(first.prf);
  const otherSalt = await signIn(page, "#sign-in-prf-b");
  expect(otherSalt).toMatchObject({ verified: true, prf: { isArrayBuffer: true } });
  expect((otherSalt.prf as { first: string }).first).not.toBe(createdPRF.first);
  await page.close();
});

test("removes a passkey the site signals as unknown and no longer offers it", async () => {
  await harness.pairWithoutPopup();
  const page = await openSite();
  const registration = await registerWithPRF(page);
  expect(harness.readState().allPasskey).toHaveLength(1);

  await resetStatus(page);
  await page.locator("#signal-foreign").click();
  expect(await readStatus(page)).toMatchObject({ error: "SecurityError" });
  expect(harness.readState().allPasskey).toHaveLength(1);

  await resetStatus(page);
  await page.locator("#signal-unknown").click();
  expect(await readStatus(page)).toEqual({ signaled: true, result: "undefined" });
  await expect.poll(() => harness.readState().allPasskey?.length).toBe(0);
  expect(harness.readState().allLogin.some((login) => login.title === "localhost")).toBe(true);

  await resetStatus(page);
  await page.locator("#sign-in").click();
  await page.waitForTimeout(1500);
  expect(findPromptFrame(page)).toBeUndefined();
  expect(await page.locator("#status").textContent()).not.toContain(String(registration.credentialID));
  await page.close();
});

test("reports Hisar's client capabilities without starting the native host", async () => {
  await harness.pairWithoutPopup();
  const page = await openSite();
  const stateWrittenAt = statSync(harness.stateFile).mtimeMs;
  await resetStatus(page);
  await page.locator("#capabilities").click();
  expect(await readStatus(page)).toMatchObject({ signalUnknownCredential: true, signalAllAcceptedCredentials: true, signalCurrentUserDetails: true });
  await page.waitForTimeout(500);
  expect(statSync(harness.stateFile).mtimeMs).toBe(stateWrittenAt);

  await registerWithPRF(page);
  await resetStatus(page);
  await page.locator("#capabilities").click();
  expect(await readStatus(page)).toMatchObject({
    passkeyPlatformAuthenticator: true,
    userVerifyingPlatformAuthenticator: true,
    conditionalGet: true,
    "extension:prf": true,
    signalUnknownCredential: true,
  });
  await page.close();
});
