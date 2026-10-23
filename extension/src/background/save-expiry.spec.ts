import { afterEach, expect, it, vi } from "vitest";
import { SaveExpiryScheduler } from "./save-expiry";
import { SaveOfferStore, type SaveOffer } from "./save";
import { BackgroundController } from "./controller";
import { NativeClient } from "./native-client";
import { createHarness, FakePort } from "./testing";
import { respond } from "../../tools/fake-native-host.mjs";

function createBrowser() {
  const allAlarm = new Map<string, { when: number }>();
  const allBadge = new Map<number, string>();
  let deliver = (_alarm: { name: string }) => {};
  return {
    allAlarm,
    allBadge,
    fire: (name: string) => deliver({ name }),
    alarms: {
      create: vi.fn(async (name: string, info: { when: number }) => { allAlarm.set(name, info); }),
      clear: vi.fn(async (name: string) => allAlarm.delete(name)),
      getAll: async () => [...allAlarm.keys()].map(name => ({ name })),
      onAlarm: { addListener: (listener: typeof deliver) => { deliver = listener; } },
    },
    tabs: { query: async () => [...allBadge.keys()].map(id => ({ id })) },
    action: {
      getBadgeText: async ({ tabId }: { tabId: number }) => allBadge.get(tabId) ?? "",
      setBadgeText: async ({ tabId, text }: { tabId: number; text: string }) => { allBadge.set(tabId, text); },
    },
  };
}

function offer(generation: string, createdAt: number): SaveOffer {
  return { offerID: generation, navigationID: generation, tabId: 1, createdAt, pairingToken: "pairing", saveContext: "authorization", url: "https://example.com/", host: "example.com", username: "alex", password: "raw candidate", kind: "save" };
}

afterEach(() => vi.useRealTimers());

for (const mechanism of ["timer", "alarm"]) {
  it(`removes candidate, authorization, generation and badge through the ${mechanism} at the capture deadline`, async () => {
    vi.useFakeTimers();
    vi.setSystemTime(0);
    const browser = createBrowser();
    const scheduler = new SaveExpiryScheduler(browser);
    await scheduler.ready;
    const store = new SaveOfferStore(Date.now, 120000, tabId => { browser.allBadge.set(tabId, ""); }, scheduler.schedule);
    store.beginSubmission(1, "first", "https://example.com/");
    vi.advanceTimersByTime(10000);
    store.put(offer("first", 0));
    browser.allBadge.set(1, "+");
    const [name, info] = [...browser.allAlarm.entries()][0];
    expect(info.when).toBe(120000);
    if (mechanism === "timer") vi.advanceTimersByTime(110000);
    else {
      vi.setSystemTime(120000);
      browser.fire(name);
    }
    expect(store.take(1, "first")).toBeUndefined();
    expect(store.isCurrent(1, "first")).toBe(false);
    expect(browser.allBadge.get(1)).toBe("");
    expect(browser.allAlarm.size).toBe(0);
  });
}

it("ignores a queued old alarm after replacement and keeps the new capture until its own deadline", async () => {
  vi.useFakeTimers();
  vi.setSystemTime(0);
  const browser = createBrowser();
  const scheduler = new SaveExpiryScheduler(browser);
  await scheduler.ready;
  const store = new SaveOfferStore(Date.now, 120000, tabId => { browser.allBadge.set(tabId, ""); }, scheduler.schedule);
  store.beginSubmission(1, "first", "https://example.com/");
  store.put(offer("first", 0));
  const oldAlarm = [...browser.allAlarm.keys()][0];
  vi.advanceTimersByTime(119999);
  store.beginSubmission(1, "replacement", "https://example.com/");
  store.put(offer("replacement", 119999));
  browser.allBadge.set(1, "+");
  vi.advanceTimersByTime(1);
  browser.fire(oldAlarm);
  expect(browser.allBadge.get(1)).toBe("+");
  expect(store.getForTab(1, "pairing")?.offerID).toBe("replacement");
  vi.advanceTimersByTime(119999);
  expect(store.take(1, "replacement")).toBeUndefined();
  expect(browser.allBadge.get(1)).toBe("");
});

it("retires a capture before a held prepare reply can install its password", async () => {
  vi.useFakeTimers();
  vi.setSystemTime(0);
  const store = new SaveOfferStore();
  store.beginSubmission(1, "pending", "https://example.com/");
  vi.advanceTimersByTime(120000);
  expect(store.isCurrent(1, "pending")).toBe(false);
  expect(store.put(offer("pending", 0))).toBe(false);
  expect(store.take(1, "pending")).toBeUndefined();
});

it("reconciles orphaned pending badges and only save-owned alarms on worker restart", async () => {
  const browser = createBrowser();
  browser.allAlarm.set("hisar-save-expiry/1/old", { when: 120000 });
  browser.allAlarm.set("another-feature", { when: 240000 });
  browser.allBadge.set(1, "+");
  browser.allBadge.set(2, "3");
  const scheduler = new SaveExpiryScheduler(browser);
  await scheduler.ready;
  expect([...browser.allAlarm.keys()]).toEqual(["another-feature"]);
  expect([...browser.allBadge.entries()]).toEqual([[1, ""], [2, "3"]]);
});

it("cancels an alarm whose creation completes after its candidate was removed", async () => {
  vi.useFakeTimers();
  vi.setSystemTime(0);
  const browser = createBrowser();
  const created = Promise.withResolvers<void>();
  const create = browser.alarms.create;
  browser.alarms.create = vi.fn(async (name, info) => {
    await created.promise;
    await create(name, info);
  });
  const scheduler = new SaveExpiryScheduler(browser);
  await scheduler.ready;
  const cancel = scheduler.schedule(1, "old", 120000, () => {});
  cancel();
  created.resolve();
  await vi.runAllTimersAsync();
  expect(browser.allAlarm.size).toBe(0);
});

it("does not restore an expired capture when the native prepare reply finally arrives", async () => {
  const harness = createHarness();
  await harness.pair();
  let controller: BackgroundController;
  let port: FakePort;
  let reply: unknown;
  const prepared = Promise.withResolvers<void>();
  const client = new NativeClient({
    connectNative: () => {
      port = new FakePort(request => {
        const response = respond(harness.state, request, harness.bridge.now());
        if (request.type !== "logins.prepare") return response;
        reply = response;
        prepared.resolve();
        return undefined;
      });
      return port;
    },
    getLastErrorMessage: () => undefined,
    getHelloParameters: () => controller.getHelloParameters(),
    getToken: () => harness.storage.getToken(),
    createID: () => crypto.randomUUID(),
    onHello: result => controller.handleHello(result),
    allReconnectDelayMilliseconds: [],
  });
  controller = new BackgroundController(client, harness.storage, harness.bridge);
  const capturing = controller.handleLoginSubmitted(1, "https://example.com/login", "new-user", "held password");
  await prepared.promise;
  harness.advanceTime(120000);
  port!.deliver(reply);
  expect(await capturing).toBeUndefined();
  expect(harness.allSaveBadge.get(1)).toBe(false);
  expect(await controller.getPendingSaveOffer(1)).toBeNull();
  client.disconnect();
});
