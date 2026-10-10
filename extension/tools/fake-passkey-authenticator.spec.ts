// @vitest-environment node
import { createECDH, createPrivateKey } from "node:crypto";
import { describe, expect, it } from "vitest";
import { createInitialState, type FakeHostState } from "./fake-native-host.mjs";
import { derivePRFSecret, evaluatePRF, handlePasskeyRequest } from "./fake-passkey-authenticator.mjs";

function createPrivateKeyDERBase64(scalar: Buffer): string {
  const ecdh = createECDH("prime256v1");
  ecdh.setPrivateKey(scalar);
  const publicKey = ecdh.getPublicKey();
  const jwk = { kty: "EC", crv: "P-256", d: scalar.toString("base64url"), x: publicKey.subarray(1, 33).toString("base64url"), y: publicKey.subarray(33).toString("base64url") };
  return createPrivateKey({ key: jwk, format: "jwk" }).export({ format: "der", type: "pkcs8" }).toString("base64");
}

const countingScalar = Buffer.from(Array.from({ length: 32 }, (_, index) => index + 1));
const origin = "https://example.com";

function createState(): FakeHostState {
  const state = createInitialState();
  state.state = "unlocked";
  state.allLogin = [];
  return state;
}

function request(state: FakeHostState, type: string, passkey: Record<string, unknown>): Record<string, unknown> {
  return handlePasskeyRequest(state, { type, passkey }, 0, () => []) as Record<string, unknown>;
}

describe("fake authenticator PRF", () => {
  it("matches the desktop's vectors (desktop/passkey/prf_test.go)", () => {
    const secret = derivePRFSecret(createPrivateKeyDERBase64(countingScalar));
    expect(secret.toString("hex")).toBe("28cfdb28b3809c915332be43efd80d047384c0754e1fdc844f7bb9e57c31efcd");
    expect(evaluatePRF(secret, Buffer.from("salt-a")).toString("hex")).toBe("2836c30ee68cc2ab1fbc5e288b7b50045f6ec2a09025e7f0e4f48a923c5ace1b");
    expect(evaluatePRF(secret, Buffer.alloc(0)).toString("hex")).toBe("e930f5231ddef59dff8ce9008816fbfea98ad3ee9d006652c3f2f263410f92fb");
    const otherSecret = derivePRFSecret(createPrivateKeyDERBase64(Buffer.alloc(32, 0x42)));
    expect(evaluatePRF(otherSecret, Buffer.from("salt-a")).toString("hex")).toBe("e81574bac72c5f498eb9087f5c62ad75578dbcc4d05c60c65c3e983439da06cd");
  });

  it("evaluates on create and get, and picks evalByCredential for the signing credential", () => {
    const state = createState();
    const salt = Buffer.from("salt-a").toString("base64url");
    const creation = request(state, "passkeys.create", { origin, userHandle: "AQ", userName: "alex", challenge: "AQ", prf: { eval: { first: salt } } });
    const createdPRF = creation.prf as { enabled: boolean; results: { first: string } };
    expect(createdPRF.enabled).toBe(true);
    const credentialId = creation.credentialId as string;
    const assertion = request(state, "passkeys.get", { origin, challenge: "AQ", credentialId, prf: { eval: { first: salt } } });
    expect(assertion.prf).toEqual({ results: { first: createdPRF.results.first } });
    const byCredential = request(state, "passkeys.get", { origin, challenge: "AQ", credentialId, allowCredentials: [credentialId], prf: { eval: { first: "" }, evalByCredential: { [credentialId]: { first: salt } } } });
    expect(byCredential.prf).toEqual(assertion.prf);
    expect(() => request(state, "passkeys.get", { origin, challenge: "AQ", credentialId, prf: { evalByCredential: { [credentialId]: { first: salt } } } })).toThrow("allowCredentials");
  });
});

describe("fake authenticator signals", () => {
  it("removes and renames passkeys of the checked RP ID only while unlocked", () => {
    const state = createState();
    const first = request(state, "passkeys.create", { origin, userHandle: "AQ", userName: "alex", challenge: "AQ" }).credentialId as string;
    const second = request(state, "passkeys.create", { origin, userHandle: "AQ", userName: "alex2", challenge: "AQ" }).credentialId as string;
    expect(() => request(state, "passkeys.signalUnknownCredential", { origin: "https://evil.example", relyingPartyId: "example.com", credentialId: first })).toThrow();
    state.state = "locked";
    expect(request(state, "passkeys.signalUnknownCredential", { origin, relyingPartyId: "example.com", credentialId: first })).toEqual({ changedPasskeyCount: 0 });
    state.state = "unlocked";
    expect(request(state, "passkeys.signalUnknownCredential", { origin, relyingPartyId: "example.com", credentialId: first })).toEqual({ changedPasskeyCount: 1 });
    expect(request(state, "passkeys.signalCurrentUserDetails", { origin, relyingPartyId: "example.com", userHandle: "AQ", userName: "new", userDisplayName: "New" })).toEqual({ changedPasskeyCount: 1 });
    expect(state.allPasskey?.map((stored) => [stored.credentialId, stored.userName])).toEqual([[second, "new"]]);
    expect(request(state, "passkeys.signalAllAcceptedCredentials", { origin, relyingPartyId: "example.com", userHandle: "AQ", allAcceptedCredentialIds: [] })).toEqual({ changedPasskeyCount: 1 });
    expect(state.allPasskey).toEqual([]);
  });
});
