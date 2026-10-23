import {
  decodeBase64URL,
  encodeBase64URL,
  passkeyCancelEventName,
  passkeyRequestEventName,
  passkeyResponseEventName,
  type PasskeyCredentialJSON,
  type PasskeyPageRequest,
  type PasskeyPageResponse,
  type SerializedCreationOptions,
  type SerializedCredentialDescriptor,
  type SerializedRequestOptions,
} from "../../shared/passkey";
import { buildPRFExtensionResult, buildPRFExtensionResultJSON, findPRFInputError, serializePRFInput } from "./prf";
import { installClientCapabilities, installPasskeySignals } from "./signal";

export const defaultTimeoutMilliseconds = 5 * 60 * 1000;
export const minimumTimeoutMilliseconds = 30 * 1000;
export const maximumTimeoutMilliseconds = 10 * 60 * 1000;
export const acceptTimeoutMilliseconds = 1000;
export const availabilityTimeoutMilliseconds = 1000;

type Settled = PasskeyPageResponse | { requestID: string; outcome: "timeout" };

export interface PasskeyTransport {
  send(request: PasskeyPageRequest): Promise<Settled>;
  cancel(requestID: string): void;
}

type CredentialMethod = (this: CredentialsContainer, options?: CredentialCreationOptions | CredentialRequestOptions) => Promise<Credential | null>;

export interface PasskeyClass {
  prototype: object;
  isUserVerifyingPlatformAuthenticatorAvailable?: () => Promise<boolean>;
  isConditionalMediationAvailable?: () => Promise<boolean>;
  getClientCapabilities?: () => Promise<Record<string, boolean>>;
  signalUnknownCredential?: (options: unknown) => Promise<void>;
  signalAllAcceptedCredentials?: (options: unknown) => Promise<void>;
  signalCurrentUserDetails?: (options: unknown) => Promise<void>;
}

export interface PasskeyGlobalScope {
  CredentialsContainer?: { prototype: { create: CredentialMethod; get: CredentialMethod } };
  PublicKeyCredential?: PasskeyClass;
  AuthenticatorAttestationResponse?: { prototype: object };
  AuthenticatorAssertionResponse?: { prototype: object };
  DOMException: typeof DOMException;
  setTimeout: typeof setTimeout;
  clearTimeout: typeof clearTimeout;
}

function toBytes(source: unknown): Uint8Array {
  if (source instanceof ArrayBuffer) return new Uint8Array(source.slice(0));
  if (ArrayBuffer.isView(source)) return new Uint8Array(source.buffer.slice(source.byteOffset, source.byteOffset + source.byteLength));
  throw new TypeError("expected a BufferSource");
}

function encodeBufferSource(source: unknown): string {
  return encodeBase64URL(toBytes(source));
}

function serializeDescriptors(allDescriptor: unknown): SerializedCredentialDescriptor[] {
  if (allDescriptor === undefined) return [];
  return Array.from(allDescriptor as Iterable<PublicKeyCredentialDescriptor>, (descriptor) => ({ type: String(descriptor.type), id: encodeBufferSource(descriptor.id) }));
}

export function serializeCreationOptions(publicKey: PublicKeyCredentialCreationOptions): SerializedCreationOptions {
  return {
    userVerification: publicKey.authenticatorSelection?.userVerification ?? "preferred",
    rp: { ...(publicKey.rp.id === undefined ? {} : { id: String(publicKey.rp.id) }), name: String(publicKey.rp.name) },
    user: { id: encodeBufferSource(publicKey.user.id), name: String(publicKey.user.name), displayName: String(publicKey.user.displayName ?? "") },
    challenge: encodeBufferSource(publicKey.challenge),
    pubKeyCredParams: Array.from(publicKey.pubKeyCredParams ?? [], (parameter) => ({ type: String(parameter.type), alg: Number(parameter.alg) })),
    ...(publicKey.timeout === undefined ? {} : { timeout: Number(publicKey.timeout) }),
    excludeCredentials: serializeDescriptors(publicKey.excludeCredentials),
    ...(publicKey.authenticatorSelection?.authenticatorAttachment ? { authenticatorAttachment: String(publicKey.authenticatorSelection.authenticatorAttachment) } : {}),
    hasCredPropsExtension: publicKey.extensions?.credProps === true,
    ...withPRF(publicKey.extensions),
  };
}

