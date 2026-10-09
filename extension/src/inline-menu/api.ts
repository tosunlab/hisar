import type { InlineMenuMessage, MessageResponse } from "../shared/messages";
import { HisarError, isErrorCode } from "../shared/protocol";

export async function sendToBackground<Result>(message: InlineMenuMessage): Promise<Result> {
  const response = (await chrome.runtime.sendMessage(message)) as MessageResponse<Result> | undefined;
  if (!response) throw new HisarError("internal", "The extension did not answer");
  if (!response.ok) {
    throw new HisarError(isErrorCode(response.error.code) ? response.error.code : "internal", response.error.message);
  }
  return response.result;
}

export function readSessionID(location: Location): string {
  return decodeURIComponent(location.hash.replace(/^#/, ""));
}

export function supportsVisibilityTracking(): boolean {
  return typeof IntersectionObserverEntry !== "undefined" && "isVisible" in IntersectionObserverEntry.prototype;
}
