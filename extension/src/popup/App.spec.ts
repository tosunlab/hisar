import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { PopupMessage, PopupState } from "../shared/messages";
import type { LoginSummary } from "../shared/protocol";
import App from "./App.vue";

type Handler = (message: PopupMessage) => unknown;

let handler: Handler;
let allMessage: PopupMessage[];
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
    tabs: { query: vi.fn(async () => [{ id: 42 }]) },
  });
}

function respondWithState(state: PopupState, overrides: Partial<Record<PopupMessage["type"], Handler>> = {}): void {
  handler = (message) => {
    const override = overrides[message.type];
    if (override) return override(message);
    if (message.type === "popup.getState") return state;
    if (message.type === "popup.matchLogins") return { url: "https://example.com/login", allLogin };
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
  installChrome();
  closeWindow.mockReset();
  vi.spyOn(window, "close").mockImplementation(closeWindow);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
  document.body.innerHTML = "";
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

  it("copies the username and the password", async () => {
    respondWithState({ view: "unlocked" });
    const wrapper = await mountApp();
    await wrapper.find('[data-testid="copy-username"]').trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "popup.copyText", text: "alex@example.com" });
    await wrapper.find('[data-testid="copy-password"]').trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "popup.copyPassword", tabId: 42, vaultID: "v1", itemID: "i1" });
    expect(wrapper.find('[data-testid="notice"]').text()).toContain("Clears in 90 s");
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
    expect(allMessage).toContainEqual({ type: "popup.copyText", text: "generated-40-true-false" });
    await wrapper.find('[data-testid="fill-generated"]').trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "popup.fillPassword", tabId: 42, password: "generated-40-true-false" });
    expect(closeWindow).toHaveBeenCalled();
  });
});