function withPRF(extensions: unknown): { prf?: ReturnType<typeof serializePRFInput> } {
  const prf = serializePRFInput(extensions);
  return prf ? { prf } : {};
}

export function serializeRequestOptions(publicKey: PublicKeyCredentialRequestOptions): SerializedRequestOptions {
  return {
    userVerification: publicKey.userVerification ?? "preferred",
    challenge: encodeBufferSource(publicKey.challenge),
    ...(publicKey.timeout === undefined ? {} : { timeout: Number(publicKey.timeout) }),
    ...(publicKey.rpId === undefined ? {} : { rpId: String(publicKey.rpId) }),
    allowCredentials: serializeDescriptors(publicKey.allowCredentials),
    ...withPRF(publicKey.extensions),
  };
}

export function computeTimeoutMilliseconds(timeout: number | undefined, isConditional: boolean): number | undefined {
  if (isConditional) return undefined;
  if (timeout === undefined || !Number.isFinite(timeout)) return defaultTimeoutMilliseconds;
  return Math.min(Math.max(timeout, minimumTimeoutMilliseconds), maximumTimeoutMilliseconds);
}

function toArrayBuffer(text: string): ArrayBuffer {
  const bytes = decodeBase64URL(text);
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}

function defineAll(target: object, allProperty: Record<string, unknown>): void {
  for (const [name, value] of Object.entries(allProperty)) {
    Object.defineProperty(target, name, { value, enumerable: true, configurable: true, writable: false });
  }
}

// Own properties on the native prototypes, so instanceof works and the native getters are never called.
export function buildCredential(json: PasskeyCredentialJSON, scope: PasskeyGlobalScope): PublicKeyCredential {
  const credential = Object.create(scope.PublicKeyCredential?.prototype ?? Object.prototype) as object;
  const clientExtensionResults: Record<string, unknown> = json.kind === "create" && json.hasCredPropsExtension ? { credProps: { rk: true } } : {};
  const clientExtensionResultsJSON: Record<string, unknown> = { ...clientExtensionResults };
  if (json.prf) {
    clientExtensionResults.prf = buildPRFExtensionResult(json.prf);
    clientExtensionResultsJSON.prf = buildPRFExtensionResultJSON(json.prf);
  }
  let response: object;
  let responseJSON: Record<string, unknown>;
  if (json.kind === "create") {
    response = Object.create(scope.AuthenticatorAttestationResponse?.prototype ?? Object.prototype) as object;
    const allTransport = [...json.transports];
    defineAll(response, {
      clientDataJSON: toArrayBuffer(json.clientDataJSON),
      attestationObject: toArrayBuffer(json.attestationObject),
      getTransports: () => [...allTransport],
      getAuthenticatorData: () => toArrayBuffer(json.authenticatorData),
      getPublicKey: () => toArrayBuffer(json.publicKey),
      getPublicKeyAlgorithm: () => json.publicKeyAlgorithm,
    });
    responseJSON = {
      clientDataJSON: json.clientDataJSON,
      attestationObject: json.attestationObject,
      authenticatorData: json.authenticatorData,
      publicKey: json.publicKey,
      publicKeyAlgorithm: json.publicKeyAlgorithm,
      transports: allTransport,
    };
  } else {
    response = Object.create(scope.AuthenticatorAssertionResponse?.prototype ?? Object.prototype) as object;
    defineAll(response, {
      clientDataJSON: toArrayBuffer(json.clientDataJSON),
      authenticatorData: toArrayBuffer(json.authenticatorData),
      signature: toArrayBuffer(json.signature),
      userHandle: json.userHandle ? toArrayBuffer(json.userHandle) : null,
    });
    responseJSON = {
      clientDataJSON: json.clientDataJSON,
      authenticatorData: json.authenticatorData,
      signature: json.signature,
      ...(json.userHandle ? { userHandle: json.userHandle } : {}),
    };
  }
  defineAll(credential, {
    id: json.credentialId,
    rawId: toArrayBuffer(json.credentialId),
    type: "public-key",
    authenticatorAttachment: "platform",
    response,
    getClientExtensionResults: () => structuredClone(clientExtensionResults),
    toJSON: () => ({
      id: json.credentialId,
      rawId: json.credentialId,
      type: "public-key",
      authenticatorAttachment: "platform",
      response: responseJSON,
      clientExtensionResults: structuredClone(clientExtensionResultsJSON),
    }),
  });
  return credential as PublicKeyCredential;
}

