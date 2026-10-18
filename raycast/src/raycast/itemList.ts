import type { Item, ItemList } from "../hisar";

export interface ItemSection {
  id: string;
  title: string;
  allItem: Item[];
}

/** ItemFilter is an entry of the item list's dropdown: every item, or one vault's or one category's. */
export interface ItemFilter {
  /** value is the dropdown's value; Search Hisar remembers it, and nothing else, between runs. */
  value: string;
  includes: (item: Item) => boolean;
}

export const allItemsFilterValue = "all";

export function toVaultFilterValue(vaultID: string): string {
  return `vault:${vaultID}`;
}

export function toCategoryFilterValue(category: string): string {
  return `category:${category}`;
}

/** listCategories returns each category that has an item, in the order of their names. */
export function listCategories(itemList: ItemList): string[] {
  return [...new Set(itemList.allItem.map((item) => item.category))].toSorted();
}

/** findItemFilter reads a dropdown value; a vault or category that no longer has items shows every item. */
export function findItemFilter(itemList: ItemList, value: string): ItemFilter {
  const vault = itemList.allVault.find((vault) => toVaultFilterValue(vault.id) === value);
  if (vault) return { value, includes: (item) => item.vaultID === vault.id };
  const category = listCategories(itemList).find((category) => toCategoryFilterValue(category) === value);
  if (category) return { value, includes: (item) => item.category === category };
  return { value: allItemsFilterValue, includes: () => true };
}

/**
 * groupItems lists favorites first, then each vault in the app's order (by name). Each item shows once, sorted by
 * title; empty sections are left out. The filter keeps the items it includes, their favorites included.
 */
export function groupItems(itemList: ItemList, filter: ItemFilter): ItemSection[] {
  const allShown = itemList.allItem
    .filter((item) => filter.includes(item))
    .toSorted((left, right) => left.title.localeCompare(right.title));
  const allSection: ItemSection[] = [
    { id: "favorites", title: "Favorites", allItem: allShown.filter((item) => item.isFavorite) },
    ...itemList.allVault.map((vault) => ({
      id: vault.id,
      title: vault.name,
      allItem: allShown.filter((item) => !item.isFavorite && item.vaultID === vault.id),
    })),
  ];
  return allSection.filter((section) => section.allItem.length > 0);
}

/**
 * getItemKeywords is what the search matches besides the title: the username, the vault's name, each website's host
 * and the tags.
 */
export function getItemKeywords(item: Item, vaultName: string): string[] {
  const allHost = item.allURL.map((url) => parseURL(url)?.hostname);
  return [...new Set([item.username, vaultName, ...allHost, ...item.allTag])].filter(
    (keyword): keyword is string => keyword !== undefined && keyword !== "",
  );
}

/** findWebsiteURL returns the first of the item's URLs a browser opens. */
export function findWebsiteURL(item: Item): string | undefined {
  return item.allURL.map(toWebsiteURL).find((url) => url !== undefined);
}

/** toWebsiteURL turns a saved URL into one a browser opens: a bare host gets https://, and only web addresses pass. */
export function toWebsiteURL(url: string): string | undefined {
  const parsed = parseURL(url);
  return parsed?.protocol === "https:" || parsed?.protocol === "http:" ? parsed.href : undefined;
}

function parseURL(url: string): URL | undefined {
  const trimmed = url.trim();
  if (trimmed === "") return undefined;
  try {
    return new URL(/^[a-z][a-z0-9+.-]*:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`);
  } catch {
    return undefined;
  }
}
