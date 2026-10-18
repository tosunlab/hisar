import { describe, expect, it } from "vitest";
import { createHarness, emptyProbe, expectHisarError } from "./testing";

describe("popup state", () => {
  it("is desktop-unavailable when the native host cannot be started", async () => {
    const harness = createHarness({ isHostMissing: true });
    expect(await harness.controller.getPopupState()).toEqual({
      view: "desktop-unavailable",
      errorMessage: "Specified native messaging host not found.",
    });
  });

  it("is desktop-unavailable when the host reports that the app is not running", async () => {
    const harness = createHarness({ state: { isDesktopUnavailable: true } });
    expect((await harness.controller.getPopupState()).view).toBe("desktop-unavailable");
  });

  it("is not-paired before pairing and signed-out when the desktop is signed out", async () => {
    expect((await createHarness().controller.getPopupState()).view).toBe("not-paired");
    expect((await createHarness({ state: { state: "signed-out" } }).controller.getPopupState()).view).toBe("signed-out");
  });

  it("follows the desktop state after pairing", async () => {
    const harness = createHarness({ state: { isOffline: true } });
    await harness.pair();
    expect(await harness.controller.getPopupState()).toEqual({ view: "unlocked", email: "alex@example.com", isOffline: true, isAddressFillAvailable: true, isVaultAvailable: true, isPasswordUnlockAvailable: true });
    harness.state.state = "locked";
    expect((await harness.controller.getPopupState()).view).toBe("locked");
  });

  it("drops the token and asks to pair again when the desktop forgot this browser", async () => {
    const harness = createHarness();
    await harness.pair();
    harness.state.allPairing = [];
    expect((await harness.controller.getPopupState()).view).toBe("not-paired");
    expect(await harness.storage.getToken()).toBeUndefined();
  });

  it("drops the token when hello says the browser is not paired", async () => {
    const harness = createHarness();
    await harness.pair();
    harness.state.allPairing = [];
    expect((await harness.controller.getPopupState(true)).view).toBe("not-paired");
    expect(await harness.storage.getToken()).toBeUndefined();
  });
});

describe("pairing", () => {
  it("sends the instance ID and browser name and stores the token", async () => {
    const harness = createHarness();
    await harness.pair();
    const token = await harness.storage.getToken();
    expect(token).toMatch(/^[0-9a-f]{64}$/);
    expect(harness.allRequest.find((request) => request.type === "pair.start")).toMatchObject({ instanceID: "instance-1", browser: "Chrome" });
    expect(harness.state.allPairing.map((pairing) => pairing.instanceID)).toEqual(["instance-1"]);
  });

  it("accepts a code typed with a space", async () => {
    const harness = createHarness();
    await harness.controller.startPairing();
    await harness.controller.finishPairing("123 456");
    expect(await harness.storage.getToken()).toBeDefined();
  });

  it("reports pairing-failed for a wrong code", async () => {
    const harness = createHarness();
    await harness.controller.startPairing();
    await expectHisarError(harness.controller.finishPairing("000000"), "pairing-failed");
    expect(await harness.storage.getToken()).toBeUndefined();
  });

  it("rejects a malformed code without asking the desktop", async () => {
    const harness = createHarness();
    await expectHisarError(harness.controller.finishPairing("12345"), "invalid-request");
    expect(harness.getRequestTypes()).not.toContain("pair.finish");
  });
});

describe("unlock", () => {
  it("asks the desktop to unlock; the state changes once the user unlocks", async () => {
    const harness = createHarness({ state: { state: "locked" } });
    await harness.pair();
    await harness.controller.unlock();
    expect((await harness.controller.getPopupState()).view).toBe("locked");
    harness.advanceTime(1000);
    expect((await harness.controller.getPopupState()).view).toBe("unlocked");
  });
});