function serializeRequest(
  kind: "create" | "get",
  requestID: string,
  options: CredentialCreationOptions | CredentialRequestOptions,
): Extract<PasskeyPageRequest, { kind: "create" | "get" }> | undefined {
  try {
    const requestedMediation = (options as { mediation?: unknown }).mediation;
    const mediation = typeof requestedMediation === "string" ? requestedMediation : undefined;
    if (kind === "create") {
      return { requestID, kind, mediation, options: serializeCreationOptions((options as CredentialCreationOptions).publicKey!) };
    }
    return { requestID, kind, mediation, options: serializeRequestOptions((options as CredentialRequestOptions).publicKey!) };
  } catch {
    return undefined;
  }
}

function findPRFInputSafely(kind: "create" | "get", options: CredentialCreationOptions | CredentialRequestOptions, scope: PasskeyGlobalScope): Error | undefined {
  try {
    return findPRFInputError(kind, options.publicKey as PublicKeyCredentialRequestOptions, (message, name) => new scope.DOMException(message, name));
  } catch {
    return undefined;
  }
}

async function runPasskeyRequest(
  kind: "create" | "get",
  original: CredentialMethod,
  container: CredentialsContainer,
  options: CredentialCreationOptions | CredentialRequestOptions,
  scope: PasskeyGlobalScope,
  transport: PasskeyTransport,
  createID: () => string,
): Promise<Credential | null> {
  const signal = options.signal;
  const prfError = signal?.aborted ? undefined : findPRFInputSafely(kind, options, scope);
  if (prfError) throw prfError;
  const request = signal?.aborted ? undefined : serializeRequest(kind, createID(), options);
  if (!request) return original.call(container, options);
  const timeoutMilliseconds = computeTimeoutMilliseconds(request.options.timeout, request.mediation === "conditional");
  let timer: ReturnType<typeof setTimeout> | undefined;
  let abortListener: (() => void) | undefined;
  const allPending: Promise<Settled | { requestID: string; outcome: "aborted" }>[] = [transport.send(request)];
  if (timeoutMilliseconds !== undefined) {
    allPending.push(new Promise((resolve) => (timer = scope.setTimeout(() => resolve({ requestID: request.requestID, outcome: "timeout" }), timeoutMilliseconds))));
  }
  if (signal) {
    allPending.push(
      new Promise((resolve) => {
        abortListener = () => resolve({ requestID: request.requestID, outcome: "aborted" });
        signal.addEventListener("abort", abortListener, { once: true });
      }),
    );
  }
  const settled = await Promise.race(allPending);
  if (timer !== undefined) scope.clearTimeout(timer);
  if (signal && abortListener) signal.removeEventListener("abort", abortListener);
  switch (settled.outcome) {
    case "credential":
      return buildCredential(settled.credential, scope);
    case "error":
      throw new scope.DOMException(settled.message, settled.name);
    case "aborted":
      transport.cancel(request.requestID);
      throw signal?.reason ?? new scope.DOMException("The operation was aborted", "AbortError");
    case "timeout":
      transport.cancel(request.requestID);
      throw new scope.DOMException("The operation either timed out or was not allowed", "NotAllowedError");
    default:
      return original.call(container, options);
  }
}

