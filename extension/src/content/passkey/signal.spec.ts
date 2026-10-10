import { describe, expect, it, vi } from "vitest";
import type { PasskeyPageRequest, PasskeyPageResponse } from "../../shared/passkey";
import type { PasskeyClass, PasskeyGlobalScope } from "./main-world";
import { installClientCapabilities, installPasskeySignals, mergeClientCapabilities, serializeSignal } from "./signal";

const scope = { DOMException } as unknown as PasskeyGlobalScope;

function createTransport(answer: (request: PasskeyPageRequest) => PasskeyPageResponse) {
  const allRequest: PasskeyPageRequest[] = [];
  return {
    allRequest,
    send: vi.fn(async (request: PasskeyPageRequest) => {
      allRequest.push(request);
      return answer(request);
    }),
  };
}

const resolvedSignal = (request: PasskeyPageRequest): PasskeyPageResponse => ({ requestID: request.requestID, outcome: "signal" });

describe("serializeSignal", () => {
  it("serializes the three signals", () => {
    expect(serializeSignal("signalUnknownCredential", { rpId: "example.com", credentialId: "AQID" })).toEqual({ method: "signalUnknownCredential", rpId: "example.com", credentialId: "AQID" });
    expect(serializeSignal("signalAllAcceptedCredentials", { rpId: "example.com", userId: "dXNlcg", allAcceptedCredentialIds: ["AQ", "Ag"] })).toEqual({
      method: "signalAllAcceptedCredentials",
      rpId: "example.com",
      userId: "dXNlcg",
      allAcceptedCredentialIds: ["AQ", "Ag"],
    });
    expect(serializeSignal("signalCurrentUserDetails", { rpId: "example.com", userId: "dXNlcg", name: "alex", displayName: "Alex" })).toEqual({
      method: "signalCurrentUserDetails",
      rpId: "example.com",
      userId: "dXNlcg",
      name: "alex",
      displayName: "Alex",
    });
  });

  it("throws TypeError for missing members and IDs that are not base64url", () => {
    for (const [method, options] of [
      ["signalUnknownCredential", undefined],
      ["signalUnknownCredential", { credentialId: "AQ" }],
      ["signalUnknownCredential", { rpId: "example.com" }],
      ["signalUnknownCredential", { rpId: "example.com", credentialId: "A+/=" }],
      ["signalAllAcceptedCredentials", { rpId: "example.com", userId: "AQ" }],
      ["signalAllAcceptedCredentials", { rpId: "example.com", userId: "AQ", allAcceptedCredentialIds: "AQ" }],
      ["signalAllAcceptedCredentials", { rpId: "example.com", userId: "AQ", allAcceptedCredentialIds: ["AQ", "*"] }],
      ["signalAllAcceptedCredentials", { rpId: "example.com", userId: "$", allAcceptedCredentialIds: [] }],
      ["signalCurrentUserDetails", { rpId: "example.com", userId: "AQ", name: "alex" }],
      ["signalCurrentUserDetails", { rpId: "example.com", userId: "A", name: "alex", displayName: "Alex" }],
    ] as const) {
      expect(() => serializeSignal(method, options), JSON.stringify(options)).toThrow(TypeError);
    }
  });
});

