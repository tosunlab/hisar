import { expect, test, type Page } from "@playwright/test";
import { hisarAAGUID } from "../tools/fake-passkey-authenticator.mjs";
import { launchHarness, type Harness } from "./harness";
import { clickInPrompt, findPromptFrame, readStatus, resetStatus, waitForPrompt } from "./passkey-prompt";
import { startPasskeySite, type PasskeySite } from "./passkey-site";

let harness: Harness;
let site: PasskeySite;

async function addVirtualAuthenticator(page: Page): Promise<void> {
  const session = await harness.context.newCDPSession(page);
  await session.send("WebAuthn.enable");
  await session.send("WebAuthn.addVirtualAuthenticator", {
    options: { protocol: "ctap2", transport: "internal", hasResidentKey: true, hasUserVerification: true, isUserVerified: true, automaticPresenceSimulation: true },
  });
}

async function openSite(): Promise<Page> {
  const page = await harness.context.newPage();
  await page.goto(site.origin);
  await expect(page.locator("#status")).toHaveText("ready");
  return page;
}

test.beforeEach(async () => {
  harness = await launchHarness();
  site = await startPasskeySite();
  harness.updateState((state) => {
    state.allLogin.push({ vaultID: "vault-personal", vaultName: "Personal", itemID: "item-passkey-site", title: "Passkey test site", username: "alex@example.com", password: "pw", url: site.origin });
  });
});

test.afterEach(async () => {
  await site?.close();
  await harness?.close();
});

test("registers a passkey in Hisar and signs in with it, verified by the relying party", async () => {
  await harness.pairWithoutPopup();
  const page = await openSite();
  await page.locator("#register").click();
  const savePrompt = await waitForPrompt(page);
  await expect(savePrompt.getByTestId("passkey-heading")).toHaveText("Save passkey");
  await expect(savePrompt.getByTestId("passkey-site")).toHaveText("localhost");
  await expect(savePrompt.getByTestId("passkey-user-name")).toHaveText("alex@example.com");
  await expect(savePrompt.getByTestId("passkey-login-option")).toHaveCount(2);
  expect(await page.evaluate(() => document.body.innerHTML.includes("Passkey test site</span>"))).toBe(false);
  await savePrompt.getByTestId("passkey-login-option").nth(1).click();
  await clickInPrompt(page, savePrompt, "save-passkey");
  const registration = await readStatus(page);
  expect(registration).toMatchObject({ verified: true, aaguid: hisarAAGUID, fmt: "none", userVerified: true, backedUp: true, isPublicKeyCredential: true, isAttestationResponse: true, toJSONMatches: true });
  await expect.poll(() => findPromptFrame(page)).toBeUndefined();
  const stored = harness.readState().allPasskey ?? [];
  expect(stored).toHaveLength(1);
  expect(stored[0]).toMatchObject({ relyingPartyId: "localhost", itemID: "item-passkey-site", userName: "alex@example.com" });

  await resetStatus(page);
  await page.locator("#sign-in").click();
  const signInPrompt = await waitForPrompt(page);
  await expect(signInPrompt.getByTestId("passkey-heading")).toHaveText("Sign in with a passkey");
  await expect(signInPrompt.getByTestId("passkey-choice")).toHaveCount(1);
  await expect(signInPrompt.getByTestId("passkey-choice")).toContainText("alex@example.com");
  await clickInPrompt(page, signInPrompt, "passkey-choice");
  expect(await readStatus(page)).toMatchObject({ verified: true, credentialID: registration.credentialID, aaguid: hisarAAGUID, isAssertionResponse: true });

  await resetStatus(page);
  await page.locator("#register-excluded").click();
  const excludedPrompt = await waitForPrompt(page);
  await expect(excludedPrompt.getByTestId("passkey-excluded")).toBeVisible();
  await clickInPrompt(page, excludedPrompt, "confirm-excluded");
  expect(await readStatus(page)).toMatchObject({ error: "InvalidStateError" });
  await page.close();
});