describe("logins", () => {
  it("matches logins for the tab's URL", async () => {
    const harness = createHarness();
    await harness.pair();
    const result = await harness.controller.matchLogins(1);
    expect(result.url).toBe("https://example.com/login");
    expect(result.allLogin.map((login) => login.title)).toEqual(["Example"]);
  });

  it("does not ask the desktop for pages that are not web pages", async () => {
    const harness = createHarness({ tabUrl: "chrome://settings/" });
    await harness.pair();
    expect((await harness.controller.matchLogins(1)).allLogin).toEqual([]);
    expect(harness.getRequestTypes()).not.toContain("logins.match");
  });

  it("reports locked when the desktop is locked", async () => {
    const harness = createHarness({ state: { state: "locked" } });
    await harness.pair();
    await expectHisarError(harness.controller.matchLogins(1), "locked");
  });

  it("gives the popup the password for copying, checked against the tab's URL", async () => {
    const harness = createHarness();
    await harness.pair();
    expect(await harness.controller.getPasswordForTab(1, "vault-personal", "item-example")).toBe("correct horse battery staple");
    expect(harness.allRequest.find((request) => request.type === "logins.fill")).toMatchObject({ url: "https://example.com/login" });
  });

  it("generates passwords with the requested options", async () => {
    const harness = createHarness();
    await harness.pair();
    const password = await harness.controller.generatePassword({ length: 32, digits: false, symbols: false });
    expect(password).toMatch(/^[a-zA-Z]{32}$/);
  });
});

describe("fill and the frame rule", () => {
  const top = { frameId: 0, url: "https://example.com/login", origin: "https://example.com", allAncestorOrigin: [] };
  const sameOriginFrame = { frameId: 4, url: "https://example.com/embedded-login", origin: "https://example.com", allAncestorOrigin: ["https://example.com"] };
  const crossOriginFrame = { frameId: 7, url: "https://example.org/login", origin: "https://example.org", allAncestorOrigin: ["https://example.com"] };

  it("fills the top frame and asks the desktop with its URL", async () => {
    const harness = createHarness();
    await harness.pair();
    await harness.controller.fillLogin(1, "vault-personal", "item-example");
    expect(harness.allRequest.find((request) => request.type === "logins.fill")).toMatchObject({ url: "https://example.com/login" });
    const fillMessage = harness.allFrameMessage.find((entry) => entry.message.type === "content.fillLogin");
    expect(fillMessage).toEqual({
      frameId: 0,
      message: { type: "content.fillLogin", expectedOrigin: "https://example.com", username: "alex@example.com", password: "correct horse battery staple" },
    });
  });

  it("fills a same-origin frame with the focused login field and never touches cross-origin frames", async () => {
    const harness = createHarness({
      allFrame: [top, sameOriginFrame, crossOriginFrame],
      allProbe: {
        0: emptyProbe,
        4: { ...emptyProbe, hasLoginForm: true, hasFocusedField: true, hasFocusedLoginField: true },
        7: { ...emptyProbe, hasLoginForm: true, hasFocusedLoginField: true },
      },
    });
    await harness.pair();
    await harness.controller.fillLogin(1, "vault-personal", "item-example");
    expect(harness.allRequest.find((request) => request.type === "logins.fill")).toMatchObject({ url: "https://example.com/embedded-login" });
    expect(harness.allFrameMessage.filter((entry) => entry.message.type === "content.fillLogin").map((entry) => entry.frameId)).toEqual([4]);
    expect(harness.allFrameMessage.some((entry) => entry.frameId === 7)).toBe(false);
  });

  it("refuses when only a cross-origin frame has a login form", async () => {
    const harness = createHarness({
      allFrame: [top, crossOriginFrame],
      allProbe: { 0: emptyProbe, 7: { ...emptyProbe, hasLoginForm: true } },
    });
    await harness.pair();
    await expectHisarError(harness.controller.fillLogin(1, "vault-personal", "item-example"), "not-found");
    expect(harness.getRequestTypes()).not.toContain("logins.fill");
    expect(harness.allFrameMessage.some((entry) => entry.frameId === 7)).toBe(false);
  });

  it("refuses to fill on pages that are not web pages", async () => {
    const harness = createHarness({ tabUrl: "chrome://newtab/" });
    await harness.pair();
    await expectHisarError(harness.controller.fillLogin(1, "vault-personal", "item-example"), "not-found");
  });

  it("passes on the desktop's no-match when the item is for another site", async () => {
    const harness = createHarness({ tabUrl: "https://example.net/login" });
    await harness.pair();
    await expectHisarError(harness.controller.fillLogin(1, "vault-personal", "item-example"), "no-match");
    expect(harness.allFrameMessage.some((entry) => entry.message.type === "content.fillLogin")).toBe(false);
  });

  it("injects the content script into tabs that were open before the extension was installed", async () => {
    const harness = createHarness({ allProbe: { 0: "no-content-script" } });
    await harness.pair();
    await harness.controller.fillLogin(1, "vault-personal", "item-example");
    expect(harness.getInjectionCount()).toBe(1);
  });

  it("fills the first match for the keyboard shortcut", async () => {
    const harness = createHarness();
    harness.state.allLogin.push({ ...harness.state.allLogin[0], itemID: "item-aardvark", title: "Aardvark", username: "first" });
    await harness.pair();
    await harness.controller.fillFirstLogin(1);
    const fillMessage = harness.allFrameMessage.find((entry) => entry.message.type === "content.fillLogin")?.message;
    expect(fillMessage).toMatchObject({ username: "first" });
  });

  it("reports no-match for the keyboard shortcut when nothing matches", async () => {
    const harness = createHarness({ tabUrl: "https://example.net/" });
    await harness.pair();
    await expectHisarError(harness.controller.fillFirstLogin(1), "no-match");
  });

  it("fills a generated password into the focused field of an allowed frame", async () => {
    const harness = createHarness({
      allFrame: [top, sameOriginFrame],
      allProbe: { 0: emptyProbe, 4: { ...emptyProbe, hasFocusedField: true } },
    });
    await harness.pair();
    await harness.controller.fillPassword(1, "generated");
    expect(harness.allFrameMessage.find((entry) => entry.message.type === "content.fillPassword")).toEqual({
      frameId: 4,
      message: { type: "content.fillPassword", expectedOrigin: "https://example.com", password: "generated" },
    });
  });

  it("asks to focus a field when none has the focus", async () => {
    const harness = createHarness();
    await harness.pair();
    await expectHisarError(harness.controller.fillPassword(1, "generated"), "not-found");
  });
});

