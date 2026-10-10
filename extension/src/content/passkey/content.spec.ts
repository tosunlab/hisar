import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  passkeyCancelEventName,
  passkeyRequestEventName,
  passkeyResponseEventName,
  type BackgroundToContentPasskeyMessage,
  type ContentToBackgroundPasskeyMessage,
  type PasskeyPageRequest,
  type PasskeyPageResponse,
} from "../../shared/passkey";
import { isWebAuthnField, keepAliveMilliseconds, PasskeyContentBridge, type PasskeyContentPort } from "./content";
import { computeTopRightRect, PasskeyPromptFrame, promptHostTagName } from "./prompt-frame";

class FakeContentPort implements PasskeyContentPort {
  readonly allSent: ContentToBackgroundPasskeyMessage[] = [];
  private readonly allMessageListener: ((message: BackgroundToContentPasskeyMessage) => void)[] = [];
  private readonly allDisconnectListener: (() => void)[] = [];
  isDisconnected = false;
  readonly onMessage = { addListener: (listener: (message: BackgroundToContentPasskeyMessage) => void) => this.allMessageListener.push(listener) };
  readonly onDisconnect = { addListener: (listener: () => void) => this.allDisconnectListener.push(listener) };

  postMessage(message: ContentToBackgroundPasskeyMessage): void {
    this.allSent.push(message);
  }

  disconnect(): void {
    this.isDisconnected = true;
  }

  deliver(message: BackgroundToContentPasskeyMessage): void {
    for (const listener of this.allMessageListener) listener(message);
  }

  simulateDisconnect(): void {
    for (const listener of this.allDisconnectListener) listener();
  }
}

let allPort: FakeContentPort[];
let allResponse: PasskeyPageResponse[];
let isPolicyAllowed: boolean;
let elementAtPoint: Element | null;
let now: number;
let bridge: PasskeyContentBridge;

const getRequest: PasskeyPageRequest = { requestID: "r1", kind: "get", options: { challenge: "AQ", allowCredentials: [] } };
const promptUrl = "chrome-extension://dynamic/passkey-prompt.html#s1";

function sendRequest(request: PasskeyPageRequest): void {
  document.dispatchEvent(new CustomEvent(passkeyRequestEventName, { detail: JSON.stringify(request) }));
}

function findHost(): Element | null {
  return document.querySelector(promptHostTagName);
}

beforeEach(() => {
  vi.useFakeTimers();
  allPort = [];
  allResponse = [];
  isPolicyAllowed = false;
  now = 0;
  elementAtPoint = null;
  document.body.innerHTML = '<input id="user" autocomplete="username webauthn"><input id="other">';
  document.addEventListener(passkeyResponseEventName, recordResponse);
  bridge = new PasskeyContentBridge(document, {
    connect: () => {
      const port = new FakeContentPort();
      allPort.push(port);
      return port;
    },
    createPromptFrame: () =>
      new PasskeyPromptFrame(document, window, {
        now: () => now,
        requestFrame: () => 0,
        cancelFrame: () => undefined,
        getRect: (element) => (element.tagName === "IFRAME" ? { left: 600, top: 12, width: 360, height: 180 } : { left: 0, top: 0, width: 100, height: 20 }),
        getElementAtPoint: () => elementAtPoint,
      }),
    getAllAncestorOrigin: () => ["https://top.example"],
    isPermissionPolicyAllowed: (feature) => isPolicyAllowed && feature.startsWith("publickey-credentials"),
    setInterval: (callback, milliseconds) => setInterval(callback, milliseconds),
    clearInterval: (handle) => clearInterval(handle),
  });
  bridge.start();
});

afterEach(() => {
  bridge.stop();
  document.removeEventListener(passkeyResponseEventName, recordResponse);
  vi.useRealTimers();
  document.documentElement.querySelectorAll(promptHostTagName).forEach((host) => host.remove());
});

function recordResponse(event: Event): void {
  allResponse.push(JSON.parse(String((event as CustomEvent).detail)) as PasskeyPageResponse);
}

