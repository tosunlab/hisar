import { describe, expect, it } from "vitest";
import { InlineMenuService } from "./inline-menu";
import { InlineMenuSessionStore } from "./inline-menu-sessions";
import { createHarness, expectHisarError, type HarnessOptions } from "./testing";

const pageUrl = "https://example.com/login";
const pageFrame = { tabId: 1, frameId: 0 };
const menuFrame = { tabId: 1, frameId: 9 };

function createService(options: HarnessOptions = {}) {
  const harness = createHarness(options);
  const service = new InlineMenuService(harness.controller, harness.bridge);
  return { ...harness, service };
}

describe("InlineMenuSessionStore", () => {
  it("binds a session to the first menu frame that claims it", () => {
    const store = new InlineMenuSessionStore(() => 0);
    store.create({ sessionID: "s", tabId: 1, frameId: 0, frameUrl: pageUrl, purpose: "login" });
    expect(store.claim("s", 1, 9)?.menuFrameId).toBe(9);
    expect(store.claim("s", 1, 9)).toBeDefined();
    expect(store.claim("s", 1, 10)).toBeUndefined();
    expect(store.claim("s", 2, 9)).toBeUndefined();
  });

  it("keeps one session per frame and expires old ones", () => {
    let now = 0;
    const store = new InlineMenuSessionStore(() => now, 1000);
    store.create({ sessionID: "a", tabId: 1, frameId: 0, frameUrl: pageUrl, purpose: "login" });
    store.create({ sessionID: "b", tabId: 1, frameId: 0, frameUrl: pageUrl, purpose: "login" });
    expect(store.get("a")).toBeUndefined();
    now = 1001;
    expect(store.get("b")).toBeUndefined();
  });
});

