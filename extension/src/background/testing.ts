import { expect } from "vitest";
import { createInitialState, respond, type FakeHostState } from "../../tools/fake-native-host.mjs";
import type { BackgroundToContentMessage, FrameProbe, InlineMenuBroadcast } from "../shared/messages";
import { HisarError } from "../shared/protocol";
import { BackgroundController, type BrowserBridge } from "./controller";
import type { FrameInfo } from "./frames";
import { NativeClient } from "./native-client";
import type { NativePort } from "./native-client";
import { ExtensionStorage } from "./storage";

export class FakePort implements NativePort {
  readonly allSent: Record<string, unknown>[] = [];
  private readonly allMessageListener: ((message: unknown) => void)[] = [];
  private readonly allDisconnectListener: (() => void)[] = [];
  isDisconnected = false;

  constructor(private readonly answer?: (request: Record<string, unknown>) => unknown) {}

  readonly onMessage = { addListener: (listener: (message: unknown) => void) => this.allMessageListener.push(listener) };
  readonly onDisconnect = { addListener: (listener: () => void) => this.allDisconnectListener.push(listener) };

  postMessage(message: unknown): void {
    if (this.isDisconnected) throw new Error("Attempting to use a disconnected port object");
    const request = message as Record<string, unknown>;
    this.allSent.push(request);
    if (!this.answer) return;
    const response = this.answer(request);
    if (response !== undefined) queueMicrotask(() => this.deliver(response));
  }

  disconnect(): void {
    this.isDisconnected = true;
  }

  deliver(message: unknown): void {
    for (const listener of this.allMessageListener) listener(message);
  }

  simulateDisconnect(): void {
    this.isDisconnected = true;
    for (const listener of this.allDisconnectListener) listener();
  }
}

export function createMemoryArea(): {
  data: Record<string, unknown>;
  get(allKey: string[]): Promise<Record<string, unknown>>;
  set(allEntry: Record<string, unknown>): Promise<void>;
  remove(allKey: string | string[]): Promise<void>;
} {
  const data: Record<string, unknown> = {};
  return {
    data,
    async get(allKey) {
      return Object.fromEntries(allKey.filter((key) => key in data).map((key) => [key, data[key]]));
    },
    async set(allEntry) {
      Object.assign(data, structuredClone(allEntry));
    },
    async remove(allKey) {
      for (const key of Array.isArray(allKey) ? allKey : [allKey]) delete data[key];
    },
  };
}

export const emptyProbe: FrameProbe = { hasLoginForm: false, hasUsernameField: false, hasFocusedField: false, hasFocusedLoginField: false };

export interface HarnessOptions {
  state?: Partial<FakeHostState>;
  allFrame?: FrameInfo[];
  allProbe?: Record<number, FrameProbe | "no-content-script">;
  tabUrl?: string;
  isHostMissing?: boolean;
  isMenuVisible?: boolean;
  isClipboardUnavailable?: boolean;
}

export function createHarness(options: HarnessOptions = {}) {
  const state: FakeHostState = { ...createInitialState(), pairingCode: "123456", unlockDelayMilliseconds: 1000, ...options.state };
  let currentTime = 1_000_000;
  let idCounter = 0;
  const allRequest: Record<string, unknown>[] = [];
  const allFrameMessage: { frameId: number; message: BackgroundToContentMessage }[] = [];
  const allBroadcast: InlineMenuBroadcast[] = [];
  const allOpenedPopupTab: number[] = [];
  const allClipboardText: string[] = [];
  let injectionCount = 0;
  const tabUrl = options.tabUrl ?? "https://example.com/login";
  const allFrame = options.allFrame ?? [{ frameId: 0, url: tabUrl, origin: new URL(tabUrl).origin, allAncestorOrigin: [] }];
  const allProbe = { ...(options.allProbe ?? { 0: { ...emptyProbe, hasLoginForm: true } }) };

  const bridge: BrowserBridge = {
    getTabUrl: async () => tabUrl,
    getAllFrameInfo: async () => allFrame,
    injectContentScript: async () => {
      injectionCount += 1;
      for (const [frameId, probe] of Object.entries(allProbe)) {
        if (probe === "no-content-script") allProbe[Number(frameId)] = { ...emptyProbe, hasLoginForm: true };
      }
    },
    sendToFrame: async <Result,>(_tabId: number, frameId: number, message: BackgroundToContentMessage): Promise<Result> => {
      allFrameMessage.push({ frameId, message });
      const probe = allProbe[frameId];
      if (probe === "no-content-script") throw new Error("Could not establish connection. Receiving end does not exist.");
      if (message.type === "content.probe") return (probe ?? emptyProbe) as Result;
      if (message.type === "content.fillLogin") return { filledUsername: true, filledPassword: true } as Result;
      if (message.type === "content.isInlineMenuVisible") return (options.isMenuVisible ?? true) as Result;
      return true as Result;
    },
    broadcastToInlineMenu: async (message) => {
      allBroadcast.push(message);
    },
    openPopup: async (tabId) => {
      allOpenedPopupTab.push(tabId);
    },
    writeToClipboard: async (text) => {
      if (options.isClipboardUnavailable) throw new Error("The clipboard is not available");
      allClipboardText.push(text);
    },
    getInlineMenuUrl: () => "chrome-extension://dynamic-id/inline-menu.html",
    getBrowserName: () => "Chrome",
    getExtensionVersion: () => "0.1.0",
    createID: () => `id-${(idCounter += 1)}`,
    now: () => currentTime,
  };

  const area = createMemoryArea();
  const storage = new ExtensionStorage(area, () => "instance-1");
  let controller: BackgroundController;
  const client = new NativeClient({
    connectNative: () => {
      if (options.isHostMissing) throw new Error("Specified native messaging host not found.");
      return new FakePort((request) => {
        allRequest.push(request);
        return respond(state, request, currentTime);
      });
    },
    getLastErrorMessage: () => undefined,
    getHelloParameters: () => controller.getHelloParameters(),
    getToken: () => storage.getToken(),
    createID: () => `request-${(idCounter += 1)}`,
    onHello: (result) => controller.handleHello(result),
    allReconnectDelayMilliseconds: [],
  });
  controller = new BackgroundController(client, storage, bridge);

  return {
    state,
    controller,
    storage,
    area,
    allRequest,
    allFrameMessage,
    allBroadcast,
    allOpenedPopupTab,
    allClipboardText,
    bridge,
    getInjectionCount: () => injectionCount,
    advanceTime: (milliseconds: number) => (currentTime += milliseconds),
    getRequestTypes: () => allRequest.map((request) => request.type),
    async pair() {
      await controller.startPairing();
      await controller.finishPairing("123456");
    },
  };
}

export async function expectHisarError(promise: Promise<unknown>, code: string): Promise<void> {
  const error = await promise.then(
    () => undefined,
    (caught: unknown) => caught,
  );
  expect(error).toBeInstanceOf(HisarError);
  expect((error as HisarError).code).toBe(code);
}