describe("PasskeyContentBridge", () => {
  it("accepts the request, forwards it with the frame facts and shows the prompt", () => {
    isPolicyAllowed = true;
    sendRequest(getRequest);
    expect(allResponse).toEqual([{ requestID: "r1", outcome: "accepted" }]);
    expect(allPort[0].allSent[0]).toEqual({ type: "start", request: getRequest, allAncestorOrigin: ["https://top.example"], isPermissionPolicyAllowed: true });
    allPort[0].deliver({ type: "prompt", promptUrl, isConditional: false });
    expect(findHost()).not.toBeNull();
    allPort[0].deliver({ type: "credential", credential: { kind: "get", credentialId: "AQ", clientDataJSON: "e30", authenticatorData: "AA", signature: "AA", userHandle: "AA" } });
    expect(allResponse.at(-1)).toMatchObject({ requestID: "r1", outcome: "credential", credential: { credentialId: "AQ" } });
    expect(findHost()).toBeNull();
    expect(allPort[0].isDisconnected).toBe(true);
  });

  it("answers fallback, errors and availability, and falls back when the service worker goes away", () => {
    sendRequest(getRequest);
    allPort[0].deliver({ type: "fallback" });
    sendRequest({ ...getRequest, requestID: "r2" });
    allPort[1].deliver({ type: "error", name: "NotAllowedError", message: "closed" });
    sendRequest({ ...getRequest, requestID: "r3" });
    allPort[2].simulateDisconnect();
    sendRequest({ requestID: "r4", kind: "availability" });
    allPort[3].deliver({ type: "availability", isAvailable: true });
    expect(allResponse.filter((response) => response.outcome !== "accepted")).toEqual([
      { requestID: "r1", outcome: "fallback" },
      { requestID: "r2", outcome: "error", name: "NotAllowedError", message: "closed" },
      { requestID: "r3", outcome: "fallback" },
      { requestID: "r4", outcome: "availability", isAvailable: true },
    ]);
  });

  it("ignores malformed events", () => {
    document.dispatchEvent(new CustomEvent(passkeyRequestEventName, { detail: "{not json" }));
    document.dispatchEvent(new CustomEvent(passkeyRequestEventName, { detail: JSON.stringify({ kind: "get" }) }));
    document.dispatchEvent(new CustomEvent(passkeyRequestEventName, { detail: JSON.stringify({ requestID: "x", kind: "delete" }) }));
    expect(allPort).toHaveLength(0);
    expect(allResponse).toHaveLength(0);
  });

  it("keeps the service worker awake and cancels when the page aborts", () => {
    sendRequest(getRequest);
    allPort[0].deliver({ type: "prompt", promptUrl, isConditional: false });
    vi.advanceTimersByTime(keepAliveMilliseconds);
    expect(allPort[0].allSent.at(-1)).toEqual({ type: "keepAlive" });
    document.dispatchEvent(new CustomEvent(passkeyCancelEventName, { detail: JSON.stringify({ requestID: "r1" }) }));
    expect(allPort[0].allSent.at(-1)).toEqual({ type: "cancel" });
    expect(allPort[0].isDisconnected).toBe(true);
    expect(findHost()).toBeNull();
    const sentCount = allPort[0].allSent.length;
    vi.advanceTimersByTime(keepAliveMilliseconds * 2);
    expect(allPort[0].allSent).toHaveLength(sentCount);
  });

  it("ends an earlier request when a new one starts", () => {
    sendRequest(getRequest);
    sendRequest({ ...getRequest, requestID: "r2" });
    expect(allResponse).toContainEqual({ requestID: "r1", outcome: "error", name: "NotAllowedError", message: "Another passkey request started" });
    expect(allPort[0].isDisconnected).toBe(true);
  });

  it("shows conditional prompts only while a webauthn field has the focus", async () => {
    sendRequest({ ...getRequest, mediation: "conditional" } as PasskeyPageRequest);
    allPort[0].deliver({ type: "prompt", promptUrl, isConditional: true });
    expect(findHost()).toBeNull();
    (document.getElementById("other") as HTMLInputElement).focus();
    expect(findHost()).toBeNull();
    (document.getElementById("user") as HTMLInputElement).focus();
    expect(findHost()).not.toBeNull();
    (document.getElementById("other") as HTMLInputElement).focus();
    await vi.advanceTimersByTimeAsync(1);
    expect(findHost()).toBeNull();
    expect(allResponse).toEqual([{ requestID: "r1", outcome: "accepted" }]);
  });

  it("reports the prompt as visible only after 300 ms uncovered", () => {
    sendRequest(getRequest);
    allPort[0].deliver({ type: "prompt", promptUrl, isConditional: false });
    const frame = bridge.getPromptFrame();
    elementAtPoint = findHost();
    frame.updatePosition();
    allPort[0].deliver({ type: "checkVisibility", checkID: "c1" });
    now = 400;
    frame.updatePosition();
    allPort[0].deliver({ type: "checkVisibility", checkID: "c2" });
    elementAtPoint = document.body;
    frame.updatePosition();
    allPort[0].deliver({ type: "checkVisibility", checkID: "c3" });
    expect(allPort[0].allSent.filter((message) => message.type === "visibility")).toEqual([
      { type: "visibility", checkID: "c1", isVisible: false },
      { type: "visibility", checkID: "c2", isVisible: true },
      { type: "visibility", checkID: "c3", isVisible: false },
    ]);
    allPort[0].deliver({ type: "resize", height: 260 });
    expect(frame.getIframe()?.style.height).toBe("260px");
  });
});

