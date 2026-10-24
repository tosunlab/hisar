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

let scrollIntoView: ReturnType<typeof vi.fn<Element["scrollIntoView"]>>;

beforeEach(() => {
  // jsdom does not lay out, so it has no scrollIntoView.
  scrollIntoView = vi.fn<Element["scrollIntoView"]>();
  Element.prototype.scrollIntoView = scrollIntoView;
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

  it("offers the one-time codes of matching logins in a code field and fills the chosen one", async () => {
    respondWith(createState({ purpose: "one-time-code", allLogin: [{ ...allLogin[0], hasOneTimePassword: true }] }));
    const wrapper = await mountMenu();
    expect(wrapper.find('[data-testid="inline-login"]').exists()).toBe(false);
    const row = wrapper.get('[data-testid="inline-code"]');
    expect(row.text()).toBe("ExampleOne-time code · alex@example.com");
    broadcast("ArrowDown");
    broadcast("Enter");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "inlineMenu.fillOneTimeCode", sessionID: "session-1", vaultID: "v1", itemID: "i1" });
    expect(allMessage.some((message) => message.type === "inlineMenu.fillLogin")).toBe(false);
  });

  it("says when no login for the site has a one-time code", async () => {
    respondWith(createState({ purpose: "one-time-code", allLogin: [] }));
    const wrapper = await mountMenu();
    expect(wrapper.get('[data-testid="inline-no-codes"]').text()).toContain("No one-time codes for example.com");
  });

  it("reports its whole height so the page can size the frame, even while it is cut and scrolls", async () => {
    // 95 px of content and padding scrolled inside a 60 px menu, with a 1 px border above and below.
    vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockReturnValue(95);
    vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockReturnValue(60);
    vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockReturnValue(58);
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

  it("refuses clicks with a message until visibility tracking has seen the menu for 300 ms, and never runs them later", async () => {
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
    expect(wrapper.find('[data-testid="inline-error"]').text()).toBe("Hisar is making sure this menu is visible. Choose again");
    now = 300;
    await flushPromises();
    expect(allMessage.some((message) => message.type === "inlineMenu.fillLogin")).toBe(false);
    await wrapper.find('[data-testid="inline-login"]').trigger("click");
    await flushPromises();
    expect(allMessage.filter((message) => message.type === "inlineMenu.fillLogin")).toHaveLength(1);
    reportVisibility(false);
    now = 1000;
    await wrapper.find('[data-testid="inline-login"]').trigger("click");
    await flushPromises();
    expect(allMessage.filter((message) => message.type === "inlineMenu.fillLogin")).toHaveLength(1);
  });

  it("fills once however many times the login is clicked while the fill runs", async () => {
    let finishFill: (() => void) | undefined;
    respondWith(createState(), { "inlineMenu.fillLogin": () => new Promise<null>((resolve) => (finishFill = () => resolve(null))) });
    const wrapper = await mountMenu();
    await wrapper.find('[data-testid="inline-login"]').trigger("click");
    await wrapper.find('[data-testid="inline-login"]').trigger("click");
    expect(wrapper.find(".menu").attributes("aria-busy")).toBe("true");
    finishFill?.();
    await flushPromises();
    expect(allMessage.filter((message) => message.type === "inlineMenu.fillLogin")).toHaveLength(1);
    expect(wrapper.find(".menu").attributes("aria-busy")).toBe("false");
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

  it("says Hisar or the extension needs an update and opens the popup for details", async () => {
    const requirement = { currentVersion: "0.1.5", minimumVersion: "0.1.6", updateURL: "https://tryhisar.com/download" };
    respondWith(createState({ view: "update-required", allLogin: [], updateRequired: { target: "app", ...requirement } }));
    const wrapper = await mountMenu();
    expect(wrapper.find('[data-testid="inline-status"]').text()).toBe("Hisar needs an update");
    expect(wrapper.find('[data-testid="inline-action"]').text()).toBe("Show details…");
    await wrapper.find('[data-testid="inline-action"]').trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "inlineMenu.openPopup", sessionID: "session-1" });
    wrapper.unmount();

    respondWith(createState({ view: "update-required", allLogin: [], updateRequired: { target: "extension", ...requirement } }));
    const extensionWrapper = await mountMenu();
    expect(extensionWrapper.find('[data-testid="inline-status"]').text()).toBe("Update the Hisar extension");
    expect(extensionWrapper.findAll('[data-testid="inline-login"]')).toHaveLength(0);
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
    // A menu cut to fit the window scrolls the chosen entry into view.
    expect(scrollIntoView).toHaveBeenLastCalledWith({ block: "nearest" });
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

describe("addresses", () => {
  const allAddress = [
    { vaultID: "v1", itemID: "home", title: "Home", summary: "Springfield, US", source: "address" as const, vaultName: "Personal" },
    { vaultID: "v1", itemID: "alice", title: "Alice", summary: "", source: "identity" as const, vaultName: "Personal" },
  ];

  it("lists addresses on an address field and fills the one chosen by click or keyboard", async () => {
    respondWith(createState({ purpose: "address", allLogin: [], allAddress, hasMoreAddress: false }));
    const wrapper = await mountMenu();
    const allRow = wrapper.findAll('[data-testid="inline-address"]');
    expect(allRow.map((row) => row.text())).toEqual(["HomeSpringfield, US", "AliceIdentity"]);
    await allRow[0].trigger("click");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "inlineMenu.fillAddress", sessionID: "session-1", vaultID: "v1", itemID: "home" });

    broadcast("ArrowDown");
    broadcast("ArrowDown");
    broadcast("Enter");
    await flushPromises();
    expect(allMessage).toContainEqual({ type: "inlineMenu.fillAddress", sessionID: "session-1", vaultID: "v1", itemID: "alice" });
  });

  it("says when Hisar has no addresses", async () => {
    respondWith(createState({ purpose: "address", allLogin: [], allAddress: [] }));
    const wrapper = await mountMenu();
    expect(wrapper.find('[data-testid="inline-no-addresses"]').text()).toContain("No addresses in Hisar");
    expect(wrapper.find('[data-testid="inline-no-logins"]').exists()).toBe(false);
  });
});
