import { afterEach, describe, expect, it, vi } from "vitest";
import { HisarError } from "../shared/protocol";
import { NativeClient, type NativeClientOptions } from "./native-client";
import { FakePort } from "./testing";

const helloResult = { protocolVersion: 1, isPaired: true, state: "unlocked" };

function answerHello(request: Record<string, unknown>): unknown {
  if (request.type === "hello") return { id: request.id, ok: true, result: helloResult };
  return undefined;
}

function createClient(allPort: FakePort[], overrides: Partial<NativeClientOptions> = {}): NativeClient {
  let nextID = 0;
  let portIndex = 0;
  return new NativeClient({
    connectNative: () => {
      const port = allPort[portIndex];
      portIndex += 1;
      if (!port) throw new Error("no more ports");
      return port;
    },
    getLastErrorMessage: () => undefined,
    getHelloParameters: async () => ({ instanceID: "instance-1", browser: "Chrome", extensionVersion: "0.1.0" }),
    getToken: async () => "token-1",
    createID: () => `request-${(nextID += 1)}`,
    ...overrides,
  });
}

async function expectHisarError(promise: Promise<unknown>, code: string): Promise<HisarError> {
  const error = await promise.then(
    () => undefined,
    (caught: unknown) => caught,
  );
  expect(error).toBeInstanceOf(HisarError);
  expect((error as HisarError).code).toBe(code);
  return error as HisarError;
}

afterEach(() => {
  vi.useRealTimers();
});

