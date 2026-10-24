import type { BackgroundToContentMessage, ContentToBackgroundMessage, FillOutcome, MessageResponse } from "../shared/messages";
import { attachSubmissionCapture } from "./capture";
import { readPageSettings, parsePageSettings, allPageSettingKey, appFeaturesKey, readAppFeatures, type PageSettings } from "../shared/settings";
import { setFillDiagnosticsEnabled } from "./diagnostics";
import { featureAddresses, featureCards, featureIdentities } from "../shared/protocol";
import { fillAddressIntoPage, fillAddressIntoScopeOf } from "./address-fill";
import { fillCardIntoPage, fillCardIntoScopeOf } from "./card-fill";
import { fillIdentityIntoPage, fillIdentityIntoScopeOf } from "./identity-fill";
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
import { showNotice } from "./notice";

// documentToken names this document for save offers in Firefox, which gives the background no documentId. Pages cannot
// read it or send runtime messages, so only this content script can claim the document.
const documentToken = crypto.randomUUID();

function sendToBackground<Result>(message: ContentToBackgroundMessage): Promise<MessageResponse<Result>> {
  return chrome.runtime.sendMessage(message);
}


// isFilled tells a fill that wrote something from one that did not, whatever its kind of result.
function isFilled(result: unknown): boolean {
  if (typeof result === "number") return result > 0;
  if (typeof result === "boolean") return result;
  const outcome = result as FillOutcome | undefined;
  return outcome?.filledUsername === true || outcome?.filledPassword === true;
}

export function handleBackgroundMessage(message: BackgroundToContentMessage, inlineMenu: InlineMenuController): unknown {
  // A fill focuses the fields it writes; the menu must not open again on them, and closes only once the fill worked.
  if (message.type.startsWith("content.fill")) {
    const sessionID = "sessionID" in message ? message.sessionID : undefined;
    return inlineMenu.runFill(sessionID, () => answerBackgroundMessage(message, inlineMenu), isFilled);
  }
  return answerBackgroundMessage(message, inlineMenu);
}

function answerBackgroundMessage(message: BackgroundToContentMessage, inlineMenu: InlineMenuController): unknown {
  switch (message.type) {
    case "content.getDocumentToken":
      return documentToken;
    case "content.probe":
      return probeFrame(document);
    case "content.fillLogin": {
      if (!isFrameFillable(window, message.expectedOrigin)) return { filledUsername: false, filledPassword: false };
      if (message.sessionID === undefined) return fillLogin(document, message.username, message.password);
      const field = inlineMenu.getAnchoredField(message.sessionID);
      if (!field) return { filledUsername: false, filledPassword: false };
      return fillLoginIntoScopeOf(document, field, message.username, message.password);
    }
    case "content.fillPassword": {
      if (!isFrameFillable(window, message.expectedOrigin)) return false;
      if (message.sessionID === undefined) return fillPasswordIntoFocusedField(document, message.password);
      const field = inlineMenu.getAnchoredField(message.sessionID);
      if (!field) return false;
      fillPasswordIntoField(document, field, message.password);
      return true;
    }
    case "content.fillOneTimeCode": {
      if (!isFrameFillable(window, message.expectedOrigin)) return false;
      if (message.sessionID === undefined) return fillOneTimeCodeIntoPage(document, message.code);
      const field = inlineMenu.getAnchoredField(message.sessionID);
      if (!field) return false;
      return fillOneTimeCodeIntoField(field, message.code);
    }
    case "content.fillAddress": {
      if (!isFrameFillable(window, message.expectedOrigin)) return 0;
      if (message.sessionID === undefined) return fillAddressIntoPage(document, message.address);
      const field = inlineMenu.getAnchoredField(message.sessionID);
      if (!field) return 0;
      return fillAddressIntoScopeOf(document, field, message.address);
    }
    case "content.fillCard": {
      if (!isFrameFillable(window, message.expectedOrigin)) return 0;
      if (message.sessionID === undefined) return fillCardIntoPage(document, message.card);
      const field = inlineMenu.getAnchoredField(message.sessionID);
      if (!field) return 0;
      return fillCardIntoScopeOf(document, field, message.card);
    }
    case "content.fillIdentity": {
      if (!isFrameFillable(window, message.expectedOrigin)) return 0;
      if (message.sessionID === undefined) return fillIdentityIntoPage(document, message.identity);
      const field = inlineMenu.getAnchoredField(message.sessionID);
      if (!field) return 0;
      return fillIdentityIntoScopeOf(document, field, message.identity);
    }
    case "content.isInlineMenuVisible":
      return inlineMenu.isMenuInteractionAllowed(message.sessionID);
    case "content.resizeInlineMenu":
      inlineMenu.resizeMenu(message.sessionID, message.height);
      return true;
    case "content.dismissInlineMenu":
      inlineMenu.dismissMenu(message.sessionID);
      return true;
    case "content.openInlineMenuHere":
      return inlineMenu.openHere();
    case "content.showNotice":
      showNotice(document, message.text);
      return true;
  }
}

