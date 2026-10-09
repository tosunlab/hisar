import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { InlineMenuBroadcast, InlineMenuMessage, InlineMenuState } from "../shared/messages";
import type { LoginSummary } from "../shared/protocol";
import { readSessionID } from "./api";
import InlineMenu from "./InlineMenu.vue";

type Handler = (message: InlineMenuMessage) => unknown;

let handler: Handler;
let allMessage: InlineMenuMessage[];
let allBroadcastListener: ((message: InlineMenuBroadcast) => void)[];

const allLogin: LoginSummary[] = [
  { vaultID: "v1", itemID: "i1", title: "Example", username: "alex@example.com", vaultName: "Personal" },
  { vaultID: "v2", itemID: "i2", title: "Example (work)", username: "alex@work.example", vaultName: "Family" },
];

function createState(overrides: Partial<InlineMenuState> = {}): InlineMenuState {
  return { view: "logins", purpose: "login", host: "example.com", allLogin, hasMoreLogin: false, ...overrides };
}

function respondWith(state: InlineMenuState, overrides: Partial<Record<InlineMenuMessage["type"], Handler>> = {}): void {
  handler = (message) => {
    const override = overrides[message.type];
    if (override) return override(message);
    if (message.type === "inlineMenu.getState") return state;
    return null;
  };
}

async function mountMenu(props: Record<string, unknown> = {}) {
  const wrapper = mount(InlineMenu, { props: { sessionID: "session-1", isEventTrusted: () => true, ...props }, attachTo: document.body });
  await flushPromises();
  return wrapper;
}

function broadcast(key: InlineMenuBroadcast["key"], sessionID = "session-1"): void {
  for (const listener of allBroadcastListener) listener({ target: "inline-menu", type: "inlineMenu.pressKey", sessionID, key });
}

