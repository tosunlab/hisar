import { describe, expect, it } from "vitest";
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
  it("keeps one offer per tab and replaces it with a newer one", () => {
    const store = new SaveOfferStore(() => 0);
    store.put(createOffer());
    store.put(createOffer({ offerID: "offer-2" }));
    expect(store.getForTab(1)?.offerID).toBe("offer-2");
    expect(store.take(1, "offer-1")).toBeUndefined();
    expect(store.take(1, "offer-2")?.offerID).toBe("offer-2");
    expect(store.getForTab(1)).toBeUndefined();
  });

  it("expires offers", () => {
    let now = 0;
    const store = new SaveOfferStore(() => now, 1000);
    store.put(createOffer());
    now = 1000;
    expect(store.getForTab(1)).toBeDefined();
    now = 1001;
    expect(store.getForTab(1)).toBeUndefined();
  });
});

describe("RememberedUsernameStore", () => {
  it("recalls a username only for the same tab and origin within its lifetime", () => {
    let now = 0;
    const store = new RememberedUsernameStore(() => now, 1000);
    store.remember(1, "https://example.com", "alex");
    expect(store.recall(1, "https://example.com")).toBe("alex");
    expect(store.recall(1, "https://example.org")).toBeUndefined();
    expect(store.recall(2, "https://example.com")).toBeUndefined();
    now = 1001;
    expect(store.recall(1, "https://example.com")).toBeUndefined();
  });
});
