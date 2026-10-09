import { passkeyContentPortName } from "../../shared/passkey";
import { PasskeyContentBridge, type PasskeyContentPort } from "./content";
import { PasskeyPromptFrame } from "./prompt-frame";

interface PermissionsPolicy {
  allowsFeature(feature: string): boolean;
}

function isPermissionPolicyAllowed(feature: string): boolean {
  const documentWithPolicy = document as Document & { permissionsPolicy?: PermissionsPolicy; featurePolicy?: PermissionsPolicy };
  const policy = documentWithPolicy.permissionsPolicy ?? documentWithPolicy.featurePolicy;
  try {
    return policy?.allowsFeature(feature) === true;
  } catch {
    return false;
  }
}

const globalScope = globalThis as typeof globalThis & { isHisarPasskeyContentLoaded?: boolean };
if (typeof chrome !== "undefined" && chrome.runtime?.id && !globalScope.isHisarPasskeyContentLoaded) {
  globalScope.isHisarPasskeyContentLoaded = true;
  new PasskeyContentBridge(document, {
    connect: () => chrome.runtime.connect({ name: passkeyContentPortName }) as unknown as PasskeyContentPort,
    createPromptFrame: () =>
      new PasskeyPromptFrame(document, window, {
        now: () => performance.now(),
        requestFrame: (callback) => window.requestAnimationFrame(callback),
        cancelFrame: (handle) => window.cancelAnimationFrame(handle),
        getRect: (element) => element.getBoundingClientRect(),
        getElementAtPoint: (x, y) => document.elementFromPoint(x, y),
      }),
    getAllAncestorOrigin: () => Array.from(location.ancestorOrigins ?? []),
    isPermissionPolicyAllowed,
    setInterval: (callback, milliseconds) => setInterval(callback, milliseconds),
    clearInterval: (handle) => clearInterval(handle),
  }).start();
}
