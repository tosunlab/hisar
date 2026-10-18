/** The fields Hisar copies for a launcher (cliprotocol.IsLauncherField). */
export const allCopyableField = ["username", "password", "one-time code"] as const;
export type CopyableField = (typeof allCopyableField)[number];

export interface Vault {
  id: string;
  name: string;
}

/** ItemReference names one item; secrets are read or copied by it. */
export interface ItemReference {
  vaultID: string;
  itemID: string;
}

/** ItemFieldReference is a field an item has, as a hisar:// reference; never its value. */
export interface ItemFieldReference {
  field: string;
  reference: string;
}

/** Item is what Hisar tells a launcher about an item: metadata only, and which secrets it may ask for. */
export interface Item extends ItemReference {
  title: string;
  category: string;
  username: string;
  allURL: string[];
  allTag: string[];
  isFavorite: boolean;
  hasPassword: boolean;
  hasOneTimeCode: boolean;
  allReference: ItemFieldReference[];
}

/** ItemList is every item the user can open, and those vaults sorted by name. */
export interface ItemList {
  allVault: Vault[];
  allItem: Item[];
}

export interface OneTimeCode {
  code: string;
  expiresAt: Date;
  period: number;
}

/** CopyReceipt: Hisar copied the field and clears the clipboard after clearAfterSeconds if it still holds it. */
export interface CopyReceipt {
  clearAfterSeconds: number;
}

/** PasswordOptions shape a generated password; Hisar's generator checks the length. */
export interface PasswordOptions {
  length: number;
  digits: boolean;
  symbols: boolean;
}

/** OpenOptions: isEditing opens the item's editor in Hisar instead of its details. */
export interface OpenOptions {
  isEditing?: boolean;
}

/** The fields Raycast offers to share; Hisar's share links can hold more (websites, notes, custom fields). */
export type ShareableField = "username" | "password";

/** ShareLinkOptions shape a share link: its fields, its lifetime (1 hour, 1 day, 7 or 30 days) and views (0: unlimited). */
export interface ShareLinkOptions {
  allField: ShareableField[];
  expiresInSeconds: number;
  maxViews: number;
}
