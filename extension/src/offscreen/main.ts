import type { OffscreenCopyMessage } from "../shared/messages";
import { ClipboardClearer, createDocumentClipboard } from "./clipboard";

const clearer = new ClipboardClearer(createDocumentClipboard(document), () => window.close());

chrome.runtime.onMessage.addListener((message: OffscreenCopyMessage, sender, sendResponse) => {
  if (message?.target !== "offscreen" || sender.id !== chrome.runtime.id) return false;
  try {
    clearer.copy(message.text, message.clearAfterMilliseconds);
    sendResponse({ ok: true });
  } catch (error) {
    sendResponse({ ok: false, message: error instanceof Error ? error.message : String(error) });
  }
  return false;
});
