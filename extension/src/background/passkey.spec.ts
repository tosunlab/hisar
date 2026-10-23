import { beforeEach, describe, expect, it, vi } from "vitest";
import type { PopupView } from "../shared/messages";
import {
  encodeBase64URL,
  passkeyContentPortName,
  passkeyPromptPortName,
  type BackgroundToContentPasskeyMessage,
  type PasskeyPageRequest,
  type PasskeyPromptCall,
  type PasskeyPromptResponse,
  type PasskeyPromptState,
} from "../shared/passkey";
import type { DesktopPasskeyParameters } from "../shared/passkey-protocol";
import { HisarError } from "../shared/protocol";
import { PasskeyService, type PasskeyDependencies, type PasskeyPort } from "./passkey";
import type { PasskeySender } from "./passkey-request";

class FakePasskeyPort implements PasskeyPort {
  readonly allReceived: unknown[] = [];
  private readonly allMessageListener: ((message: unknown) => void)[] = [];
  private readonly allDisconnectListener: (() => void)[] = [];
  isDisconnected = false;

  constructor(
    readonly name: string,
    readonly sender: PasskeySender,
    private readonly onReceive?: (message: unknown, port: FakePasskeyPort) => void,
  ) {}

  readonly onMessage = { addListener: (listener: (message: unknown) => void) => this.allMessageListener.push(listener) };
  readonly onDisconnect = { addListener: (listener: () => void) => this.allDisconnectListener.push(listener) };

  postMessage(message: unknown): void {
    this.allReceived.push(message);
    this.onReceive?.(message, this);
  }

  disconnect(): void {
    this.isDisconnected = true;
  }

  send(message: unknown): void {
    for (const listener of this.allMessageListener) listener(message);
  }

  simulateDisconnect(): void {
    for (const listener of this.allDisconnectListener) listener();
  }
}

const challenge = encodeBase64URL(new Uint8Array(16).fill(1));
const tab = { id: 7, url: "https://login.example.com/" };
const frameSender: PasskeySender = { url: "https://login.example.com/sign-in", origin: "https://login.example.com", frameId: 0, tab };
const promptSender: PasskeySender = { url: "chrome-extension://dynamic-id/passkey-prompt.html#s", frameId: 12, tab };

const creationRequest: PasskeyPageRequest = {
  requestID: "r1",
  kind: "create",
  options: {
    rp: { id: "example.com", name: "Example" },
    user: { id: "dXNlcg", name: "alex@example.com", displayName: "Alex" },
    challenge,
    pubKeyCredParams: [{ type: "public-key", alg: -7 }],
    excludeCredentials: [],
    hasCredPropsExtension: true,
  },
};

const assertionRequest: PasskeyPageRequest = { requestID: "r2", kind: "get", options: { challenge, rpId: "example.com", allowCredentials: [] } };

interface Harness {
  service: PasskeyService;
  dependencies: PasskeyDependencies;
  desktopView: PopupView;
  allDesktopRequest: { type: string; passkey: DesktopPasskeyParameters }[];
  allPasskey: { credentialId: string; vaultID: string; itemID: string; title: string; userName: string; userDisplayName: string }[];
  isExcluded: boolean;
  isConnected: boolean;
  isVisible: boolean;
}

let harness: Harness;
let idCount: number;

beforeEach(() => {
  idCount = 0;
  harness = {
    desktopView: "unlocked",
    allDesktopRequest: [],
    allPasskey: [{ credentialId: "Y3JlZA", vaultID: "v1", itemID: "i1", title: "Example", userName: "alex@example.com", userDisplayName: "Alex" }],
    isExcluded: false,
    isConnected: true,
    isVisible: true,
  } as unknown as Harness;
  harness.dependencies = {
    getDesktopView: vi.fn(async () => harness.desktopView),
    isDesktopAvailable: vi.fn(async () => harness.isConnected),
    isDesktopConnected: () => harness.isConnected,
    requestDesktop: vi.fn(async (type: string, passkey: DesktopPasskeyParameters) => {
      harness.allDesktopRequest.push({ type, passkey });
      if (harness.desktopView === "locked") throw new HisarError("locked", "Hisar is locked");
      switch (type) {
        case "passkeys.list":
          return { passkeys: harness.allPasskey };
        case "passkeys.prepare":
          return { isExcluded: harness.isExcluded, logins: [{ vaultID: "v1", itemID: "i1", title: "Example", username: "alex@example.com", vaultName: "Personal" }] };
        case "passkeys.create":
          return { credentialId: "bmV3", clientDataJSON: "e30", attestationObject: "o2M", authenticatorData: "SZY", publicKey: "MFk", publicKeyAlgorithm: -7, vaultID: "v1", itemID: "i9" };
        case "passkeys.get":
          return { credentialId: passkey.credentialId, clientDataJSON: "e30", authenticatorData: "SZY", signature: "MEU", userHandle: "dXNlcg" };
      }
      throw new Error(type);
    }) as never,
    unlock: vi.fn(async () => {
      harness.desktopView = "unlocked";
    }),
    getPromptUrl: () => "chrome-extension://dynamic-id/passkey-prompt.html",
    createID: () => `id-${++idCount}`,
  };
  harness.service = new PasskeyService(harness.dependencies);
});

