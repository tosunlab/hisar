import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { PopupMessage, PopupState } from "../shared/messages";
import type { LoginSummary } from "../shared/protocol";
import App from "./App.vue";

type Handler = (message: PopupMessage) => unknown;

let handler: Handler;
let allMessage: PopupMessage[];
let storedSettings: Record<string, unknown>;
let allClipboardText: string[];
const closeWindow = vi.fn();

const allLogin: LoginSummary[] = [
  { vaultID: "v1", itemID: "i1", title: "Example", username: "alex@example.com", vaultName: "Personal" },
  { vaultID: "v2", itemID: "i2", title: "Example (work)", username: "alex@work.example", vaultName: "Family" },
];

function installChrome(): void {
  vi.stubGlobal("chrome", {
    runtime: {
      sendMessage: vi.fn(async (message: PopupMessage) => {
        allMessage.push(message);
        try {
          return { ok: true, result: (await handler(message)) ?? null };
        } catch (error) {
          const typed = error as { code?: string; message: string };
          return { ok: false, error: { code: typed.code ?? "internal", message: typed.message } };
        }
      }),
    },
    tabs: { query: vi.fn(async () => [{ id: 42 }]), create: vi.fn(async () => ({})) },
    storage: {
      local: {
        get: vi.fn(async (allKey: string[]) => Object.fromEntries(allKey.filter((key) => key in storedSettings).map((key) => [key, storedSettings[key]]))),
        set: vi.fn(async (allEntry: Record<string, unknown>) => Object.assign(storedSettings, allEntry)),
      },
    },
  });
}

function respondWithState(state: PopupState, overrides: Partial<Record<PopupMessage["type"], Handler>> = {}): void {
  handler = (message) => {
    const override = overrides[message.type];
    if (override) return override(message);
    if (message.type === "popup.getState") return state;
    if (message.type === "popup.matchLogins") return { url: "https://example.com/login", allLogin };
    if (message.type === "popup.getPassword") return `password-of-${message.itemID}`;
    if (message.type === "popup.generatePassword") return `generated-${message.options.length}-${message.options.digits}-${message.options.symbols}`;
    return null;
  };
}

async function mountApp(props: Record<string, unknown> = {}) {
  const wrapper = mount(App, { props, attachTo: document.body });
  await flushPromises();
  return wrapper;
}

beforeEach(() => {
  allMessage = [];
  storedSettings = {};
  allClipboardText = [];
  installChrome();
  vi.stubGlobal("navigator", { clipboard: { writeText: vi.fn(async (text: string) => void allClipboardText.push(text)) } });
  closeWindow.mockReset();
  vi.spyOn(window, "close").mockImplementation(closeWindow);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
  document.body.innerHTML = "";
});

describe("one-time codes", () => {
  it("shows the code with the seconds left for logins that have one, asks again when it expires, and copies it", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "setTimeout", "clearTimeout", "Date"] });
    vi.setSystemTime(new Date("2026-10-09T12:00:20Z"));
    let requestCount = 0;
    respondWithState(
      { view: "unlocked", email: "alex@example.com", isOffline: false },
      {
        "popup.matchLogins": () => ({ url: "https://example.com/login", allLogin: [{ ...allLogin[0], hasOneTimePassword: true }, allLogin[1]] }),
        "popup.getOneTimeCode": () => {
          requestCount += 1;
          return requestCount === 1
            ? { code: "123456", expiresAt: "2026-10-09T12:00:30Z", period: 30 }
            : { code: "654321", expiresAt: "2026-10-09T12:01:00Z", period: 30 };
        },
      },
    );
    const wrapper = await mountApp();
    expect(wrapper.findAll('[data-testid="one-time-code"]')).toHaveLength(1);
    expect(allMessage).toContainEqual({ type: "popup.getOneTimeCode", tabId: 42, vaultID: "v1", itemID: "i1" });
    expect(wrapper.get('[data-testid="one-time-code-value"]').text()).toBe("123 456");
    expect(wrapper.get('[data-testid="one-time-code-seconds"]').text()).toBe("10s");
    await wrapper.get('[data-testid="copy-one-time-code"]').trigger("click");
    await flushPromises();
    expect(allClipboardText).toEqual(["123456"]);
    expect(wrapper.get('[data-testid="notice"]').text()).toBe("One-time code copied");
    vi.advanceTimersByTime(10_000);
    await flushPromises();
    expect(wrapper.get('[data-testid="one-time-code-value"]').text()).toBe("654 321");
    expect(wrapper.get('[data-testid="one-time-code-seconds"]').text()).toBe("30s");
  });

  it("shows why the code is not available", async () => {
    respondWithState(
      { view: "unlocked", email: "alex@example.com", isOffline: false },
      {
        "popup.matchLogins": () => ({ url: "https://example.com/login", allLogin: [{ ...allLogin[0], hasOneTimePassword: true }] }),
        "popup.getOneTimeCode": () => {
          throw Object.assign(new Error("This login is not saved for this site"), { code: "no-match" });
        },
      },
    );
    const wrapper = await mountApp();
    expect(wrapper.get('[data-testid="one-time-code"]').text()).toBe("This login is not saved for this site");
  });
});

