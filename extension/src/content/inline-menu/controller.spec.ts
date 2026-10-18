import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ContentToBackgroundMessage } from "../../shared/messages";
import addressAutocompletePage from "../fixtures/address-autocomplete.html?raw";
import signUpPage from "../fixtures/sign-up.html?raw";
import simpleLoginPage from "../fixtures/simple-login.html?raw";
import { InlineMenuController, type InlineMenuDependencies } from "./controller";
import type { Rect } from "./placement";

let allSent: ContentToBackgroundMessage[];
let allFrameCallback: (() => void)[];
let now: number;
let fieldRect: Rect;
let elementAtPoint: () => Element | null;
let controller: InlineMenuController;
let sessionCounter: number;

function getInput(selector: string): HTMLInputElement {
  return document.querySelector<HTMLInputElement>(selector)!;
}

function createController(overrides: Partial<InlineMenuDependencies> = {}, window: Window = globalThis.window): InlineMenuController {
  const dependencies: InlineMenuDependencies = {
    sendToBackground: (async (message: ContentToBackgroundMessage) => {
      allSent.push(message);
      if (message.type === "content.openInlineMenu") {
        sessionCounter += 1;
        return { ok: true, result: { sessionID: `session-${sessionCounter}`, menuUrl: `chrome-extension://dynamic/inline-menu.html#session-${sessionCounter}` } };
      }
      return { ok: true, result: null };
    }) as InlineMenuDependencies["sendToBackground"],
    now: () => now,
    requestFrame: (callback) => allFrameCallback.push(callback),
    cancelFrame: () => undefined,
    getRect: (element) => (element.tagName === "IFRAME" ? { left: 10, top: 60, width: 300, height: 120 } : fieldRect),
    getElementAtPoint: () => elementAtPoint(),
    isTrustedEvent: () => true,
    ...overrides,
  };
  return new InlineMenuController(document, window, dependencies);
}

function runFrames(count = 1, step = 50): void {
  for (let index = 0; index < count; index += 1) {
    now += step;
    const allCallback = allFrameCallback.splice(0);
    for (const callback of allCallback) callback();
  }
}

function getHost(): Element | undefined {
  return (controller.getIconElement()?.getRootNode() as ShadowRoot | undefined)?.host;
}

function pressKey(field: HTMLElement, key: string): KeyboardEvent {
  const event = new KeyboardEvent("keydown", { key, bubbles: true, composed: true, cancelable: true });
  field.dispatchEvent(event);
  return event;
}

beforeEach(() => {
  allSent = [];
  allFrameCallback = [];
  now = 1000;
  sessionCounter = 0;
  fieldRect = { left: 10, top: 20, width: 300, height: 36 };
  document.body.innerHTML = simpleLoginPage;
  elementAtPoint = () => getHost() ?? null;
  controller = createController();
  controller.start();
});

