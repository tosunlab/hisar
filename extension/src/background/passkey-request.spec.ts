import { describe, expect, it } from "vitest";
import { encodeBase64URL, type PasskeyPageRequest } from "../shared/passkey";
import { readFrameOrigin, validatePasskeyRequest } from "./passkey-request";

const challenge = encodeBase64URL(new Uint8Array(32).fill(7));

function createCreation(overrides: Record<string, unknown> = {}, mediation?: string): PasskeyPageRequest {
  return {
    requestID: "r1",
    kind: "create",
    mediation,
    options: {
      rp: { id: "example.com", name: "Example" },
      user: { id: encodeBase64URL(new Uint8Array([1, 2, 3])), name: "alex@example.com", displayName: "Alex" },
      challenge,
      pubKeyCredParams: [
        { type: "public-key", alg: -8 },
        { type: "public-key", alg: -7 },
      ],
      excludeCredentials: [{ type: "public-key", id: "AQID" }],
      hasCredPropsExtension: true,
      ...overrides,
    },
  } as PasskeyPageRequest;
}

function createAssertion(overrides: Record<string, unknown> = {}, mediation?: string): PasskeyPageRequest {
  return { requestID: "r2", kind: "get", mediation, options: { challenge, rpId: "example.com", allowCredentials: [], ...overrides } } as PasskeyPageRequest;
}

describe("validatePasskeyRequest", () => {
  it("accepts a creation that allows ES256", () => {
    expect(validatePasskeyRequest(createCreation())).toEqual({
      isValid: true,
      request: {
        kind: "create",
        relyingPartyID: "example.com",
        relyingPartyName: "Example",
        userHandle: "AQID",
        userName: "alex@example.com",
        userDisplayName: "Alex",
        challenge,
        allExcludedCredentialID: ["AQID"],
        hasCredPropsExtension: true,
      },
    });
    expect(validatePasskeyRequest(createCreation({ pubKeyCredParams: [] })).isValid).toBe(true);
    expect(validatePasskeyRequest(createCreation({ rp: { name: "Example" } }))).toMatchObject({ isValid: true, request: { relyingPartyID: undefined } });
  });

  it("leaves requests Hisar cannot serve to the browser", () => {
    const allRejected: [string, PasskeyPageRequest | unknown][] = [
      ["no ES256", createCreation({ pubKeyCredParams: [{ type: "public-key", alg: -257 }] })],
      ["cross-platform", createCreation({ authenticatorAttachment: "cross-platform" })],
      ["empty user handle", createCreation({ user: { id: "", name: "a", displayName: "" } })],
      ["65-byte user handle", createCreation({ user: { id: encodeBase64URL(new Uint8Array(65)), name: "a", displayName: "" } })],
      ["missing user name", createCreation({ user: { id: "AQ", displayName: "" } })],
      ["long user name", createCreation({ user: { id: "AQ", name: "x".repeat(257), displayName: "" } })],
      ["missing rp name", createCreation({ rp: { id: "example.com" } })],
      ["empty rp id", createCreation({ rp: { id: "", name: "x" } })],
      ["bad challenge", createCreation({ challenge: "not base64url!" })],
      ["empty challenge", createCreation({ challenge: "" })],
      ["large challenge", createCreation({ challenge: encodeBase64URL(new Uint8Array(1025)) })],
      ["65 excluded credentials", createCreation({ excludeCredentials: Array.from({ length: 65 }, () => ({ type: "public-key", id: "AQ" })) })],
      ["bad excluded credential", createCreation({ excludeCredentials: [{ type: "public-key", id: "" }] })],
      ["conditional create", createCreation({}, "conditional")],
      ["silent get", createAssertion({}, "silent")],
      ["bad allowed credential", createAssertion({ allowCredentials: [{ type: "public-key", id: "*" }] })],
      ["no options", { requestID: "r", kind: "get" }],
      ["unknown kind", { requestID: "r", kind: "delete", options: {} }],
      ["not an object", "create"],
    ];
    for (const [name, request] of allRejected) expect(validatePasskeyRequest(request).isValid, name).toBe(false);
  });

  it("reads assertions, conditional ones included, and ignores unknown descriptor types", () => {
    expect(validatePasskeyRequest(createAssertion({ allowCredentials: [{ type: "public-key", id: "AQ" }, { type: "other", id: "Ag" }] }, "conditional"))).toEqual({
      isValid: true,
      request: { kind: "get", relyingPartyID: "example.com", challenge, allAllowedCredentialID: ["AQ"], isConditional: true },
    });
  });
});