describe("InlineMenuService", () => {
  it("opens sessions only for web frames with the top frame's origin, with a menu URL from the service worker", () => {
    const { service } = createService();
    const opened = service.openSession(pageFrame, pageUrl, "https://example.com/", "login");
    expect(opened.menuUrl).toBe(`chrome-extension://dynamic-id/inline-menu.html#${opened.sessionID}`);
    expect(() => service.openSession({ tabId: 1, frameId: 3 }, "https://example.org/frame", "https://example.com/", "login")).toThrow(
      "Hisar does not fill this frame",
    );
    expect(() => service.openSession(pageFrame, "file:///tmp/login.html", "file:///tmp/login.html", "login")).toThrow();
    expect(() => service.openSession(pageFrame, pageUrl, undefined, "login")).toThrow();
  });

  it("reports not-paired, locked and desktop-unavailable states", async () => {
    const notPaired = createService();
    const first = notPaired.service.openSession(pageFrame, pageUrl, pageUrl, "login");
    expect(await notPaired.service.getState(menuFrame, first.sessionID)).toMatchObject({ view: "not-paired", host: "example.com" });

    const locked = createService({ state: { state: "locked" } });
    await locked.pair();
    const second = locked.service.openSession(pageFrame, pageUrl, pageUrl, "login");
    expect((await locked.service.getState(menuFrame, second.sessionID)).view).toBe("locked");

    const unavailable = createService({ isHostMissing: true });
    const third = unavailable.service.openSession(pageFrame, pageUrl, pageUrl, "login");
    expect((await unavailable.service.getState(menuFrame, third.sessionID)).view).toBe("desktop-unavailable");
  });

  it("reports update-required with the requirement, and lists no logins", async () => {
    const updateRequired = { target: "extension" as const, currentVersion: "0.1.0", minimumVersion: "0.3.0", updateURL: "https://chromewebstore.google.com/detail/x" };
    const outdated = createService();
    await outdated.pair();
    outdated.state.updateRequired = updateRequired;
    await outdated.controller.getPopupState(true);
    const opened = outdated.service.openSession(pageFrame, pageUrl, pageUrl, "login");
    expect(await outdated.service.getState(menuFrame, opened.sessionID)).toMatchObject({ view: "update-required", updateRequired, allLogin: [] });
  });

  it("lists at most six matching logins for the frame and says when there are more", async () => {
    const harness = createService();
    for (let index = 0; index < 7; index += 1) {
      harness.state.allLogin.push({ ...harness.state.allLogin[0], itemID: `extra-${index}`, title: `Extra ${index}` });
    }
    await harness.pair();
    const { sessionID } = harness.service.openSession(pageFrame, pageUrl, pageUrl, "login");
    const state = await harness.service.getState(menuFrame, sessionID);
    expect(state.view).toBe("logins");
    expect(state.allLogin).toHaveLength(6);
    expect(state.hasMoreLogin).toBe(true);
    expect(harness.allRequest.find((request) => request.type === "logins.match")).toMatchObject({ url: pageUrl });
  });

  it("suggests a generated password for new-password fields", async () => {
    const harness = createService();
    await harness.pair();
    const { sessionID } = harness.service.openSession(pageFrame, pageUrl, pageUrl, "new-password");
    const state = await harness.service.getState(menuFrame, sessionID);
    expect(state.suggestedPassword).toMatch(/^.{24}$/);
    await harness.service.useSuggestedPassword(menuFrame, sessionID);
    expect(harness.allFrameMessage.at(-1)).toEqual({
      frameId: 0,
      message: { type: "content.fillPassword", expectedOrigin: "https://example.com", password: state.suggestedPassword, sessionID },
    });
  });

  it("refuses requests from a frame other than the one that claimed the session", async () => {
    const harness = createService();
    await harness.pair();
    const { sessionID } = harness.service.openSession(pageFrame, pageUrl, pageUrl, "login");
    await harness.service.getState(menuFrame, sessionID);
    await expectHisarError(harness.service.getState({ tabId: 1, frameId: 10 }, sessionID), "invalid-request");
    await expectHisarError(harness.service.fillLogin({ tabId: 1, frameId: 10 }, sessionID, "vault-personal", "item-example"), "invalid-request");
  });

  it("fills the chosen login through the anchored field after checking that the menu is visible", async () => {
    const harness = createService();
    await harness.pair();
    const { sessionID } = harness.service.openSession(pageFrame, pageUrl, pageUrl, "login");
    await harness.service.fillLogin(menuFrame, sessionID, "vault-personal", "item-example");
    const allType = harness.allFrameMessage.map((entry) => entry.message.type);
    expect(allType.indexOf("content.isInlineMenuVisible")).toBeLessThan(allType.indexOf("content.fillLogin"));
    expect(harness.allFrameMessage.find((entry) => entry.message.type === "content.fillLogin")?.message).toEqual({
      type: "content.fillLogin",
      expectedOrigin: "https://example.com",
      username: "alex@example.com",
      password: "correct horse battery staple",
      sessionID,
    });
    expect(harness.allRequest.find((request) => request.type === "logins.fill")).toMatchObject({ url: pageUrl });
    await expectHisarError(harness.service.getState(menuFrame, sessionID), "invalid-request");
  });

  it("does not ask the desktop for the password when the menu was not visible long enough", async () => {
    const harness = createService({ isMenuVisible: false });
    await harness.pair();
    const { sessionID } = harness.service.openSession(pageFrame, pageUrl, pageUrl, "login");
    await expectHisarError(harness.service.fillLogin(menuFrame, sessionID, "vault-personal", "item-example"), "invalid-request");
    expect(harness.getRequestTypes()).not.toContain("logins.fill");
    expect(harness.allFrameMessage.some((entry) => entry.message.type === "content.fillLogin")).toBe(false);
  });

  it("lists addresses on an address field and fills the chosen one after checking that the menu is visible", async () => {
    const harness = createService();
    await harness.pair();
    const { sessionID } = harness.service.openSession(pageFrame, pageUrl, pageUrl, "address");
    const state = await harness.service.getState(menuFrame, sessionID);
    expect(state).toMatchObject({ view: "logins", purpose: "address", allLogin: [], hasMoreAddress: false });
    expect(state.allAddress).toEqual([{ vaultID: "vault-personal", itemID: "item-home", title: "Home", summary: "Springfield, US", source: "address", vaultName: "Personal" }]);
    expect(harness.getRequestTypes()).not.toContain("addresses.fill");

    await harness.service.fillAddress(menuFrame, sessionID, "vault-personal", "item-home");
    const allType = harness.allFrameMessage.map((entry) => entry.message.type);
    expect(allType.indexOf("content.isInlineMenuVisible")).toBeLessThan(allType.indexOf("content.fillAddress"));
    expect(harness.allFrameMessage.find((entry) => entry.message.type === "content.fillAddress")?.message).toEqual({
      type: "content.fillAddress",
      expectedOrigin: "https://example.com",
      address: { street: "1 Main St\nApt 4", city: "Springfield", region: "IL", postalCode: "62704", country: "US" },
      sessionID,
    });
    await expectHisarError(harness.service.getState(menuFrame, sessionID), "invalid-request");
  });

  it("does not ask for an address when the menu was not visible, or on a login field", async () => {
    const hidden = createService({ isMenuVisible: false });
    await hidden.pair();
    const first = hidden.service.openSession(pageFrame, pageUrl, pageUrl, "address");
    await expectHisarError(hidden.service.fillAddress(menuFrame, first.sessionID, "vault-personal", "item-home"), "invalid-request");
    const login = hidden.service.openSession(pageFrame, pageUrl, pageUrl, "login");
    await expectHisarError(hidden.service.fillAddress(menuFrame, login.sessionID, "vault-personal", "item-home"), "invalid-request");
    expect(hidden.getRequestTypes()).not.toContain("addresses.fill");
  });

  it("opens no address menu with an app that does not fill addresses", async () => {
    const harness = createService({ state: { features: [] } });
    await harness.pair();
    expect(() => harness.service.openSession(pageFrame, pageUrl, pageUrl, "address")).toThrow("Update the Hisar app to fill addresses");
    expect(harness.area.data.appFeatures).toEqual([]);
  });

  it("relays keys from the page's frame to the menu, and only from that frame", async () => {
    const harness = createService();
    const { sessionID } = harness.service.openSession(pageFrame, pageUrl, pageUrl, "login");
    await harness.service.pressKey(pageFrame, sessionID, "ArrowDown");
    await harness.service.pressKey({ tabId: 1, frameId: 5 }, sessionID, "Enter");
    await harness.service.pressKey({ tabId: 2, frameId: 0 }, sessionID, "Enter");
    expect(harness.allBroadcast).toEqual([{ target: "inline-menu", type: "inlineMenu.pressKey", sessionID, key: "ArrowDown" }]);
  });

  it("unlocks, opens the popup, resizes and closes", async () => {
    const harness = createService({ state: { state: "locked" } });
    await harness.pair();
    const { sessionID } = harness.service.openSession(pageFrame, pageUrl, pageUrl, "login");
    await harness.service.unlock(menuFrame, sessionID);
    expect(harness.getRequestTypes()).toContain("unlock");
    await harness.service.resize(menuFrame, sessionID, 180);
    expect(harness.allFrameMessage.at(-1)?.message).toEqual({ type: "content.resizeInlineMenu", sessionID, height: 180 });
    await harness.service.openPopup(menuFrame, sessionID);
    expect(harness.allOpenedPopupTab).toEqual([1]);
    await harness.service.close(menuFrame, sessionID);
    expect(harness.allFrameMessage.at(-1)?.message).toEqual({ type: "content.dismissInlineMenu", sessionID });
    await expectHisarError(harness.service.unlock(menuFrame, sessionID), "invalid-request");
  });

  it("forgets sessions when the content script closes them or the tab closes", () => {
    const harness = createService();
    const first = harness.service.openSession(pageFrame, pageUrl, pageUrl, "login");
    harness.service.closeSession({ tabId: 1, frameId: 4 }, first.sessionID);
    expect(harness.service.allSession.get(first.sessionID)).toBeDefined();
    harness.service.closeSession(pageFrame, first.sessionID);
    expect(harness.service.allSession.get(first.sessionID)).toBeUndefined();
    const second = harness.service.openSession(pageFrame, pageUrl, pageUrl, "login");
    harness.service.handleTabRemoved(1);
    expect(harness.service.allSession.get(second.sessionID)).toBeUndefined();
  });
});