function applyPageSettings(inlineMenu: InlineMenuController, settings: PageSettings): void {
  inlineMenu.updateSettings(settings);
  setFillDiagnosticsEnabled(settings.isFillDiagnosticsEnabled);
}

function startInlineMenu(): InlineMenuController {
  const inlineMenu = new InlineMenuController(document, window, {
    sendToBackground,
    now: () => performance.now(),
    requestFrame: (callback) => window.requestAnimationFrame(callback),
    cancelFrame: (handle) => window.cancelAnimationFrame(handle),
    getRect: (element) => element.getBoundingClientRect(),
    getElementAtPoint: (x, y) => document.elementFromPoint(x, y),
    extensionBaseUrl: chrome.runtime.getURL(""),
  });
  readPageSettings(chrome.storage.local)
    .then((settings) => applyPageSettings(inlineMenu, settings))
    .catch(() => undefined);
  const updateAppFeatures = (): void => {
    readAppFeatures(chrome.storage.local)
      .then((allFeature) => {
        inlineMenu.setAddressFillAvailable(allFeature.includes(featureAddresses));
        inlineMenu.setCardFillAvailable(allFeature.includes(featureCards));
        inlineMenu.setIdentityFillAvailable(allFeature.includes(featureIdentities));
      })
      .catch(() => {
        inlineMenu.setAddressFillAvailable(false);
        inlineMenu.setCardFillAvailable(false);
        inlineMenu.setIdentityFillAvailable(false);
      });
  };
  updateAppFeatures();
  chrome.storage.onChanged.addListener((allChange, areaName) => {
    if (areaName === "local" && appFeaturesKey in allChange) updateAppFeatures();
    if (areaName !== "local" || !allPageSettingKey.some((key) => key in allChange)) return;
    readPageSettings(chrome.storage.local)
      .then((settings) => applyPageSettings(inlineMenu, settings))
      .catch(() => applyPageSettings(inlineMenu, parsePageSettings({})));
  });
  inlineMenu.start();
  return inlineMenu;
}

function startContentScript(): void {
  const inlineMenu = startInlineMenu();
  chrome.runtime.onMessage.addListener((message: BackgroundToContentMessage, _sender, sendResponse) => {
    const answer = handleBackgroundMessage(message, inlineMenu);
    if (!(answer instanceof Promise)) {
      sendResponse(answer);
      return false;
    }
    answer.then(sendResponse, () => sendResponse(undefined));
    return true;
  });
  attachSubmissionCapture(document, {
    onSubmission: (submission) => {
      const message: ContentToBackgroundMessage =
        submission.kind === "login"
          ? { type: "content.loginSubmitted", username: submission.username, password: submission.password, documentToken }
          : { type: "content.usernameSubmitted", username: submission.username };
      sendToBackground(message).catch(() => undefined);
    },
  });
}

const globalScope = globalThis as typeof globalThis & { isHisarContentScriptLoaded?: boolean };
// The isolated world is shared between the manifest injection and scripting.executeScript, so load once.
if (typeof chrome !== "undefined" && chrome.runtime?.id && !globalScope.isHisarContentScriptLoaded) {
  globalScope.isHisarContentScriptLoaded = true;
  startContentScript();
}