describe("addresses", () => {
  const top = { frameId: 0, url: "https://shop.example.com/checkout", origin: "https://shop.example.com", allAncestorOrigin: [] };
  const sameOriginFrame = { frameId: 4, url: "https://shop.example.com/address", origin: "https://shop.example.com", allAncestorOrigin: ["https://shop.example.com"] };
  const crossOriginFrame = { frameId: 7, url: "https://pay.example.net/address", origin: "https://pay.example.net", allAncestorOrigin: ["https://shop.example.com"] };

  it("lists addresses and fills the frame with the focused address field", async () => {
    const harness = createHarness({
      tabUrl: top.url,
      allFrame: [top, sameOriginFrame, crossOriginFrame],
      allProbe: { 0: { ...emptyProbe, hasAddressForm: true }, 4: { ...emptyProbe, hasAddressForm: true, hasFocusedAddressField: true }, 7: { ...emptyProbe, hasAddressForm: true, hasFocusedAddressField: true } },
    });
    await harness.pair();
    expect((await harness.controller.listAddresses()).map((address) => address.title)).toEqual(["Home"]);
    expect(await harness.controller.fillAddress(1, "vault-personal", "item-home")).toBe(true);
    expect(harness.allFrameMessage.filter((entry) => entry.message.type === "content.fillAddress")).toEqual([
      {
        frameId: 4,
        message: {
          type: "content.fillAddress",
          expectedOrigin: "https://shop.example.com",
          address: { street: "1 Main St\nApt 4", city: "Springfield", region: "IL", postalCode: "62704", country: "US" },
        },
      },
    ]);
    expect(harness.allFrameMessage.some((entry) => entry.frameId === 7 && entry.message.type !== "content.probe")).toBe(false);
  });

  it("does not ask Hisar for the address when the page has no address form", async () => {
    const harness = createHarness({ allProbe: { 0: { ...emptyProbe, hasLoginForm: true } } });
    await harness.pair();
    await expectHisarError(harness.controller.fillAddress(1, "vault-personal", "item-home"), "not-found");
    expect(harness.getRequestTypes()).not.toContain("addresses.fill");
  });

  it("offers no addresses with an app that does not fill them", async () => {
    const harness = createHarness({ state: { features: [] }, allProbe: { 0: { ...emptyProbe, hasAddressForm: true } } });
    await harness.pair();
    expect((await harness.controller.getPopupState()).isAddressFillAvailable).toBe(false);
    await expectHisarError(harness.controller.listAddresses(), "invalid-request");
    await expectHisarError(harness.controller.fillAddress(1, "vault-personal", "item-home"), "invalid-request");
    expect(harness.getRequestTypes().filter((type) => String(type).startsWith("addresses."))).toEqual([]);
  });
});