describe("popup states", () => {
  it("shows the pairing flow when not paired and pairs with the 6-digit code", async () => {
    let state: PopupState = { view: "not-paired" };
    handler = (message) => {
      if (message.type === "popup.getState") return state;
      if (message.type === "popup.finishPairing") {
        state = { view: "unlocked", email: "alex@example.com", isOffline: false };
      }
      if (message.type === "popup.matchLogins") return { url: "https://example.com/", allLogin: [] };
      return null;
    };
    const wrapper = await mountApp();
    expect(wrapper.find('[data-testid="pairing-view"]').exists()).toBe(true);
    await wrapper.find('[data-testid="start-pairing"]').trigger("click");
    await flushPromises();
    expect(allMessage.map((message) => message.type)).toContain("popup.startPairing");
    const codeInput = wrapper.find('[data-testid="pairing-code"]');
    await codeInput.setValue("12345");
    expect(wrapper.find('[data-testid="finish-pairing"]').attributes("disabled")).toBeDefined();
    await codeInput.setValue("123 456");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "popup.finishPairing", code: "123456" });
    expect(wrapper.find('[data-testid="unlocked-view"]').exists()).toBe(true);
  });

  it("shows the desktop's error when pairing fails", async () => {
    respondWithState(
      { view: "not-paired" },
      {
        "popup.finishPairing": () => {
          throw Object.assign(new Error("The code does not match"), { code: "pairing-failed" });
        },
      },
    );
    const wrapper = await mountApp();
    await wrapper.find('[data-testid="start-pairing"]').trigger("click");
    await flushPromises();
    await wrapper.find('[data-testid="pairing-code"]').setValue("000000");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(wrapper.find(".error").text()).toBe("The code does not match");
  });

  it("offers Open Hisar when the desktop is unavailable and reconnects", async () => {
    respondWithState({ view: "desktop-unavailable", errorMessage: "Specified native messaging host not found." });
    const wrapper = await mountApp();
    expect(wrapper.find('[data-testid="status-desktop-unavailable"]').text()).toContain("Hisar is not running");
    expect(wrapper.text()).toContain("Specified native messaging host not found.");
    await wrapper.find('[data-testid="open-hisar"]').trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "popup.getState", shouldReconnect: true });
  });

  it("asks to sign in when the desktop is signed out", async () => {
    respondWithState({ view: "signed-out" });
    const wrapper = await mountApp();
    expect(wrapper.find('[data-testid="status-signed-out"]').text()).toContain("Sign in to Hisar");
  });

  it("unlocks in Hisar and polls until the desktop is unlocked", async () => {
    vi.useFakeTimers();
    let state: PopupState = { view: "locked" };
    handler = (message) => {
      if (message.type === "popup.getState") return state;
      if (message.type === "popup.matchLogins") return { url: "https://example.com/", allLogin };
      return null;
    };
    const wrapper = await mountApp({ pollIntervalMilliseconds: 500 });
    await wrapper.find('[data-testid="unlock"]').trigger("click");
    await flushPromises();
    expect(allMessage.map((message) => message.type)).toContain("popup.unlock");
    expect(wrapper.find('[data-testid="unlock"]').text()).toContain("Waiting for Hisar");
    await vi.advanceTimersByTimeAsync(500);
    expect(wrapper.find('[data-testid="status-locked"]').exists()).toBe(true);
    state = { view: "unlocked", email: "alex@example.com", isOffline: false };
    await vi.advanceTimersByTimeAsync(500);
    await flushPromises();
    expect(wrapper.find('[data-testid="unlocked-view"]').exists()).toBe(true);
  });

  it("unlocks with the master password typed in the popup, and shows a wrong one", async () => {
    let state: PopupState = { view: "locked", isPasswordUnlockAvailable: true };
    handler = (message) => {
      if (message.type === "popup.getState") return state;
      if (message.type === "popup.matchLogins") return { url: "https://example.com/", allLogin };
      if (message.type === "popup.unlockWithPassword") {
        if (message.password !== "master password") throw Object.assign(new Error("That master password is not right"), { code: "wrong-password" });
        state = { view: "unlocked", email: "alex@example.com" };
      }
      return null;
    };
    const wrapper = await mountApp();
    const input = wrapper.find('[data-testid="master-password"]');
    expect(document.activeElement).toBe(input.element);
    expect(wrapper.find('[data-testid="unlock-with-password"]').attributes("disabled")).toBeDefined();
    await input.setValue("guess");
    await wrapper.find('[data-testid="unlock-form"]').trigger("submit");
    await flushPromises();
    expect(wrapper.find('[data-testid="unlock-error"]').text()).toBe("That master password is not right");
    expect((wrapper.find('[data-testid="master-password"]').element as HTMLInputElement).value).toBe("");

    await wrapper.find('[data-testid="master-password"]').setValue("master password");
    await wrapper.find('[data-testid="unlock-form"]').trigger("submit");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "popup.unlockWithPassword", password: "master password" });
    expect(wrapper.find('[data-testid="unlocked-view"]').exists()).toBe(true);
    // Unlocking in the app stays on offer.
    expect(allMessage.some((message) => message.type === "popup.unlock")).toBe(false);
  });

  it("still offers to unlock in the app next to the password", async () => {
    respondWithState({ view: "locked", isPasswordUnlockAvailable: true });
    const wrapper = await mountApp();
    await wrapper.find('[data-testid="unlock"]').trigger("click");
    await flushPromises();
    expect(allMessage.map((message) => message.type)).toContain("popup.unlock");
  });

  it("stops polling after the limit", async () => {
    vi.useFakeTimers();
    respondWithState({ view: "locked" });
    const wrapper = await mountApp({ pollIntervalMilliseconds: 100, pollLimit: 3 });
    await wrapper.find('[data-testid="unlock"]').trigger("click");
    await flushPromises();
    await vi.advanceTimersByTimeAsync(1000);
    expect(allMessage.filter((message) => message.type === "popup.getState")).toHaveLength(4);
    expect(wrapper.find('[data-testid="unlock"]').attributes("disabled")).toBeUndefined();
  });
});

