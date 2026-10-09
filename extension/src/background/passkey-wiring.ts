import "../shared/passkey-protocol";
import { toHisarError } from "../shared/protocol";
import type { BackgroundController } from "./controller";
import type { NativeClient } from "./native-client";
import { PasskeyService, type PasskeyDependencies } from "./passkey";
import type { ExtensionStorage } from "./storage";

export function createPasskeyDependencies(
  client: NativeClient,
  storage: ExtensionStorage,
  getController: () => BackgroundController,
  createID: () => string,
  getPromptUrl: () => string,
): PasskeyDependencies {
  return {
    getDesktopView: async () => (await getController().getPopupState()).view,
    // Never connects: pages ask this on load, and connecting would start Hisar.
    isDesktopAvailable: async () =>
      (await storage.getToken()) !== undefined && client.isConnected() && (client.getLastHello()?.state ?? "signed-out") !== "signed-out",
    isDesktopConnected: () => client.isConnected(),
    async requestDesktop(type, passkey) {
      try {
        return await client.request(type, { passkey });
      } catch (error) {
        const hisarError = toHisarError(error);
        if (hisarError.code === "not-paired") await storage.removeToken();
        throw hisarError;
      }
    },
    unlock: () => getController().unlock(),
    getPromptUrl,
    createID,
  };
}

// In the service worker getURL uses the dynamic ID of web_accessible_resources (use_dynamic_url).
export function createPasskeyService(client: NativeClient, storage: ExtensionStorage, getController: () => BackgroundController, createID: () => string): PasskeyService {
  return new PasskeyService(createPasskeyDependencies(client, storage, getController, createID, () => chrome.runtime.getURL("passkey-prompt.html")));
}
