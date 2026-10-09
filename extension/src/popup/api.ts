import type { MessageResponse, PopupMessage } from "../shared/messages";
import { HisarError, isErrorCode } from "../shared/protocol";

export async function sendToBackground<Result>(message: PopupMessage): Promise<Result> {
  const response = (await chrome.runtime.sendMessage(message)) as MessageResponse<Result> | undefined;
  if (!response) throw new HisarError("internal", "The extension did not answer");
  if (!response.ok) {
    throw new HisarError(isErrorCode(response.error.code) ? response.error.code : "internal", response.error.message);
  }
  return response.result;
}

export async function getTargetTabId(): Promise<number | undefined> {
  const tabParameter = new URLSearchParams(window.location.search).get("tabId");
  if (tabParameter) return Number(tabParameter);
  const [activeTab] = await chrome.tabs.query({ active: true, currentWindow: true });
  return activeTab?.id;
}

const allFriendlyMessage: Partial<Record<string, string>> = {
  "not-paired": "This browser is no longer paired with Hisar.",
  "signed-out": "Sign in to Hisar on this computer first.",
  locked: "Hisar is locked.",
  offline: "Hisar is offline and cannot make changes.",
  "desktop-unavailable": "Hisar is not running.",
};

export function describeError(error: unknown): string {
  if (error instanceof HisarError) {
    if (error.message && error.message !== error.code) return error.message;
    return allFriendlyMessage[error.code] ?? "Something went wrong.";
  }
  return error instanceof Error ? error.message : String(error);
}

export function closePopup(): void {
  if (!new URLSearchParams(window.location.search).has("tabId")) window.close();
}