describe("unlocked popup", () => {
  it("lists matching logins for the active tab with the account and offline state", async () => {
    respondWithState({ view: "unlocked", email: "alex@example.com", isOffline: true });
    const wrapper = await mountApp();
    expect(allMessage).toContainEqual({ type: "popup.matchLogins", tabId: 42 });
    expect(wrapper.findAll('[data-testid="login-row"]').map((row) => row.find(".title").text())).toEqual(["Example", "Example (work)"]);
    expect(wrapper.find('[data-testid="offline"]').exists()).toBe(true);
    expect(wrapper.text()).toContain("alex@example.com");
  });

  it("searches the matching logins", async () => {
    respondWithState({ view: "unlocked", email: "alex@example.com", isOffline: false });
    const wrapper = await mountApp();
    await wrapper.find('[data-testid="search"]').setValue("family");
    expect(wrapper.findAll('[data-testid="login-row"]').map((row) => row.find(".title").text())).toEqual(["Example (work)"]);
    await wrapper.find('[data-testid="search"]').setValue("nothing");
    expect(wrapper.text()).toContain('No logins match "nothing"');
  });

  it("says when no login matches the page", async () => {
    respondWithState({ view: "unlocked" }, { "popup.matchLogins": () => ({ url: "https://example.org/", allLogin: [] }) });
    const wrapper = await mountApp();
    expect(wrapper.find('[data-testid="no-logins"]').text()).toBe("No saved logins for example.org.");
  });

  it("fills a login and closes the popup", async () => {
    respondWithState({ view: "unlocked" });
    const wrapper = await mountApp();
    await wrapper.findAll('[data-testid="fill"]')[1].trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "popup.fillLogin", tabId: 42, vaultID: "v2", itemID: "i2" });
    expect(closeWindow).toHaveBeenCalled();
  });

  it("shows fill errors and stays open", async () => {
    respondWithState(
      { view: "unlocked" },
      {
        "popup.fillLogin": () => {
          throw Object.assign(new Error("No login form found on this page"), { code: "not-found" });
        },
      },
    );
    const wrapper = await mountApp();
    await wrapper.find('[data-testid="fill"]').trigger("click");
    await flushPromises();
    expect(wrapper.find(".error").text()).toBe("No login form found on this page");
    expect(closeWindow).not.toHaveBeenCalled();
  });

  it("copies the username and the password straight from the popup", async () => {
    respondWithState({ view: "unlocked" });
    const wrapper = await mountApp();
    await wrapper.find('[data-testid="copy-username"]').trigger("click");
    await flushPromises();
    expect(allClipboardText).toEqual(["alex@example.com"]);
    await wrapper.find('[data-testid="copy-password"]').trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "popup.getPassword", tabId: 42, vaultID: "v1", itemID: "i1" });
    expect(allClipboardText).toEqual(["alex@example.com", "password-of-i1"]);
    expect(wrapper.find('[data-testid="notice"]').text()).toBe("Password copied");
  });

  it("generates passwords with the chosen options, copies them and fills the focused field", async () => {
    respondWithState({ view: "unlocked" });
    const wrapper = await mountApp();
    await wrapper.find('[data-testid="section-generator"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="generated-password"]').text()).toBe("generated-24-true-true");
    await wrapper.find('[data-testid="length"]').setValue("40");
    await wrapper.find('input[name="symbols"]').setValue(false);
    await flushPromises();
    expect(wrapper.find('[data-testid="generated-password"]').text()).toBe("generated-40-true-false");
    await wrapper.find('[data-testid="copy-generated"]').trigger("click");
    await flushPromises();
    expect(allClipboardText).toEqual(["generated-40-true-false"]);
    await wrapper.find('[data-testid="fill-generated"]').trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "popup.fillPassword", tabId: 42, password: "generated-40-true-false" });
    expect(closeWindow).toHaveBeenCalled();
  });
});

