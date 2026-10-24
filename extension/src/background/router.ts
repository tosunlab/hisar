import { isExtensionPageUrl } from "../shared/extension-url";
import type { ContentToBackgroundMessage, InlineMenuMessage, MessageResponse, PopupMessage } from "../shared/messages";
import { HisarError, toHisarError } from "../shared/protocol";
import type { BackgroundController } from "./controller";
import type { InlineMenuSender, InlineMenuService } from "./inline-menu";

export interface RouterServices {
  controller: BackgroundController;
  inlineMenu: InlineMenuService;
}

export interface MessageSender {
  id?: string;
  url?: string;
  tab?: { id?: number; url?: string };
  frameId?: number;
  documentId?: string;
}

export interface RouterContext {
  extensionID: string;
  extensionOrigin: string;
}

function isFromExtensionPage(sender: MessageSender, context: RouterContext): boolean {
  return sender.id === context.extensionID && (sender.url ?? "").startsWith(context.extensionOrigin);
}

async function routePopupMessage(controller: BackgroundController, message: PopupMessage): Promise<unknown> {
  switch (message.type) {
    case "popup.getState":
      return controller.getPopupState(message.shouldReconnect === true, message.isOverOpenConnection === true);
    case "popup.getPendingSaveOffer":
      return controller.getPendingSaveOffer(message.tabId);
    case "popup.actOnSaveOffer":
      return controller.actOnSaveOffer(message.tabId, message.offerID, message.action, message.edits);
    case "popup.startPairing":
      return controller.startPairing();
    case "popup.finishPairing":
      return controller.finishPairing(message.code);
    case "popup.unlock":
      return controller.unlock();
    case "popup.unlockWithPassword":
      return controller.unlockWithPassword(message.password);
    case "popup.matchLogins":
      return controller.matchLogins(message.tabId);
    case "popup.fillLogin":
      return controller.fillLogin(message.tabId, message.vaultID, message.itemID);
    case "popup.getPassword":
      return controller.getPasswordForTab(message.tabId, message.vaultID, message.itemID);
    case "popup.getOneTimeCode":
      return controller.getOneTimeCodeForTab(message.tabId, message.vaultID, message.itemID);
    case "popup.generatePassword":
      return controller.generatePassword(message.options);
    case "popup.fillPassword":
      return controller.fillPassword(message.tabId, message.password);
    case "popup.listAddresses":
      return controller.listAddresses();
    case "popup.fillAddress":
      return controller.fillAddress(message.tabId, message.vaultID, message.itemID);
    case "popup.listIdentities":
      return controller.listIdentities();
    case "popup.fillIdentity":
      return controller.fillIdentity(message.tabId, message.vaultID, message.itemID);
    case "popup.listCards":
      return controller.listCards();
    case "popup.fillCard":
      return controller.fillCard(message.tabId, message.vaultID, message.itemID);
    case "popup.getAllLastUsed":
      return controller.getAllLastUsed();
    case "popup.listVaults":
      return controller.listVaults();
    case "popup.createItem":
      return controller.createItem(message.vaultID, message.item);
    case "popup.listItems":
      return controller.listItems(message.query);
    case "popup.getItem":
      return controller.getItem(message.vaultID, message.itemID);
    case "popup.revealField":
      return controller.revealItemField(message.vaultID, message.itemID, message.fieldID);
    case "popup.getItemCode":
      return controller.getItemCode(message.vaultID, message.itemID);
    case "popup.getIcon":
      return controller.getItemIcon(message.url);
    case "popup.setFavorite":
      return controller.setItemFavorite(message.vaultID, message.itemID, message.isFavorite);
  }
}

async function routeContentMessage(
  { controller, inlineMenu }: RouterServices,
  message: ContentToBackgroundMessage,
  sender: InlineMenuSender,
  frameUrl: string,
  tabUrl: string | undefined,
  documentID: string | undefined,
): Promise<unknown> {
  const tabId = sender.tabId;
  switch (message.type) {
    case "content.openInlineMenu":
      return inlineMenu.openSession(sender, frameUrl, tabUrl ?? (await controller.getTabUrl(tabId)), message.purpose);
    case "content.closeInlineMenu":
      inlineMenu.closeSession(sender, message.sessionID);
      return null;
    case "content.pressInlineMenuKey":
      return inlineMenu.pressKey(sender, message.sessionID, message.key);
    case "content.loginSubmitted": {
      // Chrome names the document itself; in Firefox the frame's content script does (getFrameDocumentID asks it back).
      const currentDocumentID = documentID ?? message.documentToken;
      if (!currentDocumentID) throw new HisarError("invalid-request", "A current document is required");
      void controller.handleLoginSubmitted(tabId, frameUrl, message.username, message.password, sender.frameId, tabUrl ?? frameUrl, currentDocumentID).catch(() => undefined);
      return null;
    }
    case "content.usernameSubmitted":
      await controller.handleUsernameSubmitted(tabId, frameUrl, message.username);
      return null;
  }
  throw new HisarError("invalid-request", "Unknown content request");
}