describe("NativeClient", () => {
  it("sends hello with the instance parameters first, then requests with the token", async () => {
    const port = new FakePort((request) => {
      if (request.type === "hello") return { id: request.id, ok: true, result: helloResult };
      return { id: request.id, ok: true, result: { state: "unlocked", email: "alex@example.com", isOffline: false } };
    });
    const onHello = vi.fn();
    const client = createClient([port], { onHello });
    const status = await client.request("status", {});
    expect(status.email).toBe("alex@example.com");
    expect(port.allSent).toEqual([
      { id: "request-1", type: "hello", instanceID: "instance-1", browser: "Chrome", extensionVersion: "0.1.0" },
      { id: "request-2", type: "status", token: "token-1" },
    ]);
    expect(onHello).toHaveBeenCalledWith(helloResult);
  });

  it("never sends the token with hello and pairing requests", async () => {
    const port = new FakePort((request) => ({ id: request.id, ok: true, result: request.type === "hello" ? helloResult : {} }));
    const client = createClient([port]);
    await client.request("pair.start", { instanceID: "instance-1", browser: "Chrome" });
    await client.request("pair.finish", { instanceID: "instance-1", code: "123456" });
    expect(port.allSent.every((request) => !("token" in request))).toBe(true);
  });

  it("matches responses to requests by id when they arrive out of order", async () => {
    const port = new FakePort(answerHello);
    const client = createClient([port]);
    await client.connect();
    const first = client.request("password.generate", { length: 20 });
    const second = client.request("password.generate", { length: 30 });
    await vi.waitFor(() => expect(port.allSent).toHaveLength(3));
    port.deliver({ id: port.allSent[2].id, ok: true, result: { password: "second" } });
    port.deliver({ id: port.allSent[1].id, ok: true, result: { password: "first" } });
    expect(await first).toEqual({ password: "first" });
    expect(await second).toEqual({ password: "second" });
  });

  it("ignores responses with unknown ids", async () => {
    const port = new FakePort(answerHello);
    const client = createClient([port]);
    await client.connect();
    port.deliver({ id: "unknown", ok: true, result: {} });
    port.deliver("not an object");
    expect(client.isConnected()).toBe(true);
  });

  it("maps error responses to their codes and unknown codes to internal", async () => {
    const port = new FakePort((request) => {
      if (request.type === "hello") return { id: request.id, ok: true, result: helloResult };
      if (request.type === "status") return { id: request.id, ok: false, error: { code: "locked", message: "Hisar is locked" } };
      return { id: request.id, ok: false, error: { code: "teapot", message: "odd" } };
    });
    const client = createClient([port]);
    const lockedError = await expectHisarError(client.request("status", {}), "locked");
    expect(lockedError.message).toBe("Hisar is locked");
    await expectHisarError(client.request("unlock", {}), "internal");
  });

  it("fails with not-paired without sending when there is no token", async () => {
    const port = new FakePort(answerHello);
    const client = createClient([port], { getToken: async () => undefined });
    await expectHisarError(client.request("status", {}), "not-paired");
    expect(port.allSent.map((request) => request.type)).toEqual(["hello"]);
  });

  it("refuses a request larger than the host reads without sending it or closing the connection", async () => {
    const port = new FakePort((request) => ({ id: request.id, ok: true, result: request.type === "hello" ? helloResult : { status: "new" } }));
    const client = createClient([port]);
    await expectHisarError(client.request("logins.prepare", { tabID: "1", url: "https://example.com/", username: "alex", password: "é".repeat(512 * 1024) }), "invalid-request");
    expect(port.allSent.map((request) => request.type)).toEqual(["hello"]);
    expect(client.isConnected()).toBe(true);
    expect(await client.request("logins.prepare", { tabID: "1", url: "https://example.com/", username: "alex", password: "pw" })).toEqual({ status: "new" });
  });

  it("times out a request with desktop-unavailable", async () => {
    vi.useFakeTimers();
    const port = new FakePort(answerHello);
    const client = createClient([port], { requestTimeoutMilliseconds: 1000 });
    await client.connect();
    const pending = client.request("status", {});
    const assertion = expectHisarError(pending, "desktop-unavailable");
    await vi.advanceTimersByTimeAsync(1001);
    expect((await assertion).message).toBe("Hisar did not answer in time");
  });

  it("times out hello and reports desktop-unavailable", async () => {
    vi.useFakeTimers();
    const port = new FakePort();
    const client = createClient([port], { helloTimeoutMilliseconds: 500 });
    const assertion = expectHisarError(client.connect(), "desktop-unavailable");
    await vi.advanceTimersByTimeAsync(501);
    await assertion;
    expect(client.isConnected()).toBe(false);
  });

  it("rejects pending requests with the browser's error when the host disconnects", async () => {
    const port = new FakePort(answerHello);
    const client = createClient([port], { getLastErrorMessage: () => "Native host has exited." });
    await client.connect();
    const pending = client.request("status", {});
    await vi.waitFor(() => expect(port.allSent).toHaveLength(2));
    port.simulateDisconnect();
    expect((await expectHisarError(pending, "desktop-unavailable")).message).toBe("Native host has exited.");
  });

  it("reports desktop-unavailable when the host is not installed", async () => {
    const port = new FakePort();
    const client = createClient([port], { getLastErrorMessage: () => "Specified native messaging host not found." });
    const connecting = client.connect();
    await vi.waitFor(() => expect(port.allSent).toHaveLength(1));
    port.simulateDisconnect();
    expect((await expectHisarError(connecting, "desktop-unavailable")).message).toBe("Specified native messaging host not found.");
  });

  it("reports desktop-unavailable when connectNative throws", async () => {
    const client = createClient([]);
    await expectHisarError(client.connect(), "desktop-unavailable");
  });

  it("connects again on the next request after a disconnect", async () => {
    const firstPort = new FakePort(answerHello);
    const secondPort = new FakePort((request) => ({ id: request.id, ok: true, result: request.type === "hello" ? helloResult : { password: "x" } }));
    const client = createClient([firstPort, secondPort], { allReconnectDelayMilliseconds: [] });
    await client.connect();
    firstPort.simulateDisconnect();
    expect(await client.request("password.generate", {})).toEqual({ password: "x" });
    expect(secondPort.allSent.map((request) => request.type)).toEqual(["hello", "password.generate"]);
  });

  it("reconnects by itself with backoff after an established connection drops", async () => {
    vi.useFakeTimers();
    const allPort = [new FakePort(answerHello), new FakePort(), new FakePort(answerHello)];
    const client = createClient(allPort, { allReconnectDelayMilliseconds: [1000, 2000], helloTimeoutMilliseconds: 100 });
    await client.connect();
    allPort[0].simulateDisconnect();
    expect(client.isConnected()).toBe(false);
    await vi.advanceTimersByTimeAsync(999);
    expect(allPort[1].allSent).toHaveLength(0);
    await vi.advanceTimersByTimeAsync(1);
    expect(allPort[1].allSent.map((request) => request.type)).toEqual(["hello"]);
    await vi.advanceTimersByTimeAsync(100);
    await vi.advanceTimersByTimeAsync(2000);
    expect(allPort[2].allSent.map((request) => request.type)).toEqual(["hello"]);
    await vi.advanceTimersByTimeAsync(0);
    expect(client.isConnected()).toBe(true);
  });

  it("does not reconnect by itself when the first connection never worked", async () => {
    vi.useFakeTimers();
    const allPort = [new FakePort(), new FakePort(answerHello)];
    const client = createClient(allPort, { allReconnectDelayMilliseconds: [10] });
    const connecting = client.connect().catch(() => undefined);
    allPort[0].simulateDisconnect();
    await connecting;
    await vi.advanceTimersByTimeAsync(1000);
    expect(allPort[1].allSent).toHaveLength(0);
  });

  it("reconnect sends a fresh hello on a new port", async () => {
    const allPort = [new FakePort(answerHello), new FakePort(answerHello)];
    const client = createClient(allPort);
    await client.connect();
    await client.reconnect();
    expect(allPort[0].isDisconnected).toBe(true);
    expect(allPort[1].allSent.map((request) => request.type)).toEqual(["hello"]);
  });
});
