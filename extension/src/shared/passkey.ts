import type { ErrorCode, LoginSummary } from "./protocol";

export const passkeyRequestEventName = "hisar-passkey-request";
export const passkeyResponseEventName = "hisar-passkey-response";
export const passkeyCancelEventName = "hisar-passkey-cancel";
export const passkeyContentPortName = "hisar-passkey";
export const passkeyPromptPortName = "hisar-passkey-prompt";
export const passkeyPromptPath = "/passkey-prompt.html";
export const algorithmES256 = -7;

export interface SerializedCredentialDescriptor {
  type: string;
  id: string;
}

export interface SerializedPRFValues {
  first: string;
  second?: string;
}

export interface SerializedPRFInput {
  eval?: SerializedPRFValues;
  evalByCredential?: Record<string, SerializedPRFValues>;
}

export interface PasskeyPRFOutputJSON {
  enabled?: boolean;
  results?: SerializedPRFValues;
}

export type SerializedSignal =
  | { method: "signalUnknownCredential"; rpId: string; credentialId: string }
  | { method: "signalAllAcceptedCredentials"; rpId: string; userId: string; allAcceptedCredentialIds: string[] }
  | { method: "signalCurrentUserDetails"; rpId: string; userId: string; name: string; displayName: string };

export interface SerializedCreationOptions {
  rp: { id?: string; name: string };
  user: { id: string; name: string; displayName: string };
  challenge: string;
  pubKeyCredParams: { type: string; alg: number }[];
  timeout?: number;
  excludeCredentials: SerializedCredentialDescriptor[];
  authenticatorAttachment?: string;
  hasCredPropsExtension: boolean;
  prf?: SerializedPRFInput;
}

export interface SerializedRequestOptions {
  challenge: string;
  timeout?: number;
  rpId?: string;
  allowCredentials: SerializedCredentialDescriptor[];
  prf?: SerializedPRFInput;
}

export type PasskeyPageRequest =
  | { requestID: string; kind: "create"; mediation?: string; options: SerializedCreationOptions }
  | { requestID: string; kind: "get"; mediation?: string; options: SerializedRequestOptions }
  | { requestID: string; kind: "availability" }
  | { requestID: string; kind: "signal"; signal: SerializedSignal };

export interface CreatedCredentialJSON {
  kind: "create";
  credentialId: string;
  clientDataJSON: string;
  attestationObject: string;
  authenticatorData: string;
  publicKey: string;
  publicKeyAlgorithm: number;
  transports: string[];
  hasCredPropsExtension: boolean;
  prf?: PasskeyPRFOutputJSON;
}

export interface AssertedCredentialJSON {
  kind: "get";
  credentialId: string;
  clientDataJSON: string;
  authenticatorData: string;
  signature: string;
  userHandle: string;
  prf?: PasskeyPRFOutputJSON;
}

export type PasskeyCredentialJSON = CreatedCredentialJSON | AssertedCredentialJSON;

export type PasskeyPageResponse =
  | { requestID: string; outcome: "accepted" }
  | { requestID: string; outcome: "fallback" }
  | { requestID: string; outcome: "credential"; credential: PasskeyCredentialJSON }
  | { requestID: string; outcome: "error"; name: string; message: string }
  | { requestID: string; outcome: "availability"; isAvailable: boolean }
  | { requestID: string; outcome: "signal"; errorName?: string; message?: string };

export type ContentToBackgroundPasskeyMessage =
  | { type: "start"; request: PasskeyPageRequest; allAncestorOrigin: string[]; isPermissionPolicyAllowed: boolean }
  | { type: "cancel" }
  | { type: "keepAlive" }
  | { type: "visibility"; checkID: string; isVisible: boolean };

export type BackgroundToContentPasskeyMessage =
  | { type: "fallback" }
  | { type: "prompt"; promptUrl: string; isConditional: boolean }
  | { type: "credential"; credential: PasskeyCredentialJSON }
  | { type: "error"; name: string; message: string }
  | { type: "availability"; isAvailable: boolean }
  | { type: "signal"; errorName?: string; message?: string }
  | { type: "checkVisibility"; checkID: string }
  | { type: "resize"; height: number };

export type PasskeyPromptView = "locked" | "choose" | "no-passkeys" | "save" | "excluded";

// PasskeyChoice names the login it is in: two logins, as one in a shared vault, may hold the same credential ID.
export interface PasskeyChoice {
  credentialId: string;
  vaultID: string;
  itemID: string;
  title: string;
  userName: string;
  userDisplayName: string;
}

export interface PasskeyPromptState {
  view: PasskeyPromptView;
  kind: "create" | "get";
  relyingPartyID: string;
  userName: string;
  allPasskey: PasskeyChoice[];
  allLogin: LoginSummary[];
}

export type PasskeyPromptCall =
  | { method: "getState" }
  | { method: "unlock" }
  | { method: "choosePasskey"; credentialId: string; vaultID: string; itemID: string }
  | { method: "savePasskey"; vaultID?: string; itemID?: string }
  | { method: "confirmExcluded" }
  | { method: "useOtherDevice" }
  | { method: "dismiss" }
  | { method: "resize"; height: number };

export type PasskeyPromptRequest = PasskeyPromptCall & { callID: string; sessionID: string };

export type PasskeyPromptResponse =
  | { callID: string; ok: true; result: unknown }
  | { callID: string; ok: false; error: { code: ErrorCode; message: string } };

export function encodeBase64URL(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export function decodeBase64URL(text: string): Uint8Array {
  if (!/^[A-Za-z0-9_-]*$/.test(text) || text.length % 4 === 1) throw new TypeError("not base64url");
  const binary = atob(text.replace(/-/g, "+").replace(/_/g, "/") + "=".repeat((4 - (text.length % 4)) % 4));
  return Uint8Array.from(binary, (character) => character.charCodeAt(0));
}