test("offers the passkey in the field for conditional mediation", async () => {
  await harness.pairWithoutPopup();
  const page = await openSite();
  await page.locator("#register").click();
  const savePrompt = await waitForPrompt(page);
  await savePrompt.getByTestId("passkey-login-option").nth(0).click();
  await clickInPrompt(page, savePrompt, "save-passkey");
  expect(await readStatus(page)).toMatchObject({ verified: true });
  const stored = harness.readState();
  expect(stored.allLogin.filter((login) => login.title === "localhost")).toHaveLength(1);

  await resetStatus(page);
  await page.locator("#conditional").click();
  await expect(page.locator("#status")).toHaveText("waiting for autofill");
  await page.waitForTimeout(300);
  expect(findPromptFrame(page)).toBeUndefined();
  await page.locator("#username").focus();
  const prompt = await waitForPrompt(page);
  await expect(prompt.getByTestId("passkey-choice")).toHaveCount(1);
  await clickInPrompt(page, prompt, "passkey-choice");
  expect(await readStatus(page)).toMatchObject({ verified: true, mediation: "conditional", aaguid: hisarAAGUID });
  await page.close();
});

test("falls back to the browser on request and when Hisar is not paired", async () => {
  await harness.pairWithoutPopup();
  const page = await openSite();
  await addVirtualAuthenticator(page);
  await page.locator("#register").click();
  const prompt = await waitForPrompt(page);
  await clickInPrompt(page, prompt, "use-other-device");
  const viaBrowser = await readStatus(page);
  expect(viaBrowser).toMatchObject({ verified: true });
  expect(viaBrowser.aaguid).not.toBe(hisarAAGUID);
  expect(harness.readState().allPasskey ?? []).toHaveLength(0);

  await resetStatus(page);
  await page.locator("#sign-in").click();
  expect(await readStatus(page)).toMatchObject({ verified: true, credentialID: viaBrowser.credentialID });
  expect(findPromptFrame(page)).toBeUndefined();
  await page.close();

  harness.updateState((state) => (state.allPairing = []));
  await harness.evaluateInExtension(() => chrome.storage.local.remove("token"), undefined);
  const unpaired = await openSite();
  await addVirtualAuthenticator(unpaired);
  await unpaired.locator("#register").click();
  const unpairedRegistration = await readStatus(unpaired);
  expect(unpairedRegistration).toMatchObject({ verified: true });
  expect(unpairedRegistration.aaguid).not.toBe(hisarAAGUID);
  expect(findPromptFrame(unpaired)).toBeUndefined();
  await unpaired.close();
});

test("asks to unlock Hisar and falls back when the prompt is closed while locked", async () => {
  await harness.pairWithoutPopup();
  harness.updateState((state) => {
    state.state = "locked";
    state.unlockDelayMilliseconds = 300;
  });
  const page = await openSite();
  await page.locator("#register").click();
  const prompt = await waitForPrompt(page);
  await expect(prompt.getByTestId("passkey-heading")).toHaveText("Hisar is locked");
  await prompt.getByTestId("unlock-hisar").click();
  await expect(prompt.getByTestId("save-passkey")).toBeVisible({ timeout: 10_000 });
  await clickInPrompt(page, prompt, "passkey-close");
  expect(await readStatus(page)).toMatchObject({ error: "NotAllowedError" });

  harness.updateState((state) => (state.state = "locked"));
  await addVirtualAuthenticator(page);
  await resetStatus(page);
  await page.locator("#register").click();
  const lockedPrompt = await waitForPrompt(page);
  await expect(lockedPrompt.getByTestId("unlock-hisar")).toBeVisible();
  await clickInPrompt(page, lockedPrompt, "passkey-close");
  const fallback = await readStatus(page);
  expect(fallback).toMatchObject({ verified: true });
  expect(fallback.aaguid).not.toBe(hisarAAGUID);
  await page.close();
});
