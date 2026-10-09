import type { BackgroundToContentMessage } from "../shared/messages";
import { nativeHostName, toHisarError } from "../shared/protocol";
import { getBrowserName } from "../shared/url";
import { BackgroundController, type BrowserBridge } from "./controller";
import { InlineMenuService } from "./inline-menu";
import type { FrameInfo } from "./frames";
import { NativeClient } from "./native-client";
import { createPasskeyService } from "./passkey-wiring";
import { routeMessage } from "./router";
import { ExtensionStorage } from "./storage";

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
  async broadcastToInlineMenu(message) {
    await chrome.runtime.sendMessage(message).catch(() => undefined);
  },
  async openPopup(tabId) {
    const tab = await chrome.tabs.get(tabId);
    try {
      await chrome.action.openPopup({ windowId: tab.windowId });
    } catch {
      await chrome.tabs.create({ url: chrome.runtime.getURL(`popup.html?tabId=${tabId}`), index: tab.index + 1, openerTabId: tabId });
    }
  },
  // In the service worker getURL uses the dynamic ID of web_accessible_resources (use_dynamic_url); content scripts get the static one.
  getInlineMenuUrl: () => chrome.runtime.getURL("inline-menu.html"),
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

let inlineMenu: InlineMenuService | undefined;

function getController(): BackgroundController {
  controller ??= new BackgroundController(client, storage, bridge);
  return controller;
}

function getInlineMenu(): InlineMenuService {
  inlineMenu ??= new InlineMenuService(getController(), bridge);
  return inlineMenu;
}

const routerContext = { extensionID: chrome.runtime.id, extensionOrigin: chrome.runtime.getURL("") };

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (message?.target !== undefined) return false;
  void routeMessage({ controller: getController(), inlineMenu: getInlineMenu() }, message, sender, routerContext).then(sendResponse);
  return true;
});

const passkeyService = createPasskeyService(client, storage, () => getController(), () => crypto.randomUUID());
chrome.runtime.onConnect.addListener((port) => passkeyService.handlePort(port));

chrome.commands.onCommand.addListener((command, tab) => {
  if (command !== "fill-login" || tab?.id === undefined) return;
  getController()
    .fillFirstLogin(tab.id)
    .catch((error: unknown) => {
      if (toHisarError(error).code === "locked") return getController().unlock();
    })
    .catch(() => undefined);
});

chrome.tabs.onRemoved.addListener((tabId) => {
  getController().handleTabRemoved(tabId);
  getInlineMenu().handleTabRemoved(tabId);
});
