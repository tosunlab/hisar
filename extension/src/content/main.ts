import type { BackgroundToContentMessage, ContentToBackgroundMessage, MessageResponse, SaveOfferView } from "../shared/messages";
import { attachSubmissionCapture } from "./capture";
import { fillLogin, fillPasswordIntoFocusedField, isFrameFillable, probeFrame } from "./fill";
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

export function handleBackgroundMessage(message: BackgroundToContentMessage): unknown {
  switch (message.type) {
    case "content.probe":
      return probeFrame(document);
    case "content.fillLogin":
      if (!isFrameFillable(window, message.expectedOrigin)) return { filledUsername: false, filledPassword: false };
      return fillLogin(document, message.username, message.password);
    case "content.fillPassword":
      if (!isFrameFillable(window, message.expectedOrigin)) return false;
      return fillPasswordIntoFocusedField(document, message.password);
    case "content.showSaveOffer":
      if (window.top !== window) return false;
      presentSaveOffer(message.offer);
      return true;
  }
}

function startContentScript(): void {
  chrome.runtime.onMessage.addListener((message: BackgroundToContentMessage, _sender, sendResponse) => {
    sendResponse(handleBackgroundMessage(message));
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