describe("unlocking with the master password", () => {
  it("unlocks with the right password and says when it is wrong", async () => {
    const harness = createHarness({ state: { state: "locked" } });
    await harness.pair();
    expect(await harness.controller.getPopupState()).toMatchObject({ view: "locked", isPasswordUnlockAvailable: true });
    await expectHisarError(harness.controller.unlockWithPassword("guess"), "wrong-password");
    await expectHisarError(harness.controller.unlockWithPassword(""), "invalid-request");
    await harness.controller.unlockWithPassword("master password");
    expect((await harness.controller.getPopupState()).view).toBe("unlocked");
  });

  it("does not send the password to an app that cannot take it", async () => {
    const harness = createHarness({ state: { state: "locked", features: ["addresses"] } });
    await harness.pair();
    expect(await harness.controller.getPopupState()).toMatchObject({ view: "locked", isPasswordUnlockAvailable: false });
    await expectHisarError(harness.controller.unlockWithPassword("master password"), "invalid-request");
    expect(harness.allRequest.some((request) => request.type === "unlock.password")).toBe(false);
  });
});

describe("vault", () => {
  it("lists every vault's items, shows one without its secrets and reveals a field when asked", async () => {
    const harness = createHarness();
    await harness.pair();
    expect((await harness.controller.listItems({})).map((item) => item.title)).toEqual(["Example", "Visa", "Wi-Fi"]);
    expect((await harness.controller.listItems({ query: "vis" })).map((item) => item.itemID)).toEqual(["item-visa"]);
    expect((await harness.controller.listItems({ category: "secure-note" })).map((item) => item.itemID)).toEqual(["item-wifi"]);
    const visa = await harness.controller.getItem("vault-personal", "item-visa");
    expect(visa.fields.find((field) => field.label === "Card number")).toEqual({ id: "field:1", label: "Card number", kind: "secret" });
    expect(await harness.controller.revealItemField("vault-personal", "item-visa", "field:1")).toBe("4111 1111 1111 1111");
    expect(harness.state.allRevealRequest).toEqual(["item-visa/field:1"]);
    await harness.controller.setItemFavorite("vault-personal", "item-wifi", true);
    expect((await harness.controller.listItems({ isFavoriteOnly: true })).map((item) => [item.itemID, item.isFavorite])).toEqual([["item-wifi", true]]);
    expect(await harness.controller.getItemIcon("https://example.com")).toBe("");
    expect(await harness.controller.getItemIcon("chrome://settings")).toBe("");
  });

  it("refuses the vault with an app that does not announce it, without asking", async () => {
    const harness = createHarness({ state: { features: ["addresses"] } });
    await harness.pair();
    await expectHisarError(harness.controller.listItems({}), "invalid-request");
    await expectHisarError(harness.controller.revealItemField("vault-personal", "item-visa", "field:1"), "invalid-request");
    expect(harness.getRequestTypes().filter((type) => String(type).startsWith("items."))).toEqual([]);
  });
});