describe("readFrameOrigin", () => {
  const tab = { id: 4, url: "https://shop.example.com/cart" };

  it("takes the top frame's origin from the sender", () => {
    expect(readFrameOrigin({ url: "https://shop.example.com/login", origin: "https://shop.example.com", frameId: 0, tab }, [])).toEqual({
      origin: "https://shop.example.com",
      topOrigin: "https://shop.example.com",
      isCrossOrigin: false,
    });
    expect(readFrameOrigin({ url: "http://localhost:5173/", frameId: 0, tab: { id: 1, url: "http://localhost:5173/" } }, [])).toMatchObject({ origin: "http://localhost:5173" });
  });

  it("marks frames that are not same-origin with every ancestor as cross-origin", () => {
    expect(readFrameOrigin({ url: "https://pay.example.org/frame", frameId: 3, tab }, ["https://shop.example.com"])).toEqual({
      origin: "https://pay.example.org",
      topOrigin: "https://shop.example.com",
      isCrossOrigin: true,
    });
    expect(readFrameOrigin({ url: "https://shop.example.com/frame", frameId: 3, tab }, ["https://pay.example.org", "https://shop.example.com"])).toMatchObject({
      isCrossOrigin: true,
    });
    expect(readFrameOrigin({ url: "https://shop.example.com/frame", frameId: 3, tab }, ["https://shop.example.com"])).toMatchObject({ isCrossOrigin: false });
  });

  it("refuses insecure, mismatched and unknown senders", () => {
    expect(readFrameOrigin({ url: "http://shop.example.com/", frameId: 0, tab: { url: "http://shop.example.com/" } }, [])).toBeUndefined();
    expect(readFrameOrigin({ url: "https://shop.example.com/", origin: "https://evil.example", frameId: 0, tab }, [])).toBeUndefined();
    expect(readFrameOrigin({ url: "about:blank", frameId: 2, tab }, ["https://shop.example.com"])).toBeUndefined();
    expect(readFrameOrigin({ url: "https://other.example/", frameId: 0, tab }, [])).toBeUndefined();
    expect(readFrameOrigin({ url: "https://pay.example.org/", frameId: 3, tab }, [])).toBeUndefined();
    expect(readFrameOrigin({ url: "https://pay.example.org/", frameId: 3, tab }, ["https://elsewhere.example"])).toBeUndefined();
    expect(readFrameOrigin({ url: "https://shop.example.com/", tab }, [])).toBeUndefined();
  });
});

describe("validatePasskeyRequest with the PRF extension", () => {
  const salt = encodeBase64URL(new Uint8Array(32).fill(3));

  it("passes valid PRF inputs on", () => {
    expect(validatePasskeyRequest(createCreation({ prf: {} }))).toMatchObject({ isValid: true, request: { prf: {} } });
    expect(validatePasskeyRequest(createCreation({ prf: { eval: { first: salt, second: "" } } }))).toMatchObject({ isValid: true, request: { prf: { eval: { first: salt, second: "" } } } });
    const assertion = validatePasskeyRequest(createAssertion({ allowCredentials: [{ type: "public-key", id: "AQID" }], prf: { eval: { first: "" }, evalByCredential: { AQID: { first: salt } } } }));
    expect(assertion).toMatchObject({ isValid: true, request: { prf: { eval: { first: "" }, evalByCredential: { AQID: { first: salt } } } } });
    const withoutPRF = validatePasskeyRequest(createAssertion());
    expect(withoutPRF.isValid && withoutPRF.request).not.toHaveProperty("prf");
  });

  it("leaves invalid PRF inputs to the browser", () => {
    const tooLong = encodeBase64URL(new Uint8Array(1025));
    for (const [name, request] of [
      ["prf is not an object", createAssertion({ prf: "salt" })],
      ["first missing", createAssertion({ prf: { eval: {} } })],
      ["first not base64url", createAssertion({ prf: { eval: { first: "*" } } })],
      ["first too long", createAssertion({ prf: { eval: { first: tooLong } } })],
      ["second too long", createAssertion({ prf: { eval: { first: salt, second: tooLong } } })],
      ["evalByCredential on create", createCreation({ prf: { evalByCredential: {} } })],
      ["evalByCredential without allowCredentials", createAssertion({ prf: { evalByCredential: { AQID: { first: salt } } } })],
      ["key outside allowCredentials", createAssertion({ allowCredentials: [{ type: "public-key", id: "AQID" }], prf: { evalByCredential: { BAUG: { first: salt } } } })],
    ] as const) {
      expect(validatePasskeyRequest(request), name).toEqual({ isValid: false, reason: "invalid prf extension" });
    }
  });
});
