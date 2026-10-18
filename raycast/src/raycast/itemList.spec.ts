import { describe, expect, it } from "vitest";
import type { Item, ItemList } from "../hisar";
import { allVaults, groupItems, toWebsiteURL } from "./itemList";

function createItem(itemID: string, vaultID: string, title: string, isFavorite = false): Item {
  return {
    vaultID,
    itemID,
    title,
    category: "login",
    username: "",
    allURL: [],
    allTag: [],
    isFavorite,
    hasPassword: true,
    hasOneTimeCode: false,
    allReference: [],
  };
}

const itemList: ItemList = {
  allVault: [
    { id: "v-empty", name: "Archive" },
    { id: "v-personal", name: "Personal" },
    { id: "v-work", name: "Work" },
  ],
  allItem: [
    createItem("i-slack", "v-work", "Slack"),
    createItem("i-github", "v-personal", "GitHub", true),
    createItem("i-aws", "v-work", "AWS", true),
    createItem("i-bank", "v-personal", "Bank"),
    createItem("i-jira", "v-work", "Jira"),
  ],
};

function summarize(itemList: ItemList, vaultFilter: string): [string, string[]][] {
  return groupItems(itemList, vaultFilter).map((section) => [section.title, section.allItem.map((item) => item.title)]);
}

describe("groupItems", () => {
  it("lists favorites of every vault first, then each vault by the app's order, every item once", () => {
    expect(summarize(itemList, allVaults)).toEqual([
      ["Favorites", ["AWS", "GitHub"]],
      ["Personal", ["Bank"]],
      ["Work", ["Jira", "Slack"]],
    ]);
  });

  it("keeps one vault's items, its favorites included", () => {
    expect(summarize(itemList, "v-work")).toEqual([
      ["Favorites", ["AWS"]],
      ["Work", ["Jira", "Slack"]],
    ]);
  });
});

describe("toWebsiteURL", () => {
  it("opens web addresses, adds https:// to a bare host and refuses anything else", () => {
    expect(toWebsiteURL("https://github.com/login")).toBe("https://github.com/login");
    expect(toWebsiteURL("http://router.local")).toBe("http://router.local/");
    expect(toWebsiteURL(" github.com ")).toBe("https://github.com/");
    expect(toWebsiteURL("localhost:3000")).toBe("https://localhost:3000/");
    expect(toWebsiteURL("ssh://git@github.com")).toBeUndefined();
    expect(toWebsiteURL("javascript:alert(1)")).toBeUndefined();
    expect(toWebsiteURL("")).toBeUndefined();
  });
});