describe("save offers", () => {
  it("offers to save a new login and pushes the bar to the top frame", async () => {
    const harness = createHarness();
    await harness.pair();
    const offer = await harness.controller.handleLoginSubmitted(1, "https://example.com/login", "new-user", "pw");
    expect(offer).toEqual({ offerID: expect.any(String), kind: "save", host: "example.com", username: "new-user", title: undefined });
    expect(harness.allFrameMessage.at(-1)).toEqual({ frameId: 0, message: { type: "content.showSaveOffer", offer } });
    expect(harness.controller.getPendingSaveOffer(1)).toEqual(offer);
  });

  it("offers to update a changed password", async () => {
    const harness = createHarness();
    await harness.pair();
    const offer = await harness.controller.handleLoginSubmitted(1, "https://example.com/login", "alex@example.com", "new password");
    expect(offer).toMatchObject({ kind: "update", title: "Example" });
  });

  it("offers nothing for an unchanged login", async () => {
    const harness = createHarness();
    await harness.pair();
    expect(await harness.controller.handleLoginSubmitted(1, "https://example.com/", "alex@example.com", "correct horse battery staple")).toBeUndefined();
    expect(harness.controller.getPendingSaveOffer(1)).toBeNull();
  });

  it("offers nothing and does not ask the desktop on a never-listed site", async () => {
    const harness = createHarness();
    await harness.pair();
    await harness.storage.addNeverOrigin("https://example.com");
    expect(await harness.controller.handleLoginSubmitted(1, "https://example.com/login", "a", "b")).toBeUndefined();
    expect(harness.getRequestTypes()).not.toContain("logins.check");
  });

  it("offers nothing while not paired, locked, or on non-web pages", async () => {
    const notPaired = createHarness();
    expect(await notPaired.controller.handleLoginSubmitted(1, "https://example.com/", "a", "b")).toBeUndefined();
    const locked = createHarness({ state: { state: "locked" } });
    await locked.pair();
    expect(await locked.controller.handleLoginSubmitted(1, "https://example.com/", "a", "b")).toBeUndefined();
    const unlocked = createHarness();
    await unlocked.pair();
    expect(await unlocked.controller.handleLoginSubmitted(1, "file:///tmp/login.html", "a", "b")).toBeUndefined();
  });

  it("uses the username from the first step of a two-step sign-in", async () => {
    const harness = createHarness();
    await harness.pair();
    harness.controller.handleUsernameSubmitted(1, "https://example.com/identifier", "alex@example.com");
    const offer = await harness.controller.handleLoginSubmitted(1, "https://example.com/password", "", "rotated");
    expect(offer).toMatchObject({ kind: "update", username: "alex@example.com" });
    expect(harness.allRequest.find((request) => request.type === "logins.check")).toMatchObject({ username: "alex@example.com" });
  });

  it("does not use a username remembered on another site or after it expired", async () => {
    const harness = createHarness();
    await harness.pair();
    harness.controller.handleUsernameSubmitted(1, "https://other.example.net/", "someone");
    expect(await harness.controller.handleLoginSubmitted(1, "https://example.com/", "", "pw")).toMatchObject({ username: "" });
    harness.controller.handleUsernameSubmitted(2, "https://example.com/", "alex@example.com");
    harness.advanceTime(11 * 60 * 1000);
    expect(await harness.controller.handleLoginSubmitted(2, "https://example.com/", "", "pw")).toMatchObject({ username: "" });
  });

  it("saves a new login in the desktop", async () => {
    const harness = createHarness();
    await harness.pair();
    const offer = (await harness.controller.handleLoginSubmitted(1, "https://shop.example.com/login", "buyer", "pw"))!;
    await harness.controller.actOnSaveOffer(1, offer.offerID, "save");
    const saved = harness.state.allLogin.find((login) => login.username === "buyer");
    expect(saved).toMatchObject({ title: "shop.example.com", password: "pw" });
    expect(harness.allRequest.find((request) => request.type === "logins.save")).not.toHaveProperty("itemID");
    expect(harness.controller.getPendingSaveOffer(1)).toBeNull();
  });

  it("updates the matching item for a changed password", async () => {
    const harness = createHarness();
    await harness.pair();
    const offer = (await harness.controller.handleLoginSubmitted(1, "https://example.com/", "alex@example.com", "rotated"))!;
    await harness.controller.actOnSaveOffer(1, offer.offerID, "save");
    expect(harness.allRequest.find((request) => request.type === "logins.save")).toMatchObject({ vaultID: "vault-personal", itemID: "item-example" });
    expect(harness.state.allLogin[0].password).toBe("rotated");
    expect(harness.state.allLogin).toHaveLength(1);
  });

  it("keeps the offer when saving fails so the user can retry", async () => {
    const harness = createHarness();
    await harness.pair();
    const offer = (await harness.controller.handleLoginSubmitted(1, "https://example.com/", "new", "pw"))!;
    harness.state.isOffline = true;
    await expectHisarError(harness.controller.actOnSaveOffer(1, offer.offerID, "save"), "offline");
    expect(harness.controller.getPendingSaveOffer(1)).toEqual(offer);
  });

  it("remembers Never for this site per origin", async () => {
    const harness = createHarness();
    await harness.pair();
    const offer = (await harness.controller.handleLoginSubmitted(1, "https://example.com/a", "new", "pw"))!;
    await harness.controller.actOnSaveOffer(1, offer.offerID, "never");
    expect(harness.area.data.allNeverOrigin).toEqual(["https://example.com"]);
    expect(await harness.controller.handleLoginSubmitted(1, "https://example.com/b", "new", "pw")).toBeUndefined();
    expect(await harness.controller.handleLoginSubmitted(1, "https://www.example.com/", "new", "pw")).toBeDefined();
  });

  it("drops the offer on Not now", async () => {
    const harness = createHarness();
    await harness.pair();
    const offer = (await harness.controller.handleLoginSubmitted(1, "https://example.com/", "new", "pw"))!;
    await harness.controller.actOnSaveOffer(1, offer.offerID, "not-now");
    expect(harness.controller.getPendingSaveOffer(1)).toBeNull();
    expect(harness.getRequestTypes()).not.toContain("logins.save");
  });

  it("expires offers after two minutes and forgets them when the tab closes", async () => {
    const harness = createHarness();
    await harness.pair();
    const offer = (await harness.controller.handleLoginSubmitted(1, "https://example.com/", "new", "pw"))!;
    harness.advanceTime(121_000);
    await expectHisarError(harness.controller.actOnSaveOffer(1, offer.offerID, "save"), "not-found");
    await harness.controller.handleLoginSubmitted(2, "https://example.com/", "new", "pw");
    harness.controller.handleTabRemoved(2);
    expect(harness.controller.getPendingSaveOffer(2)).toBeNull();
  });

  it("refuses an offer ID from another tab", async () => {
    const harness = createHarness();
    await harness.pair();
    const offer = (await harness.controller.handleLoginSubmitted(1, "https://example.com/", "new", "pw"))!;
    await expectHisarError(harness.controller.actOnSaveOffer(2, offer.offerID, "save"), "not-found");
  });
});