describe("prompt placement and fields", () => {
  it("places the prompt at the top right and fits narrow viewports", () => {
    expect(computeTopRightRect(200, { width: 1200, height: 800 })).toEqual({ left: 828, top: 12, width: 360, height: 200 });
    expect(computeTopRightRect(200, { width: 320, height: 600 })).toEqual({ left: 12, top: 12, width: 296, height: 200 });
  });

  it("recognizes webauthn autocomplete tokens", () => {
    const field = document.createElement("input");
    field.setAttribute("autocomplete", "Username WebAuthn");
    expect(isWebAuthnField(field)).toBe(true);
    field.setAttribute("autocomplete", "username");
    expect(isWebAuthnField(field)).toBe(false);
    expect(isWebAuthnField(document.createElement("textarea"))).toBe(false);
  });

  it("refuses prompt URLs outside the extension", () => {
    const frame = new PasskeyPromptFrame(document, window, { now: () => 0, requestFrame: () => 0, cancelFrame: () => undefined, getRect: () => ({ left: 0, top: 0, width: 0, height: 0 }), getElementAtPoint: () => null });
    frame.show("https://evil.example/passkey-prompt.html");
    expect(frame.isShown()).toBe(false);
  });
});

describe("PasskeyContentBridge signals", () => {
  const signalRequest: PasskeyPageRequest = { requestID: "s1", kind: "signal", signal: { method: "signalUnknownCredential", rpId: "example.com", credentialId: "AQ" } };

  it("relays a signal without accepting it as a session or ending the active request", () => {
    sendRequest(getRequest);
    allPort[0].deliver({ type: "prompt", promptUrl, isConditional: false });
    sendRequest(signalRequest);
    expect(allPort[1].allSent[0]).toMatchObject({ type: "start", request: signalRequest });
    expect(allResponse).toEqual([{ requestID: "r1", outcome: "accepted" }]);
    allPort[1].deliver({ type: "signal" });
    expect(allResponse.at(-1)).toEqual({ requestID: "s1", outcome: "signal" });
    expect(allPort[1].isDisconnected).toBe(true);
    expect(allPort[0].isDisconnected).toBe(false);
    expect(findHost()).not.toBeNull();
  });

  it("passes the service worker's error on, and resolves when the port goes away", () => {
    sendRequest(signalRequest);
    allPort[0].deliver({ type: "signal", errorName: "SecurityError", message: "wrong rpId" });
    expect(allResponse.at(-1)).toEqual({ requestID: "s1", outcome: "signal", errorName: "SecurityError", message: "wrong rpId" });
    sendRequest({ ...signalRequest, requestID: "s2" });
    allPort[1].simulateDisconnect();
    expect(allResponse.at(-1)).toEqual({ requestID: "s2", outcome: "signal" });
  });
});
