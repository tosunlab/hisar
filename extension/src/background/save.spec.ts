import { afterEach, describe, expect, it, vi } from "vitest";
import { decideSaveOffer, RememberedUsernameStore, SaveOfferStore, type SaveOffer } from "./save";

describe("decideSaveOffer", () => {
  it.each([
    [{ status: "new" as const }, false, "save"],
    [{ status: "changed" as const, vaultID: "v", itemID: "i", title: "T" }, false, "update"],
    [{ status: "unchanged" as const, vaultID: "v", itemID: "i", title: "T" }, false, undefined],
    [{ status: "new" as const }, true, undefined],
    [{ status: "changed" as const, vaultID: "v", itemID: "i" }, true, undefined],
    [{ status: "changed" as const }, false, undefined],
  ])("check %j with never=%s gives %s", (checkResult, isNever, expected) => {
    expect(decideSaveOffer(checkResult, isNever)).toBe(expected);
  });
});

function createOffer(overrides: Partial<SaveOffer> = {}): SaveOffer {
  return {
    offerID: "offer-1",
    pairingToken: "token-1",
    saveContext: "context-1",
    tabId: 1,
    url: "https://example.com/",
    host: "example.com",
    username: "alex",
    password: "pw",
    kind: "save",
    createdAt: 0,
    ...overrides,
  };
}

describe("SaveOfferStore", () => {
  afterEach(() => vi.useRealTimers());

  it("actively removes the raw candidate without a later popup lookup", () => {
    vi.useFakeTimers();
    vi.setSystemTime(0);
    const store = new SaveOfferStore();
    store.put(createOffer());
    vi.advanceTimersByTime(120000);
    expect(store.take(1, "offer-1")).toBeUndefined();
  });

  it("keeps a replacement through the old deadline and removes it at its own deadline", () => {
    vi.useFakeTimers();
    vi.setSystemTime(0);
    const store = new SaveOfferStore();
    store.put(createOffer());
    vi.advanceTimersByTime(119999);
    store.put(createOffer({ offerID: "offer-2", createdAt: Date.now() }));
    vi.advanceTimersByTime(1);
    expect(store.getForTab(1, "token-1")?.offerID).toBe("offer-2");
    vi.advanceTimersByTime(119999);
    expect(store.take(1, "offer-2")).toBeUndefined();
  });

  it("keeps one offer per tab and replaces it with a newer one", () => {
    const store = new SaveOfferStore(() => 0);
    store.put(createOffer());
    store.put(createOffer({ offerID: "offer-2" }));
    expect(store.getForTab(1, "token-1")?.offerID).toBe("offer-2");
    expect(store.take(1, "offer-1")).toBeUndefined();
    expect(store.take(1, "offer-2")?.offerID).toBe("offer-2");
    expect(store.getForTab(1, "token-1")).toBeUndefined();
  });

  it("expires offers", () => {
    let now = 0;
    const store = new SaveOfferStore(() => now, 1000);
    store.put(createOffer());
    now = 999;
    expect(store.getForTab(1, "token-1")).toBeDefined();
    now = 1000;
    expect(store.getForTab(1, "token-1")).toBeUndefined();
  });

  it("drops an offer made under another pairing", () => {
    const store = new SaveOfferStore(() => 0);
    store.put(createOffer());
    expect(store.getForTab(1, "token-2")).toBeUndefined();
    expect(store.getForTab(1, "token-1")).toBeUndefined();
  });
});

describe("RememberedUsernameStore", () => {
  it("recalls a username only for the same tab and origin within its lifetime", () => {
    let now = 0;
    const store = new RememberedUsernameStore(() => now, 1000);
    store.remember(1, "https://example.com", "alex", "token-1");
    expect(store.recall(1, "https://example.com", "token-1")).toBe("alex");
    expect(store.recall(1, "https://example.com", "token-2")).toBeUndefined();
    expect(store.recall(1, "https://example.org", "token-1")).toBeUndefined();
    expect(store.recall(2, "https://example.com", "token-1")).toBeUndefined();
    now = 1001;
    expect(store.recall(1, "https://example.com", "token-1")).toBeUndefined();
  });
});