function connectContent(sender: PasskeySender = frameSender): FakePasskeyPort {
  const port = new FakePasskeyPort(passkeyContentPortName, sender, (message, contentPort) => {
    const contentMessage = message as BackgroundToContentPasskeyMessage;
    if (contentMessage.type === "checkVisibility") queueMicrotask(() => contentPort.send({ type: "visibility", checkID: contentMessage.checkID, isVisible: harness.isVisible }));
  });
  harness.service.handlePort(port);
  return port;
}

async function startRequest(request: PasskeyPageRequest, sender: PasskeySender = frameSender, extra: Record<string, unknown> = {}): Promise<FakePasskeyPort> {
  const port = connectContent(sender);
  port.send({ type: "start", request, allAncestorOrigin: [], isPermissionPolicyAllowed: false, ...extra });
  await vi.waitFor(() => expect(port.allReceived.length).toBeGreaterThan(0));
  return port;
}

function readSessionID(port: FakePasskeyPort): string {
  const prompt = port.allReceived.find((message) => (message as BackgroundToContentPasskeyMessage).type === "prompt") as { promptUrl: string };
  return prompt.promptUrl.split("#")[1];
}

function connectPrompt(sender: PasskeySender = promptSender): { call: <Result>(call: PasskeyPromptCall, sessionID: string) => Promise<Result>; port: FakePasskeyPort } {
  const port = new FakePasskeyPort(passkeyPromptPortName, sender);
  harness.service.handlePort(port);
  let callCount = 0;
  return {
    port,
    call: async <Result,>(call: PasskeyPromptCall, sessionID: string) => {
      const callID = `call-${++callCount}`;
      port.send({ ...call, callID, sessionID });
      let response: PasskeyPromptResponse | undefined;
      await vi.waitFor(() => {
        response = port.allReceived.find((message) => (message as PasskeyPromptResponse).callID === callID) as PasskeyPromptResponse | undefined;
        expect(response).toBeDefined();
      });
      if (!response!.ok) throw new HisarError(response!.error.code, response!.error.message);
      return response!.result as Result;
    },
  };
}