describe("update required", () => {
  const appRequirement = { target: "app" as const, currentVersion: "0.1.5", minimumVersion: "0.1.6", updateURL: "https://tryhisar.com/download" };
  const extensionRequirement = {
    target: "extension" as const,
    currentVersion: "0.1.0",
    minimumVersion: "0.3.0",
    updateURL: "https://chromewebstore.google.com/detail/mmipiffoogblblocokimpemdopmdjghf",
  };

  it("shows the update view when the Mac app is too old and the desktop refuses fills", async () => {
    const harness = createHarness();
    await harness.pair();
    harness.state.updateRequired = appRequirement;
    expect(await harness.controller.getPopupState(true)).toEqual({ view: "update-required", updateRequired: appRequirement });
    await expectHisarError(harness.controller.fillLogin(1, "vault-personal", "item-example"), "update-required");
    expect(harness.allFrameMessage.some((entry) => entry.message.type === "content.fillLogin")).toBe(false);
  });

  it("maps an update-required answer to the status request to the update view", async () => {
    const harness = createHarness();
    await harness.pair();
    harness.state.updateRequired = appRequirement;
    const popupState = await harness.controller.getPopupState();
    expect(popupState.view).toBe("update-required");
    expect(popupState.errorMessage).toBe("Hisar needs an update: the server requires version 0.1.6 or later. Open Hisar to update.");
  });

  it("refuses matches and fills without asking the desktop while the extension is too old", async () => {
    const harness = createHarness();
    await harness.pair();
    harness.state.updateRequired = extensionRequirement;
    expect(await harness.controller.getPopupState(true)).toEqual({ view: "update-required", updateRequired: extensionRequirement });
    const requestCount = harness.allRequest.length;
    await expectHisarError(harness.controller.matchLogins(1), "update-required");
    await expectHisarError(harness.controller.fillFirstLogin(1), "update-required");
    await expectHisarError(harness.controller.getPasswordForTab(1, "vault-personal", "item-example"), "update-required");
    expect(harness.allRequest.length).toBe(requestCount);
    const error = await harness.controller.matchLogins(1).catch((caught: Error) => caught);
    expect((error as Error).message).toBe("Update the Hisar extension: this server requires version 0.3.0 or later.");
  });

  it("fills again once a newer hello no longer asks for an update", async () => {
    const harness = createHarness();
    await harness.pair();
    harness.state.updateRequired = extensionRequirement;
    await harness.controller.getPopupState(true);
    harness.state.updateRequired = null;
    expect((await harness.controller.getPopupState(true)).view).toBe("unlocked");
    expect((await harness.controller.matchLogins(1)).allLogin).toHaveLength(1);
  });
});
