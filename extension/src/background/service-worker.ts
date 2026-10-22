import type { BackgroundToContentMessage, OffscreenCopyMessage } from "../shared/messages";
import { nativeHostName, toHisarError } from "../shared/protocol";
import { getBrowserName } from "../shared/url";
import { BackgroundController, type BrowserBridge } from "./controller";
import { InlineMenuService } from "./inline-menu";
import type { FrameInfo } from "./frames";
import { NativeClient } from "./native-client";
import { createPasskeyService } from "./passkey-wiring";
import { routeMessage } from "./router";
import { SaveExpiryScheduler } from "./save-expiry";
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

let offscreenCreation: Promise<void> | undefined;

async function ensureOffscreenDocument(): Promise<void> {
  const documentUrl = chrome.runtime.getURL("offscreen.html");
  const allContext = await chrome.runtime.getContexts({ contextTypes: [chrome.runtime.ContextType.OFFSCREEN_DOCUMENT], documentUrls: [documentUrl] });
  if (allContext.length > 0) return;
  offscreenCreation ??= chrome.offscreen
    .createDocument({ url: "offscreen.html", reasons: [chrome.offscreen.Reason.CLIPBOARD], justification: "Copy the one-time code after filling a login" })
    .finally(() => (offscreenCreation = undefined));
  await offscreenCreation;
}

// A service worker has no clipboard, so an offscreen document copies.
async function writeToClipboard(text: string): Promise<void> {
  await ensureOffscreenDocument();
  const message: OffscreenCopyMessage = { target: "offscreen", type: "offscreen.copy", text };
  const isCopied = await chrome.runtime.sendMessage(message);
  if (isCopied !== true) throw new Error("The clipboard is not available");
}

const navigatorWithBrands = navigator as Navigator & { userAgentData?: { brands?: { brand: string }[] }; brave?: unknown };
const saveExpiry = new SaveExpiryScheduler(chrome);
// Every badge write takes the tab's next generation. A login count asked for before a later write answers after it, so
// updateBadge shows it only while its generation is still the tab's latest.
const allBadgeGeneration = new Map<number, number>();

function takeBadgeGeneration(tabId: number): number {
  const generation = (allBadgeGeneration.get(tabId) ?? 0) + 1;
  allBadgeGeneration.set(tabId, generation);
  return generation;
}

const bridge: BrowserBridge = {
  async getTabUrl(tabId) {
    return (await chrome.tabs.get(tabId)).url;
  },
  async getFrameDocumentID(tabId, frameId) {
    return (await chrome.webNavigation.getFrame({ tabId, frameId }))?.documentId;
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
  async setSaveBadge(tabId, isPending) {
    takeBadgeGeneration(tabId);
    await saveExpiry.ready;
    await chrome.action.setBadgeText({ tabId, text: isPending ? "+" : "" }).catch(() => undefined);
  },
  scheduleSaveExpiry: saveExpiry.schedule,
  writeToClipboard,
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
  void saveExpiry.ready.then(() => routeMessage({ controller: getController(), inlineMenu: getInlineMenu() }, message, sender, routerContext)).then(sendResponse);
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

// The toolbar icon shows how many logins are saved for the tab, while the native host is connected.
async function updateBadge(tabId: number): Promise<void> {
  const generation = takeBadgeGeneration(tabId);
  await saveExpiry.ready;
  const text = await getController().getBadgeText(tabId);
  if (allBadgeGeneration.get(tabId) !== generation) return;
  await chrome.action.setBadgeText({ tabId, text }).catch(() => undefined);
}

void chrome.action.setBadgeBackgroundColor({ color: "#30343a" }).catch(() => undefined);
chrome.tabs.onUpdated.addListener((tabId, change) => {
  if (change.status === "complete") void updateBadge(tabId);
});
// Messages wait for the restart cleanup, so the tab events that end a submission wait behind them, in the order the
// browser delivered them: a submit navigation that reaches a woken worker right after its submission then still finds it.
chrome.webNavigation.onCommitted.addListener(({ tabId, url, frameId, documentId }) => void saveExpiry.ready.then(() => getController().handleTabNavigation(tabId, url, frameId, documentId)));
chrome.webNavigation.onHistoryStateUpdated.addListener(({ tabId, url, frameId }) => void saveExpiry.ready.then(() => getController().handleTabNavigation(tabId, url, frameId)));
chrome.tabs.onActivated.addListener(({ tabId }) => void updateBadge(tabId));

const allContextMenu = [
  { id: "hisar-open-menu", title: "Show Hisar here" },
  { id: "hisar-fill-login", title: "Fill login" },
  { id: "hisar-suggest-password", title: "Fill a suggested password" },
] as const;

chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.removeAll(() => {
    for (const item of allContextMenu) chrome.contextMenus.create({ id: item.id, title: item.title, contexts: ["editable"] });
  });
});

chrome.contextMenus.onClicked.addListener((info, tab) => {
  const tabId = tab?.id;
  if (tabId === undefined) return;
  let action: Promise<unknown>;
  switch (info.menuItemId) {
    case "hisar-open-menu":
      action = chrome.tabs.sendMessage(tabId, { type: "content.openInlineMenuHere" } satisfies BackgroundToContentMessage, { frameId: info.frameId ?? 0 });
      break;
    case "hisar-fill-login":
      action = getController().fillFirstLogin(tabId);
      break;
    case "hisar-suggest-password":
      action = getController().fillSuggestedPassword(tabId);
      break;
    default:
      return;
  }
  action
    .catch((error: unknown) => {
      // A locked Hisar asks to be unlocked; anything else, such as a page without a login form, is ignored, as with the
      // keyboard shortcut.
      if (toHisarError(error).code === "locked") return getController().unlock();
    })
    .catch(() => undefined);
});

chrome.tabs.onRemoved.addListener((tabId) => {
  void saveExpiry.ready.then(() => {
    getController().handleTabRemoved(tabId);
    getInlineMenu().handleTabRemoved(tabId);
    allBadgeGeneration.delete(tabId);
  });
});