describe("settings", () => {
  it("shows the inline menu settings from any state and stores changes", async () => {
    storedSettings = { isInlineMenuOpenedOnFocus: false };
    respondWithState({ view: "locked" });
    const wrapper = await mountApp();
    await wrapper.find('[data-testid="open-settings"]').trigger("click");
    await flushPromises();
    const enabled = wrapper.find<HTMLInputElement>('[data-testid="setting-inline-menu"]');
    const openOnFocus = wrapper.find<HTMLInputElement>('[data-testid="setting-open-on-focus"]');
    expect(enabled.element.checked).toBe(true);
    expect(openOnFocus.element.checked).toBe(false);
    expect(wrapper.find('[data-testid="status-locked"]').exists()).toBe(false);
    await openOnFocus.setValue(true);
    await flushPromises();
    expect(storedSettings).toEqual({ isInlineMenuOpenedOnFocus: true });
    await enabled.setValue(false);
    await flushPromises();
    expect(storedSettings).toEqual({ isInlineMenuOpenedOnFocus: true, isInlineMenuEnabled: false });
    expect(wrapper.find<HTMLInputElement>('[data-testid="setting-open-on-focus"]').element.disabled).toBe(true);
    await wrapper.find('[data-testid="settings-done"]').trigger("click");
    expect(wrapper.find('[data-testid="status-locked"]').exists()).toBe(true);
  });
});

