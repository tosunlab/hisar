import { describe, expect, it } from "vitest";
import { encodeBase64URL } from "../shared/passkey";
import { checkPasskeySignal, isRelyingPartyIDValidForOrigin } from "./passkey-signal";

const frame = { origin: "https://login.example.com", topOrigin: "https://login.example.com", isCrossOrigin: false };

describe("isRelyingPartyIDValidForOrigin", () => {
  it("accepts the host and its parents at a label boundary", () => {
    expect(isRelyingPartyIDValidForOrigin("login.example.com", "https://login.example.com")).toBe(true);
    expect(isRelyingPartyIDValidForOrigin("example.com", "https://login.example.com")).toBe(true);
    expect(isRelyingPartyIDValidForOrigin("Example.COM", "https://login.example.com")).toBe(true);
    expect(isRelyingPartyIDValidForOrigin("localhost", "http://localhost:5173")).toBe(true);
    expect(isRelyingPartyIDValidForOrigin("127.0.0.1", "https://127.0.0.1:8443")).toBe(true);
  });

  it("refuses other domains, partial labels, single labels and IP suffixes", () => {
    for (const [relyingPartyID, origin] of [
      ["evil.example", "https://login.example.com"],
      ["ample.com", "https://login.example.com"],
      ["sub.login.example.com", "https://login.example.com"],
      ["com", "https://login.example.com"],
      ["", "https://login.example.com"],
      [".example.com", "https://login.example.com"],
      ["example.com.", "https://login.example.com"],
      ["0.0.1", "https://127.0.0.1"],
      ["example.com", "not a url"],
      ["a".repeat(254), "https://login.example.com"],
    ]) {
      expect(isRelyingPartyIDValidForOrigin(relyingPartyID, origin), `${relyingPartyID} for ${origin}`).toBe(false);
    }
  });
});

describe("checkPasskeySignal", () => {
  it("rejects an rpId that does not fit the sender origin with SecurityError", () => {
    expect(checkPasskeySignal({ method: "signalUnknownCredential", rpId: "evil.example", credentialId: "AQ" }, frame)).toMatchObject({ outcome: "reject", errorName: "SecurityError" });
  });

  it("ignores signals without a trusted frame origin or with impossible values", () => {
    expect(checkPasskeySignal({ method: "signalUnknownCredential", rpId: "example.com", credentialId: "AQ" }, undefined)).toEqual({ outcome: "ignore" });
    expect(checkPasskeySignal(null, frame)).toEqual({ outcome: "ignore" });
    expect(checkPasskeySignal({ method: "signalUnknownCredential", rpId: 1 }, frame)).toEqual({ outcome: "ignore" });
    expect(checkPasskeySignal({ method: "signalUnknownCredential", rpId: "example.com", credentialId: "" }, frame)).toEqual({ outcome: "ignore" });
    expect(checkPasskeySignal({ method: "signalUnknownCredential", rpId: "example.com", credentialId: encodeBase64URL(new Uint8Array(1024)) }, frame)).toEqual({ outcome: "ignore" });
    expect(checkPasskeySignal({ method: "signalAllAcceptedCredentials", rpId: "example.com", userId: encodeBase64URL(new Uint8Array(65)), allAcceptedCredentialIds: [] }, frame)).toEqual({
      outcome: "ignore",
    });
    expect(checkPasskeySignal({ method: "signalAllAcceptedCredentials", rpId: "example.com", userId: "AQ", allAcceptedCredentialIds: new Array(65).fill("AQ") }, frame)).toEqual({ outcome: "ignore" });
    expect(checkPasskeySignal({ method: "signalAllAcceptedCredentials", rpId: "example.com", userId: "AQ", allAcceptedCredentialIds: ["AQ", ""] }, frame)).toEqual({ outcome: "ignore" });
    expect(checkPasskeySignal({ method: "signalCurrentUserDetails", rpId: "example.com", userId: "AQ", name: "x".repeat(257), displayName: "" }, frame)).toEqual({ outcome: "ignore" });
    expect(checkPasskeySignal({ method: "signalSomethingElse", rpId: "example.com" }, frame)).toEqual({ outcome: "ignore" });
  });

  it("builds the desktop request with the sender origin", () => {
    expect(checkPasskeySignal({ method: "signalUnknownCredential", rpId: "Example.com", credentialId: "AQID" }, frame)).toEqual({
      outcome: "forward",
      type: "passkeys.signalUnknownCredential",
      parameters: { origin: "https://login.example.com", relyingPartyId: "example.com", credentialId: "AQID" },
    });
    expect(checkPasskeySignal({ method: "signalAllAcceptedCredentials", rpId: "example.com", userId: "dXNlcg", allAcceptedCredentialIds: ["AQ"] }, frame)).toEqual({
      outcome: "forward",
      type: "passkeys.signalAllAcceptedCredentials",
      parameters: { origin: "https://login.example.com", relyingPartyId: "example.com", userHandle: "dXNlcg", allAcceptedCredentialIds: ["AQ"] },
    });
    expect(checkPasskeySignal({ method: "signalCurrentUserDetails", rpId: "login.example.com", userId: "dXNlcg", name: "alex", displayName: "Alex" }, frame)).toEqual({
      outcome: "forward",
      type: "passkeys.signalCurrentUserDetails",
      parameters: { origin: "https://login.example.com", relyingPartyId: "login.example.com", userHandle: "dXNlcg", userName: "alex", userDisplayName: "Alex" },
    });
  });
});
