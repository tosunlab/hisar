// @vitest-environment node
import { spawn } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { createFrameReader, createInitialState, doesUrlMatch, encodeFrame, generateOneTimeCode, respond } from "./fake-native-host.mjs";
import { registerFakeHost } from "./register-fake-host.mjs";

const allTemporaryDirectory: string[] = [];

function createTemporaryDirectory(): string {
  const directory = mkdtempSync(join(tmpdir(), "hisar-fake-host-"));
  allTemporaryDirectory.push(directory);
  return directory;
}

afterEach(() => {
  for (const directory of allTemporaryDirectory.splice(0)) rmSync(directory, { recursive: true, force: true });
});

describe("generateOneTimeCode", () => {
  it.each([
    [59, "287082", "1970-01-01T00:01:00Z"],
    [1111111109, "081804", "2005-03-18T01:58:30Z"],
    [1234567890, "005924", "2009-02-13T23:32:00Z"],
  ])("matches the RFC 6238 SHA-1 vector at %i s", (seconds, code, expiresAt) => {
    expect(generateOneTimeCode("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", seconds * 1000)).toEqual({ code, expiresAt, period: 30 });
  });

  it("answers logins.code only for a matching login with a one-time password", () => {
    const state = { ...createInitialState(), allPairing: [{ instanceID: "i", browser: "Chrome", tokenHash: "", pairedAt: 0 }] };
    state.allLogin[0].oneTimePassword = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ";
    state.allPairing[0].tokenHash = "e3b98a4da31a127d4bde6e43033f66ba274cab0eb7eb1c70ec41402bf6273dd8";
    const request = { id: "1", type: "logins.code", token: "t", vaultID: "vault-personal", itemID: "item-example" };
    expect(respond(state, { ...request, url: "https://example.com/2fa" }, 59_000).result).toEqual({ code: "287082", expiresAt: "1970-01-01T00:01:00Z", period: 30 });
    expect(respond(state, { ...request, url: "https://example.org/2fa" }, 59_000).error?.code).toBe("no-match");
  });
});

describe("doesUrlMatch", () => {
  it.each([
    ["https://example.com", "https://example.com/login", true],
    ["example.com", "https://example.com/login", true],
    ["google.com", "https://accounts.google.com/signin", true],
    ["http://example.com", "https://example.com/", true],
    ["https://example.com", "http://example.com/", false],
    ["https://example.com:8443", "https://example.com/", false],
    ["https://example.com:443", "https://example.com/", true],
    ["https://example.com", "https://example.org/", false],
    ["http://127.0.0.1:8080", "http://127.0.0.1:8080/login", true],
    ["http://localhost:8080", "http://127.0.0.1:8080/login", false],
  ])("item %s and page %s match: %s", (itemUrl, pageUrl, expected) => {
    expect(doesUrlMatch(itemUrl, pageUrl, false)).toBe(expected);
  });

  it("requires equal hosts with the exact host option", () => {
    expect(doesUrlMatch("google.com", "https://accounts.google.com/", true)).toBe(false);
  });
});

describe("respond", () => {
  it("answers hello and refuses token-protected requests without a valid token", () => {
    const state = createInitialState();
    expect(respond(state, { id: "1", type: "hello", instanceID: "i", browser: "Chrome", extensionVersion: "0.1.0" })).toEqual({
      id: "1",
      ok: true,
      result: { protocolVersion: 1, isPaired: false, state: "unlocked", features: ["addresses", "vault", "unlock-password", "cards"] },
    });
    expect(respond(state, { id: "2", type: "status", token: "wrong" })).toMatchObject({ ok: false, error: { code: "not-paired" } });
  });

  it("pairs with the code, limits attempts and expires codes", () => {
    const state = { ...createInitialState(), pairingCode: "111111" };
    respond(state, { id: "1", type: "pair.start", instanceID: "i", browser: "Chrome" }, 0);
    for (let attempt = 0; attempt < 5; attempt += 1) {
      expect(respond(state, { id: "x", type: "pair.finish", instanceID: "i", code: "000000" }, 0)).toMatchObject({ error: { code: "pairing-failed" } });
    }
    expect(respond(state, { id: "x", type: "pair.finish", instanceID: "i", code: "111111" }, 0)).toMatchObject({ error: { code: "pairing-failed" } });
    respond(state, { id: "1", type: "pair.start", instanceID: "i", browser: "Chrome" }, 0);
    expect(respond(state, { id: "x", type: "pair.finish", instanceID: "i", code: "111111" }, 121_000)).toMatchObject({ error: { code: "pairing-failed" } });
    respond(state, { id: "1", type: "pair.start", instanceID: "i", browser: "Chrome" }, 0);
    const finished = respond(state, { id: "x", type: "pair.finish", instanceID: "i", code: "111111" }, 1000);
    expect(finished.ok).toBe(true);
    const token = (finished.result as { token: string }).token;
    expect(respond(state, { id: "s", type: "status", token })).toMatchObject({ ok: true, result: { state: "unlocked" } });
  });

  it("rejects requests without an id or type", () => {
    expect(respond(createInitialState(), { type: "hello" })).toMatchObject({ ok: false, error: { code: "invalid-request" } });
  });
});

describe("framing", () => {
  it("encodes and decodes frames split across chunks", () => {
    const allMessage: unknown[] = [];
    const readFrame = createFrameReader((message) => allMessage.push(message));
    const bytes = Buffer.concat([encodeFrame({ a: 1 }), encodeFrame({ b: "ü" })]);
    readFrame(bytes.subarray(0, 3));
    readFrame(bytes.subarray(3, 12));
    readFrame(bytes.subarray(12));
    expect(allMessage).toEqual([{ a: 1 }, { b: "ü" }]);
  });
});

describe("registered launcher", () => {
  it("writes the host manifest and answers over stdio through the launcher", async () => {
    const directory = createTemporaryDirectory();
    const registration = registerFakeHost({
      userDataDirectory: join(directory, "profile"),
      stateFile: join(directory, "state.json"),
      hostDirectory: join(directory, "host"),
    });
    const manifest = JSON.parse(readFileSync(registration.manifestPath, "utf8"));
    expect(manifest).toMatchObject({
      name: "com.tryhisar.hisar",
      type: "stdio",
      path: registration.launcherPath,
      allowed_origins: ["chrome-extension://mmipiffoogblblocokimpemdopmdjghf/"],
    });
    const child = spawn(registration.launcherPath, ["chrome-extension://mmipiffoogblblocokimpemdopmdjghf/"], { stdio: ["pipe", "pipe", "ignore"] });
    const response = await new Promise<unknown>((resolve, reject) => {
      child.on("error", reject);
      child.stdout.on("data", createFrameReader(resolve));
      child.stdin.write(encodeFrame({ id: "1", type: "hello", instanceID: "i", browser: "Chrome", extensionVersion: "0.1.0" }));
    });
    child.stdin.end();
    expect(response).toEqual({ id: "1", ok: true, result: { protocolVersion: 1, isPaired: false, state: "unlocked", features: ["addresses", "vault", "unlock-password", "cards"] } });
  });
});