beforeEach(() => {
  allMessage = [];
  allBroadcastListener = [];
  vi.stubGlobal("chrome", {
    runtime: {
      sendMessage: vi.fn(async (message: InlineMenuMessage) => {
        allMessage.push(message);
        try {
          return { ok: true, result: (await handler(message)) ?? null };
        } catch (error) {
          return { ok: false, error: { code: "invalid-request", message: (error as Error).message } };
        }
      }),
      onMessage: {
        addListener: (listener: (message: InlineMenuBroadcast) => void) => allBroadcastListener.push(listener),
        removeListener: vi.fn(),
      },
    },
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
  document.body.innerHTML = "";
});

describe("inline menu page", () => {
  it("reads the session ID from the URL fragment", () => {
    expect(readSessionID({ hash: "#abc-123" } as Location)).toBe("abc-123");
  });

  it("lists the matching logins with title and username and fills the chosen one", async () => {
    respondWith(createState());
    const wrapper = await mountMenu();
    expect(allMessage[0]).toEqual({ type: "inlineMenu.getState", sessionID: "session-1" });
    const allRow = wrapper.findAll('[data-testid="inline-login"]');
    expect(allRow.map((row) => row.text())).toEqual(["EExamplealex@example.com", "EExample (work)alex@work.example"]);
    await allRow[1].trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "inlineMenu.fillLogin", sessionID: "session-1", vaultID: "v2", itemID: "i2" });
  });

  it("reports its height so the page can size the frame", async () => {
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({ height: 96.4 } as DOMRect);
    respondWith(createState());
    await mountMenu();
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "inlineMenu.resize", sessionID: "session-1", height: 97 });
    vi.restoreAllMocks();
  });

  it("ignores untrusted clicks", async () => {
    respondWith(createState());
    const wrapper = await mountMenu({ isEventTrusted: (event: Event) => event.isTrusted });
    await wrapper.find('[data-testid="inline-login"]').trigger("click");
    await flushPromises();
    expect(allMessage.some((message) => message.type === "inlineMenu.fillLogin")).toBe(false);
  });

  it("ignores clicks until visibility tracking has seen the menu for 300 ms", async () => {
    let observerCallback: IntersectionObserverCallback = () => undefined;
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        constructor(callback: IntersectionObserverCallback) {
          observerCallback = callback;
        }
        observe(): void {}
        disconnect(): void {}
      },
    );
    let now = 0;
    respondWith(createState());
    const wrapper = await mountMenu({ isVisibilityTracked: true, now: () => now });
    const reportVisibility = (isVisible: boolean): void =>
      observerCallback([{ isVisible } as unknown as IntersectionObserverEntry], {} as IntersectionObserver);
    await wrapper.find('[data-testid="inline-login"]').trigger("click");
    reportVisibility(true);
    now = 200;
    await wrapper.find('[data-testid="inline-login"]').trigger("click");
    await flushPromises();
    expect(allMessage.some((message) => message.type === "inlineMenu.fillLogin")).toBe(false);
    now = 300;
    await wrapper.find('[data-testid="inline-login"]').trigger("click");
    await flushPromises();
    expect(allMessage.filter((message) => message.type === "inlineMenu.fillLogin")).toHaveLength(1);
    reportVisibility(false);
    now = 1000;
    await wrapper.find('[data-testid="inline-login"]').trigger("click");
    await flushPromises();
    expect(allMessage.filter((message) => message.type === "inlineMenu.fillLogin")).toHaveLength(1);
  });

  it("offers More in Hisar when there are more logins", async () => {
    respondWith(createState({ hasMoreLogin: true }));
    const wrapper = await mountMenu();
    await wrapper.find('[data-testid="inline-more"]').trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "inlineMenu.openPopup", sessionID: "session-1" });
  });

  it("says when no login matches and that Hisar offers to save after sign-in", async () => {
    respondWith(createState({ allLogin: [] }));
    const wrapper = await mountMenu();
    expect(wrapper.find('[data-testid="inline-no-logins"]').text()).toBe("No logins for example.comHisar offers to save the login after you sign in.");
  });

  it("offers the suggested password on new-password fields", async () => {
    respondWith(createState({ purpose: "new-password", allLogin: [], suggestedPassword: "Suggested-Password-123" }));
    const wrapper = await mountMenu();
    expect(wrapper.find('[data-testid="suggested-password"]').text()).toContain("Suggested-Password-123");
    await wrapper.find('[data-testid="suggested-password"]').trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "inlineMenu.useSuggestedPassword", sessionID: "session-1" });
  });

  it("offers pairing through the popup when not paired", async () => {
    respondWith(createState({ view: "not-paired", allLogin: [] }));
    const wrapper = await mountMenu();
    expect(wrapper.find('[data-testid="inline-status"]').text()).toBe("This browser is not paired with Hisar");
    await wrapper.find('[data-testid="inline-action"]').trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "inlineMenu.openPopup", sessionID: "session-1" });
  });

  it("unlocks and polls until the logins appear", async () => {
    vi.useFakeTimers();
    let state = createState({ view: "locked", allLogin: [] });
    handler = (message) => (message.type === "inlineMenu.getState" ? state : null);
    const wrapper = await mountMenu({ pollIntervalMilliseconds: 100 });
    expect(wrapper.find('[data-testid="inline-status"]').text()).toBe("Hisar is locked");
    await wrapper.find('[data-testid="inline-action"]').trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "inlineMenu.unlock", sessionID: "session-1" });
    expect(wrapper.find('[data-testid="inline-action"]').text()).toBe("Waiting for Hisar…");
    state = createState();
    await vi.advanceTimersByTimeAsync(100);
    await flushPromises();
    expect(wrapper.findAll('[data-testid="inline-login"]')).toHaveLength(2);
  });

  it("opens Hisar when it is not running and shows the new state", async () => {
    respondWith(createState({ view: "desktop-unavailable", allLogin: [] }), { "inlineMenu.openHisar": () => createState() });
    const wrapper = await mountMenu();
    expect(wrapper.find('[data-testid="inline-action"]').text()).toBe("Open Hisar");
    await wrapper.find('[data-testid="inline-action"]').trigger("click");
    await flushPromises();
    expect(wrapper.findAll('[data-testid="inline-login"]')).toHaveLength(2);
  });

  it("says to sign in when Hisar is signed out, with nothing to click", async () => {
    respondWith(createState({ view: "signed-out", allLogin: [] }));
    const wrapper = await mountMenu();
    expect(wrapper.find('[data-testid="inline-status"]').text()).toBe("Sign in to Hisar on this computer");
    expect(wrapper.find('[data-testid="inline-action"]').exists()).toBe(false);
  });

  it("moves the selection with relayed arrow keys and fills on Enter", async () => {
    respondWith(createState({ hasMoreLogin: true }));
    const wrapper = await mountMenu();
    broadcast("Enter");
    broadcast("ArrowDown", "another-session");
    await flushPromises();
    expect(wrapper.findAll(".selected")).toHaveLength(0);
    broadcast("ArrowDown");
    broadcast("ArrowDown");
    await flushPromises();
    expect(wrapper.findAll('[data-testid="inline-login"]')[1].classes()).toContain("selected");
    broadcast("ArrowDown");
    broadcast("ArrowDown");
    broadcast("ArrowUp");
    await flushPromises();
    expect(wrapper.find('[data-testid="inline-more"]').classes()).toContain("selected");
    broadcast("ArrowUp");
    broadcast("Enter");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "inlineMenu.fillLogin", sessionID: "session-1", vaultID: "v2", itemID: "i2" });
    expect(allMessage.filter((message) => message.type === "inlineMenu.fillLogin")).toHaveLength(1);
  });

  it("closes on Escape pressed inside the menu", async () => {
    respondWith(createState());
    await mountMenu();
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "inlineMenu.close", sessionID: "session-1" });
  });

  it("shows errors from the service worker", async () => {
    respondWith(createState(), {
      "inlineMenu.fillLogin": () => {
        throw new Error("The menu was not fully visible. Try again");
      },
    });
    const wrapper = await mountMenu();
    await wrapper.find('[data-testid="inline-login"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="inline-error"]').text()).toBe("The menu was not fully visible. Try again");
  });
});