describe("update required", () => {
  it("says the Mac app needs an update and offers to open Hisar", async () => {
    respondWithState({
      view: "update-required",
      updateRequired: { target: "app", currentVersion: "0.1.5", minimumVersion: "0.1.6", updateURL: "https://tryhisar.com/download" },
    });
    const wrapper = await mountApp();
    const panel = wrapper.get('[data-testid="status-update-required"]');
    expect(panel.get("h1").text()).toBe("Hisar needs an update");
    expect(panel.text()).toContain("This server needs Hisar 0.1.6 or later. This Mac has 0.1.5. Open Hisar to update it.");
    expect(wrapper.find('[data-testid="update-extension"]').exists()).toBe(false);
    await wrapper.get('[data-testid="open-hisar"]').trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "popup.getState", shouldReconnect: true });
  });

  it("says the extension needs an update and opens its store page", async () => {
    const storeURL = "https://chromewebstore.google.com/detail/mmipiffoogblblocokimpemdopmdjghf";
    respondWithState({
      view: "update-required",
      updateRequired: { target: "extension", currentVersion: "0.2.0", minimumVersion: "0.3.0", updateURL: storeURL },
    });
    const wrapper = await mountApp();
    const panel = wrapper.get('[data-testid="status-update-required"]');
    expect(panel.get("h1").text()).toBe("Update the Hisar extension");
    expect(panel.text()).toContain("This server needs version 0.3.0 or later of the extension; this is 0.2.0.");
    expect(wrapper.find('[data-testid="open-hisar"]').exists()).toBe(false);
    await wrapper.get('[data-testid="update-extension"]').trigger("click");
    await flushPromises();
    expect(chrome.tabs.create).toHaveBeenCalledWith({ url: storeURL });
  });

  it("opens only known update pages", async () => {
    respondWithState({
      view: "update-required",
      updateRequired: { target: "extension", currentVersion: "0.2.0", minimumVersion: "0.3.0", updateURL: "https://evil.example/extension" },
    });
    const wrapper = await mountApp();
    await wrapper.get('[data-testid="update-extension"]').trigger("click");
    await flushPromises();
    expect(chrome.tabs.create).toHaveBeenCalledWith({ url: "https://chromewebstore.google.com/detail/mmipiffoogblblocokimpemdopmdjghf" });
  });

  it("shows the desktop's message when only the status request says so", async () => {
    respondWithState({ view: "update-required", errorMessage: "Hisar needs an update: the server requires version 0.1.6 or later. Open Hisar to update." });
    const wrapper = await mountApp();
    const panel = wrapper.get('[data-testid="status-update-required"]');
    expect(panel.get("h1").text()).toBe("Hisar needs an update");
    expect(panel.text()).toContain("the server requires version 0.1.6 or later");
  });
});

describe("addresses", () => {
  const allAddress = [
    { vaultID: "v1", itemID: "home", title: "Home", summary: "Springfield, US", source: "address", vaultName: "Personal" },
    { vaultID: "v1", itemID: "alice", title: "Alice", summary: "Chicago", source: "identity", vaultName: "Personal" },
  ];

  it("offers the Addresses tab only when the app fills addresses", async () => {
    respondWithState({ view: "unlocked", email: "alex@example.com" });
    const older = await mountApp();
    expect(older.find('[data-testid="section-addresses"]').exists()).toBe(false);
    older.unmount();

    respondWithState({ view: "unlocked", email: "alex@example.com", isAddressFillAvailable: true });
    const wrapper = await mountApp();
    expect(wrapper.find('[data-testid="section-addresses"]').exists()).toBe(true);
  });

  it("lists addresses and identities and fills the chosen one into the page", async () => {
    respondWithState({ view: "unlocked", email: "alex@example.com", isAddressFillAvailable: true }, { "popup.listAddresses": () => allAddress });
    const wrapper = await mountApp();
    await wrapper.find('[data-testid="section-addresses"]').trigger("click");
    await flushPromises();
    const allRow = wrapper.findAll('[data-testid="address-row"]');
    expect(allRow.map((row) => row.text())).toEqual([expect.stringContaining("Springfield, US"), expect.stringContaining("Chicago · Identity")]);
    expect(allMessage.some((message) => message.type === "popup.fillAddress")).toBe(false);
    await allRow[1].find('[data-testid="fill-address"]').trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "popup.fillAddress", tabId: 42, vaultID: "v1", itemID: "alice" });
    expect(closeWindow).toHaveBeenCalled();
  });

  it("shows why an address was not filled and stays open", async () => {
    respondWithState(
      { view: "unlocked", email: "alex@example.com", isAddressFillAvailable: true },
      {
        "popup.listAddresses": () => allAddress,
        "popup.fillAddress": () => {
          throw Object.assign(new Error("No address form found on this page"), { code: "not-found" });
        },
      },
    );
    const wrapper = await mountApp();
    await wrapper.find('[data-testid="section-addresses"]').trigger("click");
    await flushPromises();
    await wrapper.find('[data-testid="fill-address"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="address-error"]').text()).toContain("No address form found on this page");
    expect(closeWindow).not.toHaveBeenCalled();
  });
});

