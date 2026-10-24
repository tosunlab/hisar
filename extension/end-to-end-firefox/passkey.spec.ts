import { expect, test } from "@playwright/test";
import { hisarAAGUID } from "../tools/fake-passkey-authenticator.mjs";
import { startPasskeySite, type PasskeySite } from "../end-to-end/passkey-site";
import { By, launchFirefoxHarness, type FirefoxHarness } from "./harness";

let harness: FirefoxHarness;
let site: PasskeySite;

async function readStatus(): Promise<Record<string, unknown>> {
  const status = harness.driver.findElement(By.css("#status"));
  await expect.poll(() => status.getText()).not.toMatch(/^(ready|waiting for autofill)$/);
  return JSON.parse(await status.getText()) as Record<string, unknown>;
}

// openPrompt clicks the page's button and returns the passkey prompt's frame once its heading reads heading; the prompt
// ignores clicks until it has been visible and uncovered for 300 ms, so it waits a little longer.
async function openPrompt(buttonSelector: string, heading: string): Promise<string> {
  await harness.driver.executeScript("document.getElementById('status').textContent = 'ready'");
  await harness.driver.findElement(By.css(buttonSelector)).click();
  const prompt = await harness.waitForFrame("/passkey-prompt.html");
  await expect.poll(() => harness.readFrame(prompt, `document.querySelector('[data-testid="passkey-heading"]')?.textContent`)).toBe(heading);
  await harness.driver.sleep(500);
  return prompt;
}

test.beforeEach(async () => {
  harness = await launchFirefoxHarness();
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
  await harness.openTab(site.origin);
  await harness.waitForContentScripts(site.origin + "/");
  await expect.poll(() => harness.driver.findElement(By.css("#status")).getText()).toBe("ready");

  const savePrompt = await openPrompt("#register", "Save passkey");
  await expect.poll(() => harness.readFrame(savePrompt, `document.querySelectorAll('[data-testid="passkey-login-option"]').length`)).toBe(2);
  await harness.clickInFrame(savePrompt, '[data-testid="passkey-login-option"]', 1);
  await harness.clickInFrame(savePrompt, '[data-testid="save-passkey"]');
  const registration = await readStatus();
  expect(registration).toMatchObject({ verified: true, aaguid: hisarAAGUID, isPublicKeyCredential: true, isAttestationResponse: true });
  const stored = harness.readState().allPasskey ?? [];
  expect(stored).toHaveLength(1);
  expect(stored[0]).toMatchObject({ relyingPartyId: "localhost", itemID: "item-passkey-site", userName: "alex@example.com" });

  const signInPrompt = await openPrompt("#sign-in", "Sign in with a passkey");
  await harness.clickInFrame(signInPrompt, '[data-testid="passkey-choice"]');
  expect(await readStatus()).toMatchObject({ verified: true, credentialID: registration.credentialID, aaguid: hisarAAGUID, isAssertionResponse: true });
});
