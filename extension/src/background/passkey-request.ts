import { algorithmES256, decodeBase64URL, type PasskeyPageRequest } from "../shared/passkey";

export const maximumChallengeBytes = 1024;
export const maximumUserHandleBytes = 64;
export const maximumCredentialIDBytes = 1023;
export const maximumCredentialCount = 64;
export const maximumNameLength = 256;
export const maximumRelyingPartyIDLength = 253;

export interface ValidatedCreation {
  kind: "create";
  relyingPartyID?: string;
  relyingPartyName: string;
  userHandle: string;
  userName: string;
  userDisplayName: string;
  challenge: string;
  allExcludedCredentialID: string[];
  hasCredPropsExtension: boolean;
}

export interface ValidatedAssertion {
  kind: "get";
  relyingPartyID?: string;
  challenge: string;
  allAllowedCredentialID: string[];
  isConditional: boolean;
}

export type PasskeyValidation = { isValid: true; request: ValidatedCreation | ValidatedAssertion } | { isValid: false; reason: string };

export interface FrameOrigin {
  origin: string;
  topOrigin: string;
  isCrossOrigin: boolean;
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function readBinary(value: unknown, minimumBytes: number, maximumBytes: number): string | undefined {
  if (typeof value !== "string") return undefined;
  try {
    const length = decodeBase64URL(value).length;
    return length >= minimumBytes && length <= maximumBytes ? value : undefined;
  } catch {
    return undefined;
  }
}

function readName(value: unknown, isRequired: boolean): string | undefined {
  if (value === undefined && !isRequired) return "";
  if (typeof value !== "string" || [...value].length > maximumNameLength) return undefined;
  return value;
}

function readRelyingPartyID(value: unknown): string | undefined | false {
  if (value === undefined) return undefined;
  if (typeof value !== "string" || value === "" || value.length > maximumRelyingPartyIDLength) return false;
  return value;
}

function readAllCredentialID(value: unknown): string[] | undefined {
  if (value === undefined) return [];
  if (!Array.isArray(value) || value.length > maximumCredentialCount) return undefined;
  const allCredentialID: string[] = [];
  for (const descriptor of value) {
    if (!isObject(descriptor) || descriptor.type !== "public-key") continue;
    const credentialID = readBinary(descriptor.id, 1, maximumCredentialIDBytes);
    if (!credentialID) return undefined;
    allCredentialID.push(credentialID);
  }
  return allCredentialID;
}

function rejectRequest(reason: string): PasskeyValidation {
  return { isValid: false, reason };
}

export function validatePasskeyRequest(request: PasskeyPageRequest | unknown): PasskeyValidation {
  if (!isObject(request) || !isObject(request.options)) return rejectRequest("no options");
  const options = request.options;
  if (request.mediation === "silent") return rejectRequest("silent mediation needs no Hisar prompt");
  const challenge = readBinary(options.challenge, 1, maximumChallengeBytes);
  if (!challenge) return rejectRequest("invalid challenge");
  if (request.kind === "get") {
    const relyingPartyID = readRelyingPartyID(options.rpId);
    const allAllowedCredentialID = readAllCredentialID(options.allowCredentials);
    if (relyingPartyID === false || !allAllowedCredentialID) return rejectRequest("invalid request options");
    return {
      isValid: true,
      request: { kind: "get", relyingPartyID, challenge, allAllowedCredentialID, isConditional: request.mediation === "conditional" },
    };
  }
  if (request.kind !== "create") return rejectRequest("unknown kind");
  if (request.mediation === "conditional") return rejectRequest("conditional create is left to the browser");
  if (!isObject(options.rp) || !isObject(options.user)) return rejectRequest("missing rp or user");
  const relyingPartyID = readRelyingPartyID(options.rp.id);
  const relyingPartyName = readName(options.rp.name, true);
  const userHandle = readBinary(options.user.id, 1, maximumUserHandleBytes);
  const userName = readName(options.user.name, true);
  const userDisplayName = readName(options.user.displayName, false);
  const allExcludedCredentialID = readAllCredentialID(options.excludeCredentials);
  if (relyingPartyID === false || relyingPartyName === undefined || !userHandle || userName === undefined || userDisplayName === undefined || !allExcludedCredentialID) {
    return rejectRequest("invalid creation options");
  }
  if (!Array.isArray(options.pubKeyCredParams)) return rejectRequest("invalid pubKeyCredParams");
  const isES256Allowed =
    options.pubKeyCredParams.length === 0 ||
    options.pubKeyCredParams.some((parameter) => isObject(parameter) && parameter.type === "public-key" && parameter.alg === algorithmES256);
  if (!isES256Allowed) return rejectRequest("ES256 is not allowed");
  if (options.authenticatorAttachment === "cross-platform") return rejectRequest("a cross-platform authenticator was asked for");
  return {
    isValid: true,
    request: {
      kind: "create",
      relyingPartyID,
      relyingPartyName,
      userHandle,
      userName,
      userDisplayName,
      challenge,
      allExcludedCredentialID,
      hasCredPropsExtension: options.hasCredPropsExtension === true,
    },
  };
}

export interface PasskeySender {
  url?: string;
  origin?: string;
  frameId?: number;
  tab?: { id?: number; url?: string };
}

function readWebOrigin(url: string | undefined): string | undefined {
  if (!url) return undefined;
  try {
    const parsed = new URL(url);
    if (parsed.protocol === "https:") return parsed.origin;
    const isLocalhost = parsed.hostname === "localhost" || parsed.hostname.endsWith(".localhost");
    return parsed.protocol === "http:" && isLocalhost ? parsed.origin : undefined;
  } catch {
    return undefined;
  }
}

// The origin comes from the browser's sender information, never from the page.
export function readFrameOrigin(sender: PasskeySender, allAncestorOrigin: unknown): FrameOrigin | undefined {
  const origin = readWebOrigin(sender.url);
  const topOrigin = readWebOrigin(sender.tab?.url);
  if (!origin || !topOrigin || sender.frameId === undefined) return undefined;
  if (sender.origin !== undefined && sender.origin !== origin) return undefined;
  if (sender.frameId === 0) return origin === topOrigin ? { origin, topOrigin, isCrossOrigin: false } : undefined;
  if (!Array.isArray(allAncestorOrigin) || allAncestorOrigin.length === 0 || allAncestorOrigin.some((ancestor) => typeof ancestor !== "string")) {
    return undefined;
  }
  if (allAncestorOrigin[allAncestorOrigin.length - 1] !== topOrigin) return undefined;
  return { origin, topOrigin, isCrossOrigin: allAncestorOrigin.some((ancestor) => ancestor !== origin) };
}
