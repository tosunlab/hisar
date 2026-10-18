import { describe, expect, it } from "vitest";
import type { Item } from "../hisar";
import { listShareableFields, orderItemActions, parseItemAction } from "./itemActions";

function createItem(change: Partial<Item> = {}): Item {
  return {
    vaultID: "v-personal",
    itemID: "i-github",
    title: "GitHub",
    category: "login",
    username: "alice",
    allURL: ["https://github.com/login"],
    allTag: [],
    isFavorite: false,
    hasPassword: true,
    hasOneTimeCode: true,
    allReference: [],
    ...change,
  };
}

const defaultPreferences = { primary: "copy-password", secondary: "paste-password" } as const;

describe("orderItemActions", () => {
  it("puts the primary action first, the secondary second, then the others in 1Password's order", () => {
    expect(orderItemActions(createItem(), defaultPreferences)).toEqual([
      "copy-password",
      "paste-password",
      "open-in-hisar",
      "open-in-browser",
      "copy-username",
      "copy-one-time-code",
      "paste-username",
      "paste-one-time-code",
    ]);
    expect(orderItemActions(createItem(), { primary: "paste-one-time-code", secondary: "open-in-browser" })).toEqual([
      "paste-one-time-code",
      "open-in-browser",
      "open-in-hisar",
      "copy-username",
      "copy-password",
      "copy-one-time-code",
      "paste-username",
      "paste-password",
    ]);
  });

  it("lists an action once when the primary and the secondary action are the same", () => {
    const allAction = orderItemActions(createItem(), { primary: "copy-username", secondary: "copy-username" });
    expect(allAction.slice(0, 3)).toEqual(["copy-username", "open-in-hisar", "open-in-browser"]);
    expect(allAction).toHaveLength(8);
  });

  it("falls through to the next action when the item has no value for the one configured", () => {
    const withoutCode = createItem({ hasOneTimeCode: false });
    expect(orderItemActions(withoutCode, { primary: "copy-one-time-code", secondary: "paste-password" })).toEqual([
      "paste-password",
      "open-in-hisar",
      "open-in-browser",
      "copy-username",
      "copy-password",
      "paste-username",
    ]);

    const apiKey = createItem({ username: "", hasOneTimeCode: false, allURL: ["ssh://git@github.com"] });
    expect(orderItemActions(apiKey, { primary: "open-in-browser", secondary: "copy-username" })).toEqual([
      "open-in-hisar",
      "copy-password",
      "paste-password",
    ]);
  });

  it("offers only Open in Hisar on an item with no username, password, one-time code or website", () => {
    const note = createItem({ category: "secure-note", username: "", hasPassword: false, hasOneTimeCode: false });
    expect(orderItemActions({ ...note, allURL: [] }, defaultPreferences)).toEqual(["open-in-hisar"]);
  });

  it("opens a bare host in the browser", () => {
    const router = createItem({ allURL: ["router.local"] });
    expect(orderItemActions(router, { primary: "open-in-browser", secondary: "copy-password" })[0]).toBe(
      "open-in-browser",
    );
  });
});

describe("parseItemAction", () => {
  it("keeps a known action and replaces anything else with the default", () => {
    expect(parseItemAction("paste-username", "copy-password")).toBe("paste-username");
    expect(parseItemAction("open-in-1password", "copy-password")).toBe("copy-password");
    expect(parseItemAction(undefined, "paste-password")).toBe("paste-password");
  });
});

describe("listShareableFields", () => {
  it("offers the username and the password the item has", () => {
    expect(listShareableFields(createItem())).toEqual(["username", "password"]);
    expect(listShareableFields(createItem({ username: "" }))).toEqual(["password"]);
    expect(listShareableFields(createItem({ hasPassword: false }))).toEqual(["username"]);
    expect(listShareableFields(createItem({ username: "", hasPassword: false }))).toEqual([]);
  });
});