describe("passkey requests from content scripts", () => {
  it("shows the prompt for a creation, with the matching logins, and saves the passkey", async () => {
    const port = await startRequest(creationRequest);
    expect(port.allReceived[0]).toEqual({ type: "prompt", promptUrl: "chrome-extension://dynamic-id/passkey-prompt.html#id-1", isConditional: false });
    expect(harness.allDesktopRequest[0]).toEqual({
      type: "passkeys.prepare",
      passkey: { origin: "https://login.example.com", topOrigin: undefined, isCrossOrigin: false, relyingPartyId: "example.com", excludeCredentials: [] },
    });
    const prompt = connectPrompt();
    const state = await prompt.call<PasskeyPromptState>({ method: "getState" }, readSessionID(port));
    expect(state).toMatchObject({ view: "save", kind: "create", relyingPartyID: "example.com", userName: "alex@example.com" });
    expect(state.allLogin).toHaveLength(1);
    await prompt.call({ method: "savePasskey", vaultID: "v1", itemID: "i1" }, readSessionID(port));
    expect(harness.allDesktopRequest[1]).toEqual({
      type: "passkeys.create",
      passkey: {
        origin: "https://login.example.com",
        topOrigin: undefined,
        isCrossOrigin: false,
        relyingPartyId: "example.com",
        challenge,
        userHandle: "dXNlcg",
        userName: "alex@example.com",
        userDisplayName: "Alex",
        excludeCredentials: [],
        vaultID: "v1",
        itemID: "i1",
      },
    });
    expect(port.allReceived.at(-1)).toEqual({
      type: "credential",
      credential: {
        kind: "create",
        credentialId: "bmV3",
        clientDataJSON: "e30",
        attestationObject: "o2M",
        authenticatorData: "SZY",
        publicKey: "MFk",
        publicKeyAlgorithm: -7,
        transports: ["hybrid", "internal"],
        hasCredPropsExtension: true,
      },
    });
    expect(harness.service.getSessionCount()).toBe(0);
  });

  it("signs in with the chosen passkey only when the prompt is visible", async () => {
    const port = await startRequest(assertionRequest);
    const sessionID = readSessionID(port);
    const prompt = connectPrompt();
    expect(await prompt.call<PasskeyPromptState>({ method: "getState" }, sessionID)).toMatchObject({ view: "choose", allPasskey: [{ credentialId: "Y3JlZA", vaultID: "v1", itemID: "i1" }] });
    await expect(prompt.call({ method: "choosePasskey", credentialId: "b3RoZXI", vaultID: "v1", itemID: "i1" }, sessionID)).rejects.toThrow("Choose one of the passkeys shown");
    harness.isVisible = false;
    await expect(prompt.call({ method: "choosePasskey", credentialId: "Y3JlZA", vaultID: "v1", itemID: "i1" }, sessionID)).rejects.toThrow("not fully visible");
    expect(harness.allDesktopRequest.map((request) => request.type)).toEqual(["passkeys.list"]);
    harness.isVisible = true;
    await prompt.call({ method: "choosePasskey", credentialId: "Y3JlZA", vaultID: "v1", itemID: "i1" }, sessionID);
    expect(harness.allDesktopRequest[1]).toMatchObject({
      type: "passkeys.get",
      passkey: { origin: "https://login.example.com", credentialId: "Y3JlZA", vaultID: "v1", itemID: "i1", challenge, allowCredentials: [] },
    });
    expect(port.allReceived.at(-1)).toMatchObject({ type: "credential", credential: { kind: "get", credentialId: "Y3JlZA", userHandle: "dXNlcg" } });
  });

  it("signs with the login chosen when two logins hold the same credential ID", async () => {
    harness.allPasskey = [
      { credentialId: "Y3JlZA", vaultID: "v-shared", itemID: "i-copy", title: "AAA Example", userName: "alex@example.com", userDisplayName: "" },
      { credentialId: "Y3JlZA", vaultID: "v1", itemID: "i1", title: "Example", userName: "alex@example.com", userDisplayName: "Alex" },
    ];
    const port = await startRequest(assertionRequest);
    const sessionID = readSessionID(port);
    const prompt = connectPrompt();
    await expect(prompt.call({ method: "choosePasskey", credentialId: "Y3JlZA", vaultID: "v1", itemID: "i-copy" }, sessionID)).rejects.toThrow("Choose one of the passkeys shown");
    await prompt.call({ method: "choosePasskey", credentialId: "Y3JlZA", vaultID: "v1", itemID: "i1" }, sessionID);
    expect(harness.allDesktopRequest.at(-1)).toMatchObject({ type: "passkeys.get", passkey: { credentialId: "Y3JlZA", vaultID: "v1", itemID: "i1" } });
  });

  it("falls back to the browser when Hisar cannot help", async () => {
    harness.allPasskey = [];
    expect((await startRequest(assertionRequest)).allReceived).toEqual([{ type: "fallback" }]);
    harness.allPasskey = [{ credentialId: "Y3JlZA", vaultID: "v1", itemID: "i1", title: "Example", userName: "a", userDisplayName: "" }];
    for (const view of ["not-paired", "signed-out", "desktop-unavailable"] as PopupView[]) {
      harness.desktopView = view;
      expect((await startRequest(assertionRequest)).allReceived, view).toEqual([{ type: "fallback" }]);
    }
    harness.desktopView = "unlocked";
    const unsupported = { ...creationRequest, options: { ...creationRequest.options, pubKeyCredParams: [{ type: "public-key", alg: -257 }] } } as PasskeyPageRequest;
    expect((await startRequest(unsupported)).allReceived).toEqual([{ type: "fallback" }]);
    expect((await startRequest(creationRequest, { ...frameSender, url: "http://login.example.com/" })).allReceived).toEqual([{ type: "fallback" }]);
    expect((await startRequest(creationRequest, { ...frameSender, origin: "https://evil.example" })).allReceived).toEqual([{ type: "fallback" }]);
    harness.dependencies.requestDesktop = vi.fn(async () => {
      throw new HisarError("invalid-request", "the relying party ID does not belong to the origin");
    }) as never;
    expect((await startRequest(creationRequest)).allReceived).toEqual([{ type: "fallback" }]);
    expect(harness.service.getSessionCount()).toBe(0);
  });

  it("serves cross-origin frames only when their permissions policy allows it", async () => {
    const frame = { url: "https://pay.example.org/frame", origin: "https://pay.example.org", frameId: 3, tab };
    const getRequest: PasskeyPageRequest = { requestID: "r3", kind: "get", options: { challenge, allowCredentials: [] } };
    expect((await startRequest(getRequest, frame, { allAncestorOrigin: ["https://login.example.com"] })).allReceived).toEqual([{ type: "fallback" }]);
    const port = await startRequest(getRequest, frame, { allAncestorOrigin: ["https://login.example.com"], isPermissionPolicyAllowed: true });
    expect(port.allReceived[0]).toMatchObject({ type: "prompt" });
    expect(harness.allDesktopRequest.at(-1)?.passkey).toMatchObject({ origin: "https://pay.example.org", topOrigin: "https://login.example.com", isCrossOrigin: true });
  });

  it("keeps conditional requests pending only while Hisar is connected and unlocked", async () => {
    const conditional = { ...assertionRequest, mediation: "conditional" } as PasskeyPageRequest;
    harness.isConnected = false;
    expect((await startRequest(conditional)).allReceived).toEqual([{ type: "fallback" }]);
    expect(harness.dependencies.getDesktopView).not.toHaveBeenCalled();
    harness.isConnected = true;
    harness.desktopView = "locked";
    expect((await startRequest(conditional)).allReceived).toEqual([{ type: "fallback" }]);
    harness.desktopView = "unlocked";
    expect((await startRequest(conditional)).allReceived[0]).toMatchObject({ type: "prompt", isConditional: true });
  });

  it("unlocks from the prompt and falls back when it is closed while locked", async () => {
    harness.desktopView = "locked";
    const port = await startRequest(creationRequest);
    const sessionID = readSessionID(port);
    const prompt = connectPrompt();
    expect(await prompt.call<PasskeyPromptState>({ method: "getState" }, sessionID)).toMatchObject({ view: "locked" });
    await prompt.call({ method: "unlock" }, sessionID);
    expect(await prompt.call<PasskeyPromptState>({ method: "getState" }, sessionID)).toMatchObject({ view: "save" });
    await prompt.call({ method: "dismiss" }, sessionID);
    expect(port.allReceived.at(-1)).toEqual({ type: "error", name: "NotAllowedError", message: "The user closed the Hisar prompt" });

    harness.desktopView = "locked";
    const lockedPort = await startRequest(creationRequest);
    await connectPrompt({ ...promptSender, frameId: 30 }).call({ method: "dismiss" }, readSessionID(lockedPort));
    expect(lockedPort.allReceived.at(-1)).toEqual({ type: "fallback" });
  });

  it("answers excluded credentials with InvalidStateError and offers the browser", async () => {
    harness.isExcluded = true;
    const port = await startRequest(creationRequest);
    const prompt = connectPrompt();
    const sessionID = readSessionID(port);
    expect(await prompt.call<PasskeyPromptState>({ method: "getState" }, sessionID)).toMatchObject({ view: "excluded" });
    await expect(prompt.call({ method: "savePasskey" }, sessionID)).rejects.toThrow("There is no passkey to save");
    await prompt.call({ method: "confirmExcluded" }, sessionID);
    expect(port.allReceived.at(-1)).toMatchObject({ type: "error", name: "InvalidStateError" });

    const second = await startRequest(creationRequest);
    await connectPrompt({ ...promptSender, frameId: 40 }).call({ method: "useOtherDevice" }, readSessionID(second));
    expect(second.allReceived.at(-1)).toEqual({ type: "fallback" });
  });

  it("refuses prompt calls from other tabs, other frames and other pages", async () => {
    const port = await startRequest(creationRequest);
    const sessionID = readSessionID(port);
    await connectPrompt().call({ method: "getState" }, sessionID);
    await expect(connectPrompt({ ...promptSender, frameId: 99 }).call({ method: "getState" }, sessionID)).rejects.toThrow("ended");
    await expect(connectPrompt({ ...promptSender, tab: { id: 8, url: tab.url } }).call({ method: "getState" }, sessionID)).rejects.toThrow("ended");
    const pagePort = new FakePasskeyPort(passkeyPromptPortName, { url: "https://login.example.com/passkey-prompt.html", frameId: 1, tab });
    harness.service.handlePort(pagePort);
    expect(pagePort.isDisconnected).toBe(true);
    await expect(connectPrompt().call({ method: "savePasskey", vaultID: "v9", itemID: "i9" }, sessionID)).rejects.toThrow("Choose one of the logins shown");
  });

  it("ends the session when the page cancels or goes away", async () => {
    const port = await startRequest(creationRequest);
    const sessionID = readSessionID(port);
    port.send({ type: "cancel" });
    await expect(connectPrompt().call({ method: "getState" }, sessionID)).rejects.toThrow("ended");
    const second = await startRequest(assertionRequest);
    second.simulateDisconnect();
    expect(harness.service.getSessionCount()).toBe(0);
  });

  it("forwards the prompt height and answers availability without a prompt", async () => {
    const port = await startRequest(creationRequest);
    await connectPrompt().call({ method: "resize", height: 240 }, readSessionID(port));
    expect(port.allReceived.at(-1)).toEqual({ type: "resize", height: 240 });
    const availability = await startRequest({ requestID: "a1", kind: "availability" });
    expect(availability.allReceived).toEqual([{ type: "availability", isAvailable: true }]);
  });
});

