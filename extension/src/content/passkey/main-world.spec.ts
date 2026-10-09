import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { decodeBase64URL, encodeBase64URL, passkeyCancelEventName, passkeyRequestEventName, passkeyResponseEventName, type PasskeyPageRequest } from "../../shared/passkey";
import {
  acceptTimeoutMilliseconds,
  buildCredential,
  computeTimeoutMilliseconds,
  createEventTransport,
  installPasskeyWrapper,
  serializeCreationOptions,
  type PasskeyGlobalScope,
  type PasskeyTransport,
} from "./main-world";

class FakePublicKeyCredential {
  static isUserVerifyingPlatformAuthenticatorAvailable = vi.fn(async () => false);
  static isConditionalMediationAvailable = vi.fn(async () => true);
  get id(): string {
    throw new TypeError("Illegal invocation");
  }
}
class FakeAttestationResponse {}
class FakeAssertionResponse {}

function createScope(): PasskeyGlobalScope & { allNativeCall: { method: string; options: unknown }[] } {
  const allNativeCall: { method: string; options: unknown }[] = [];
  class FakeCredentialsContainer {
    async create(options: unknown) {
      allNativeCall.push({ method: "create", options });
      return { native: "create" } as unknown as Credential;
    }
    async get(options: unknown) {
      allNativeCall.push({ method: "get", options });
      return { native: "get" } as unknown as Credential;
    }
  }
  FakePublicKeyCredential.isUserVerifyingPlatformAuthenticatorAvailable = vi.fn(async () => false);
  return {
    allNativeCall,
    CredentialsContainer: FakeCredentialsContainer as never,
    PublicKeyCredential: FakePublicKeyCredential,
    AuthenticatorAttestationResponse: FakeAttestationResponse,
    AuthenticatorAssertionResponse: FakeAssertionResponse,
    DOMException,
    setTimeout: ((callback: () => void, milliseconds: number) => setTimeout(callback, milliseconds)) as typeof setTimeout,
    clearTimeout: ((handle: ReturnType<typeof setTimeout>) => clearTimeout(handle)) as typeof clearTimeout,
  };
}

const creationOptions: PublicKeyCredentialCreationOptions = {
  rp: { id: "example.com", name: "Example" },
  user: { id: new Uint8Array([1, 2, 3]), name: "alex@example.com", displayName: "Alex" },
  challenge: new Uint8Array([9, 9, 9, 9]).buffer,
  pubKeyCredParams: [{ type: "public-key", alg: -7 }],
  excludeCredentials: [{ type: "public-key", id: new Uint8Array([5, 6]) }],
  authenticatorSelection: { authenticatorAttachment: "platform", residentKey: "required" },
  extensions: { credProps: true },
  timeout: 60_000,
};

const createdJSON = {
  kind: "create" as const,
  credentialId: "AQIDBA",
  clientDataJSON: encodeBase64URL(new TextEncoder().encode('{"type":"webauthn.create"}')),
  attestationObject: "o2NmbXQ",
  authenticatorData: "SZYN",
  publicKey: "MFkw",
  publicKeyAlgorithm: -7,
  transports: ["hybrid", "internal"],
  hasCredPropsExtension: true,
};

function createTransport(answer: (request: PasskeyPageRequest) => Promise<unknown>): PasskeyTransport & { allRequest: PasskeyPageRequest[]; allCancelled: string[] } {
  const allRequest: PasskeyPageRequest[] = [];
  const allCancelled: string[] = [];
  return {
    allRequest,
    allCancelled,
    send: (request) => {
      allRequest.push(request);
      return answer(request) as never;
    },
    cancel: (requestID) => allCancelled.push(requestID),
  };
}

afterEach(() => vi.useRealTimers());

describe("serializeCreationOptions", () => {
  it("turns BufferSources into base64url and keeps only what Hisar needs", () => {
    expect(serializeCreationOptions(creationOptions)).toEqual({
      rp: { id: "example.com", name: "Example" },
      user: { id: "AQID", name: "alex@example.com", displayName: "Alex" },
      challenge: "CQkJCQ",
      pubKeyCredParams: [{ type: "public-key", alg: -7 }],
      timeout: 60_000,
      excludeCredentials: [{ type: "public-key", id: "BQY" }],
      authenticatorAttachment: "platform",
      hasCredPropsExtension: true,
    });
    const view = new Uint8Array([0, 1, 2, 3]).subarray(1, 3);
    expect(serializeCreationOptions({ ...creationOptions, challenge: view }).challenge).toBe(encodeBase64URL(new Uint8Array([1, 2])));
  });

  it("clamps timeouts and gives conditional requests none", () => {
    expect(computeTimeoutMilliseconds(undefined, false)).toBe(300_000);
    expect(computeTimeoutMilliseconds(1000, false)).toBe(30_000);
    expect(computeTimeoutMilliseconds(3_600_000, false)).toBe(600_000);
    expect(computeTimeoutMilliseconds(60_000, true)).toBeUndefined();
  });
});