describe("vault", () => {
  const allItem = [
    { vaultID: "v1", itemID: "i1", category: "login", title: "Example", subtitle: "alex@example.com", vaultName: "Personal", url: "https://example.com", isFavorite: false, hasOneTimePassword: false },
    { vaultID: "v1", itemID: "github", category: "login", title: "GitHub", subtitle: "alex", vaultName: "Personal", url: "https://github.com", isFavorite: false, hasOneTimePassword: false },
    { vaultID: "v1", itemID: "visa", category: "credit-card", title: "Visa", subtitle: "Visa •••• 1111", vaultName: "Personal", isFavorite: true, hasOneTimePassword: false },
  ];
  const visa = {
    ...allItem[2],
    fields: [
      { id: "field:0", label: "Cardholder", kind: "text", value: "Alex Example" },
      { id: "field:1", label: "Card number", kind: "secret" },
    ],
  };

  let extraState: Partial<PopupState> = {};

  function respondWithStateOverride(state: Partial<PopupState>): void {
    extraState = state;
    const previous = handler;
    handler = (message) => (message.type === "popup.getState" ? { view: "unlocked", email: "alex@example.com", isVaultAvailable: true, ...extraState } : previous(message));
  }

  function respondWithVault(overrides: Partial<Record<PopupMessage["type"], Handler>> = {}): void {
    respondWithState(
      { view: "unlocked", email: "alex@example.com", isVaultAvailable: true },
      {
        "popup.listItems": (message) => {
          const { query = {} } = message as Extract<PopupMessage, { type: "popup.listItems" }>;
          return allItem
            .filter((item) => !query.category || item.category === query.category)
            .filter((item) => !query.query || item.title.toLowerCase().includes(query.query.toLowerCase()));
        },
        "popup.getItem": () => visa,
        "popup.revealField": () => "4111 1111 1111 1111",
        "popup.getIcon": () => "",
        ...overrides,
      },
    );
  }

  it("keeps the site's logins only with an app that cannot browse the vault", async () => {
    respondWithState({ view: "unlocked", email: "alex@example.com" });
    const wrapper = await mountApp();
    expect(wrapper.find('[data-testid="vault-view"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="section-logins"]').text()).toBe("Logins");
  });

  it("suggests the page's logins above every other item, and searches the whole vault", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    respondWithVault();
    const wrapper = await mountApp();
    expect(wrapper.find('[data-testid="section-logins"]').text()).toBe("Vault");
    expect(wrapper.findAll('[data-testid="login-row"]')).toHaveLength(2);
    // The suggested logins are not listed again below.
    expect(wrapper.findAll('[data-testid="item-row"]').map((row) => row.find(".title").text())).toEqual(["GitHub", "Visa"]);
    expect(document.activeElement).toBe(wrapper.find('[data-testid="vault-search"]').element);

    await wrapper.find('[data-testid="vault-search"]').setValue("git");
    await vi.advanceTimersByTimeAsync(150);
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "popup.listItems", query: { query: "git" } });
    expect(wrapper.findAll('[data-testid="item-row"]').map((row) => row.find(".title").text())).toEqual(["GitHub"]);
    expect(wrapper.findAll('[data-testid="login-row"]')).toHaveLength(0);

    await wrapper.find('[data-testid="category-credit-card"]').trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "popup.listItems", query: { query: "git", category: "credit-card" } });
  });

  it("opens an item without its secrets, reveals and copies one when asked, and stars it", async () => {
    respondWithVault();
    const wrapper = await mountApp();
    await wrapper.findAll('[data-testid="open-item"]')[1].trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="item-title"]').text()).toBe("Visa");
    expect(wrapper.find('[data-testid="item-fill"]').exists()).toBe(false);
    const number = wrapper.find('[data-testid="field-field:1"]');
    expect(number.text()).not.toContain("4111");
    expect(allMessage.some((message) => message.type === "popup.revealField")).toBe(false);

    await number.find('[data-testid="reveal-field"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="field-field:1"]').text()).toContain("4111 1111 1111 1111");
    expect(allMessage).toContainEqual({ type: "popup.revealField", vaultID: "v1", itemID: "visa", fieldID: "field:1" });

    await wrapper.find('[data-testid="field-field:0"] [data-testid="copy-field"]').trigger("click");
    await flushPromises();
    expect(allClipboardText).toEqual(["Alex Example"]);

    await wrapper.find('[data-testid="item-favorite"]').trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "popup.setFavorite", vaultID: "v1", itemID: "visa", isFavorite: false });

    await wrapper.find('[data-testid="item-back"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="vault-view"]').isVisible()).toBe(true);
  });

  it("fills a suggested login from its details", async () => {
    respondWithVault({ "popup.getItem": () => ({ ...allItem[0], fields: [{ id: "username", label: "Username", kind: "text", value: "alex@example.com" }] }) });
    const wrapper = await mountApp();
    await wrapper.find('[data-testid="open-login"]').trigger("click");
    await flushPromises();
    await wrapper.find('[data-testid="item-fill"]').trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "popup.fillLogin", tabId: 42, vaultID: "v1", itemID: "i1" });
    expect(closeWindow).toHaveBeenCalled();
  });

  it("creates a login for the page with a generated password, into the chosen vault", async () => {
    let created: unknown;
    respondWithVault({
      "popup.listVaults": () => [
        { vaultID: "v1", name: "Personal", isPersonal: true },
        { vaultID: "v2", name: "Family", isPersonal: false },
      ],
      "popup.createItem": (message) => {
        created = message;
        return { ...allItem[1], itemID: "new", title: "example.com" };
      },
    });
    respondWithStateOverride({ isCreateAvailable: true });
    const wrapper = await mountApp();
    await wrapper.find('[data-testid="new-item-open"]').trigger("click");
    await flushPromises();
    expect((wrapper.find('[data-testid="new-title"]').element as HTMLInputElement).value).toBe("example.com");
    expect((wrapper.find('[data-testid="new-url"]').element as HTMLInputElement).value).toBe("https://example.com");
    expect((wrapper.find('[data-testid="new-password"]').element as HTMLInputElement).value).toBe("generated-undefined-undefined-undefined");
    await wrapper.find('[data-testid="new-username"]').setValue("alex");
    await wrapper.find('[data-testid="new-vault"]').setValue("v2");
    await wrapper.find('[data-testid="new-item"]').trigger("submit");
    await flushPromises();
    expect(created).toEqual({
      type: "popup.createItem",
      vaultID: "v2",
      item: { category: "login", title: "example.com", username: "alex", password: "generated-undefined-undefined-undefined", url: "https://example.com" },
    });
    expect(wrapper.find('[data-testid="item-detail"]').exists()).toBe(true);
  });

  it("puts the logins filled most recently first", async () => {
    respondWithVault({ "popup.getAllLastUsed": () => ({ "v2/i2": 5 }) });
    const wrapper = await mountApp();
    expect(wrapper.findAll('[data-testid="login-row"]').map((row) => row.find(".title").text())).toEqual(["Example (work)", "Example"]);
  });

  it("moves between the search box and the rows with the arrow keys, and copies a login's password with Ctrl+C", async () => {
    respondWithVault();
    const wrapper = await mountApp();
    const search = wrapper.find('[data-testid="vault-search"]');
    await search.trigger("keydown", { key: "ArrowDown" });
    const allRow = wrapper.findAll("[data-row]");
    expect(document.activeElement).toBe(allRow[0].element);
    await allRow[0].trigger("keydown", { key: "ArrowDown" });
    expect(document.activeElement).toBe(allRow[1].element);
    await allRow[1].trigger("keydown", { key: "c", ctrlKey: true });
    await flushPromises();
    expect(allClipboardText).toEqual(["password-of-i2"]);
    await allRow[1].trigger("keydown", { key: "ArrowUp" });
    await wrapper.findAll("[data-row]")[0].trigger("keydown", { key: "ArrowUp" });
    expect(document.activeElement).toBe(search.element);
  });

  it("copies a login's password from the list", async () => {
    respondWithVault({ "popup.revealField": () => "hunter2" });
    const wrapper = await mountApp();
    await wrapper.find('[data-testid="item-copy-password"]').trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "popup.revealField", vaultID: "v1", itemID: "github", fieldID: "password" });
    expect(allClipboardText).toEqual(["hunter2"]);
  });
});