afterEach(() => {
  controller.stop();
  document.documentElement.style.opacity = "";
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("InlineMenuController", () => {
  it("opens the menu at the focused field when asked from the right-click menu, even where it does not open on focus", async () => {
    controller.updateSettings({ isInlineMenuEnabled: true, isInlineMenuOpenedOnFocus: false });
    getInput("#login_field").focus();
    expect(allSent).toEqual([]);
    expect(controller.openHere()).toBe(true);
    await vi.waitFor(() => expect(allSent).toContainEqual({ type: "content.openInlineMenu", purpose: "login" }));
    document.body.innerHTML = "<input id='search' type='search'>";
    getInput("#search").focus();
    expect(controller.openHere()).toBe(false);
  });

  it("offers addresses only when the app fills them", async () => {
    document.body.innerHTML = addressAutocompletePage;
    getInput("#ship-city").focus();
    expect(controller.getIconElement()).toBeUndefined();
    expect(allSent).toEqual([]);

    getInput("#ship-city").blur();
    controller.setAddressFillAvailable(true);
    getInput("#ship-city").focus();
    await vi.waitFor(() => expect(allSent).toContainEqual({ type: "content.openInlineMenu", purpose: "address" }));
    expect(controller.getIconElement()).toBeDefined();

    controller.setAddressFillAvailable(false);
    expect(controller.getIconElement()).toBeUndefined();
  });

  it("shows the icon in a focused login field and opens the menu from the service worker's URL", async () => {
    getInput("#password").focus();
    expect(controller.getIconElement()).toBeDefined();
    expect(getHost()?.tagName).toBe("HISAR-INLINE-MENU");
    expect(getHost()?.shadowRoot).toBeNull();
    await vi.waitFor(() => expect(controller.getMenuElement()).toBeDefined());
    expect(allSent[0]).toEqual({ type: "content.openInlineMenu", purpose: "login" });
    expect(controller.getMenuElement()?.src).toBe("chrome-extension://dynamic/inline-menu.html#session-1");
  });

  it("does not open the menu on a field that a fill focuses", async () => {
    controller.runWithoutOpeningOnFocus(() => getInput("#password").focus());
    expect(controller.getIconElement()).toBeDefined();
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(allSent).toEqual([]);
    getInput("#login_field").focus();
    await vi.waitFor(() => expect(controller.getMenuElement()).toBeDefined());
  });

  it("places the icon at the right edge of the field and the menu under it", async () => {
    getInput("#login_field").focus();
    await vi.waitFor(() => expect(controller.getMenuElement()).toBeDefined());
    const icon = controller.getIconElement()!;
    expect([icon.style.left, icon.style.top]).toEqual(["282px", "28px"]);
    const menu = controller.getMenuElement()!;
    expect([menu.style.left, menu.style.top, menu.style.width, menu.style.height]).toEqual(["10px", "60px", "300px", "120px"]);
    controller.resizeMenu("session-1", 200);
    expect(menu.style.height).toBe("200px");
  });

  it("asks for a suggested password on a sign-up password field", async () => {
    controller.stop();
    document.body.innerHTML = signUpPage;
    controller = createController();
    controller.start();
    getInput("#new-password").focus();
    await vi.waitFor(() => expect(allSent).toContainEqual({ type: "content.openInlineMenu", purpose: "new-password" }));
    getInput("[name=email]").focus();
    await vi.waitFor(() => expect(controller.getIconElement()).toBeUndefined());
  });

  it("keeps one menu for split code boxes, placed under all of them, while the focus moves between boxes", async () => {
    vi.useFakeTimers();
    controller.stop();
    document.body.innerHTML = `<div>${Array.from({ length: 6 }, (_, index) => `<input id="box-${index}" maxlength="1" />`).join("")}</div>`;
    controller = createController({
      getRect: (element) =>
        element.tagName === "IFRAME" ? { left: 10, top: 60, width: 300, height: 120 } : { left: 10 + Number(element.id.slice(4)) * 50, top: 20, width: 40, height: 40 },
    });
    controller.start();
    getInput("#box-0").focus();
    await vi.waitFor(() => expect(controller.getMenuElement()).toBeDefined());
    expect(allSent[0]).toEqual({ type: "content.openInlineMenu", purpose: "one-time-code" });
    expect(controller.getIconElement()!.style.left).toBe(`${10 + 290 + 8}px`);
    getInput("#box-1").focus();
    vi.runAllTimers();
    expect(controller.getMenuElement()).toBeDefined();
    expect(controller.getAnchoredField("session-1")).toBe(getInput("#box-0"));
    expect(allSent.filter((message) => message.type === "content.openInlineMenu")).toHaveLength(1);
    getInput("#box-1").dispatchEvent(new InputEvent("input", { bubbles: true, composed: true }));
    expect(controller.getMenuElement()).toBeUndefined();
  });

  it("shows only the icon when opening on focus is off, and opens on ArrowDown", async () => {
    controller.updateSettings({ isInlineMenuEnabled: true, isInlineMenuOpenedOnFocus: false });
    const field = getInput("#login_field");
    field.focus();
    expect(controller.getIconElement()).toBeDefined();
    await Promise.resolve();
    expect(controller.getMenuElement()).toBeUndefined();
    expect(pressKey(field, "ArrowDown").defaultPrevented).toBe(true);
    await vi.waitFor(() => expect(controller.getMenuElement()).toBeDefined());
  });

  it("shows nothing when the setting is off and hides an open menu when it is turned off", async () => {
    getInput("#login_field").focus();
    await vi.waitFor(() => expect(controller.getMenuElement()).toBeDefined());
    controller.updateSettings({ isInlineMenuEnabled: false, isInlineMenuOpenedOnFocus: true });
    expect(controller.getIconElement()).toBeUndefined();
    expect(allSent).toContainEqual({ type: "content.closeInlineMenu", sessionID: "session-1" });
    getInput("#password").focus();
    expect(controller.getIconElement()).toBeUndefined();
  });

  it("does nothing in frames that the frame rule would never fill", () => {
    controller.stop();
    const crossOriginWindow = {
      location: { protocol: "https:", origin: "https://login.example.net", ancestorOrigins: ["https://example.com"] },
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      innerWidth: 1000,
      innerHeight: 800,
    } as unknown as Window;
    controller = createController({}, crossOriginWindow);
    controller.start();
    getInput("#password").focus();
    expect(controller.getIconElement()).toBeUndefined();
  });

  it("ignores untrusted clicks on the icon and untrusted keys", async () => {
    controller.stop();
    controller = createController({ isTrustedEvent: (event) => event.isTrusted });
    controller.start();
    controller.updateSettings({ isInlineMenuEnabled: true, isInlineMenuOpenedOnFocus: false });
    const field = getInput("#login_field");
    field.focus();
    controller.getIconElement()!.click();
    expect(pressKey(field, "ArrowDown").defaultPrevented).toBe(false);
    await Promise.resolve();
    expect(allSent).toEqual([]);
  });

  it("toggles the menu with the icon and keeps the focus in the field", async () => {
    controller.updateSettings({ isInlineMenuEnabled: true, isInlineMenuOpenedOnFocus: false });
    const field = getInput("#login_field");
    field.focus();
    const mouseDown = new MouseEvent("mousedown", { bubbles: true, cancelable: true });
    controller.getIconElement()!.dispatchEvent(mouseDown);
    expect(mouseDown.defaultPrevented).toBe(true);
    controller.getIconElement()!.click();
    await vi.waitFor(() => expect(controller.getMenuElement()).toBeDefined());
    controller.getIconElement()!.click();
    expect(controller.getMenuElement()).toBeUndefined();
    expect(document.activeElement).toBe(field);
  });

  it("relays arrow keys, and Enter only after an arrow key selected an entry", async () => {
    const field = getInput("#login_field");
    field.focus();
    await vi.waitFor(() => expect(controller.getMenuElement()).toBeDefined());
    expect(pressKey(field, "Enter").defaultPrevented).toBe(false);
    expect(pressKey(field, "ArrowDown").defaultPrevented).toBe(true);
    expect(pressKey(field, "Enter").defaultPrevented).toBe(true);
    await Promise.resolve();
    expect(allSent.filter((message) => message.type === "content.pressInlineMenuKey")).toEqual([
      { type: "content.pressInlineMenuKey", sessionID: "session-1", key: "ArrowDown" },
      { type: "content.pressInlineMenuKey", sessionID: "session-1", key: "Enter" },
    ]);
  });

  it("closes the menu on Escape, typing, and a click outside", async () => {
    const field = getInput("#login_field");
    field.focus();
    await vi.waitFor(() => expect(controller.getMenuElement()).toBeDefined());
    expect(pressKey(field, "Escape").defaultPrevented).toBe(true);
    expect(controller.getMenuElement()).toBeUndefined();
    expect(controller.getIconElement()).toBeDefined();

    await controller.openMenu();
    field.dispatchEvent(new InputEvent("input", { bubbles: true, composed: true }));
    expect(controller.getMenuElement()).toBeUndefined();

    await controller.openMenu();
    document.querySelector("h1")!.dispatchEvent(new Event("pointerdown", { bubbles: true, composed: true }));
    expect(controller.getMenuElement()).toBeUndefined();
  });

  it("does not open the menu when the user typed while it was loading", async () => {
    controller.updateSettings({ isInlineMenuEnabled: true, isInlineMenuOpenedOnFocus: false });
    const field = getInput("#login_field");
    field.focus();
    const opening = controller.openMenu();
    field.dispatchEvent(new InputEvent("input", { bubbles: true, composed: true }));
    await opening;
    expect(controller.getMenuElement()).toBeUndefined();
    expect(allSent).toContainEqual({ type: "content.closeInlineMenu", sessionID: "session-1" });
  });

  it("hides everything when the focus leaves the field and on pagehide", async () => {
    vi.useFakeTimers();
    getInput("#login_field").focus();
    document.querySelector<HTMLInputElement>("[name=q]")!.focus();
    vi.runAllTimers();
    expect(controller.getIconElement()).toBeUndefined();
    getInput("#password").focus();
    expect(controller.getIconElement()).toBeDefined();
    window.dispatchEvent(new Event("pagehide"));
    expect(controller.getIconElement()).toBeUndefined();
  });

  it("hides when the field scrolls out of view", async () => {
    vi.spyOn(document.documentElement, "getClientRects").mockReturnValue([{}] as unknown as DOMRectList);
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({ left: 0, top: 0, width: 300, height: 36 } as DOMRect);
    getInput("#password").focus();
    expect(controller.getIconElement()).toBeDefined();
    fieldRect = { left: 10, top: -100, width: 300, height: 36 };
    runFrames();
    expect(controller.getIconElement()).toBeUndefined();
  });

  it("allows clicks in the menu only after it has been visible and uncovered for 300 ms", async () => {
    getInput("#password").focus();
    await vi.waitFor(() => expect(controller.getMenuElement()).toBeDefined());
    runFrames(1);
    expect(controller.isMenuInteractionAllowed("session-1")).toBe(false);
    runFrames(6);
    expect(controller.isMenuInteractionAllowed("session-1")).toBe(true);
    expect(controller.isMenuInteractionAllowed("another-session")).toBe(false);
  });

  it("refuses clicks while something covers the menu", async () => {
    getInput("#password").focus();
    await vi.waitFor(() => expect(controller.getMenuElement()).toBeDefined());
    runFrames(8);
    expect(controller.isMenuInteractionAllowed("session-1")).toBe(true);
    elementAtPoint = () => document.body;
    runFrames(1);
    expect(controller.isMenuInteractionAllowed("session-1")).toBe(false);
    elementAtPoint = () => getHost() ?? null;
    runFrames(2);
    expect(controller.isMenuInteractionAllowed("session-1")).toBe(false);
    runFrames(6);
    expect(controller.isMenuInteractionAllowed("session-1")).toBe(true);
  });

  it("refuses clicks while the page makes the menu transparent", async () => {
    getInput("#password").focus();
    await vi.waitFor(() => expect(controller.getMenuElement()).toBeDefined());
    document.documentElement.style.opacity = "0.2";
    runFrames(10);
    expect(controller.isMenuInteractionAllowed("session-1")).toBe(false);
  });

  it("starts the visibility wait again after a resize", async () => {
    getInput("#password").focus();
    await vi.waitFor(() => expect(controller.getMenuElement()).toBeDefined());
    runFrames(8);
    controller.resizeMenu("session-1", 160);
    expect(controller.isMenuInteractionAllowed("session-1")).toBe(false);
  });

  it("returns the anchored field only for the current session and dismisses it", async () => {
    getInput("#password").focus();
    await vi.waitFor(() => expect(controller.getMenuElement()).toBeDefined());
    expect(controller.getAnchoredField("session-1")).toBe(getInput("#password"));
    expect(controller.getAnchoredField("session-2")).toBeUndefined();
    controller.dismissMenu("session-1");
    expect(controller.getMenuElement()).toBeUndefined();
    expect(controller.getAnchoredField("session-1")).toBeUndefined();
  });
});
