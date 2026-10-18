import { describe, expect, it } from "vitest";
import type { Item, ItemList } from "../hisar";
import {
  allItemsFilterValue,
  findItemFilter,
  getItemKeywords,
  groupItems,
  listCategories,
  toCategoryFilterValue,
  toVaultFilterValue,
  toWebsiteURL,
} from "./itemList";

function createItem(itemID: string, vaultID: string, title: string, change: Partial<Item> = {}): Item {
  return {
    vaultID,
    itemID,
    title,
    category: "login",
    username: "",
    allURL: [],
    allTag: [],
    isFavorite: false,
    hasPassword: true,
    hasOneTimeCode: false,
    allReference: [],
    ...change,
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
    createItem("i-github", "v-personal", "GitHub", { isFavorite: true }),
    createItem("i-aws", "v-work", "AWS", { isFavorite: true, category: "api-credential" }),
    createItem("i-bank", "v-personal", "Bank"),
    createItem("i-jira", "v-work", "Jira"),
    createItem("i-deploy", "v-work", "Deploy key", { category: "ssh-key" }),
    createItem("i-pin", "v-personal", "Recovery codes", { category: "secure-note" }),
  ],
};

function summarize(filterValue: string): [string, string[]][] {
  return groupItems(itemList, findItemFilter(itemList, filterValue)).map((section) => [
    section.title,
    section.allItem.map((item) => item.title),
  ]);
}

describe("groupItems", () => {
  it("lists favorites of every vault first, then each vault by the app's order, every item once", () => {
    expect(summarize(allItemsFilterValue)).toEqual([
      ["Favorites", ["AWS", "GitHub"]],
      ["Personal", ["Bank", "Recovery codes"]],
      ["Work", ["Deploy key", "Jira", "Slack"]],
    ]);
  });

  it("keeps one vault's items, its favorites included", () => {
    expect(summarize(toVaultFilterValue("v-work"))).toEqual([
      ["Favorites", ["AWS"]],
      ["Work", ["Deploy key", "Jira", "Slack"]],
    ]);
  });

  it("keeps one category's items across vaults", () => {
    expect(summarize(toCategoryFilterValue("login"))).toEqual([
      ["Favorites", ["GitHub"]],
      ["Personal", ["Bank"]],
      ["Work", ["Jira", "Slack"]],
    ]);
    expect(summarize(toCategoryFilterValue("ssh-key"))).toEqual([["Work", ["Deploy key"]]]);
  });
});

describe("findItemFilter", () => {
  it("shows every item for a remembered vault or category that is gone, or a value it does not know", () => {
    for (const value of [toVaultFilterValue("v-deleted"), toCategoryFilterValue("credit-card"), "v-work", ""]) {
      expect(findItemFilter(itemList, value).value, value).toBe(allItemsFilterValue);
      expect(summarize(value), value).toEqual(summarize(allItemsFilterValue));
    }
  });

  it("keeps a vault that has no items, which shows nothing", () => {
    expect(findItemFilter(itemList, toVaultFilterValue("v-empty")).value).toBe(toVaultFilterValue("v-empty"));
    expect(summarize(toVaultFilterValue("v-empty"))).toEqual([]);
  });
});

describe("listCategories", () => {
  it("offers each category that has an item, once, in the order of their names", () => {
    expect(listCategories(itemList)).toEqual(["api-credential", "login", "secure-note", "ssh-key"]);
  });
});

describe("getItemKeywords", () => {
  it("lets the search find an item by its username, vault, website hosts and tags", () => {
    const github = createItem("i-github", "v-personal", "GitHub", {
      username: "alice@example.com",
      allURL: ["https://github.com/login", "gist.github.com", "ssh://git@ssh.github.com:443", "not a url"],
      allTag: ["dev", "Personal"],
    });

    expect(getItemKeywords(github, "Personal")).toEqual([
      "alice@example.com",
      "Personal",
      "github.com",
      "gist.github.com",
      "ssh.github.com",
      "dev",
    ]);
  });

  it("leaves out an empty username", () => {
    expect(getItemKeywords(createItem("i-note", "v-work", "Note"), "Work")).toEqual(["Work"]);
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
