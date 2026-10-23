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

// watchTabUrl reports the tab's address now and after every change until the returned function stops it; "" where the
// extension may not read it. Once the tab has changed, the first read is stale and dropped.
export function watchTabUrl(tabId: number, onUrl: (url: string) => void): () => void {
  let hasChanged = false;
  const listener = (changedTabId: number, _change: chrome.tabs.OnUpdatedInfo, tab: chrome.tabs.Tab) => {
    if (changedTabId !== tabId) return;
    hasChanged = true;
    onUrl(tab.url ?? "");
  };
  chrome.tabs.onUpdated.addListener(listener);
  chrome.tabs.get(tabId).then(
    (tab) => hasChanged || onUrl(tab.url ?? ""),
    () => hasChanged || onUrl(""),
  );
  return () => chrome.tabs.onUpdated.removeListener(listener);
}

const allFriendlyMessage: Partial<Record<string, string>> = {
  "not-paired": "This browser is no longer paired with Hisar.",
  "signed-out": "Sign in to Hisar on this computer first.",
  locked: "Hisar is locked.",
  offline: "Hisar is offline and cannot make changes.",
  "desktop-unavailable": "Hisar is not running.",
  "update-required": "Hisar needs an update.",
};

export function describeError(error: unknown): string {
  if (error instanceof HisarError) {
    if (error.message && error.message !== error.code) return error.message;
    return allFriendlyMessage[error.code] ?? "Something went wrong.";
  }
  return error instanceof Error ? error.message : String(error);
}

const allUpdateOrigin = ["https://chromewebstore.google.com", "https://tryhisar.com"];

// The URL comes from the server through the desktop, so only known update pages open.
export async function openUpdatePage(url: string): Promise<void> {
  const isKnownPage = URL.canParse(url) && allUpdateOrigin.includes(new URL(url).origin);
  await chrome.tabs.create({ url: isKnownPage ? url : allUpdateOrigin[0] + "/detail/mmipiffoogblblocokimpemdopmdjghf" });
  closePopup();
}

export function closePopup(): void {
  if (!new URLSearchParams(window.location.search).has("tabId")) window.close();
}
