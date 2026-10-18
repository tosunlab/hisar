import type { Item, ShareableField } from "../hisar";
import { findWebsiteURL } from "./itemList";

/** The actions the primary and secondary action preferences choose from, in the order they follow those two. */
export const allItemAction = [
  "open-in-hisar",
  "open-in-browser",
  "copy-username",
  "copy-password",
  "copy-one-time-code",
  "paste-username",
  "paste-password",
  "paste-one-time-code",
] as const;

export type ItemAction = (typeof allItemAction)[number];

export interface ItemActionPreferences {
  primary: ItemAction;
  secondary: ItemAction;
}

const isOfferedByAction: Record<ItemAction, (item: Item) => boolean> = {
  "open-in-hisar": () => true,
  "open-in-browser": (item) => findWebsiteURL(item) !== undefined,
  "copy-username": (item) => item.username !== "",
  "copy-password": (item) => item.hasPassword,
  "copy-one-time-code": (item) => item.hasOneTimeCode,
  "paste-username": (item) => item.username !== "",
  "paste-password": (item) => item.hasPassword,
  "paste-one-time-code": (item) => item.hasOneTimeCode,
};

/**
 * orderItemActions lists the actions an item offers: the primary action, the secondary one, then the rest, each once.
 * An action the item has no value for is left out, so the next one takes its place (Return, then ⌘ Return).
 */
export function orderItemActions(item: Item, preferences: ItemActionPreferences): ItemAction[] {
  return [...new Set([preferences.primary, preferences.secondary, ...allItemAction])].filter((action) =>
    isOfferedByAction[action](item),
  );
}

/** parseItemAction reads a preference's value; a value this extension does not know is fallback. */
export function parseItemAction(value: unknown, fallback: ItemAction): ItemAction {
  return allItemAction.find((action) => action === value) ?? fallback;
}

/** listShareableFields returns the fields Share Item offers: the username and the password the item has. */
export function listShareableFields(item: Item): ShareableField[] {
  const allField: ShareableField[] = [];
  if (item.username !== "") allField.push("username");
  if (item.hasPassword) allField.push("password");
  return allField;
}
