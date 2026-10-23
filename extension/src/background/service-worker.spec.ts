import { createHash } from "node:crypto";
import { afterEach, expect, it, vi } from "vitest";
import { createInitialState, respond } from "../../tools/fake-native-host.mjs";
import { createMemoryArea, FakePort } from "./testing";

type Listener = (...allArgument: never[]) => unknown;

function createEvent() {
  const allListener: Listener[] = [];
  return {
    addListener: (listener: Listener) => void allListener.push(listener),
    dispatch: (...allArgument: unknown[]) => allListener.map((listener) => (listener as (...allArgument: unknown[]) => unknown)(...allArgument)),
  };
}

const extensionID = "hisar-extension";
const token = "pairing-token";

// startWokenWorker loads the service worker into a browser whose restart cleanup has not answered yet, as when a
// submission wakes a stopped worker.
async function startWokenWorker() {
  const state = createInitialState();
  state.allPairing.push({ instanceID: "instance-1", browser: "Chrome", tokenHash: createHash("sha256").update(token).digest("hex"), pairedAt: Date.now() });
  const local = createMemoryArea();
  await local.set({ instanceID: "instance-1", token });
  const allAlarm = Promise.withResolvers<{ name: string }[]>();
  const allBadge = new Map<number, string>();
  const allRequestType: unknown[] = [];
  const allHeldType = new Set<string>();
  const allHeld: { type: string; release: () => void }[] = [];
  const frame = { url: "https://example.com/login", documentId: "login-document" };
  const browser = {
    runtime: {
      id: extensionID,
      lastError: undefined,
      onMessage: createEvent(),
      onConnect: createEvent(),
      onInstalled: createEvent(),
      getURL: (path: string) => `chrome-extension://${extensionID}/${path}`,
      getManifest: () => ({ version: "0.3.0" }),
      connectNative: () => {
        const port: FakePort = new FakePort((request) => {
          allRequestType.push(request.type);
          const response = respond(state, request, Date.now());
          if (!allHeldType.has(request.type as string)) return response;
          allHeld.push({ type: request.type as string, release: () => port.deliver(response) });
          return undefined;
        });
        return port;
      },
      sendMessage: async () => undefined,
    },
    storage: { local, session: createMemoryArea() },
    action: {
      setBadgeBackgroundColor: async () => undefined,
      setBadgeText: async ({ tabId, text }: { tabId: number; text: string }) => void allBadge.set(tabId, text),
      getBadgeText: async ({ tabId }: { tabId: number }) => allBadge.get(tabId) ?? "",
    },
    alarms: { create: async () => undefined, clear: async () => true, getAll: () => allAlarm.promise, onAlarm: createEvent() },
    tabs: {
      query: async () => [{ id: 1 }],
      get: async (tabId: number) => ({ id: tabId, url: frame.url, windowId: 1, index: 0 }),
      onUpdated: createEvent(),
      onActivated: createEvent(),
      onRemoved: createEvent(),
    },
    webNavigation: {
      getFrame: async () => ({ documentId: frame.documentId }),
      onCommitted: createEvent(),
      onHistoryStateUpdated: createEvent(),
    },
    commands: { onCommand: createEvent() },
    contextMenus: { onClicked: createEvent(), removeAll: () => undefined, create: () => undefined },
  };
  vi.stubGlobal("chrome", browser);
  vi.resetModules();
  // The worker registers its listeners when it loads, so each test loads a fresh copy into its own browser.
  await import("./service-worker");
  return {
    allBadge,
    allRequestType,
    submit(password: string, username = "new-user") {
      const sendResponse = vi.fn();
      browser.runtime.onMessage.dispatch(
        { type: "content.loginSubmitted", username, password },
        { id: extensionID, url: "https://example.com/login", tab: { id: 1, url: "https://example.com/login" }, frameId: 0, documentId: "login-document" },
        sendResponse,
      );
      return sendResponse;
    },
    navigate(url: string, documentId: string) {
      Object.assign(frame, { url, documentId });
      browser.webNavigation.onCommitted.dispatch({ tabId: 1, url, frameId: 0, documentId });
    },
    hold(type: string) {
      allHeldType.add(type);
    },
    stopHolding(type: string) {
      allHeldType.delete(type);
    },
    release(type: string) {
      const index = allHeld.findIndex((held) => held.type === type);
      allHeld.splice(index, 1)[0].release();
    },
    isHeld: (type: string) => allHeld.some((held) => held.type === type),
    completeLoad(url = frame.url) {
      frame.url = url;
      browser.tabs.onUpdated.dispatch(1, { status: "complete" });
    },
    finishRestartCleanup: () => allAlarm.resolve([]),
  };
}

function settle(): Promise<void> {
  const { promise, resolve } = Promise.withResolvers<void>();
  setTimeout(resolve, 50);
  return promise;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

it("offers a save when the submit navigation reaches a woken worker before its restart cleanup finishes", async () => {
  const worker = await startWokenWorker();
  const sendResponse = worker.submit("woken password");
  worker.navigate("https://example.com/welcome", "welcome-document");
  worker.finishRestartCleanup();
  await vi.waitFor(() => expect(sendResponse).toHaveBeenCalledWith({ ok: true, result: null }));
  await vi.waitFor(() => expect(worker.allBadge.get(1)).toBe("+"));
  expect(worker.allRequestType).toContain("logins.prepare");
});

it("still refuses a submission whose document was replaced before the browser delivered it", async () => {
  const worker = await startWokenWorker();
  worker.navigate("https://example.com/welcome", "welcome-document");
  const sendResponse = worker.submit("late password");
  worker.finishRestartCleanup();
  await vi.waitFor(() => expect(sendResponse).toHaveBeenCalledWith({ ok: true, result: null }));
  await settle();
  expect(worker.allRequestType).not.toContain("logins.prepare");
  expect(worker.allBadge.get(1) ?? "").toBe("");
});

it("keeps the save badge when a login count asked for before the offer arrives after it", async () => {
  const worker = await startWokenWorker();
  worker.finishRestartCleanup();
  worker.hold("logins.prepare");
  worker.submit("new password");
  await vi.waitFor(() => expect(worker.isHeld("logins.prepare")).toBe(true));
  worker.hold("logins.match");
  worker.completeLoad();
  await vi.waitFor(() => expect(worker.isHeld("logins.match")).toBe(true));
  worker.release("logins.prepare");
  await vi.waitFor(() => expect(worker.allBadge.get(1)).toBe("+"));
  worker.release("logins.match");
  await settle();
  expect(worker.allBadge.get(1)).toBe("+");
});

it("shows the login count of the page the tab shows now, not of the page it left", async () => {
  const worker = await startWokenWorker();
  worker.finishRestartCleanup();
  worker.submit("correct horse battery staple", "alex@example.com");
  await vi.waitFor(() => expect(worker.allRequestType).toContain("logins.prepare"));
  worker.hold("logins.match");
  worker.completeLoad();
  await vi.waitFor(() => expect(worker.isHeld("logins.match")).toBe(true));
  worker.stopHolding("logins.match");
  worker.completeLoad("https://other.example/");
  await vi.waitFor(() => expect(worker.allRequestType.filter((type) => type === "logins.match")).toHaveLength(2));
  await settle();
  worker.release("logins.match");
  await settle();
  expect(worker.allBadge.get(1) ?? "").toBe("");
});
