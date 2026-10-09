import { describe, expect, it } from "vitest";
import { generateOneTimeCode } from "../../tools/fake-native-host.mjs";
import { routeMessage } from "./router";
import { InlineMenuService } from "./inline-menu";
import { createHarness, emptyProbe, expectHisarError, type HarnessOptions } from "./testing";

const setupKey = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ";
const pageUrl = "https://example.com/login";
const pageFrame = { tabId: 1, frameId: 0 };
const menuFrame = { tabId: 1, frameId: 9 };
const expectedCode = generateOneTimeCode(setupKey, 1_000_000).code;

async function createPairedHarness(options: HarnessOptions = {}) {
  const harness = createHarness(options);
  harness.state.allLogin[0].oneTimePassword = setupKey;
  harness.state.allLogin.push({ ...harness.state.allLogin[0], itemID: "item-without-code", title: "Without code", oneTimePassword: undefined });
  await harness.pair();
  const service = new InlineMenuService(harness.controller, harness.bridge);
  return { ...harness, service };
}

describe("one-time codes in the service worker", () => {
  it("copies the code to the clipboard after filling a login that has one", async () => {
    const harness = await createPairedHarness();
    expect(await harness.controller.fillLogin(1, "vault-personal", "item-example")).toEqual({ filledUsername: true, filledPassword: true });
    expect(harness.allClipboardText).toEqual([expectedCode]);
    expect(harness.allRequest.find((request) => request.type === "logins.code")).toMatchObject({ itemID: "item-example", url: pageUrl });
  });

  it("copies nothing for a login without a one-time password", async () => {
    const harness = await createPairedHarness();
    await harness.controller.fillLogin(1, "vault-personal", "item-without-code");
    expect(harness.allClipboardText).toEqual([]);
    expect(harness.getRequestTypes()).not.toContain("logins.code");
  });

  it("still fills when the clipboard is not available", async () => {
    const harness = await createPairedHarness({ isClipboardUnavailable: true });
    expect(await harness.controller.fillLogin(1, "vault-personal", "item-example")).toMatchObject({ filledPassword: true });
  });

  it("fills the code into a two-factor page that has no login form", async () => {
    const harness = await createPairedHarness({ allProbe: { 0: { ...emptyProbe, hasOneTimeCodeField: true } } });
    expect(await harness.controller.fillLogin(1, "vault-personal", "item-example")).toEqual({
      filledUsername: false,
      filledPassword: false,
      filledOneTimeCode: true,
    });
    expect(harness.allFrameMessage.at(-1)).toEqual({ frameId: 0, message: { type: "content.fillOneTimeCode", expectedOrigin: "https://example.com", code: expectedCode } });
    expect(harness.getRequestTypes()).not.toContain("logins.fill");
    await expectHisarError(harness.controller.fillLogin(1, "vault-personal", "item-without-code"), "not-found");
  });

  it("gives the popup the code of a login for the tab's site only", async () => {
    const harness = await createPairedHarness();
    expect(await harness.controller.getOneTimeCodeForTab(1, "vault-personal", "item-example")).toEqual(generateOneTimeCode(setupKey, 1_000_000));
    harness.state.allLogin[0].url = "https://example.org";
    await expectHisarError(harness.controller.getOneTimeCodeForTab(1, "vault-personal", "item-example"), "no-match");
  });

  it("offers only logins with a one-time password in a code field's menu and fills the code there", async () => {
    const harness = await createPairedHarness();
    const { sessionID } = harness.service.openSession(pageFrame, pageUrl, pageUrl, "one-time-code");
    const state = await harness.service.getState(menuFrame, sessionID);
    expect(state.allLogin.map((login) => login.itemID)).toEqual(["item-example"]);
    await harness.service.fillOneTimeCode(menuFrame, sessionID, "vault-personal", "item-example");
    expect(harness.allFrameMessage.at(-1)).toEqual({
      frameId: 0,
      message: { type: "content.fillOneTimeCode", expectedOrigin: "https://example.com", code: expectedCode, sessionID },
    });
    expect(harness.service.allSession.get(sessionID)).toBeUndefined();
  });

  it("does not fill a code while the menu is covered, or into a login field's menu", async () => {
    const covered = await createPairedHarness({ isMenuVisible: false });
    const first = covered.service.openSession(pageFrame, pageUrl, pageUrl, "one-time-code");
    await covered.service.getState(menuFrame, first.sessionID);
    await expectHisarError(covered.service.fillOneTimeCode(menuFrame, first.sessionID, "vault-personal", "item-example"), "invalid-request");
    expect(covered.getRequestTypes()).not.toContain("logins.code");

    const loginMenu = await createPairedHarness();
    const second = loginMenu.service.openSession(pageFrame, pageUrl, pageUrl, "login");
    await expectHisarError(loginMenu.service.fillOneTimeCode(menuFrame, second.sessionID, "vault-personal", "item-example"), "invalid-request");
  });

  it("copies the code after the inline menu fills a login", async () => {
    const harness = await createPairedHarness();
    const { sessionID } = harness.service.openSession(pageFrame, pageUrl, pageUrl, "login");
    await harness.service.getState(menuFrame, sessionID);
    await harness.service.fillLogin(menuFrame, sessionID, "vault-personal", "item-example");
    expect(harness.allClipboardText).toEqual([expectedCode]);
  });

  it("routes the popup and menu messages", async () => {
    const harness = await createPairedHarness();
    const context = { extensionID: "extension", extensionOrigin: "chrome-extension://extension/" };
    const services = { controller: harness.controller, inlineMenu: harness.service };
    const popupResponse = await routeMessage(
      services,
      { type: "popup.getOneTimeCode", tabId: 1, vaultID: "vault-personal", itemID: "item-example" } as never,
      { id: "extension", url: "chrome-extension://extension/popup.html" },
      context,
    );
    expect(popupResponse).toMatchObject({ ok: true, result: { code: expectedCode, period: 30 } });
    const fromPage = await routeMessage(
      services,
      { type: "popup.getOneTimeCode", tabId: 1, vaultID: "vault-personal", itemID: "item-example" } as never,
      { id: "extension", url: pageUrl, tab: { id: 1 }, frameId: 0 },
      context,
    );
    expect(fromPage).toMatchObject({ ok: false, error: { code: "invalid-request" } });
    const { sessionID } = harness.service.openSession(pageFrame, pageUrl, pageUrl, "one-time-code");
    const menuResponse = await routeMessage(
      services,
      { type: "inlineMenu.fillOneTimeCode", sessionID, vaultID: "vault-personal", itemID: "item-example" } as never,
      { id: "extension", url: "chrome-extension://dynamic/inline-menu.html", tab: { id: 1 }, frameId: 9 },
      context,
    );
    expect(menuResponse).toEqual({ ok: true, result: null });
  });
});
