import type { BackgroundToContentMessage, OffscreenCopyMessage } from "../shared/messages";
import { nativeHostName, toHisarError } from "../shared/protocol";
import { getBrowserName } from "../shared/url";
import { BackgroundController, type BrowserBridge } from "./controller";
import type { FrameInfo } from "./frames";
import { NativeClient } from "./native-client";
import { routeMessage } from "./router";
import { ExtensionStorage } from "./storage";

const clipboardClearMilliseconds = 90000;
let offscreenCreation: Promise<void> | undefined;

async function ensureOffscreenDocument(): Promise<void> {
  const documentUrl = chrome.runtime.getURL("offscreen.html");
  const allContext = await chrome.runtime.getContexts({
    contextTypes: [chrome.runtime.ContextType.OFFSCREEN_DOCUMENT],
    documentUrls: [documentUrl],
  });
  if (allContext.length > 0) return;
  offscreenCreation ??= chrome.offscreen
    .createDocument({
      url: "offscreen.html",
      reasons: [chrome.offscreen.Reason.CLIPBOARD],
      justification: "Copy usernames and passwords and clear the clipboard after 90 seconds",
    })
    .finally(() => {
      offscreenCreation = undefined;
    });
  await offscreenCreation;
}

async function getAllFrameInfo(tabId: number): Promise<FrameInfo[]> {
  const allResult = await chrome.scripting.executeScript({
    target: { tabId, allFrames: true },
    func: () => ({
      url: location.href,
      origin: location.origin,
      allAncestorOrigin: Array.from(location.ancestorOrigins ?? []),
    }),
  });
  return allResult.flatMap((injection) => (injection.result ? [{ frameId: injection.frameId, ...injection.result }] : []));
}

const navigatorWithBrands = navigator as Navigator & { userAgentData?: { brands?: { brand: string }[] }; brave?: unknown };

const bridge: BrowserBridge = {
  async getTabUrl(tabId) {
    return (await chrome.tabs.get(tabId)).url;
  },
  getAllFrameInfo,
  async injectContentScript(tabId) {
    await chrome.scripting.executeScript({ target: { tabId, allFrames: true }, files: ["content.js"] });
  },
  sendToFrame<Result>(tabId: number, frameId: number, message: BackgroundToContentMessage): Promise<Result> {
    return chrome.tabs.sendMessage(tabId, message, { frameId });
  },
  async copyToClipboard(text) {
    await ensureOffscreenDocument();
    const message: OffscreenCopyMessage = {
      target: "offscreen",
      type: "offscreen.copy",
      text,
      clearAfterMilliseconds: clipboardClearMilliseconds,
    };
    const response: { ok: boolean; message?: string } | undefined = await chrome.runtime.sendMessage(message);
    if (!response?.ok) throw new Error(response?.message || "Could not copy to the clipboard");
  },
  getBrowserName: () => getBrowserName(navigatorWithBrands.userAgentData, navigator.userAgent, navigatorWithBrands.brave !== undefined),
  getExtensionVersion: () => chrome.runtime.getManifest().version,
  createID: () => crypto.randomUUID(),
  now: () => Date.now(),
};

const storage = new ExtensionStorage(chrome.storage.local, () => crypto.randomUUID());
let controller: BackgroundController | undefined;
const client = new NativeClient({
  connectNative: () => chrome.runtime.connectNative(nativeHostName),
  getLastErrorMessage: () => chrome.runtime.lastError?.message,
  getHelloParameters: () => getController().getHelloParameters(),
  getToken: () => storage.getToken(),
  createID: () => crypto.randomUUID(),
  onHello: (result) => getController().handleHello(result),
  // A timed reconnect would start the host, which relaunches Hisar after the user quit it; reconnect on the next request instead.
  allReconnectDelayMilliseconds: [],
});

function getController(): BackgroundController {
  controller ??= new BackgroundController(client, storage, bridge);
  return controller;
}

const routerContext = { extensionID: chrome.runtime.id, extensionOrigin: chrome.runtime.getURL("") };

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (message?.target === "offscreen") return false;
  void routeMessage(getController(), message, sender, routerContext).then(sendResponse);
  return true;
});

chrome.commands.onCommand.addListener((command, tab) => {
  if (command !== "fill-login" || tab?.id === undefined) return;
  getController()
    .fillFirstLogin(tab.id)
    .catch((error: unknown) => {
      if (toHisarError(error).code === "locked") return getController().unlock();
    })
    .catch(() => undefined);
});

chrome.tabs.onRemoved.addListener((tabId) => getController().handleTabRemoved(tabId));
