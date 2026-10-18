import type { Item, ItemList } from "../hisar";

/** allVaults is the vault filter that keeps every vault; any other filter is a vault ID. */
export const allVaults = "all";

export interface ItemSection {
  id: string;
  title: string;
  allItem: Item[];
}

/**
 * groupItems lists favorites first, then each vault in the app's order (by name). Each item shows once, sorted by
 * title; empty sections are left out. A vault filter keeps that vault's items, its favorites included.
 */
export function groupItems(itemList: ItemList, vaultFilter: string): ItemSection[] {
  const allShown = itemList.allItem
    .filter((item) => vaultFilter === allVaults || item.vaultID === vaultFilter)
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

/** toWebsiteURL turns a saved URL into one a browser opens: a bare host gets https://, and only web addresses pass. */
export function toWebsiteURL(url: string): string | undefined {
  const trimmed = url.trim();
  if (trimmed === "") return undefined;
  try {
    const parsed = new URL(/^[a-z][a-z0-9+.-]*:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`);
    return parsed.protocol === "https:" || parsed.protocol === "http:" ? parsed.href : undefined;
  } catch {
    return undefined;
  }
}
