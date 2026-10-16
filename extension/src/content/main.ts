import type { BackgroundToContentMessage, ContentToBackgroundMessage, MessageResponse, SaveOfferView } from "../shared/messages";
import { attachSubmissionCapture } from "./capture";
import { readInlineMenuSettings, parseInlineMenuSettings, allInlineMenuSettingKey, appFeaturesKey, readAppFeatures } from "../shared/settings";
import { featureAddresses } from "../shared/protocol";
import { fillAddressIntoPage, fillAddressIntoScopeOf } from "./address-fill";
import {
  fillLogin,
  fillLoginIntoScopeOf,
  fillOneTimeCodeIntoField,
  fillOneTimeCodeIntoPage,
  fillPasswordIntoField,
  fillPasswordIntoFocusedField,
  isFrameFillable,
  probeFrame,
} from "./fill";
import { InlineMenuController } from "./inline-menu/controller";
import { showSaveBar } from "./save-bar";

function sendToBackground<Result>(message: ContentToBackgroundMessage): Promise<MessageResponse<Result>> {
  return chrome.runtime.sendMessage(message);
}

function presentSaveOffer(offer: SaveOfferView): void {
  showSaveBar(document, offer, async (action) => {
    const response = await sendToBackground<Record<string, never>>({ type: "content.actOnSaveOffer", offerID: offer.offerID, action });
    return response.ok ? { ok: true } : { ok: false, message: response.error.message };
  });
}

export function handleBackgroundMessage(message: BackgroundToContentMessage, inlineMenu: InlineMenuController): unknown {
  switch (message.type) {
    case "content.probe":
      return probeFrame(document);
    case "content.fillLogin": {
      if (!isFrameFillable(window, message.expectedOrigin)) return { filledUsername: false, filledPassword: false };
      if (message.sessionID === undefined) return fillLogin(document, message.username, message.password);
      const field = inlineMenu.getAnchoredField(message.sessionID);
      if (!field) return { filledUsername: false, filledPassword: false };
      inlineMenu.dismissMenu(message.sessionID);
      return fillLoginIntoScopeOf(document, field, message.username, message.password);
    }
    case "content.fillPassword": {
      if (!isFrameFillable(window, message.expectedOrigin)) return false;
      if (message.sessionID === undefined) return fillPasswordIntoFocusedField(document, message.password);
      const field = inlineMenu.getAnchoredField(message.sessionID);
      if (!field) return false;
      inlineMenu.dismissMenu(message.sessionID);
      fillPasswordIntoField(document, field, message.password);
      return true;
    }
    case "content.fillOneTimeCode": {
      if (!isFrameFillable(window, message.expectedOrigin)) return false;
      if (message.sessionID === undefined) return fillOneTimeCodeIntoPage(document, message.code);
      const field = inlineMenu.getAnchoredField(message.sessionID);
      if (!field) return false;
      inlineMenu.dismissMenu(message.sessionID);
      return fillOneTimeCodeIntoField(field, message.code);
    }
    case "content.fillAddress": {
      if (!isFrameFillable(window, message.expectedOrigin)) return 0;
      if (message.sessionID === undefined) return fillAddressIntoPage(document, message.address);
      const field = inlineMenu.getAnchoredField(message.sessionID);
      if (!field) return 0;
      inlineMenu.dismissMenu(message.sessionID);
      return fillAddressIntoScopeOf(document, field, message.address);
    }
    case "content.showSaveOffer":
      if (window.top !== window) return false;
      presentSaveOffer(message.offer);
      return true;
    case "content.isInlineMenuVisible":
      return inlineMenu.isMenuInteractionAllowed(message.sessionID);
    case "content.resizeInlineMenu":
      inlineMenu.resizeMenu(message.sessionID, message.height);
      return true;
    case "content.dismissInlineMenu":
      inlineMenu.dismissMenu(message.sessionID);
      return true;
  }
}

function startInlineMenu(): InlineMenuController {
  const inlineMenu = new InlineMenuController(document, window, {
    sendToBackground,
    now: () => performance.now(),
    requestFrame: (callback) => window.requestAnimationFrame(callback),
    cancelFrame: (handle) => window.cancelAnimationFrame(handle),
    getRect: (element) => element.getBoundingClientRect(),
    getElementAtPoint: (x, y) => document.elementFromPoint(x, y),
  });
  readInlineMenuSettings(chrome.storage.local)
    .then((settings) => inlineMenu.updateSettings(settings))
    .catch(() => undefined);
  const updateAppFeatures = (): void => {
    readAppFeatures(chrome.storage.local)
      .then((allFeature) => inlineMenu.setAddressFillAvailable(allFeature.includes(featureAddresses)))
      .catch(() => inlineMenu.setAddressFillAvailable(false));
  };
  updateAppFeatures();
  chrome.storage.onChanged.addListener((allChange, areaName) => {
    if (areaName === "local" && appFeaturesKey in allChange) updateAppFeatures();
    if (areaName !== "local" || !allInlineMenuSettingKey.some((key) => key in allChange)) return;
    readInlineMenuSettings(chrome.storage.local)
      .then((settings) => inlineMenu.updateSettings(settings))
      .catch(() => inlineMenu.updateSettings(parseInlineMenuSettings({})));
  });
  inlineMenu.start();
  return inlineMenu;
}

function startContentScript(): void {
  const inlineMenu = startInlineMenu();
  chrome.runtime.onMessage.addListener((message: BackgroundToContentMessage, _sender, sendResponse) => {
    sendResponse(handleBackgroundMessage(message, inlineMenu));
    return false;
  });
  attachSubmissionCapture(document, {
    onSubmission: (submission) => {
      const message: ContentToBackgroundMessage =
        submission.kind === "login"
          ? { type: "content.loginSubmitted", username: submission.username, password: submission.password }
          : { type: "content.usernameSubmitted", username: submission.username };
      sendToBackground(message).catch(() => undefined);
    },
  });
  if (window.top === window) {
    sendToBackground<SaveOfferView | null>({ type: "content.getPendingSaveOffer" })
      .then((response) => {
        if (response.ok && response.result) presentSaveOffer(response.result);
      })
      .catch(() => undefined);
  }
}

const globalScope = globalThis as typeof globalThis & { isHisarContentScriptLoaded?: boolean };
// The isolated world is shared between the manifest injection and scripting.executeScript, so load once.
if (typeof chrome !== "undefined" && chrome.runtime?.id && !globalScope.isHisarContentScriptLoaded) {
  globalScope.isHisarContentScriptLoaded = true;
  startContentScript();
}
