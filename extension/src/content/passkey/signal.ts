import { decodeBase64URL, type PasskeyPageRequest, type PasskeyPageResponse, type SerializedSignal } from "../../shared/passkey";
import type { PasskeyClass, PasskeyGlobalScope } from "./main-world";

type SignalMethodName = SerializedSignal["method"];

interface SignalTransport {
  send(request: PasskeyPageRequest): Promise<PasskeyPageResponse | { requestID: string; outcome: "timeout" }>;
}

export const allSignalMethodName: SignalMethodName[] = ["signalUnknownCredential", "signalAllAcceptedCredentials", "signalCurrentUserDetails"];
export const allHisarCapability = ["passkeyPlatformAuthenticator", "userVerifyingPlatformAuthenticator", "conditionalGet", "extension:prf"];

function readRequiredString(options: Record<string, unknown>, name: string): string {
  const value = options[name];
  if (value === undefined) throw new TypeError(`${name} is required`);
  return String(value);
}

function checkBase64URL(value: string, name: string): string {
  try {
    decodeBase64URL(value);
  } catch {
    throw new TypeError(`${name} is not base64url`);
  }
  return value;
}

function readBase64URL(options: Record<string, unknown>, name: string): string {
  return checkBase64URL(readRequiredString(options, name), name);
}

function readAllBase64URL(options: Record<string, unknown>, name: string): string[] {
  const value = options[name];
  if (value === undefined || value === null || typeof value !== "object" || typeof (value as Iterable<unknown>)[Symbol.iterator] !== "function" || typeof value === "string") {
    throw new TypeError(`${name} must be a sequence`);
  }
  return Array.from(value as Iterable<unknown>, (entry, index) => checkBase64URL(String(entry), `${name}[${index}]`));
}

// TypeError for the checks the spec makes before the origin; the service worker checks rpId against the sender origin.
export function serializeSignal(method: SignalMethodName, options: unknown): SerializedSignal {
  if (typeof options !== "object" || options === null) throw new TypeError("options are required");
  const record = options as Record<string, unknown>;
  const rpId = readRequiredString(record, "rpId");
  switch (method) {
    case "signalUnknownCredential":
      return { method, rpId, credentialId: readBase64URL(record, "credentialId") };
    case "signalAllAcceptedCredentials":
      return { method, rpId, userId: readBase64URL(record, "userId"), allAcceptedCredentialIds: readAllBase64URL(record, "allAcceptedCredentialIds") };
    case "signalCurrentUserDetails":
      return { method, rpId, userId: readBase64URL(record, "userId"), name: readRequiredString(record, "name"), displayName: readRequiredString(record, "displayName") };
  }
}

async function callOriginal(original: ((options: unknown) => Promise<void>) | undefined, publicKeyCredential: PasskeyClass, options: unknown): Promise<unknown> {
  if (typeof original !== "function") return undefined;
  try {
    await original.call(publicKeyCredential, options);
    return undefined;
  } catch (error) {
    return error ?? new TypeError("signal failed");
  }
}

export function installPasskeySignals(publicKeyCredential: PasskeyClass, scope: PasskeyGlobalScope, transport: SignalTransport, createID: () => string): void {
  for (const method of allSignalMethodName) {
    const original = publicKeyCredential[method];
    Object.defineProperty(publicKeyCredential, method, {
      configurable: true,
      writable: true,
      value: async function signal(options: unknown): Promise<void> {
        const serialized = serializeSignal(method, options);
        const originalOutcome = callOriginal(original, publicKeyCredential, options);
        const settled = await transport.send({ requestID: createID(), kind: "signal", signal: serialized });
        const originalError = await originalOutcome;
        if (settled.outcome === "signal" && settled.errorName) throw new scope.DOMException(settled.message ?? "", settled.errorName);
        if (originalError) throw originalError;
      },
    });
  }
}

export function mergeClientCapabilities(allBrowserCapability: Record<string, unknown> | undefined, isHisarAvailable: boolean): Record<string, boolean> {
  const merged: Record<string, boolean> = {};
  for (const [name, value] of Object.entries(allBrowserCapability ?? {})) {
    if (typeof value === "boolean") merged[name] = value;
  }
  for (const name of allHisarCapability) merged[name] = merged[name] === true || isHisarAvailable;
  for (const name of allSignalMethodName) merged[name] = true;
  return merged;
}

// Asks Hisar with the availability request, which never starts the native host.
export function installClientCapabilities(publicKeyCredential: PasskeyClass, isHisarAvailable: () => Promise<boolean>): void {
  const original = publicKeyCredential.getClientCapabilities;
  Object.defineProperty(publicKeyCredential, "getClientCapabilities", {
    configurable: true,
    writable: true,
    value: async function getClientCapabilities(): Promise<Record<string, boolean>> {
      const [allBrowserCapability, isAvailable] = await Promise.all([
        typeof original === "function" ? original.call(publicKeyCredential).catch(() => undefined) : Promise.resolve(undefined),
        isHisarAvailable(),
      ]);
      return mergeClientCapabilities(allBrowserCapability, isAvailable);
    },
  });
}
