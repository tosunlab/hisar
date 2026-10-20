import { expect, test } from "@playwright/test";
import { featurePasskeyVerification } from "../src/shared/passkey";
import { launchHarness, type Harness } from "./harness";
import { clickInPrompt, readStatus, resetStatus, waitForPrompt } from "./passkey-prompt";
import { startPasskeySite, type PasskeySite } from "./passkey-site";

let harness: Harness;
let site: PasskeySite;

test.beforeEach(async () => {
  harness = await launchHarness();
});

test.afterEach(async () => {
  await site?.close();
  await harness?.close();
});

for (const policy of ["required", "preferred", "discouraged"] as const) {
  test(`carries ${policy} through the browser caller and RP signature verification`, async () => {
    site = await startPasskeySite(policy);
    await harness.pairWithoutPopup();
    const page = await harness.context.newPage();
    await page.goto(site.origin);
    await page.locator("#register").click();
    const prompt = await waitForPrompt(page);
    await clickInPrompt(page, prompt, "save-passkey");
    expect(await readStatus(page)).toMatchObject({ verified: true, userVerified: policy !== "discouraged" });
    await resetStatus(page);
    await page.locator("#sign-in").click();
    await clickInPrompt(page, await waitForPrompt(page), "passkey-choice");
    expect(await readStatus(page)).toMatchObject({ verified: true, userVerified: policy !== "discouraged" });
  });
}

test("refuses a required ceremony with an older host instead of silently downgrading", async () => {
  site = await startPasskeySite("required");
  harness.updateState((state) => {
    state.features = state.features?.filter((feature) => feature !== featurePasskeyVerification);
  });
  await harness.pairWithoutPopup();
  const page = await harness.context.newPage();
  await page.goto(site.origin);
  await page.locator("#register").click();
  expect(await readStatus(page)).toMatchObject({ error: "NotSupportedError" });
  expect(harness.readState().allPasskey ?? []).toHaveLength(0);
});