describe("passkey PRF and signals", () => {
  const prfResults = { first: "AQ", second: "Ag" };

  function returnPRF(): void {
    const requestDesktop = harness.dependencies.requestDesktop;
    harness.dependencies.requestDesktop = vi.fn(async (type: string, passkey: DesktopPasskeyParameters) => {
      const result = (await requestDesktop(type as never, passkey)) as Record<string, unknown>;
      if (type === "passkeys.create" && passkey.prf) return { ...result, prf: { enabled: true, ...(passkey.prf.eval ? { results: prfResults } : {}) } };
      if (type === "passkeys.get" && passkey.prf) return { ...result, prf: { results: prfResults } };
      return result;
    }) as never;
  }

  it("passes PRF inputs to the desktop and its outputs to the page", async () => {
    returnPRF();
    const withPRF = { ...creationRequest, options: { ...creationRequest.options, prf: { eval: { first: "c2FsdA" } } } } as PasskeyPageRequest;
    const createPort = await startRequest(withPRF);
    await connectPrompt().call({ method: "savePasskey" }, readSessionID(createPort));
    expect(harness.allDesktopRequest.at(-1)).toMatchObject({ type: "passkeys.create", passkey: { prf: { eval: { first: "c2FsdA" } } } });
    expect(createPort.allReceived.at(-1)).toMatchObject({ type: "credential", credential: { kind: "create", prf: { enabled: true, results: prfResults } } });

    const assertionWithPRF = {
      ...assertionRequest,
      options: { ...assertionRequest.options, allowCredentials: [{ type: "public-key", id: "Y3JlZA" }], prf: { eval: { first: "" }, evalByCredential: { Y3JlZA: { first: "AQ", second: "" } } } },
    } as PasskeyPageRequest;
    const getPort = await startRequest(assertionWithPRF);
    await connectPrompt({ ...promptSender, frameId: 50 }).call({ method: "choosePasskey", credentialId: "Y3JlZA", vaultID: "v1", itemID: "i1" }, readSessionID(getPort));
    expect(harness.allDesktopRequest.at(-1)).toMatchObject({
      type: "passkeys.get",
      passkey: { credentialId: "Y3JlZA", prf: { eval: { first: "" }, evalByCredential: { Y3JlZA: { first: "AQ", second: "" } } } },
    });
    expect(getPort.allReceived.at(-1)).toMatchObject({ type: "credential", credential: { kind: "get", prf: { results: prfResults } } });
  });

  it("reports PRF as not enabled when the desktop does not answer it, and leaves it out when not asked", async () => {
    const withPRF = { ...creationRequest, options: { ...creationRequest.options, prf: {} } } as PasskeyPageRequest;
    const createPort = await startRequest(withPRF);
    await connectPrompt().call({ method: "savePasskey" }, readSessionID(createPort));
    expect((createPort.allReceived.at(-1) as { credential: { prf: unknown } }).credential.prf).toEqual({ enabled: false });

    const getPort = await startRequest({ ...assertionRequest, options: { ...assertionRequest.options, prf: {} } } as PasskeyPageRequest);
    await connectPrompt({ ...promptSender, frameId: 51 }).call({ method: "choosePasskey", credentialId: "Y3JlZA", vaultID: "v1", itemID: "i1" }, readSessionID(getPort));
    expect((getPort.allReceived.at(-1) as { credential: { prf: unknown } }).credential.prf).toEqual({});

    const plainPort = await startRequest(assertionRequest);
    await connectPrompt({ ...promptSender, frameId: 52 }).call({ method: "choosePasskey", credentialId: "Y3JlZA", vaultID: "v1", itemID: "i1" }, readSessionID(plainPort));
    expect(harness.allDesktopRequest.at(-1)?.passkey).not.toHaveProperty("prf");
    expect((plainPort.allReceived.at(-1) as { credential: object }).credential).not.toHaveProperty("prf");
  });

  it("forwards a valid signal over an existing connection with the sender origin", async () => {
    const signalRequest: PasskeyPageRequest = { requestID: "s1", kind: "signal", signal: { method: "signalUnknownCredential", rpId: "example.com", credentialId: "Y3JlZA" } };
    const port = await startRequest(signalRequest);
    expect(port.allReceived).toEqual([{ type: "signal" }]);
    await vi.waitFor(() =>
      expect(harness.allDesktopRequest).toEqual([{ type: "passkeys.signalUnknownCredential", passkey: { origin: "https://login.example.com", relyingPartyId: "example.com", credentialId: "Y3JlZA" } }]),
    );
    expect(harness.dependencies.getDesktopView).not.toHaveBeenCalled();
    expect(harness.service.getSessionCount()).toBe(0);
  });

  it("never forwards a signal without a connection, and rejects an rpId of another site", async () => {
    harness.isConnected = false;
    const signalRequest: PasskeyPageRequest = {
      requestID: "s2",
      kind: "signal",
      signal: { method: "signalCurrentUserDetails", rpId: "example.com", userId: "dXNlcg", name: "alex", displayName: "Alex" },
    };
    expect((await startRequest(signalRequest)).allReceived).toEqual([{ type: "signal" }]);
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(harness.dependencies.isDesktopAvailable).toHaveBeenCalled();
    expect(harness.dependencies.requestDesktop).not.toHaveBeenCalled();

    harness.isConnected = true;
    const foreign = await startRequest({ ...signalRequest, signal: { ...signalRequest.signal, rpId: "evil.example" } } as PasskeyPageRequest);
    expect(foreign.allReceived).toEqual([{ type: "signal", errorName: "SecurityError", message: expect.any(String) }]);
    const fromHTTP = await startRequest(signalRequest, { ...frameSender, url: "http://login.example.com/" });
    expect(fromHTTP.allReceived).toEqual([{ type: "signal" }]);
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(harness.dependencies.requestDesktop).not.toHaveBeenCalled();
  });

  it("answers the page before the desktop and ignores desktop errors", async () => {
    let finishDesktop: () => void = () => undefined;
    harness.dependencies.requestDesktop = vi.fn(() => new Promise((_, reject) => (finishDesktop = () => reject(new HisarError("locked", "locked"))))) as never;
    const port = await startRequest({ requestID: "s3", kind: "signal", signal: { method: "signalAllAcceptedCredentials", rpId: "example.com", userId: "dXNlcg", allAcceptedCredentialIds: [] } });
    expect(port.allReceived).toEqual([{ type: "signal" }]);
    await vi.waitFor(() => expect(harness.dependencies.requestDesktop).toHaveBeenCalledWith("passkeys.signalAllAcceptedCredentials", expect.objectContaining({ allAcceptedCredentialIds: [] })));
    finishDesktop();
  });
});
