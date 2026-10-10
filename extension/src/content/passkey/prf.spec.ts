import { describe, expect, it } from "vitest";
import { encodeBase64URL } from "../../shared/passkey";
import { buildPRFExtensionResult, buildPRFExtensionResultJSON, findPRFInputError, serializePRFInput } from "./prf";

const createException = (message: string, name: string) => new DOMException(message, name);
const allowed = [{ type: "public-key" as const, id: new Uint8Array([1, 2, 3]) }];
const salt = new Uint8Array([7, 7]);

describe("PRF client checks", () => {
  it("accepts requests without prf and eval-only requests", () => {
    expect(findPRFInputError("get", {}, createException)).toBeUndefined();
    expect(findPRFInputError("get", { extensions: { credProps: true } }, createException)).toBeUndefined();
    expect(findPRFInputError("create", { extensions: { prf: {} } }, createException)).toBeUndefined();
    expect(findPRFInputError("create", { extensions: { prf: { eval: { first: salt } } } }, createException)).toBeUndefined();
    expect(findPRFInputError("get", { extensions: { prf: { eval: { first: salt } } } }, createException)).toBeUndefined();
    expect(findPRFInputError("get", { extensions: { prf: { evalByCredential: {} } } }, createException)).toBeUndefined();
  });

  it("rejects evalByCredential on create with NotSupportedError", () => {
    expect(findPRFInputError("create", { extensions: { prf: { evalByCredential: {} } } }, createException)?.name).toBe("NotSupportedError");
  });

  it("rejects evalByCredential without allowCredentials with NotSupportedError", () => {
    const error = findPRFInputError("get", { extensions: { prf: { evalByCredential: { AQID: { first: salt } } } } }, createException);
    expect(error?.name).toBe("NotSupportedError");
  });

  it("rejects keys that are empty, not base64url or not in allowCredentials with SyntaxError", () => {
    for (const key of ["", "*", "AQ==", "BAUG"]) {
      const error = findPRFInputError("get", { allowCredentials: allowed, extensions: { prf: { evalByCredential: { [key]: { first: salt } } } } }, createException);
      expect(error?.name, key).toBe("SyntaxError");
    }
    expect(findPRFInputError("get", { allowCredentials: allowed, extensions: { prf: { evalByCredential: { AQID: { first: salt } } } } }, createException)).toBeUndefined();
  });
});

describe("PRF serialization", () => {
  it("encodes inputs as base64url and normalizes evalByCredential keys", () => {
    expect(serializePRFInput(undefined)).toBeUndefined();
    expect(serializePRFInput({})).toBeUndefined();
    expect(serializePRFInput({ prf: {} })).toEqual({});
    expect(
      serializePRFInput({ prf: { eval: { first: salt.buffer, second: new Uint8Array(0) }, evalByCredential: { AQID: { first: new DataView(new Uint8Array([9]).buffer) } } } }),
    ).toEqual({ eval: { first: "Bwc", second: "" }, evalByCredential: { AQID: { first: "CQ" } } });
  });

  it("builds ArrayBuffer results for the page and base64url results for toJSON", () => {
    const output = { enabled: true, results: { first: encodeBase64URL(new Uint8Array([1, 2])), second: encodeBase64URL(new Uint8Array([3])) } };
    const result = buildPRFExtensionResult(output) as { enabled: boolean; results: { first: ArrayBuffer; second: ArrayBuffer } };
    expect(result.enabled).toBe(true);
    expect(result.results.first).toBeInstanceOf(ArrayBuffer);
    expect([...new Uint8Array(result.results.first)]).toEqual([1, 2]);
    expect([...new Uint8Array(result.results.second)]).toEqual([3]);
    expect(buildPRFExtensionResultJSON(output)).toEqual({ enabled: true, results: { first: "AQI", second: "Aw" } });
    expect(buildPRFExtensionResult({})).toEqual({});
    expect(buildPRFExtensionResult({ results: { first: "AQ" } })).not.toHaveProperty("enabled");
  });
});