async function routeInlineMenuMessage(inlineMenu: InlineMenuService, message: InlineMenuMessage, sender: InlineMenuSender): Promise<unknown> {
  switch (message.type) {
    case "inlineMenu.getState":
      return inlineMenu.getState(sender, message.sessionID);
    case "inlineMenu.fillLogin":
      return inlineMenu.fillLogin(sender, message.sessionID, message.vaultID, message.itemID);
    case "inlineMenu.fillOneTimeCode":
      return inlineMenu.fillOneTimeCode(sender, message.sessionID, message.vaultID, message.itemID);
    case "inlineMenu.fillAddress":
      return inlineMenu.fillAddress(sender, message.sessionID, message.vaultID, message.itemID);
    case "inlineMenu.fillIdentity":
      return inlineMenu.fillIdentity(sender, message.sessionID, message.vaultID, message.itemID);
    case "inlineMenu.fillCard":
      return inlineMenu.fillCard(sender, message.sessionID, message.vaultID, message.itemID);
    case "inlineMenu.useSuggestedPassword":
      return inlineMenu.useSuggestedPassword(sender, message.sessionID);
    case "inlineMenu.unlock":
      return inlineMenu.unlock(sender, message.sessionID);
    case "inlineMenu.openHisar":
      return inlineMenu.openHisar(sender, message.sessionID);
    case "inlineMenu.openPopup":
      return inlineMenu.openPopup(sender, message.sessionID);
    case "inlineMenu.resize":
      return inlineMenu.resize(sender, message.sessionID, message.height);
    case "inlineMenu.close":
      return inlineMenu.close(sender, message.sessionID);
  }
}

function isFromInlineMenuPage(sender: MessageSender, context: RouterContext): boolean {
  return isExtensionPageUrl(sender.url, context.extensionOrigin, "/inline-menu.html");
}

export async function routeMessage(
  services: RouterServices,
  message: { type?: unknown; target?: unknown },
  sender: MessageSender,
  context: RouterContext,
): Promise<MessageResponse<unknown> | undefined> {
  if (typeof message?.type !== "string" || message.target !== undefined) return undefined;
  try {
    if (sender.id !== context.extensionID) throw new HisarError("invalid-request", "Unknown sender");
    let result: unknown;
    if (message.type.startsWith("popup.")) {
      if (!isFromExtensionPage(sender, context) || new URL(sender.url!).pathname !== "/popup.html" || sender.frameId !== undefined && sender.frameId !== 0) throw new HisarError("invalid-request", "Only the popup may send this message");
      result = await routePopupMessage(services.controller, message as PopupMessage);
    } else if (message.type.startsWith("content.")) {
      const tabId = sender.tab?.id;
      if (tabId === undefined || sender.frameId === undefined || !sender.url) {
        throw new HisarError("invalid-request", "Only content scripts may send this message");
      }
      const frameSender = { tabId, frameId: sender.frameId };
      result = await routeContentMessage(services, message as ContentToBackgroundMessage, frameSender, sender.url, sender.tab?.url, sender.documentId);
    } else if (message.type.startsWith("inlineMenu.")) {
      const tabId = sender.tab?.id;
      if (tabId === undefined || sender.frameId === undefined || !isFromInlineMenuPage(sender, context)) {
        throw new HisarError("invalid-request", "Only the inline menu may send this message");
      }
      result = await routeInlineMenuMessage(services.inlineMenu, message as InlineMenuMessage, { tabId, frameId: sender.frameId });
    } else {
      throw new HisarError("invalid-request", `Unknown message type ${message.type}`);
    }
    return { ok: true, result: result ?? null };
  } catch (error) {
    const hisarError = toHisarError(error);
    return { ok: false, error: { code: hisarError.code, message: hisarError.message } };
  }
}
