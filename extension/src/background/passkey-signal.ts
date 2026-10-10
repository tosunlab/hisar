import { decodeBase64URL, type SerializedSignal } from "../shared/passkey";
import type { DesktopPasskeyParameters, PasskeySignalType } from "../shared/passkey-protocol";
import { maximumCredentialCount, maximumCredentialIDBytes, maximumNameLength, maximumRelyingPartyIDLength, maximumUserHandleBytes, type FrameOrigin } from "./passkey-request";

export type SignalCheck =
  | { outcome: "ignore" }
  | { outcome: "reject"; errorName: string; message: string }
  | { outcome: "forward"; type: PasskeySignalType; parameters: DesktopPasskeyParameters };

function isIPAddress(host: string): boolean {
  return host.startsWith("[") || host.includes(":") || /^\d{1,3}(\.\d{1,3}){3}$/.test(host);
}

// A suffix needs two labels; the browser's own method and the desktop also check the public suffix list.
export function isRelyingPartyIDValidForOrigin(relyingPartyID: string, origin: string): boolean {
  let host: string;
  try {
    host = new URL(origin).hostname.toLowerCase();
  } catch {
    return false;
  }
  const normalizedID = relyingPartyID.toLowerCase();
  if (normalizedID === "" || normalizedID.length > maximumRelyingPartyIDLength || normalizedID.startsWith(".") || normalizedID.endsWith(".")) return false;
  if (normalizedID === host) return true;
  if (isIPAddress(host) || isIPAddress(normalizedID) || !normalizedID.includes(".")) return false;
  return host.endsWith(`.${normalizedID}`);
}

function hasLength(text: unknown, minimumBytes: number, maximumBytes: number): text is string {
  if (typeof text !== "string") return false;
  try {
    const length = decodeBase64URL(text).length;
    return length >= minimumBytes && length <= maximumBytes;
  } catch {
    return false;
  }
}

function isName(value: unknown): value is string {
  return typeof value === "string" && [...value].length <= maximumNameLength;
}

function buildForward(signal: SerializedSignal, parameters: DesktopPasskeyParameters): SignalCheck {
  switch (signal.method) {
    case "signalUnknownCredential":
      if (!hasLength(signal.credentialId, 1, maximumCredentialIDBytes)) return { outcome: "ignore" };
      return { outcome: "forward", type: "passkeys.signalUnknownCredential", parameters: { ...parameters, credentialId: signal.credentialId } };
    case "signalAllAcceptedCredentials": {
      const allAccepted = signal.allAcceptedCredentialIds;
      if (!hasLength(signal.userId, 1, maximumUserHandleBytes) || !Array.isArray(allAccepted) || allAccepted.length > maximumCredentialCount) return { outcome: "ignore" };
      if (!allAccepted.every((credentialID) => hasLength(credentialID, 1, maximumCredentialIDBytes))) return { outcome: "ignore" };
      return { outcome: "forward", type: "passkeys.signalAllAcceptedCredentials", parameters: { ...parameters, userHandle: signal.userId, allAcceptedCredentialIds: [...allAccepted] } };
    }
    case "signalCurrentUserDetails":
      if (!hasLength(signal.userId, 1, maximumUserHandleBytes) || !isName(signal.name) || !isName(signal.displayName)) return { outcome: "ignore" };
      return {
        outcome: "forward",
        type: "passkeys.signalCurrentUserDetails",
        parameters: { ...parameters, userHandle: signal.userId, userName: signal.name, userDisplayName: signal.displayName },
      };
  }
  return { outcome: "ignore" };
}

// The origin is the sender's (readFrameOrigin), never the page's.
export function checkPasskeySignal(signal: unknown, frame: FrameOrigin | undefined): SignalCheck {
  if (!frame || typeof signal !== "object" || signal === null) return { outcome: "ignore" };
  const serialized = signal as SerializedSignal;
  if (typeof serialized.rpId !== "string") return { outcome: "ignore" };
  if (!isRelyingPartyIDValidForOrigin(serialized.rpId, frame.origin)) {
    return { outcome: "reject", errorName: "SecurityError", message: "The relying party ID is not a registrable domain suffix of, nor equal to, the current domain" };
  }
  return buildForward(serialized, { origin: frame.origin, relyingPartyId: serialized.rpId.toLowerCase() });
}