describe("buildCredential", () => {
  it("builds a PublicKeyCredential-shaped object from own properties", () => {
    const scope = createScope();
    const credential = buildCredential(createdJSON, scope) as PublicKeyCredential & { toJSON(): Record<string, unknown> };
    expect(credential).toBeInstanceOf(FakePublicKeyCredential);
    expect(credential.id).toBe("AQIDBA");
    expect(new Uint8Array(credential.rawId)).toEqual(new Uint8Array([1, 2, 3, 4]));
    expect(credential.type).toBe("public-key");
    expect(credential.authenticatorAttachment).toBe("platform");
    expect(credential.getClientExtensionResults()).toEqual({ credProps: { rk: true } });
    const response = credential.response as AuthenticatorAttestationResponse;
    expect(response).toBeInstanceOf(FakeAttestationResponse);
    expect(new TextDecoder().decode(response.clientDataJSON)).toBe('{"type":"webauthn.create"}');
    expect(response.getTransports()).toEqual(["hybrid", "internal"]);
    expect(response.getPublicKeyAlgorithm()).toBe(-7);
    expect(encodeBase64URL(new Uint8Array(response.getPublicKey()!))).toBe("MFkw");
    expect(encodeBase64URL(new Uint8Array(response.getAuthenticatorData()))).toBe("SZYN");
    expect(credential.toJSON()).toEqual({
      id: "AQIDBA",
      rawId: "AQIDBA",
      type: "public-key",
      authenticatorAttachment: "platform",
      response: {
        clientDataJSON: createdJSON.clientDataJSON,
        attestationObject: "o2NmbXQ",
        authenticatorData: "SZYN",
        publicKey: "MFkw",
        publicKeyAlgorithm: -7,
        transports: ["hybrid", "internal"],
      },
      clientExtensionResults: { credProps: { rk: true } },
    });
  });

  it("builds assertion responses with the user handle", () => {
    const credential = buildCredential(
      { kind: "get", credentialId: "AQ", clientDataJSON: "e30", authenticatorData: "SZYN", signature: "MEUC", userHandle: "dXNlcg" },
      createScope(),
    );
    const response = credential.response as AuthenticatorAssertionResponse;
    expect(response).toBeInstanceOf(FakeAssertionResponse);
    expect(new TextDecoder().decode(response.userHandle!)).toBe("user");
    expect(encodeBase64URL(new Uint8Array(response.signature))).toBe("MEUC");
    expect(credential.getClientExtensionResults()).toEqual({});
  });
});

describe("installPasskeyWrapper", () => {
  let scope: ReturnType<typeof createScope>;
  let container: CredentialsContainer;
  let requestCount: number;

  beforeEach(() => {
    scope = createScope();
    container = new (scope.CredentialsContainer as unknown as new () => CredentialsContainer)();
    requestCount = 0;
  });

  function install(transport: PasskeyTransport): void {
    installPasskeyWrapper(scope, transport, () => `request-${++requestCount}`);
  }

  it("passes requests without publicKey to the browser untouched", async () => {
    const transport = createTransport(async () => ({ outcome: "fallback" }));
    install(transport);
    const options = { password: true } as CredentialRequestOptions;
    await container.get(options);
    expect(scope.allNativeCall).toEqual([{ method: "get", options }]);
    expect(transport.allRequest).toEqual([]);
  });

  it("returns Hisar's credential for create", async () => {
    const transport = createTransport(async (request) => ({ requestID: request.requestID, outcome: "credential", credential: createdJSON }));
    install(transport);
    const credential = (await container.create({ publicKey: creationOptions })) as PublicKeyCredential;
    expect(credential.id).toBe("AQIDBA");
    expect(transport.allRequest[0]).toMatchObject({ requestID: "request-1", kind: "create", options: { challenge: "CQkJCQ" } });
    expect(scope.allNativeCall).toEqual([]);
  });

  it("falls back to the browser with the same options", async () => {
    install(createTransport(async (request) => ({ requestID: request.requestID, outcome: "fallback" })));
    const options = { publicKey: { challenge: new Uint8Array([1]), allowCredentials: [] }, mediation: "conditional" } as unknown as CredentialRequestOptions;
    expect(await container.get(options)).toEqual({ native: "get" });
    expect(scope.allNativeCall).toEqual([{ method: "get", options }]);
  });

  it("falls back when the options cannot be read or the signal is already aborted", async () => {
    const transport = createTransport(async () => ({ outcome: "credential", credential: createdJSON }));
    install(transport);
    await container.create({ publicKey: { ...creationOptions, challenge: "text" as never } });
    await container.create({ publicKey: creationOptions, signal: AbortSignal.abort() });
    expect(scope.allNativeCall.map((call) => call.method)).toEqual(["create", "create"]);
    expect(transport.allRequest).toEqual([]);
  });

  it("rejects with the error name Hisar answers", async () => {
    install(createTransport(async (request) => ({ requestID: request.requestID, outcome: "error", name: "InvalidStateError", message: "exists" })));
    await expect(container.create({ publicKey: creationOptions })).rejects.toMatchObject({ name: "InvalidStateError", message: "exists" });
  });

  it("cancels on abort and on timeout", async () => {
    const transport = createTransport(() => new Promise(() => undefined));
    install(transport);
    const controller = new AbortController();
    const aborted = container.get({ publicKey: { challenge: new Uint8Array([1]) }, signal: controller.signal });
    controller.abort(new DOMException("stop", "AbortError"));
    await expect(aborted).rejects.toMatchObject({ name: "AbortError", message: "stop" });
    expect(transport.allCancelled).toEqual(["request-1"]);

    vi.useFakeTimers();
    const timedOut = container.get({ publicKey: { challenge: new Uint8Array([1]), timeout: 1 } });
    const assertion = expect(timedOut).rejects.toMatchObject({ name: "NotAllowedError" });
    await vi.advanceTimersByTimeAsync(30_000);
    await assertion;
    expect(transport.allCancelled).toEqual(["request-1", "request-2"]);
  });

  it("reports a platform authenticator when the browser or a paired Hisar has one", async () => {
    const transport = createTransport(async (request) => ({ requestID: request.requestID, outcome: "availability", isAvailable: true }));
    install(transport);
    const publicKeyCredential = scope.PublicKeyCredential!;
    expect(await publicKeyCredential.isUserVerifyingPlatformAuthenticatorAvailable!()).toBe(true);
    expect(transport.allRequest[0]).toMatchObject({ kind: "availability" });
    expect(await publicKeyCredential.isConditionalMediationAvailable!()).toBe(true);
    expect(transport.allRequest).toHaveLength(1);
  });
});

