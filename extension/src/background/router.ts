import type { ContentToBackgroundMessage, MessageResponse, PopupMessage } from "../shared/messages";
import { HisarError, toHisarError } from "../shared/protocol";
import type { BackgroundController } from "./controller";

export interface MessageSender {
  id?: string;
  url?: string;
  tab?: { id?: number };
  frameId?: number;
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
      return controller.getPopupState(message.shouldReconnect === true);
    case "popup.startPairing":
      return controller.startPairing();
    case "popup.finishPairing":
      return controller.finishPairing(message.code);
    case "popup.unlock":
      return controller.unlock();
    case "popup.matchLogins":
      return controller.matchLogins(message.tabId);
    case "popup.fillLogin":
      return controller.fillLogin(message.tabId, message.vaultID, message.itemID);
    case "popup.copyPassword":
      return controller.copyPassword(message.tabId, message.vaultID, message.itemID);
    case "popup.copyText":
      return controller.copyText(message.text);
    case "popup.generatePassword":
      return controller.generatePassword(message.options);
    case "popup.fillPassword":
      return controller.fillPassword(message.tabId, message.password);
  }
}

async function routeContentMessage(
  controller: BackgroundController,
  message: ContentToBackgroundMessage,
  tabId: number,
  frameUrl: string,
): Promise<unknown> {
  switch (message.type) {
    case "content.loginSubmitted":
      return (await controller.handleLoginSubmitted(tabId, frameUrl, message.username, message.password)) ?? null;
    case "content.usernameSubmitted":
      controller.handleUsernameSubmitted(tabId, frameUrl, message.username);
      return null;
    case "content.getPendingSaveOffer":
      return controller.getPendingSaveOffer(tabId);
    case "content.actOnSaveOffer":
      return controller.actOnSaveOffer(tabId, message.offerID, message.action);
  }
}

export async function routeMessage(
  controller: BackgroundController,
  message: { type?: unknown; target?: unknown },
  sender: MessageSender,
  context: RouterContext,
): Promise<MessageResponse<unknown> | undefined> {
  if (typeof message?.type !== "string" || message.target !== undefined) return undefined;
  try {
    if (sender.id !== context.extensionID) throw new HisarError("invalid-request", "Unknown sender");
    let result: unknown;
    if (message.type.startsWith("popup.")) {
      if (!isFromExtensionPage(sender, context)) throw new HisarError("invalid-request", "Only the popup may send this message");
      result = await routePopupMessage(controller, message as PopupMessage);
    } else if (message.type.startsWith("content.")) {
      const tabId = sender.tab?.id;
      if (tabId === undefined || sender.frameId === undefined || !sender.url) {
        throw new HisarError("invalid-request", "Only content scripts may send this message");
      }
      result = await routeContentMessage(controller, message as ContentToBackgroundMessage, tabId, sender.url);
    } else {
      throw new HisarError("invalid-request", `Unknown message type ${message.type}`);
    }
    return { ok: true, result: result ?? null };
  } catch (error) {
    const hisarError = toHisarError(error);
    return { ok: false, error: { code: hisarError.code, message: hisarError.message } };
  }
}
