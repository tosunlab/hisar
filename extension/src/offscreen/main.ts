import { copyText, isOffscreenCopyMessage } from "./copy";

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (!isOffscreenCopyMessage(message, sender, chrome.runtime.id)) return false;
  sendResponse(copyText(document, message.text));
  return false;
});