describe("installPasskeySignals", () => {
  it("defines the signal methods, forwards to Hisar and resolves with undefined", async () => {
    const publicKeyCredential = {} as PasskeyClass;
    const transport = createTransport(resolvedSignal);
    installPasskeySignals(publicKeyCredential, scope, transport, () => "id-1");
    await expect(publicKeyCredential.signalUnknownCredential!({ rpId: "example.com", credentialId: "AQID" })).resolves.toBeUndefined();
    expect(transport.allRequest).toEqual([{ requestID: "id-1", kind: "signal", signal: { method: "signalUnknownCredential", rpId: "example.com", credentialId: "AQID" } }]);
    await expect(publicKeyCredential.signalCurrentUserDetails!({ rpId: "example.com", userId: "AQ", name: "a", displayName: "A" })).resolves.toBeUndefined();
    await expect(publicKeyCredential.signalAllAcceptedCredentials!({ rpId: "example.com", userId: "AQ", allAcceptedCredentialIds: [] })).resolves.toBeUndefined();
  });

  it("rejects with TypeError before contacting Hisar", async () => {
    const publicKeyCredential = {} as PasskeyClass;
    const transport = createTransport(resolvedSignal);
    installPasskeySignals(publicKeyCredential, scope, transport, () => "id-1");
    await expect(publicKeyCredential.signalUnknownCredential!({ rpId: "example.com", credentialId: "%%" })).rejects.toBeInstanceOf(TypeError);
    expect(transport.send).not.toHaveBeenCalled();
  });

  it("rejects with the service worker's SecurityError", async () => {
    const publicKeyCredential = {} as PasskeyClass;
    const transport = createTransport((request) => ({ requestID: request.requestID, outcome: "signal", errorName: "SecurityError", message: "wrong rpId" }));
    installPasskeySignals(publicKeyCredential, scope, transport, () => "id-1");
    const error = await publicKeyCredential.signalUnknownCredential!({ rpId: "evil.example", credentialId: "AQ" }).catch((rejection: unknown) => rejection);
    expect(error).toBeInstanceOf(DOMException);
    expect((error as DOMException).name).toBe("SecurityError");
  });

  it("also calls the browser's method and passes on its rejection", async () => {
    const original = vi.fn(async () => undefined);
    const publicKeyCredential = { signalUnknownCredential: original } as unknown as PasskeyClass;
    installPasskeySignals(publicKeyCredential, scope, createTransport(resolvedSignal), () => "id-1");
    const options = { rpId: "example.com", credentialId: "AQ" };
    await publicKeyCredential.signalUnknownCredential!(options);
    expect(original).toHaveBeenCalledWith(options);
    expect(original.mock.contexts[0]).toBe(publicKeyCredential);

    const rejecting = { signalCurrentUserDetails: vi.fn(async () => Promise.reject(new DOMException("public suffix", "SecurityError"))) } as unknown as PasskeyClass;
    installPasskeySignals(rejecting, scope, createTransport(resolvedSignal), () => "id-2");
    await expect(rejecting.signalCurrentUserDetails!({ rpId: "co.uk", userId: "AQ", name: "a", displayName: "A" })).rejects.toMatchObject({ name: "SecurityError" });
  });

  it("resolves when Hisar does not answer", async () => {
    const publicKeyCredential = {} as PasskeyClass;
    installPasskeySignals(publicKeyCredential, scope, createTransport((request) => ({ requestID: request.requestID, outcome: "fallback" })), () => "id-1");
    await expect(publicKeyCredential.signalUnknownCredential!({ rpId: "example.com", credentialId: "AQ" })).resolves.toBeUndefined();
  });
});

describe("getClientCapabilities", () => {
  it("merges the browser's capabilities with Hisar's", () => {
    expect(mergeClientCapabilities({ conditionalCreate: true, hybridTransport: true, passkeyPlatformAuthenticator: false, relatedOrigins: "x" }, true)).toEqual({
      conditionalCreate: true,
      hybridTransport: true,
      passkeyPlatformAuthenticator: true,
      userVerifyingPlatformAuthenticator: true,
      conditionalGet: true,
      "extension:prf": true,
      signalUnknownCredential: true,
      signalAllAcceptedCredentials: true,
      signalCurrentUserDetails: true,
    });
    expect(mergeClientCapabilities(undefined, false)).toEqual({
      passkeyPlatformAuthenticator: false,
      userVerifyingPlatformAuthenticator: false,
      conditionalGet: false,
      "extension:prf": false,
      signalUnknownCredential: true,
      signalAllAcceptedCredentials: true,
      signalCurrentUserDetails: true,
    });
    expect(mergeClientCapabilities({ conditionalGet: true, "extension:prf": true }, false)).toMatchObject({ conditionalGet: true, "extension:prf": true, passkeyPlatformAuthenticator: false });
  });

  it("wraps the browser's method, or defines it when missing", async () => {
    const original = vi.fn(async () => ({ hybridTransport: true, conditionalCreate: false }));
    const withOriginal = { getClientCapabilities: original } as unknown as PasskeyClass;
    installClientCapabilities(withOriginal, async () => true);
    await expect(withOriginal.getClientCapabilities!()).resolves.toMatchObject({ hybridTransport: true, conditionalCreate: false, passkeyPlatformAuthenticator: true, "extension:prf": true });
    expect(original).toHaveBeenCalledOnce();

    const failing = { getClientCapabilities: vi.fn(async () => Promise.reject(new Error("broken"))) } as unknown as PasskeyClass;
    installClientCapabilities(failing, async () => false);
    await expect(failing.getClientCapabilities!()).resolves.toMatchObject({ conditionalGet: false, signalUnknownCredential: true });

    const withoutOriginal = {} as PasskeyClass;
    const isHisarAvailable = vi.fn(async () => false);
    installClientCapabilities(withoutOriginal, isHisarAvailable);
    await expect(withoutOriginal.getClientCapabilities!()).resolves.toMatchObject({ userVerifyingPlatformAuthenticator: false, signalCurrentUserDetails: true });
    expect(isHisarAvailable).toHaveBeenCalledOnce();
  });
});