export function installPasskeyWrapper(scope: PasskeyGlobalScope, transport: PasskeyTransport, createID: () => string): void {
  const containerPrototype = scope.CredentialsContainer?.prototype;
  if (!containerPrototype || !scope.PublicKeyCredential) return;
  const originalCreate = containerPrototype.create;
  const originalGet = containerPrototype.get;
  containerPrototype.create = function create(this: CredentialsContainer, options?: CredentialCreationOptions | CredentialRequestOptions) {
    if (!options?.publicKey) return originalCreate.call(this, options);
    return runPasskeyRequest("create", originalCreate, this, options, scope, transport, createID);
  };
  containerPrototype.get = function get(this: CredentialsContainer, options?: CredentialCreationOptions | CredentialRequestOptions) {
    if (!options?.publicKey) return originalGet.call(this, options);
    return runPasskeyRequest("get", originalGet, this, options, scope, transport, createID);
  };

  const publicKeyCredential = scope.PublicKeyCredential;
  const askHisar = async (): Promise<boolean> => {
    const settled = await transport.send({ requestID: createID(), kind: "availability" });
    return settled.outcome === "availability" && settled.isAvailable;
  };
  for (const name of ["isUserVerifyingPlatformAuthenticatorAvailable", "isConditionalMediationAvailable"] as const) {
    const original = publicKeyCredential[name];
    if (typeof original !== "function") continue;
    Object.defineProperty(publicKeyCredential, name, {
      configurable: true,
      writable: true,
      value: async function isAvailable(): Promise<boolean> {
        const isNativeAvailable = await original.call(publicKeyCredential).catch(() => false);
        return isNativeAvailable || askHisar().catch(() => false);
      },
    });
  }
  installPasskeySignals(publicKeyCredential, scope, transport, createID);
  installClientCapabilities(publicKeyCredential, () => askHisar().catch(() => false));
}

export function createEventTransport(document: Document, scope: PasskeyGlobalScope): PasskeyTransport {
  const allResolver = new Map<string, { resolve: (settled: Settled) => void; acceptTimer?: ReturnType<typeof setTimeout> }>();
  document.addEventListener(passkeyResponseEventName, (event) => {
    let response: PasskeyPageResponse;
    try {
      response = JSON.parse(String((event as CustomEvent).detail)) as PasskeyPageResponse;
    } catch {
      return;
    }
    const pending = allResolver.get(response?.requestID);
    if (!pending) return;
    if (pending.acceptTimer !== undefined) scope.clearTimeout(pending.acceptTimer);
    pending.acceptTimer = undefined;
    if (response.outcome === "accepted") return;
    allResolver.delete(response.requestID);
    pending.resolve(response);
  });
  return {
    send(request) {
      return new Promise((resolve) => {
        const notAccepted: Settled =
          request.kind === "availability"
            ? { requestID: request.requestID, outcome: "availability", isAvailable: false }
            : request.kind === "signal"
              ? { requestID: request.requestID, outcome: "signal" }
              : { requestID: request.requestID, outcome: "fallback" };
        const pending = {
          resolve,
          acceptTimer: scope.setTimeout(() => {
            allResolver.delete(request.requestID);
            resolve(notAccepted);
          }, request.kind === "availability" || request.kind === "signal" ? availabilityTimeoutMilliseconds : acceptTimeoutMilliseconds),
        };
        allResolver.set(request.requestID, pending);
        document.dispatchEvent(new CustomEvent(passkeyRequestEventName, { detail: JSON.stringify(request) }));
      });
    },
    cancel(requestID) {
      allResolver.delete(requestID);
      document.dispatchEvent(new CustomEvent(passkeyCancelEventName, { detail: JSON.stringify({ requestID }) }));
    },
  };
}