describe("createEventTransport", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("sends JSON strings through document events and waits for the answer", async () => {
    const allRequestDetail: string[] = [];
    const listener = (event: Event) => {
      const detail = String((event as CustomEvent).detail);
      allRequestDetail.push(detail);
      const request = JSON.parse(detail) as PasskeyPageRequest;
      document.dispatchEvent(new CustomEvent(passkeyResponseEventName, { detail: JSON.stringify({ requestID: request.requestID, outcome: "accepted" }) }));
      document.dispatchEvent(new CustomEvent(passkeyResponseEventName, { detail: JSON.stringify({ requestID: "someone-else", outcome: "fallback" }) }));
      setTimeout(() => document.dispatchEvent(new CustomEvent(passkeyResponseEventName, { detail: JSON.stringify({ requestID: request.requestID, outcome: "error", name: "NotAllowedError", message: "closed" }) })), 1500);
    };
    document.addEventListener(passkeyRequestEventName, listener);
    const transport = createEventTransport(document, createScope());
    const settled = await transport.send({ requestID: "r1", kind: "get", options: { challenge: "AQ", allowCredentials: [] } });
    expect(settled).toEqual({ requestID: "r1", outcome: "error", name: "NotAllowedError", message: "closed" });
    expect(typeof allRequestDetail[0]).toBe("string");
    const cancelled: string[] = [];
    document.addEventListener(passkeyCancelEventName, (event) => cancelled.push(String((event as CustomEvent).detail)));
    transport.cancel("r1");
    expect(cancelled).toEqual(['{"requestID":"r1"}']);
    document.removeEventListener(passkeyRequestEventName, listener);
  });

  it("falls back when no content script accepts the request", async () => {
    vi.useFakeTimers();
    const transport = createEventTransport(document, createScope());
    const settled = transport.send({ requestID: "r9", kind: "create", options: {} as never });
    await vi.advanceTimersByTimeAsync(acceptTimeoutMilliseconds);
    expect(await settled).toEqual({ requestID: "r9", outcome: "fallback" });
    const availability = transport.send({ requestID: "r10", kind: "availability" });
    await vi.advanceTimersByTimeAsync(acceptTimeoutMilliseconds);
    expect(await availability).toEqual({ requestID: "r10", outcome: "availability", isAvailable: false });
  });
});

describe("base64url helpers", () => {
  it("round-trip bytes and reject other alphabets", () => {
    const bytes = new Uint8Array([251, 255, 0, 1]);
    expect(encodeBase64URL(bytes)).toBe("-_8AAQ");
    expect(decodeBase64URL("-_8AAQ")).toEqual(bytes);
    expect(() => decodeBase64URL("+/8=")).toThrow();
    expect(() => decodeBase64URL("A")).toThrow();
  });
});
